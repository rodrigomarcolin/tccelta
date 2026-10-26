# 01 — Firmware: chaves, perfis de segurança e autenticação

## Objetivo

Dar ao firmware a identidade criptográfica do dongle (`DevId`, `K_auth`,
`K_rec`) guardada com todas as camadas de proteção previstas no documento de
segurança (§4.1: NVS encriptada, *flash encryption*, Secure Boot V2, eFuse), e
as primitivas e interfaces que as specs 02–05 usam para autenticar blocos
(tag de 32 B com `K_rec`) e mensagens de handshake (tag de 16 B com `K_auth`).
A autenticação de blocos passa por uma interface genérica com três perfis
selecionados por *build flag* — `none`, `hmac` e `asym` — para o benchmark
comparativo exigido pelo documento (§4: "comparar com benchmarks duas
implementações diferentes: MAC com PSK e assinatura digital").

## Escopo

**Dentro**
- Primitivas puras e testáveis no `native`: SHA-256, HMAC-SHA256, CRC32,
  comparação em tempo constante.
- Adaptadores mbedTLS (ESP32): HMAC-SHA256 com SHA em hardware e ECDSA P-256.
- `IMessageAuth` (já existe) com `NoAuth`, `HmacBlockAuth`, `EcdsaBlockAuth`;
  nova `IHandshakeAuth` com `HmacHandshakeAuth` e `NoHandshakeAuth`.
- `KeyStore`: leitura das chaves de uma partição NVS dedicada (`secnvs`),
  sempre por NVS encryption (um só caminho de código); chaves fixas de
  bancada por *build flag*.
- **Passo 0 da fase:** migração de todos os envs do firmware para Arduino
  como componente do ESP-IDF (`framework = arduino, espidf`), com *timebox*
  e plano B (§9).
- **Mesmo firmware em todos os perfis de hardware** — `dev`, `devsec`
  (ensaio de produção) e `prod` —, com uma só tabela de partições; a
  diferença entre eles é só o arquivo de *defaults* do `sdkconfig` e o
  procedimento de gravação/queima de eFuse (§8–§10), com passo a passo em
  `espsecure.py`/`espefuse.py`/`nvs_partition_gen.py`.
- Script de provisionamento (`DevId`, `K_auth`, `K_rec` e, no perfil `asym`,
  `k_sig`) que gera a imagem NVS e o registro para o KMS (Fase 3).

**Fora**
- Uso das tags no formato de bloco e na flash (spec 02), no handshake
  (spec 05) e eventos completos (spec 06).
- Registro efetivo no KMS/Vault (Fase 3) e LESC/bonds (Fase 2).
- Elemento seguro externo (ATECC608B): descartado pelo próprio documento.

## Estado atual (verificado no código)

| Item | Onde | Situação |
|---|---|---|
| Interface de autenticação | `esp32-firmware/src/collector/IMessageAuth.h` | `IMessageAuth { tagSize(); sign(msg,len,tagOut); verify(msg,len,tag); }` e `NoAuth` (tag de 0 B). Usada só por `DataProtocol`. |
| Uso | `src/app/CollectorApp.cpp:92` | `static collector::NoAuth auth; // TODO(segurança)` |
| Chaves | — | Não existem. Nenhuma partição ou namespace de chaves. |
| NVS | `src/storage/NvsStore.cpp` | `Preferences` no namespace `collector` da partição padrão `nvs` (config, `boot_id`, `last_seq`). |
| Partições | `esp32-firmware/partitions_collector.csv` | `nvs 0x9000/0x5000`, `phy_init 0xE000/0x1000`, `factory 0x10000/0x1E0000`, `spiffs 0x1F0000/0x200000` (LittleFS), `coredump 0x3F0000/0x10000`. Tabela em `0x8000` (padrão). |
| Plataforma | `platformio.ini` | `platform = espressif32@7.1.3`, `framework = arduino`, `board = esp32dev`, NimBLE-Arduino 2.5.1. **Instalada** em `~/.platformio/platforms/espressif32`: **7.0.1** (`.piopm`); o `framework-espidf` **não** está instalado. O `platform.json` da 7.0.1 lista `framework-espidf ~4.60001.0` com `optionalVersions: ~3.40407.0` (= ESP-IDF 4.4.7). |
| Núcleo Arduino instalado | `~/.platformio/packages/framework-arduinoespressif32/package.json` | `3.20017…` = **Arduino-ESP32 2.0.17 sobre ESP-IDF 4.4.7** (`esp_idf_version.h`: 4.4.7). |
| *Bootloader* atual | `.pio/build/{mock,twai,mcp2515}/bootloader.bin` | 17 536 B (pré-compilado, log desligado). App atual: ~630 KB (`firmware.bin` de 626–633 KB). |

Fatos do núcleo Arduino que determinam o desenho (conferidos com `nm` e no
`sdkconfig` pré-compilado em `tools/sdk/esp32/`):

| Recurso | Arduino puro (envs atuais) | Consequência |
|---|---|---|
| Secure Boot V2 | `# CONFIG_SECURE_BOOT is not set`; *bootloader* pré-compilado (`tools/sdk/esp32/bin/bootloader_*.elf`) sem verificação de assinatura | Impossível sem compilar o *bootloader* pelo ESP-IDF. |
| *Flash encryption* | `# CONFIG_SECURE_FLASH_ENC_ENABLED is not set` | O *bootloader* não cifra no primeiro boot; só daria para cifrar manualmente no host, sem modo *release*. |
| NVS encryption | `libnvs_flash.a` **não exporta** `nvs_flash_secure_init`, `nvs_flash_secure_init_partition`, `nvs_flash_generate_keys`, `nvs_flash_read_security_cfg` e **não contém** `nvs_encrypted_partition.cpp.obj` (`ar t`); no `sdkconfig` a seção NVS só lista `CONFIG_NVS_ASSERT_ERROR_CHECK` — `CONFIG_NVS_ENCRYPTION` nem aparece, sinal de dependência não satisfeita: no IDF 4.4 ela **depende de `SECURE_FLASH_ENC_ENABLED`** | Impossível no Arduino puro; no Arduino + IDF, só via `sdkconfig` com *flash encryption* ligada — o perfil `dev` precisa do ajuste do §9.3. |
| HMAC-SHA256 | `mbedtls_md_hmac_*` presentes; `CONFIG_MBEDTLS_HARDWARE_SHA=y` | Suportado, com SHA em hardware. |
| ECDSA P-256 | `mbedtls_ecdsa_sign_det_ext`, `CONFIG_MBEDTLS_ECP_DP_SECP256R1_ENABLED=y`, `CONFIG_MBEDTLS_HARDWARE_MPI=y` | Suportado. |
| Ed25519 | Não existe no mbedTLS 2.28 | Perfil `asym` usa **ECDSA P-256**. |
| eFuse | `libefuse.a` presente; `espefuse.py` no `tool-esptoolpy` | Leitura/queima pelo host funciona; o *release mode* automático depende do *bootloader* do IDF. |

Fatos do núcleo Arduino que afetam a NVS (conferidos no código):

| Onde | O que faz | Consequência |
|---|---|---|
| `cores/esp32/esp32-hal-misc.c:249` (`initArduino`, antes do `setup`) | `nvs_flash_init()`; se `NO_FREE_PAGES`/`NEW_VERSION_FOUND`, apaga a partição `nvs` e repete | Com `CONFIG_NVS_ENCRYPTION`, o `nvs_flash_init()` do IDF 4.4 lê a primeira partição `nvs_keys`, **gera e grava chaves se ela estiver em branco**, e inicializa a `nvs` **cifrada** (comentário de `nvs_flash.h`). |
| `libraries/Preferences/src/Preferences.cpp:39–47` | Sem rótulo: `nvs_open` na partição padrão já inicializada; com rótulo: `nvs_flash_init_partition(label)` (sempre em claro) | `Preferences` sem rótulo (o `NvsStore`, namespace `collector`) herda a cifra da `nvs`; `Preferences` **com** rótulo nunca pode abrir a `secnvs`. |

Conclusão: **todos os envs do firmware passam a Arduino como componente do
ESP-IDF** (`framework = arduino, espidf`), que compila *bootloader*, NVS e
mbedTLS com o `sdkconfig` do projeto. Dev, ensaio e produção usam o mesmo
código, o mesmo *build* e a mesma tabela de partições; a migração é o passo
0 da fase (§9).

## Design

### 1. Perfis de segurança (`SECURITY_PROFILE`)

| Perfil | Valor | Tag do bloco (`K_rec`) | Tag do handshake | Chaves exigidas | Uso |
|---|---|---|---|---|---|
| `none` | 0 | 0 B (`NoAuth`) | 16 B **zerados**, nunca verificados (`NoHandshakeAuth`) | `DevId` (pode vir do MAC) | Linha de base do benchmark "sem segurança". Nunca em campo. |
| `hmac` | 1 (padrão) | 32 B HMAC-SHA256 (`HmacBlockAuth`) | 16 B HMAC-SHA256 truncado (`HmacHandshakeAuth`) | `DevId`, `K_auth`, `K_rec` | Produção. |
| `asym` | 2 | 64 B ECDSA P-256 `r‖s` (`EcdsaBlockAuth`) | igual ao `hmac` | `DevId`, `K_auth`, `k_sig` (+ `K_rec` opcional) | Desenvolvimento/benchmark. |

**[Decisão]** O formato das mensagens de handshake é o mesmo nos três perfis
(tag sempre com 16 B), para o app e o servidor de bancada não mudarem de
perfil para perfil; só a tag de bloco varia. No perfil `asym` o handshake
continua em HMAC com `K_auth`: assinar o desafio do servidor exigiria um par
de chaves do servidor, fora do escopo do documento (que compara os dois
esquemas para os blocos).

`src/security/SecurityProfile.h`:

```cpp
#define SECURITY_PROFILE_NONE 0
#define SECURITY_PROFILE_HMAC 1
#define SECURITY_PROFILE_ASYM 2
#ifndef SECURITY_PROFILE
#define SECURITY_PROFILE SECURITY_PROFILE_HMAC
#endif

namespace security {
enum class Profile : uint8_t { None = 0, Hmac = 1, Asym = 2 };
constexpr Profile kProfile = static_cast<Profile>(SECURITY_PROFILE);
constexpr size_t kHandshakeTagSize = 16;                  // fixo (Tabela 2)
constexpr size_t kBlockTagSize =
    kProfile == Profile::None ? 0 : kProfile == Profile::Hmac ? 32 : 64;
constexpr const char* kProfileName =
    kProfile == Profile::None ? "none" : kProfile == Profile::Hmac ? "hmac" : "asym";
static_assert(SECURITY_PROFILE >= 0 && SECURITY_PROFILE <= 2, "SECURITY_PROFILE inválido");
}  // namespace security
```

Quem lê o tamanho da tag em tempo de execução usa `IMessageAuth::tagSize()`,
nunca a macro: os testes nativos instanciam qualquer perfil na mesma
compilação. A macro só escolhe as instâncias em `CollectorApp.cpp`, as pilhas
das tarefas e o `kMaxTagSize` de buffers (64 B, ver spec 02).

### 2. Material de chaves

| Item | Tamanho | Uso | Quem mais conhece |
|---|---|---|---|
| `DevId` | 8 B | Identidade; entra em toda tag; vai no aviso 0x07 | Servidor (público) |
| `K_auth` | 32 B | HMAC das mensagens 0x01/0x02/0x04 | KMS (gerar + verificar) |
| `K_rec` | 32 B | HMAC dos blocos 0x03 | KMS (só verificar) |
| `k_sig` | 32 B (escalar P-256) | Assinatura dos blocos no perfil `asym` | Ninguém; o servidor recebe só a chave pública (65 B, `04‖X‖Y`) |

Layout na partição `secnvs` (NVS), namespace `sec`:

| Chave NVS | Tipo NVS | Conteúdo |
|---|---|---|
| `prov_ver` | `u8` | Versão do provisionamento (`1`). Ausente = não provisionado. |
| `dev_id` | blob | 8 B |
| `k_auth` | blob | 32 B |
| `k_rec` | blob | 32 B |
| `k_sig` | blob | 32 B (só perfil `asym`; opcional nos demais) |
| `prof` | `u8` | Perfil para o qual o dongle foi provisionado (informativo). |

**[Decisão] Partição própria para as chaves** (`secnvs`), separada da `nvs`
que guarda config, `boot_id`, reserva de `seq` e `c_S`: (a) reprovisionar uma
placa de desenvolvimento não apaga a reserva de `seq` nem o último `c_S`
(apagar esses dois faz o servidor rejeitar o dongle) — desde que o
reprovisionamento reaproveite a chave XTS da `nvs_key` já presente na placa
(§11); (b) a imagem de fábrica contém só chaves; (c) em **todos** os perfis
as duas partições são cifradas com a mesma chave XTS da partição `nvs_key`
(no `dev` essa chave fica legível na flash, §8).

Chaves fixas de bancada (`-DSECURITY_DEV_FIXED_KEYS`, **nunca em produção**),
usadas quando `secnvs` não está provisionada. São as mesmas do servidor de
bancada do app (spec 09) e dos vetores de teste:

| Item | Valor (hex) |
|---|---|
| `DevId` | `544343454C544101` (`"TCCELTA\x01"`) |
| `K_auth` | `0102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F20` |
| `K_rec` | `2122232425262728292A2B2C2D2E2F303132333435363738393A3B3C3D3E3F40` |
| `k_sig` | `11` repetido 32 vezes |
| chave pública de `k_sig` | `040217E617F0B6443928278F96999E69A23A4F2C152BDF6D6CDF66E5B80282D4ED194A7DEBCB97712D2DDA3CA85AA8765A56F45FC758599652F2897C65306E5794` (calculada em Python puro, aritmética de curva própria; **conferência obrigatória** com `cryptography` no `vectors.py` antes de qualquer teste usá-la) |

### 3. Primitivas (`src/crypto/`, C++ puro)

```cpp
namespace crypto {
constexpr size_t kSha256Size = 32;

/** SHA-256 portátil (FIPS 180-4). Usado no native e como referência. */
class Sha256 {
public:
    void begin();
    void update(const uint8_t* data, size_t len);
    void finish(uint8_t out[kSha256Size]);
private:
    uint32_t _h[8]; uint64_t _bits; uint8_t _buf[64]; size_t _used;
};

/** HMAC-SHA256 com a chave fixada na construção; reutilizável. */
class IHmacSha256 {
public:
    virtual ~IHmacSha256() = default;
    virtual void begin() = 0;                                  // novo MAC
    virtual void update(const uint8_t* data, size_t len) = 0;
    virtual void finish(uint8_t out[kSha256Size]) = 0;
};

/** RFC 2104 sobre `Sha256`; guarda ipad/opad já misturados com a chave. */
class PortableHmacSha256 final : public IHmacSha256 {
public:
    PortableHmacSha256(const uint8_t* key, size_t keyLen);
    ~PortableHmacSha256() override;  // zera ipad/opad
    // ...
};

/** CRC-32/ISO-HDLC (zlib): poly 0xEDB88320 refletido, init/xorout 0xFFFFFFFF. */
uint32_t crc32(const uint8_t* data, size_t len, uint32_t previous = 0);

/** Comparação em tempo constante (não para no primeiro byte diferente). */
bool ctEqual(const uint8_t* a, const uint8_t* b, size_t len);

/** Zera memória sem que o compilador remova a escrita. */
void secureZero(void* p, size_t len);
}  // namespace crypto
```

- `crc32` com tabela de 256 entradas `constexpr` (1 KB em `.rodata`).
  `crc32(p, n, crc32(q, m))` encadeia (dados e depois tag sem cópia).
  **[Decisão]** implementação própria em vez de `esp_rom_crc32_le`: o mesmo
  código roda no native, e a convenção de valor inicial da ROM não precisa
  ser conferida. Custo esperado < 30 µs por bloco de 1 KB (medido pelo evento
  `FLUSH crc_us`, spec 06).
- `ctEqual`: `uint8_t d = 0; for (...) d |= a[i] ^ b[i]; return d == 0;` com
  ponteiros `volatile` para o compilador não abreviar.
- `secureZero`: `volatile` byte a byte (ou `mbedtls_platform_zeroize` no ESP32).

Adaptadores mbedTLS (`src/security/mbed/`, só ESP32; excluídos do native):

```cpp
/** HMAC-SHA256 do mbedTLS com SHA em hardware. Um contexto por instância. */
class MbedHmacSha256 final : public crypto::IHmacSha256 {
public:
    MbedHmacSha256(const uint8_t* key, size_t keyLen);  // md_setup(hmac=1) + hmac_starts uma vez
    ~MbedHmacSha256() override;                          // mbedtls_md_free
    void begin() override;                               // mbedtls_md_hmac_reset
    void update(const uint8_t* d, size_t n) override;    // mbedtls_md_hmac_update
    void finish(uint8_t out[32]) override;               // mbedtls_md_hmac_finish
private:
    mbedtls_md_context_t _ctx;
};
```

**[Decisão]** A chave é carregada uma vez por instância e cada MAC usa
`mbedtls_md_hmac_reset`, que reaproveita o estado ipad/opad: o custo medido
por bloco fica só no hash dos dados, o que interessa ao benchmark.

### 4. Autenticação de blocos (`IMessageAuth`)

A interface existente fica, com o contrato redocumentado:

```cpp
/**
 * Autentica a área de dados de um bloco (cabeçalho + registros, `len` =
 * `tamanho`). A implementação acrescenta o separador de contexto
 * ("MALCONG" || 0x03 || DevId) antes dos dados. `tagSize()` é constante
 * durante toda a vida da instância. Não é reentrante: uma instância por tarefa.
 */
class IMessageAuth {
public:
    virtual ~IMessageAuth() = default;
    virtual size_t tagSize() const = 0;
    virtual void sign(const uint8_t* data, size_t len, uint8_t* tagOut) = 0;
    virtual bool verify(const uint8_t* data, size_t len, const uint8_t* tag) = 0;
};
```

Entrada autenticada (Eq. 4.2 do documento):

| Offset | Campo | Tamanho | Valor |
|---|---|---|---|
| 0 | rótulo | 7 | ASCII `MALCONG` = `4D 41 4C 43 4F 4E 47` (sem terminador) |
| 7 | tipo | 1 | `0x03` |
| 8 | `DevId` | 8 | bytes do `DevId` na ordem em que estão na NVS |
| 16 | bloco | `tamanho` | área de dados exatamente como gravada (cabeçalho de 12 B + registros) |

| Classe | Arquivo | `tagSize()` | `sign` |
|---|---|---|---|
| `NoAuth` | `collector/IMessageAuth.h` (existente) | 0 | nada |
| `HmacBlockAuth` | `security/BlockAuth.{h,cpp}` (puro) | 32 | `hmac.begin(); update(rótulo,7); update(&0x03,1); update(devId,8); update(data,len); finish(tag)` |
| `EcdsaBlockAuth` | `security/mbed/EcdsaBlockAuth.{h,cpp}` | 64 | `h = SHA-256(rótulo‖0x03‖DevId‖data)`; `mbedtls_ecdsa_sign_det_ext(grp, r, s, d, h, 32, MBEDTLS_MD_SHA256, rngBlind, nullptr)`; `tag = r(32, big-endian) ‖ s(32, big-endian)` |

```cpp
class HmacBlockAuth final : public collector::IMessageAuth {
public:
    HmacBlockAuth(crypto::IHmacSha256& hmacRec, const uint8_t devId[8]);
    size_t tagSize() const override { return 32; }
    void sign(const uint8_t* data, size_t len, uint8_t* tagOut) override;
    bool verify(const uint8_t* data, size_t len, const uint8_t* tag) override;  // ctEqual
};

class EcdsaBlockAuth final : public collector::IMessageAuth {
public:
    EcdsaBlockAuth(const uint8_t kSig[32], const uint8_t devId[8]);  // ecp_group_load P-256 + mpi_read d (uma vez)
    ~EcdsaBlockAuth() override;                                       // ecdsa_free + zera d
    size_t tagSize() const override { return 64; }
    void sign(const uint8_t* data, size_t len, uint8_t* tagOut) override;
    bool verify(const uint8_t* data, size_t len, const uint8_t* tag) override;  // mbedtls_ecdsa_verify com Q = d·G
};
```

**[Decisão]** ECDSA determinística (RFC 6979, `sign_det_ext`) com cegamento
pelo RNG do ESP32 (`esp_fill_random`): a assinatura não depende da qualidade
do RNG e o custo é estável para o benchmark. **[Decisão]** `r‖s` crus e de
tamanho fixo (64 B), não DER (70–72 B variáveis): o tamanho da tag precisa ser
constante para o formato da flash (spec 02). `r` e `s` são inteiros
**big-endian** como em SEC1/RFC 6979 — exceção consciente à regra
little-endian, porque tags são cadeias de bytes, não campos do protocolo.

### 5. Autenticação do handshake (`IHandshakeAuth`)

```cpp
namespace security {
class IHandshakeAuth {
public:
    virtual ~IHandshakeAuth() = default;
    /** tag = HMAC-SHA256_{K_auth}("MALCONG" || type || DevId || payload)[0:16] */
    virtual void tag(uint8_t type, const uint8_t* payload, size_t len, uint8_t out[16]) = 0;
    /** Recalcula e compara em tempo constante. */
    virtual bool verify(uint8_t type, const uint8_t* payload, size_t len, const uint8_t tag[16]) = 0;
};
class HmacHandshakeAuth final : public IHandshakeAuth { /* IHmacSha256& (K_auth) + devId */ };
class NoHandshakeAuth final : public IHandshakeAuth { /* tag → 16 zeros; verify → true */ };
}
```

| Tipo | `payload` (bytes autenticados) | Tamanho |
|---|---|---|
| 0x01 Desafio | `c_S ‖ N_S ‖ ACK_seq ‖ c_conf ‖ flags [‖ diag_min]` | 29 ou 31 |
| 0x02 Resposta | `c_S ‖ N_S ‖ TAIL_seq ‖ HEAD_seq ‖ boot_id ‖ t_ms` | 34 |
| 0x04 ACK | `ACK_seq ‖ c_conf ‖ flags` | 9 |

`payload` é exatamente o `V` do TLV sem os 16 B finais (layouts byte a byte na
spec 04). O `T` e o `L` do TLV não entram: o `tipo` entra explicitamente e o
tamanho é implícito no tipo.

### 6. Instâncias e concorrência

As implementações não são reentrantes (contexto mbedTLS por instância). Cada
tarefa tem as suas; as chaves em RAM (`DeviceKeys`, estático) são só lidas.

| Tarefa (spec) | Instâncias | Para quê |
|---|---|---|
| `FlushTask` (02) | `MbedHmacSha256 hmacRecFlush(K_rec)` + `HmacBlockAuth` — ou `EcdsaBlockAuth` — ou `NoAuth` | Assinar cada bloco antes de gravar |
| `SyncTask` (05) | `MbedHmacSha256 hmacAuth(K_auth)` + `HmacHandshakeAuth`; `MbedHmacSha256 hmacRecSync(K_rec)` + `HmacBlockAuth` (ou `EcdsaBlockAuth`/`NoAuth`) | Verificar 0x01/0x04, assinar 0x02, assinar blocos LOSS |

O motor SHA do ESP32 é compartilhado entre núcleos; o `esp_sha` do IDF
serializa o acesso e cai para software se estiver ocupado — correto, mas pode
aparecer como variação no `sign_us` (registrar no benchmark).

### 7. `KeyStore` (`src/storage/KeyStore.{h,cpp}`, ESP32)

```cpp
namespace security {
struct DeviceKeys {
    uint8_t devId[8];
    uint8_t kAuth[32];
    uint8_t kRec[32];
    uint8_t kSig[32];
    bool    hasRec, hasSig;
};
enum class KeySource : uint8_t { Partition, DevFixed, MacDerived, Missing };
}

class KeyStore {
public:
    /** Lê `secnvs`; cai para as chaves fixas (se compiladas) ou, no perfil
     *  none, para um DevId derivado do MAC. false = faltam chaves exigidas. */
    bool load(security::DeviceKeys& out, security::KeySource& source);
};
```

Algoritmo de `load` (**um só caminho**, igual em `dev`, `devsec` e `prod`):
1. `part = esp_partition_find_first(DATA, NVS_KEYS, "nvs_key")` →
   `nvs_flash_read_security_cfg(part, &cfg)`. Quando o `KeyStore` roda, o
   `initArduino` já chamou `nvs_flash_init()`, que gera as chaves se a
   `nvs_key` estiver em branco; por isso `ESP_ERR_NVS_KEYS_NOT_INITIALIZED`
   aqui é erro, e `ESP_ERR_NVS_CORRUPT_KEY_PART` → `[keys] nvs_key corrompida`
   e `Missing`.
2. `nvs_flash_secure_init_partition("secnvs", &cfg)`; `crypto::secureZero(&cfg)`.
3. `nvs_open_from_partition("secnvs", "sec", NVS_READONLY, &h)`; ler `prov_ver`
   e os blobs com `nvs_get_blob`, conferindo o tamanho exato de cada um.
4. Exigências por perfil: `hmac` → `dev_id`, `k_auth`, `k_rec`; `asym` →
   `dev_id`, `k_auth`, `k_sig`; `none` → só `dev_id` (se ausente,
   `DevId = 0x00 0x00 ‖ MAC Wi-Fi (6 B)`, `source = MacDerived`).
5. Se faltar algo e `SECURITY_DEV_FIXED_KEYS` estiver definido → copiar as
   chaves fixas (§2), `source = DevFixed`. Senão `source = Missing`, `false`.
   `secnvs` em branco (placa nunca provisionada) inicializa normalmente e cai
   aqui: o atalho das chaves fixas continua valendo com NVS encryption.
6. `nvs_close`; `crypto::secureZero` em todo buffer temporário.
7. Linha de boot `[sec] fe=<off|dev|release> sb=<0|1> jtag_off=<0|1> dl_off=<0|1> crypt_cnt=<n> nvs_enc=1`
   com `esp_get_flash_encryption_mode()`, `esp_secure_boot_enabled()`,
   `esp_efuse_read_field_bit(ESP_EFUSE_JTAG_DISABLE / ESP_EFUSE_UART_DOWNLOAD_DIS)`
   e `esp_efuse_read_field_blob(ESP_EFUSE_FLASH_CRYPT_CNT, …, 7)`, **em todos
   os envs** (mesmo código); é a verificação de eFuse do perfil `prod`, em que
   o `espefuse.py` não alcança mais a placa.

**[Decisão]** O `KeyStore` usa a API C da NVS, não `Preferences`: o
`Preferences::begin(name, ro, label)` chama `nvs_flash_init_partition`, que
abre a partição **em claro**. Se alguém abrir a `secnvs` por `Preferences`
antes do `KeyStore`, ela fica registrada como partição em claro e as chaves
viram lixo; regra de revisão: nenhum `Preferences::begin(…, "secnvs")`.

**[Decisão] `Preferences`/`NvsStore` na `nvs` padrão ficam cifrados sem
mudança de código.** Conferido no núcleo: `initArduino` chama
`nvs_flash_init()` antes do `setup`, e com `CONFIG_NVS_ENCRYPTION` essa
função inicializa a `nvs` por `nvs_flash_secure_init_partition` com as chaves
da `nvs_key` (gerando-as se ausentes); `Preferences::begin(name)` sem rótulo
só faz `nvs_open` nessa partição. Logo o namespace `collector` (spec 03) e os
*bonds* do NimBLE (`nimble_bond`, Fase 2) ficam cifrados nos três perfis.
Consequência operacional: a `nvs_key` precisa estar gravada **antes** do
primeiro boot do firmware novo, ou o próprio dongle gera uma chave diferente
da usada pelo `provision.py` para cifrar a `secnvs` (§11 trata os dois casos).

**[Decisão] Guardas derivadas do `sdkconfig`, não de *build flag*** (em
`KeyStore.cpp`, que só compila no ESP32):

```cpp
#if CONFIG_SECURE_FLASH_ENCRYPTION_MODE_RELEASE
static_assert(SECURITY_PROFILE == SECURITY_PROFILE_HMAC, "prod exige o perfil hmac");
#  ifdef SECURITY_DEV_FIXED_KEYS
#    error "chaves fixas de bancada não podem ir para o perfil prod"
#  endif
#endif
```

Sem chaves (`Missing`) nos perfis `hmac`/`asym`, `startCollector` falha com
`[keys] não provisionado` na serial e o `main` para, como já faz com as
outras falhas de inicialização. **[Decisão]** não há modo degradado: um bloco
sem tag válida não serve para nada no servidor, e coletar sem poder assinar
apenas enche a flash.

Higiene: `DeviceKeys` é `static` em `CollectorApp.cpp` (nunca na pilha); o
evento `KEYS` (spec 06) mostra só `DevId`, perfil, origem e o estado de
*flash encryption*/Secure Boot (`fe`, `sb`); nenhum log imprime chave. Nos
perfis `devsec` e `prod` o *core dump* fica desligado (§9.2), porque um dump
de RAM conteria as chaves e a partição `coredump` não é cifrada.

### 8. Perfis de hardware e camadas de proteção

**[Decisão] Mesmo firmware em todos os perfis.** Código-fonte, framework
(`arduino, espidf`), tabela de partições (§9.4), `CMakeLists.txt` e o caminho
do `KeyStore` (§7) são idênticos. Passar de um perfil para outro é só:
(a) acrescentar um arquivo de *defaults* ao `sdkconfig` do *build*;
(b) executar o procedimento de gravação/queima de eFuse do perfil (§10).
Os perfis de hardware são ortogonais ao `SECURITY_PROFILE` (`none`/`hmac`/`asym`, §1).

| Perfil | Env (`platformio.ini`) | Arquivos de *defaults* | eFuse | Uso |
|---|---|---|---|---|
| `dev` | todos os `collector_*` e `*_bench_*` | `sdkconfig.defaults` | nenhum | Bancada do dia a dia; placas reaproveitáveis. |
| `devsec` (ensaio de produção) | `collector_mock_devsec` | `sdkconfig.defaults` + `sdkconfig.devsec.defaults` | queima parte (§10.2), reversível só no que o modo de desenvolvimento permite | **Só placa descartável.** Valida assinatura, cifra e o procedimento antes do `prod`. |
| `prod` | `collector_twai_prod` | `sdkconfig.defaults` + `sdkconfig.prod.defaults` | queima tudo, irreversível (§10.3) | Demonstração. Placa não é mais atualizável sem OTA (§9.6). |

| Camada (§4.1 do documento) | `dev` | `devsec` | `prod` |
|---|---|---|---|
| NVS encryption (`secnvs` e `nvs`, XTS-AES) | **Sim, mas sem proteção real:** a `nvs_key` fica legível em claro na flash; serve para validar o caminho de código | Sim; `nvs_key` cifrada pela FE, mas a chave FE existe no host (protege só de quem não tem o arquivo) | Sim; `nvs_key` cifrada pela FE com chave gerada no chip |
| *Flash encryption* (cifra *bootloader*, tabela, app, `nvs_key`) | Não | Sim, modo **desenvolvimento**; chave gerada no host e queimada em BLOCK1 | Sim, modo **release**; chave gerada no chip no 1º boot (BLOCK1 protegido contra leitura/escrita) |
| Secure Boot V2 (exige ESP32 **rev. v3.0+**) | Não | Sim, com chave de assinatura própria do ensaio | Sim, chave de produção |
| eFuse: JTAG | Livre | Livre (`CONFIG_SECURE_BOOT_ALLOW_JTAG`) | Desabilitado (`JTAG_DISABLE`) |
| eFuse: UART download | Livre | **Livre** (é por onde se regrava) | Desabilitado (`UART_DOWNLOAD_DIS`, `CONFIG_SECURE_DISABLE_ROM_DL_MODE`) |
| eFuse: cifra/decifra/cache via download | — | `DISABLE_DL_ENCRYPT` livre (gravação cifrada pela ROM); `DISABLE_DL_DECRYPT`/`DISABLE_DL_CACHE` queimados **[verificar]** no `summary` | Todos queimados (automático no modo *release*) |
| *Core dump* | Flash (partição `coredump`) | Desligado | Desligado |
| LittleFS (blocos) | Em claro | Em claro | **Em claro** (integridade pela tag + CRC) |

**[Decisão] O log de blocos (LittleFS) não é cifrado**, nem em produção: o
`esp_littlefs` escreve em granularidade arbitrária e partições `encrypted`
exigem escrita alinhada em 16 B (`esp_flash_write_encrypted`); a propriedade
exigida dos blocos é integridade/autenticidade, garantida pela tag `K_rec`.
Limitação documentada: um atacante com acesso físico lê a telemetria
armazenada (privacidade), mas não a altera sem ser detectado.

### 9. Build único: Arduino como componente do ESP-IDF (passo 0 da fase)

#### 9.1 `platformio.ini`

Todos os envs do firmware (inclusive `_debug` e `*_bench_*`) passam a herdar
de `[base]`; o `env:native` (testes de host) não muda.

```ini
[base]
platform          = espressif32@7.1.3
board             = esp32dev
framework         = arduino, espidf
; ESP-IDF 4.4.7, a linha do Arduino 2.0.17 (optionalVersions do platform.json)
platform_packages = framework-espidf @ ~3.40407.0
monitor_speed     = 115200
build_flags       =
board_build.partitions = partitions_collector.csv
board_build.filesystem = littlefs
lib_deps = h2zero/NimBLE-Arduino@2.5.1

; Ensaio de produção: o env de depuração + um arquivo de defaults.
[env:collector_mock_devsec]
extends = env:collector_mock_debug
board_build.cmake_extra_args = -DSDKCONFIG_DEFAULTS="sdkconfig.defaults;sdkconfig.devsec.defaults"
board_build.encryption_key   = keys/devsec/fe_key_devsec.bin   ; usado só por `-t encrypt`

; Produção: o env collector_twai + um arquivo de defaults (única diferença).
[env:collector_twai_prod]
extends = env:collector_twai
board_build.cmake_extra_args = -DSDKCONFIG_DEFAULTS="sdkconfig.defaults;sdkconfig.prod.defaults"
```

`collector_twai_prod` herda `collector_twai` (sem `[debug]`), logo sem
`-DSECURITY_DEV_FIXED_KEYS` e com `SECURITY_PROFILE` padrão (`hmac`); as
guardas do §7 recusam o contrário. O flag `-DSECURITY_NVS_ENCRYPTION` da
versão anterior desta spec **deixa de existir**.

**Mecanismo conferido** no *builder* da plataforma instalada
(`~/.platformio/platforms/espressif32/builder/frameworks/espidf.py`, versão
**7.0.1**; **[verificar]** o mesmo arquivo depois de `pio pkg install` baixar a
7.1.3 pedida no `platformio.ini`):

| Fato | Onde (`espidf.py`) | Consequência |
|---|---|---|
| `board_build.cmake_extra_args` é lido como `board.get("build.cmake_extra_args")`, quebrado por `click.parser.split_arg_string` e anexado à chamada do CMake do projeto | l. 1921–1932 | `-DSDKCONFIG_DEFAULTS="a;b"` chega ao CMake do IDF como uma lista. |
| Cada env tem o próprio `sdkconfig.<env>` na raiz (`SDKCONFIG_PATH = sdkconfig.$PIOENV`, ou `board_build.esp-idf.sdkconfig_path`) | l. 106–111 | `dev`, `devsec` e `prod` não se contaminam. Os `sdkconfig.<env>` são gerados: ignorar no git (`/sdkconfig.*` com exceção `!/sdkconfig.defaults`, `!/sdkconfig.*.defaults`). |
| Reconfiguração automática só olha a data do `sdkconfig.defaults` e dos `CMakeLists.txt` | l. 131–167 | Mudar `sdkconfig.devsec.defaults`/`sdkconfig.prod.defaults` **não** reconfigura; e um `sdkconfig.<env>` existente vence os *defaults*. Regra: depois de editar qualquer `*.defaults`, apagar o `sdkconfig.<env>` correspondente. |
| Com `CONFIG_SECURE_BOOT`: versão 2 se `SECURE_BOOT_V2_ENABLED`; assina app, *bootloader* e tabela com `espsecure.py sign_data --version 2`; `--secure-pad-v2` no `elf2image`; recusa `SECURE_BOOT_V1` e `ESP32_REV_MIN_FULL < 300`; chave = `board_build.secure_boot_signing_key` ou `CONFIG_SECURE_BOOT_SIGNING_KEY` | l. 1063–1130, 1657–1675, 1953–1978, 2288–2292 | Artefatos `bootloader-signed.bin`, `partitions-signed.bin`, `firmware-signed.bin` em `.pio/build/<env>/` (**[verificar]** nomes no 1º *build*). |
| Em Secure Boot o `upload` **não** grava o *bootloader* e usa `--after no_reset` | l. 2154–2177, 2364–2373 | Primeira gravação de `devsec`/`prod` sempre pelo `esptool.py` explícito (§10). |
| `-t encrypt` com `board_build.encryption_key` gera `*-encrypted.bin` (`espsecure.py encrypt_flash_data --address <offset>`) | l. 1763–1786; `main.py` l. 389–394 | Regravação do `devsec` com imagem cifrada no host (§10.2). |
| Offset da tabela vem de `CONFIG_PARTITION_TABLE_OFFSET` (padrão `0x8000`); `gen_esp32part.py` recebe `--secure v2` em Secure Boot | l. 1677–1730, 2137 | O offset é configurado no `sdkconfig.defaults` comum (§9.2). |

Alternativas avaliadas: `board_build.esp-idf.sdkconfig_path` apontando para
um `sdkconfig` completo versionado por perfil (troca "acrescentar um arquivo
de *defaults*" por manter ~2 000 linhas por perfil — não recomendado);
`custom_sdkconfig` não existe nesta plataforma (é do *fork* pioarduino).
**Recomendação:** `board_build.cmake_extra_args` com `SDKCONFIG_DEFAULTS`.
Envs sem `cmake_extra_args` usam o `sdkconfig.defaults` da raiz
automaticamente (padrão do `project.cmake` do IDF). **[verificar]** que o IDF
4.4.7 resolve os caminhos relativos de `SDKCONFIG_DEFAULTS` a partir da raiz
do projeto e que o último arquivo da lista vence em chaves repetidas —
*checkpoint*: `grep CONFIG_SECURE_BOOT_V2_ENABLED=y sdkconfig.collector_twai_prod`.

#### 9.2 Arquivos do *build* híbrido

- `CMakeLists.txt` (raiz) — ver §9.3.
- `src/CMakeLists.txt`: `FILE(GLOB_RECURSE app_sources ${CMAKE_SOURCE_DIR}/src/*.*)` +
  `idf_component_register(SRCS ${app_sources})` (o mesmo que o *builder* gera
  sozinho quando falta; versionar para não depender disso).
- `sdkconfig.defaults` (comum aos três perfis). **[Decisão]** Partir das
  opções do `tools/sdk/esp32/sdkconfig` do Arduino 2.0.17 que diferem do
  padrão do IDF, para o passo 0 mudar o mínimo de comportamento:

```
CONFIG_FREERTOS_HZ=1000
CONFIG_AUTOSTART_ARDUINO=y
# BT como no Arduino pré-compilado: Bluedroid no IDF, NimBLE do IDF desligado,
# para o NimBLE-Arduino continuar com a pilha própria e os -DCONFIG_BT_NIMBLE_*
# (spec 04 §7, Fase 2) continuarem valendo
CONFIG_BT_ENABLED=y
CONFIG_BT_BLUEDROID_ENABLED=y
# CONFIG_BT_NIMBLE_ENABLED is not set
# CONFIG_ESP32_SPIRAM_SUPPORT is not set
CONFIG_ESPTOOLPY_FLASHSIZE_4MB=y
CONFIG_PARTITION_TABLE_CUSTOM=y
CONFIG_PARTITION_TABLE_CUSTOM_FILENAME="partitions_collector.csv"
CONFIG_PARTITION_TABLE_OFFSET=0x10000
CONFIG_ESP_COREDUMP_ENABLE_TO_FLASH=y
```

- `sdkconfig.devsec.defaults`:

```
CONFIG_ESP32_REV_MIN_3=y
CONFIG_SECURE_SIGNED_APPS_RSA_SCHEME=y
CONFIG_SECURE_BOOT=y
CONFIG_SECURE_BOOT_V2_ENABLED=y
CONFIG_SECURE_BOOT_BUILD_SIGNED_BINARIES=y
CONFIG_SECURE_BOOT_SIGNING_KEY="keys/devsec/secure_boot_signing_key_devsec.pem"
CONFIG_SECURE_BOOT_INSECURE=y
CONFIG_SECURE_BOOT_ALLOW_JTAG=y
CONFIG_SECURE_FLASH_ENC_ENABLED=y
CONFIG_SECURE_FLASH_ENCRYPTION_MODE_DEVELOPMENT=y
CONFIG_SECURE_FLASH_UART_BOOTLOADER_ALLOW_ENC=y
CONFIG_SECURE_INSECURE_ALLOW_DL_MODE=y
CONFIG_NVS_ENCRYPTION=y
CONFIG_ESP_COREDUMP_ENABLE_TO_NONE=y
```

- `sdkconfig.prod.defaults`:

```
CONFIG_ESP32_REV_MIN_3=y
CONFIG_SECURE_SIGNED_APPS_RSA_SCHEME=y
CONFIG_SECURE_BOOT=y
CONFIG_SECURE_BOOT_V2_ENABLED=y
CONFIG_SECURE_BOOT_BUILD_SIGNED_BINARIES=y
CONFIG_SECURE_BOOT_SIGNING_KEY="secure_boot_signing_key.pem"
# CONFIG_SECURE_BOOT_ALLOW_JTAG is not set
CONFIG_SECURE_FLASH_ENC_ENABLED=y
CONFIG_SECURE_FLASH_ENCRYPTION_MODE_RELEASE=y
CONFIG_SECURE_DISABLE_ROM_DL_MODE=y
CONFIG_NVS_ENCRYPTION=y
CONFIG_ESP_COREDUMP_ENABLE_TO_NONE=y
CONFIG_BOOTLOADER_LOG_LEVEL_WARN=y
```

**[verificar]** Os nomes das opções de segurança contra o Kconfig do IDF
4.4.7 (`components/bootloader/Kconfig.projbuild`, `components/nvs_flash/Kconfig`)
assim que o `framework-espidf` for instalado: opção com nome errado é ignorada
em silêncio pelo `kconfig`. *Checkpoint* obrigatório antes de gravar qualquer
placa: **cada** linha `=y` acima aparece igual no `sdkconfig.<env>` gerado
(não só `SECURE_BOOT_V2_ENABLED`). Pontos específicos: `CONFIG_SECURE_BOOT_INSECURE`
deve ser só o portão do menu "opções potencialmente inseguras" (necessário
para `ALLOW_JTAG`), sem desligar a verificação de assinatura do V2 — se no
Kconfig real ela for ligada ao Secure Boot V1 ou tiver outro efeito, remover
a linha e aceitar JTAG desabilitado no `devsec`; `CONFIG_SECURE_INSECURE_ALLOW_DL_MODE`
é a opção explícita da escolha "UART ROM download mode" — se o nome não
existir, apagar a linha (não definir `CONFIG_SECURE_DISABLE_ROM_DL_MODE` já
mantém o *download* livre).
`CONFIG_ESP32_REV_MIN_3` fica só nos perfis com Secure Boot: o *bootloader*
do IDF recusa boot em chip mais antigo, e o `dev` precisa rodar em qualquer
placa.

#### 9.3 NVS encryption no perfil `dev` (ajuste do componente `nvs_flash`)

No IDF 4.4, `CONFIG_NVS_ENCRYPTION` **depende de `SECURE_FLASH_ENC_ENABLED`**
(evidência local no estado atual); sem *flash encryption* não dá para ligá-la
pelo `sdkconfig`, e o componente `nvs_flash` é compilado sem
`nvs_encrypted_partition.cpp` e sem as funções `nvs_flash_secure_init*`,
`nvs_flash_generate_keys`, `nvs_flash_read_security_cfg`. Para o caminho
único do `KeyStore` (§7) existir também no `dev`:

```cmake
# CMakeLists.txt (raiz) — igual nos três perfis
cmake_minimum_required(VERSION 3.16.0)
include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(tccelta_dongle)

# NVS encryption em todos os perfis (spec 01 §9.3). Com FE ligada o sdkconfig
# já traz CONFIG_NVS_ENCRYPTION=y e este bloco não faz nada.
if(NOT CONFIG_NVS_ENCRYPTION)
  idf_component_get_property(nvs_lib nvs_flash COMPONENT_LIB)
  idf_component_get_property(nvs_dir nvs_flash COMPONENT_DIR)
  target_compile_definitions(${nvs_lib} PRIVATE CONFIG_NVS_ENCRYPTION=1)
  target_sources(${nvs_lib} PRIVATE "${nvs_dir}/src/nvs_encrypted_partition.cpp")
  target_link_libraries(${nvs_lib} PRIVATE idf::mbedtls)
endif()
```

- O XTS do mbedTLS já está ligado incondicionalmente
  (`mbedtls/esp_config.h`: `#define MBEDTLS_CIPHER_MODE_XTS`).
- Sem FE, a flag `encrypted` da `nvs_key` não tem efeito (a partição é lida e
  gravada em claro) — **[verificar]** no *checkpoint* abaixo.
- **[verificar]** que o *builder* do PlatformIO compila as fontes e
  definições acrescentadas depois de `project()` (ele lê o *code model* do
  CMake, que já as inclui). Proteção natural: se o ajuste falhar, o
  `KeyStore` não liga (`undefined reference to nvs_flash_secure_init_partition`).

*Checkpoint* do §9.3 (placa `dev`, depois de `erase_flash`): (1) o *build*
liga; (2) primeiro boot imprime `[sec] fe=off … nvs_enc=1` e `KEYS
source=dev_fixed`; (3) `esptool.py read_flash 0x17000 0x1000 nvs_key.bin`
mostra 64 B de chave + CRC em claro (não `FF…`); (4) `read_flash` da `nvs`
não mostra em claro as chaves `seq_rsv`/`collector` nos valores (só os
cabeçalhos de página); (5) reboot preserva `seq_rsv`.

#### 9.4 Tabela de partições única (`partitions_collector.csv`)

Requisitos do Secure Boot V2 + *flash encryption* considerados: *bootloader*
maior (com assinatura de 4 KB no fim, alinhado a 4 KB) — o atual pré-compilado
tem 17 536 B sem log, mas o do IDF com SB V2 + FE não cabe com folga nos 28 KB
de `0x1000–0x8000` — **por isso o offset da tabela passa de `0x8000` para
`0x10000`** (60 KB para o *bootloader*); tamanho real **[verificar]** no 1º
*build* `devsec` (o IDF falha o *build* se o *bootloader* invadir a tabela); tabela assinada pelo PlatformIO
(`partitions-signed.bin` = tabela preenchida até 4 KB + setor de assinatura de
4 KB = 8 KB); app com início alinhado a 64 KB e tamanho múltiplo de 64 KB,
com a imagem preenchida (`--secure-pad-v2`) + bloco de assinatura de 4 KB
dentro da partição; partições `encrypted` alinhadas a 4 KB.

```
# Coleta (esp32dev, 4 MB). Mesma tabela em dev, devsec e prod.
# Tabela em 0x10000 (CONFIG_PARTITION_TABLE_OFFSET). Sem OTA (spec 01 §9.6).
# Name,   Type, SubType,  Offset,   Size,     Flags
nvs,      data, nvs,      0x12000,  0x5000,
nvs_key,  data, nvs_keys, 0x17000,  0x1000,   encrypted
phy_init, data, phy,      0x18000,  0x1000,
secnvs,   data, nvs,      0x19000,  0x3000,
factory,  app,  factory,  0x20000,  0x1D0000,
spiffs,   data, spiffs,   0x1F0000, 0x200000,
coredump, data, coredump, 0x3F0000, 0x10000,
```

| Faixa | Conteúdo | Por quê |
|---|---|---|
| `0x1000–0x10000` (60 KB) | *Bootloader* (assinado em `devsec`/`prod`) | Folga para SB V2 + FE + log `WARN`. |
| `0x10000–0x12000` (8 KB) | Tabela (3 KB) e, se gravada, a assinatura de 4 KB do `partitions-signed.bin` | No SB V2 a tabela não é verificada pelo *bootloader*; a gravação manual usa o `partitions.bin` sem assinatura, mas os 8 KB evitam que o `upload` do PlatformIO (que grava a versão assinada) atropele a `nvs`. |
| `0x12000` `nvs` (20 KB) | `collector`, `nimble_bond` | Mesmo tamanho de hoje. |
| `0x17000` `nvs_key` (4 KB) | Chaves XTS da NVS | Única partição de dados `encrypted`; tamanho exigido pelo IDF. |
| `0x19000` `secnvs` (12 KB) | Chaves do dongle | Mínimo de 3 páginas do `nvs_partition_gen`. |
| `0x1C000–0x20000` (16 KB) | Livre | Reserva (ex.: `otadata`, §9.6). |
| `0x20000` `factory` (1,8125 MB = 29 × 64 KB) | App | Cabem o app atual (~630 KB) e o *build* híbrido com folga; **[verificar]** com `pio run -e collector_twai_prod -t size`. |
| `0x1F0000` `spiffs` (2 MB) | LittleFS | Mesmo tamanho e offset de hoje (Tabela 14 da Investigação inalterada). |
| `0x3F0000` `coredump` (64 KB) | *Core dump* | Usado só no `dev`; inerte em `devsec`/`prod` (`ENABLE_TO_NONE`). |

**[Decisão] `partitions_collector_prod.csv` deixa de existir**: a tabela
acima vale nos três perfis. Custo único: a tabela muda de offset
(`0x8000 → 0x10000`) e a `nvs` muda de lugar, então **toda placa precisa de
`erase_flash` uma vez** ao entrar no passo 0 (sem isso a `nvs_key` cai sobre
bytes do app antigo, `nvs_flash_read_security_cfg` acusa partição corrompida
e a NVS não sobe). Consequência: a reserva de `seq`, o `last_cs` e os *bonds*
de bancada recomeçam do zero (servidor de bancada: "Esquecer estado do
dongle", spec 09), e as migrações de `last_seq` (spec 03) e de `/log` v1
(spec 02) só rodam em casos defensivos.

#### 9.5 Passo 0: migração do *build* (com *timebox*)

Ordem: (1) `[base]` com `framework = arduino, espidf` e `platform_packages`;
`CMakeLists.txt`, `src/CMakeLists.txt`, `sdkconfig.defaults` (sem o bloco do
§9.3 ainda) e a tabela nova; (2) `pio run -e collector_mock_debug` e
`collector_twai_debug`; (3) gravar com `erase_flash` + `pio run -t upload`;
(4) acrescentar o bloco do §9.3.

*Checkpoint* do passo 0 (antes de **qualquer** código de segurança): o
firmware atual, sem mudança de código, compila em todos os envs e roda numa
placa sobre Arduino + ESP-IDF — coleta (`collector_mock_debug`: `FLUSH`
periódico), NUS BLE (app conecta e `AT#LIST` responde), LittleFS (blocos
sobrevivem a reboot), NVS (`boot_id` incrementa entre boots) — e o pool de
*mbufs* do NimBLE continua o configurado por `-D` (spec 04 §7). Conferir
também no `.map`/`nm` do `firmware.elf` que só há um host NimBLE linkado (o
do NimBLE-Arduino; nenhum `ble_hs_*` vindo de `components/bt/host/nimble`) e
anotar o tamanho do app híbrido (entra na decisão de OTA, §9.6).

**[Aberto] Risco da migração** (maior risco da fase: compatibilidade do
`framework-espidf 3.40407` com o Arduino 2.0.17 na plataforma 7.x; NimBLE-Arduino
com o BT do IDF; tempo de compilação — cada env compila o IDF inteiro).
**Recomendação:** *timebox* de ~2 dias de trabalho para o *checkpoint*
acima. Planos B, nesta ordem:
- **B1** — o híbrido funciona, mas o ajuste do §9.3 não: o `KeyStore` passa a
  escolher pelo `CONFIG_NVS_ENCRYPTION` do `sdkconfig` (`#if`, único ponto do
  código que varia; `dev` abre a `secnvs` em claro com
  `nvs_flash_init_partition`); a diferença entre perfis continua sendo só
  `sdkconfig` + procedimento, e o caminho cifrado é validado no `devsec`.
- **B2** — o híbrido não fecha: todos os envs voltam ao Arduino puro
  (`framework = arduino`, tabela atual), `KeyStore` em claro; as camadas de
  hardware são demonstradas numa placa descartável com um projeto IDF
  mínimo (`idf.py`) que só gera *bootloader*/`sdkconfig` assinados; a
  limitação é registrada no TCC.

#### 9.6 OTA

**[Aberto]** A tabela não tem OTA: com 2 MB de LittleFS, a escolha foi um só
app de 1,8 MB. Em `prod`, com o *download* da ROM desabilitado
(`UART_DOWNLOAD_DIS`) e a FE em *release*, **firmware novo só entra por OTA
assinado**; sem OTA, a placa `prod` nunca mais é atualizada (nem
reprovisionada). A tabela também não pode mudar depois do `prod`.
**Recomendação para o TCC:** manter as placas de bancada em `dev`/`devsec` e
tratar `prod` como demonstração (uma placa dedicada, com o firmware final).
Alternativa a decidir **antes** de queimar qualquer placa `prod`: o app atual
tem ~630 KB, então dois *slots* de `0xE0000` (896 KB) — `otadata 0x1C000/0x2000`,
`ota_0 0x20000/0xE0000`, `ota_1 0x100000/0xE0000` — caberiam sem tirar nada do
LittleFS (`0x1E0000–0x1F0000` livre), se o app híbrido assinado ficar abaixo
de ~800 KB; o OTA em si (transporte pelo app via BLE, rollback, anti-rollback
com `SECURE_VERSION`) fica fora da Fase 1.

### 10. Procedimentos por perfil

Comandos com o `esptool`/`espsecure`/`espefuse` 4.x do PlatformIO
(`~/.platformio/packages/tool-esptoolpy/`). Material sensível em `keys/`
(fora do git) e em `tools/provision/out/` (apagar no fim).

#### 10.1 Conferência do chip (antes de `devsec`/`prod`)

`esptool.py --port COMx chip_id` → "ESP32-D0WD-V3 (revision v3.0)" ou maior;
`espefuse.py --port COMx summary > efuse_antes.txt` → `FLASH_CRYPT_CNT = 0`,
`ABS_DONE_1 = 0`, BLOCK1/BLOCK2 zerados, `UART_DOWNLOAD_DIS = 0`.

**Placa antiga (< v3.0):** sem Secure Boot V2 (o *builder* do PlatformIO
também recusa Secure Boot V1) e sem `UART_DOWNLOAD_DIS`. Plano: comprar
módulos `ESP32-D0WD-V3`/`ESP32-WROOM-32E`; se não houver, a placa antiga
fica em `dev` ou num `devsec` sem as linhas de Secure Boot e sem
`CONFIG_ESP32_REV_MIN_3` (só FE de desenvolvimento + NVS encryption),
registrado como limitação.

#### 10.2 `devsec` — ensaio de produção (placa descartável)

> **AVISO.** Consome eFuses de forma permanente. A placa passa a aceitar só
> firmware assinado com a chave de ensaio e cifrado com a chave FE de ensaio.
> **O ESP32 tem um único *digest* de Secure Boot V2 (BLOCK2) e não tem
> revogação de chave** (revogação existe a partir do ESP32-S2/C3/S3): a chave
> de ensaio fica gravada para sempre. Nunca usar a chave de produção aqui.

| # | Ação | Comando | Consome eFuse | Verificação |
|---|---|---|---|---|
| 0 | (uma vez) Chaves de ensaio | `espsecure.py generate_signing_key --version 2 --scheme rsa3072 keys/devsec/secure_boot_signing_key_devsec.pem`; `espsecure.py generate_flash_encryption_key keys/devsec/fe_key_devsec.bin` | Não | Fora do git; guardar junto da placa. Sem a chave FE ainda se regrava pela ROM (`--encrypt`), mas não se decifra a flash no host. |
| 1 | Conferir o chip | §10.1 | Não | `efuse_antes.txt` guardado. |
| 2 | Compilar | `pio run -e collector_mock_devsec` | Não | `sdkconfig.collector_mock_devsec` com as linhas do §9.2; `espsecure.py verify_signature --version 2 --keyfile keys/devsec/secure_boot_signing_key_devsec.pem .pio/build/collector_mock_devsec/firmware-signed.bin`. |
| 3 | Provisionar | `provision.py --profile devsec --nvs-key new --out out/` | Não | `out/<DevId>/{secnvs.csv, secnvs.bin, keys/nvs_keys.bin, bench_keys.json, kms_record.json}` (§11). |
| 4 | Queimar a chave FE do host | `espefuse.py --port COMx burn_key flash_encryption keys/devsec/fe_key_devsec.bin` | **Sim**: BLOCK1 + proteção de leitura/escrita dele | `summary`: BLOCK1 ilegível (`??`). |
| 5 | Apagar e gravar em claro | `esptool.py --port COMx erase_flash`; `esptool.py --port COMx --chip esp32 write_flash 0x1000 bootloader-signed.bin 0x10000 partitions.bin 0x17000 out/<DevId>/keys/nvs_keys.bin 0x19000 out/<DevId>/secnvs.bin 0x20000 firmware-signed.bin` | Não | Tudo gravado **antes** do primeiro boot (inclusive `nvs_key`, §7). |
| 6 | Primeiro boot, **sem desligar** | `pio device monitor -e collector_mock_devsec` | **Sim**: `FLASH_CRYPT_CONFIG = 0xF`, 1 bit de `FLASH_CRYPT_CNT` (de 7), `DISABLE_DL_DECRYPT`, `DISABLE_DL_CACHE`; Secure Boot: *digest* em BLOCK2 (protegido contra escrita), `ABS_DONE_1`, e o que mais o *bootloader* queimar | O *bootloader* encontra a chave FE já queimada (não gera outra), cifra *bootloader*/tabela/app/`nvs_key` no lugar, grava o *digest*; `[sec] fe=dev sb=1 jtag_off=0 dl_off=0 crypt_cnt=1 nvs_enc=1`; `KEYS source=partition fe=dev sb=1`. |
| 7 | Conferir | `espefuse.py --port COMx summary > efuse_depois.txt`; `diff efuse_antes.txt efuse_depois.txt` | — | Só os campos dos passos 4 e 6 mudaram; `UART_DOWNLOAD_DIS = 0`, `DISABLE_DL_ENCRYPT = 0`. **[verificar]** a lista exata do passo 6 contra este *diff* e registrá-la na spec. |
| 8 | Regravar o app (ciclo normal) | `pio run -e collector_mock_devsec -t encrypt` → `esptool.py --port COMx write_flash 0x20000 .pio/build/collector_mock_devsec/firmware-encrypted.bin` (pré-cifrado no host); alternativa: `esptool.py --port COMx write_flash --encrypt 0x20000 firmware-signed.bin` (cifra pela ROM) | **Não** consome `FLASH_CRYPT_CNT` **[verificar]** | `summary`: `FLASH_CRYPT_CNT` igual ao do passo 7; boot normal. **Não desconectar durante a gravação:** uma gravação cifrada interrompida deixa a partição corrompida sem que o *bootloader* perceba (paridade continua ímpar); repetir a gravação inteira. |
| 9 | Recusa de imagem não assinada | Assinar um `firmware.bin` com outra chave, cifrar (`espsecure.py encrypt_flash_data --keyfile keys/devsec/fe_key_devsec.bin --address 0x20000`) e gravar como no passo 8 | Não | Log do *bootloader*: `secure boot verification failed`; regravar o correto (passo 8) recupera a placa. |

Contador de regravação e ciclos — o que foi conferido localmente:
- `FLASH_CRYPT_CNT` tem **7 bits** (`soc/efuse_reg.h`: `EFUSE_RD_FLASH_CRYPT_CNT`,
  bits 26:20); a FE está ligada quando o número de bits é ímpar
  (`esp_flash_encrypt.h`).
- O *bootloader* só cifra no lugar e acrescenta um bit ao contador quando a
  paridade é **par** ("assume the flash has just been written with
  plaintext"); com paridade ímpar ele retorna sem tocar no eFuse
  (`esp_flash_encrypt_check_and_update` em `esp_flash_encrypt.h`). Daí a afirmação
  do passo 8: gravar imagem já cifrada (no host ou pela ROM) **não** consome
  o contador — conferida no comentário do código, **[verificar]** na placa
  pelo *diff* do `summary`.
- O caminho "desligar a FE → gravar em claro → religar" gasta 2 bits por
  ciclo; partindo do bit do 1º boot, sobram **3 ciclos** **[verificar]** na
  documentação do IDF 4.4 e na revisão do chip. Com Secure Boot V2 ativo
  esse caminho não é usado neste ensaio (regravar sempre cifrado, passo 8).

#### 10.3 `prod` — irreversível

> **AVISO.** Depois do primeiro boot: a placa só executa firmware assinado
> com a chave de produção; não aceita mais *download* pela UART (não dá para
> regravar, ler a flash pela serial, reprovisionar nem recuperar a placa); a
> chave de *flash encryption* nunca sai do chip; JTAG fica morto; sem OTA
> (§9.6) a placa não é mais atualizável. Fazer o `devsec` (§10.2) antes.
> Perder `secure_boot_signing_key.pem` = não conseguir gerar firmware para
> nenhum dongle já produzido.

| # | Ação | Comando | Verificação |
|---|---|---|---|
| 0 | (uma vez) Gerar a chave de Secure Boot | `espsecure.py generate_signing_key --version 2 --scheme rsa3072 secure_boot_signing_key.pem` | Guardar offline (cofre/HSM); nunca no repositório (`.gitignore`). |
| 1 | (por versão) Compilar assinado | `pio run -e collector_twai_prod` | `sdkconfig.collector_twai_prod` com as linhas do §9.2; `espsecure.py verify_signature --version 2 --keyfile secure_boot_signing_key.pem .pio/build/collector_twai_prod/firmware-signed.bin`. |
| 2 | Conferir o chip | §10.1 | Se < v3.0: parar. `efuse_antes.txt` guardado (última leitura possível pelo host). |
| 3 | Provisionar | `python tools/provision/provision.py --profile prod --nvs-key new --kms-wrapping kms_wrapping.json --out out/` (`kms_wrapping.json` vem de `tccelta provisionar exportar-wrapping`, Fase 3) | `out/<DevId>/{secnvs.csv, secnvs.bin, keys/nvs_keys.bin, kms_record.json}` (§11). |
| 4 | Apagar a flash | `esptool.py --port COMx erase_flash` | Flash em branco. |
| 5 | Gravar em claro | `esptool.py --port COMx --chip esp32 write_flash 0x1000 bootloader-signed.bin 0x10000 partitions.bin 0x17000 out/<DevId>/keys/nvs_keys.bin 0x19000 out/<DevId>/secnvs.bin 0x20000 firmware-signed.bin` | Nunca `pio run -t upload` (não grava o *bootloader* em Secure Boot, §9.1). `nvs_key` gravada antes do 1º boot (§7). |
| 6 | Primeiro boot, **sem desligar** (até ~1 min) | `pio device monitor -e collector_twai_prod` | O *bootloader* gera a chave FE, cifra *bootloader*/tabela/app/`nvs_key` no lugar, queima `FLASH_CRYPT_CNT` e as proteções do modo *release* (`DISABLE_DL_*`, proteção de escrita do contador); o Secure Boot grava o *digest* e `ABS_DONE_1`; o app desabilita o *download* da ROM. `KEYS source=partition fe=release sb=1`. |
| 7 | Conferir eFuses **pelo próprio firmware** (o `espefuse.py` precisa do modo *download*, que o passo 6 desabilitou) | Linha `[sec]` (§7) | `fe=release sb=1 jtag_off=1 dl_off=1`, `crypt_cnt` ímpar. |
| 8 | Registrar no KMS | `tccelta provisionar importar-registro --arquivo kms_record.json` (Fase 3, spec 07) | Dongle no banco e chaves importadas no Vault; teste dourado OK. |
| 9 | Destruir material local | Apagar com sobrescrita `out/<DevId>/` | A estação não guarda chaves (documento §4.1). |

**[Decisão]** A `nvs_key` (chaves XTS da NVS) é gerada **na estação**
(passo 3) e gravada em claro antes do primeiro boot; o *bootloader* a cifra
no lugar com a chave de *flash encryption* gerada no chip. Isso permite gravar
`secnvs.bin` já cifrada sem que a chave FE saia do chip, ao custo de as chaves
XTS existirem por instantes na estação — o mesmo modelo de confiança que o
documento aceita para `K_auth`/`K_rec`. Ordem de queima no ESP32 v3: o IDF
desabilita o *download* da ROM antes de proteger `FLASH_CRYPT_CNT` contra
escrita (nota de `esp_flash_write_protect_crypt_cnt` em `esp_flash_encrypt.h`).

### 11. Provisionamento (`esp32-firmware/tools/provision/`)

`provision.py` (Python 3.11, dependências em `requirements.txt`:
`esp-idf-nvs-partition-gen`, `cryptography`, `esptool`):

```
provision.py --profile {dev,devsec,prod} [--security {hmac,asym,none}] [--port COMx]
             [--kms-wrapping kms_wrapping.json] [--dev-id HEX16] [--out DIR] [--flash]
             [--nvs-key {auto,new,board}] [--fe-key keys/devsec/fe_key_devsec.bin]
             [--import secnvs.csv]     # bancada: usa chaves geradas por `tccelta provisionar novo` (Fase 3)
```

| Passo | Detalhe |
|---|---|
| Entropia | `secrets.token_bytes(n)` (CSPRNG do SO: `BCryptGenRandom`/`getrandom`). `DevId` aleatório de 8 B, rejeitando tudo-zero e tudo-FF; `--dev-id` só para reprovisionar a mesma placa. `k_sig`: `ec.generate_private_key(SECP256R1())`, exportado como escalar de 32 B. |
| CSV | `secnvs.csv` no formato do gerador: `key,type,encoding,value` / `sec,namespace,,` / `prov_ver,data,u8,1` / `dev_id,data,hex2bin,<16 hex>` / `k_auth,data,hex2bin,<64 hex>` / `k_rec,…` / `k_sig,…` (só `asym`) / `prof,data,u8,<0-2>`. |
| Chave XTS (`nvs_key`) | **[Decisão]** Igual nos três perfis (a NVS é sempre cifrada). `--nvs-key new`: `python -m esp_idf_nvs_partition_gen generate-key --keyfile nvs_keys.bin --outdir out/<DevId>` → `out/<DevId>/keys/nvs_keys.bin` (placa em branco; obrigatório em `prod`). `--nvs-key board`: lê a `nvs_key` da placa (`esptool.py read_flash 0x17000 0x1000`), decifra no `devsec` com `espsecure.py decrypt_flash_data --keyfile <--fe-key> --address 0x17000`, confere o CRC do formato `nvs_keys` e a reaproveita — mantém legíveis a `nvs` (reserva de `seq`, `last_cs`, *bonds*) de uma placa que já rodou o firmware. `auto` (padrão): `board` se `--flash` e a placa tiver chave válida (`dev`/`devsec`), senão `new`. |
| Nome do módulo | **[verificar]** ao montar o ambiente que o pacote PyPI `esp-idf-nvs-partition-gen` expõe o módulo `esp_idf_nvs_partition_gen` (`python -m esp_idf_nvs_partition_gen --help`); se não, usar o nome real em todas as chamadas abaixo. |
| Imagem `secnvs` | Sempre cifrada: `python -m esp_idf_nvs_partition_gen encrypt secnvs.csv secnvs.bin 0x3000 --inputkey out/<DevId>/keys/nvs_keys.bin`. A imagem em claro (`generate`) deixa de existir. |
| Gravação (`--flash`, `dev`/`devsec`) | `esptool.py --port COMx write_flash 0x19000 secnvs.bin` (a `secnvs` não é `encrypted`: grava-se igual com ou sem FE) e, só com `--nvs-key new`, `0x17000 nvs_keys.bin` — no `dev` em claro; no `devsec` já com FE ativa, com `write_flash --encrypt` (cifra pela ROM). Com `--nvs-key new` numa placa que já rodou o firmware, a `nvs` existente fica ilegível e é recriada (perde reserva de `seq`, `last_cs`, *bonds*): o script exige `--yes` e avisa. Offsets lidos de `partitions_collector.csv`, não fixos no script. `prod` não tem `--flash`: a gravação é o passo 5 do §10.3. |
| Registro KMS | `kms_record.json` (ver abaixo). Com `--kms-wrapping kms_wrapping.json` (exportado por `tccelta provisionar exportar-wrapping`, Fase 3 spec 07: as *wrapping keys* RSA-4096 dos mounts `transit-auth` e `transit-rec`), `k_auth` e `k_rec` vão no formato BYOK do Vault Transit (`alg: "vault-byok-rsa-oaep-sha256-aes-kwp"`: chave AES-256 efêmera cifrada com RSA-OAEP-SHA256 pela wrapping key do mount, seguida do material embrulhado com AES-KWP, RFC 5649), cada uma com o `kid` do seu mount; sem ela (só `--profile dev`/`devsec`) vão em claro com `"alg":"none"` e o script imprime aviso. A opção `--kms-pubkey` não existe mais. O esquema do `kms_record.json` **não muda** com os perfis de hardware. |
| Saída de bancada | `--profile dev`/`devsec` também grava `out/<DevId>/bench_keys.json` (`dev_id`, `k_auth`, `k_rec` em hex) para o servidor de bancada do app (spec 09). |

```json
{
  "schema": "tccelta.kms.v1",
  "dev_id": "544343454C544101",
  "security_profile": "hmac",
  "created_at": "2026-10-26T12:00:00Z",
  "k_auth": { "alg": "vault-byok-rsa-oaep-sha256-aes-kwp", "kid": "<sha256 da wrapping key do transit-auth, 8 B hex>", "ciphertext_b64": "..." },
  "k_rec":  { "alg": "vault-byok-rsa-oaep-sha256-aes-kwp", "kid": "<idem, transit-rec>", "ciphertext_b64": "..." },
  "k_sig_public": "04…"
}
```

**[Decisão, reconciliação com a Fase 3]** O envelope é o formato BYOK do
Vault Transit, para que `tccelta provisionar importar-registro` (Fase 3,
spec 07) passe `ciphertext_b64` direto a `transit-*/keys/<DevId>/import`.
`kms_record.json` é o contrato de saída do provisionamento e a Fase 3 o
consome sem transformação; `k_sig_public` (65 B, `04‖X‖Y`) vai para
`dongle.chave_publica` no perfil `asym`. Este arquivo é a fonte única do
esquema; a Fase 3 o copia.

### 12. Vetores de teste (chaves fixas de bancada)

Calculados com `hmac`/`hashlib`/`zlib` do Python 3.11. **[Decisão]** O
gerador vira código versionado — `esp32-firmware/tools/provision/vectors.py`
(só biblioteca padrão) gera `vectors.json`, copiado para
`esp32-firmware/test/support/vectors.h` (gerado) e
`mobile-app/test/support/sync_vectors.dart` (gerado); `test_crypto` e
`bench_crypto_test.dart` consomem esses arquivos, e um teste em cada lado
falha se o arquivo gerado divergir do `vectors.json`. Assim firmware e app
nunca ficam "verdes isoladamente" com vetores diferentes. A Fase 3 (spec 03
§Vetores) copia o mesmo `vectors.json` para
`backend/internal/protocolo/testdata/vetores_firmware.json` e o verifica no
pacote `protocolo` — os três lados consomem um único gerador. Atenção ao `DevId`:
`"TCCELTA\x01"` = `54 43 43 45 4C 54 41 01`.

| Caso | Entrada (`payload`/dados, hex) | Saída esperada |
|---|---|---|
| Desafio, `tipo=0x01`: `c_S=7`, `N_S=00..0F`, `ACK_seq=100`, `c_conf=6`, `flags=0` | `07000000 000102030405060708090A0B0C0D0E0F 64000000 06000000 00` (29 B) | tag `5E08174F9D21CAA313D52A39DCD31943` |
| Desafio com diagnóstico: `c_S=8`, `ACK_seq=100`, `c_conf=7`, `flags=1`, `diag_min=30` | `08000000 000102…0F 64000000 07000000 01 1E00` (31 B) | tag `2689763B1FF2BAD8FC7955D4C7ECDEB3` |
| Resposta, `tipo=0x02`: `c_S=7`, `N_S=00..0F`, `TAIL=65`, `HEAD=130`, `boot_id=3`, `t_ms=123456` | `07000000 000102…0F 41000000 82000000 0300 40E20100` (34 B) | tag `3A93F9DC93EDA9E7DBAD035426EEB061` |
| ACK, `tipo=0x04`: `ACK_seq=130`, `c_conf=7`, `flags=0` | `82000000 07000000 00` | tag `84EB2C1203B2720E492DB89E04F1FAE7` |
| Bloco BOOT `seq=192`, `boot_id=4`, `t_ms=812`, `[131,191]` | `C0000000 0400 2C030000 1700 FE 0000 83000000 BF000000` (23 B) | tag `E55ECDF18FE15DDDF31661086B87DF51544C089829BD9DF2B44B278BAC333D8D` |
| Bloco LOSS `seq=140`, `[140,141]` | `8C000000 0000 00000000 1700 FD 0000 8C000000 8D000000` | tag `558FF3EEA60111C3ED541CD7BA3D1BB42CCB5A81CE6ABF71835DA7F747C0FF9C` |
| SHA-256 da entrada ECDSA do BOOT acima | `"MALCONG"‖03‖DevId‖dados` | `35828A8792008036988697D32C62577927F779F84199216BF701CEB4226BC40D` |
| CRC-32 | `"123456789"` | `CBF43926` |

Mais os vetores padrão: SHA-256(`""`) = `e3b0c442…b855`; SHA-256(`"abc"`) =
`ba7816bf…15ad`; HMAC RFC 4231 caso 1 (chave `0b`×20, `"Hi There"`) =
`b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7`; caso 2
(chave `"Jefe"`, `"what do ya want for nothing?"`) =
`5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843`.

## Plano arquivo por arquivo

| Ação | Arquivo | Conteúdo |
|---|---|---|
| Criar | `src/crypto/Sha256.{h,cpp}` | `crypto::Sha256` portátil. |
| Criar | `src/crypto/HmacSha256.{h,cpp}` | `IHmacSha256`, `PortableHmacSha256`. |
| Criar | `src/crypto/Crc32.{h,cpp}` | `crc32()` com tabela `constexpr`. |
| Criar | `src/crypto/ConstantTime.h` | `ctEqual`, `secureZero`. |
| Criar | `src/security/SecurityProfile.h` | Macros e `constexpr` do §1. |
| Criar | `src/security/DeviceKeys.h` | `DeviceKeys`, `KeySource`, `kDevFixedKeys` (só com `SECURITY_DEV_FIXED_KEYS`), `kDomainLabel[7]`. |
| Criar | `src/security/BlockAuth.{h,cpp}` | `HmacBlockAuth` (puro). |
| Criar | `src/security/HandshakeAuth.{h,cpp}` | `IHandshakeAuth`, `HmacHandshakeAuth`, `NoHandshakeAuth` (puros). |
| Criar | `src/security/mbed/MbedHmacSha256.{h,cpp}` | Adaptador mbedTLS (§3). |
| Criar | `src/security/mbed/EcdsaBlockAuth.{h,cpp}` | Perfil `asym`; compila em todos os envs ESP32, instanciado só com `SECURITY_PROFILE=2`. |
| Alterar | `src/collector/IMessageAuth.h` | Só o comentário do contrato (§4); `NoAuth` fica. |
| Criar | `src/storage/KeyStore.{h,cpp}` | §7 (caminho único por NVS encryption, linha `[sec]`, guardas do `sdkconfig`). |
| Alterar | `src/app/CollectorApp.cpp` | Passo 0.5 do boot: `KeyStore::load` → evento `KEYS`; criar as instâncias da §6 conforme `security::kProfile`. |
| Alterar | `platformio.ini` | **Passo 0:** `[base]` com `framework = arduino, espidf` e `platform_packages` (§9.1). Depois: `env:native` ganha `+<crypto/> +<security/> -<security/mbed/>`; `[debug]` ganha `-DSECURITY_DEV_FIXED_KEYS`; seções `[sec_none]`, `[sec_asym]` e envs de benchmark (spec 06); `env:collector_mock_devsec` e `env:collector_twai_prod` (só `extends` + `board_build.cmake_extra_args`). |
| Alterar | `partitions_collector.csv` | **Passo 0:** tabela única do §9.4 (vale nos três perfis). |
| Criar | `CMakeLists.txt`, `src/CMakeLists.txt`, `sdkconfig.defaults` | **Passo 0** (§9.2); o bloco do `nvs_flash` (§9.3) entra no fim do passo 0. |
| Criar | `sdkconfig.devsec.defaults`, `sdkconfig.prod.defaults` | §9.2 (únicos arquivos que distinguem os perfis). |
| Alterar | `.gitignore` (firmware) | `secure_boot_signing_key.pem`, `keys/`, `tools/provision/out/`, `/sdkconfig.*` com `!/sdkconfig.defaults` e `!/sdkconfig.*.defaults`. |
| Criar | `tools/provision/provision.py`, `requirements.txt`, `README.md` | §11. |
| Criar | `tools/provision/vectors.py`, `vectors.json`, `test/support/vectors.h` (gerado) | §12; o `vectors.py --emit-dart` gera também `mobile-app/test/support/sync_vectors.dart`. |
| Criar | `test/test_crypto/test_main.cpp` | Ver Testes. |

## Testes

Nativos (`pio test -e native -f test_crypto`):
- `Sha256`: vetores `""`, `"abc"`, 1 000 000 × `'a'` (`cdc76e5c…c7c0`), e
  atualização em pedaços de 1, 63, 64 e 65 bytes igual a uma só.
- `PortableHmacSha256`: RFC 4231 casos 1, 2, 6 (chave de 131 B); `begin()`
  reutilizado dá o mesmo resultado que uma instância nova.
- `crc32`: `"123456789"` → `CBF43926`; encadeamento `crc32(b, crc32(a)) == crc32(a‖b)`.
- `ctEqual`: iguais, diferença no primeiro e no último byte.
- `HmacBlockAuth` e `HmacHandshakeAuth` com as chaves fixas: todos os vetores
  do §12; `verify` rejeita a tag com 1 bit trocado e aceita a correta.
- `NoHandshakeAuth`: tag de 16 zeros, `verify` sempre `true`; `NoAuth::tagSize() == 0`.

Em placa (`collector_mock_debug`, perfil `dev`):
- Boot com `secnvs` vazia e chaves fixas: `[sec] fe=off sb=0 … nvs_enc=1`,
  `KEYS source=dev_fixed fe=off sb=0`.
- `provision.py --profile dev --flash` (com `--nvs-key auto` → `board`) e
  reboot: `KEYS source=partition dev_id=<o gerado>`; `seq_rsv` e `boot_id`
  continuam de onde estavam (a `nvs` não foi perdida).
- `esptool.py read_flash` da `secnvs`: nenhum `K_auth`/`K_rec` em claro (o
  hexa a procurar está no `bench_keys.json`).
- Perfil `asym` (`collector_mock_debug_asym`): `FLUSH tag_len=64`; a
  assinatura de um bloco verifica com a chave pública impressa pelo
  `provision.py` (script `tools/provision/verify_block.py <hex do FLUSH>`).
- Ensaio de produção (`collector_mock_devsec`) numa placa descartável: tabela
  do §10.2 inteira.

## Passos de implementação

0. **Build único** (§9.5): todos os envs em `framework = arduino, espidf`,
   tabela única, `CMakeLists.txt`/`sdkconfig.defaults`, depois o bloco do
   `nvs_flash` (§9.3). *Checkpoint:* o do §9.5 (firmware atual roda sobre
   Arduino + ESP-IDF: coleta, NUS BLE, LittleFS, NVS) e, ao fim, o do §9.3.
   *Timebox* e planos B1/B2 no §9.5.
1. **Primitivas** (`crypto/`) + `test_crypto`. *Checkpoint:* `pio test -e native -f test_crypto` verde.
2. **Perfis e interfaces** (`security/` puro: `SecurityProfile.h`,
   `DeviceKeys.h`, `BlockAuth`, `HandshakeAuth`) + testes com os vetores do
   §12. *Checkpoint:* vetores passam no native.
3. **Adaptadores mbedTLS** e `KeyStore`; ligar em `CollectorApp` (sem usar
   ainda nas specs 02/05). *Checkpoint:* `pio run -e collector_mock_debug`
   compila; boot imprime `[sec] fe=off … nvs_enc=1` e `KEYS source=dev_fixed`;
   um teste de placa temporário compara `MbedHmacSha256` com
   `PortableHmacSha256` no vetor RFC 4231.
4. **Provisionamento dev** (a tabela já veio no passo 0). *Checkpoint:*
   `provision.py --profile dev --flash` numa placa que já rodou o firmware e
   `KEYS source=partition` com o `DevId` gerado, sem perder `seq_rsv`.
5. **Perfil `asym`.** *Checkpoint:* assinatura de bloco verificada no host.
6. **Ensaio de produção `devsec`** numa placa descartável (§10.2).
   *Checkpoint:* passos 0–9 do §10.2 executados; `efuse_antes.txt` ×
   `efuse_depois.txt` registrados na spec; regravação cifrada sem mudar
   `FLASH_CRYPT_CNT`; recusa de imagem com assinatura de outra chave
   (`secure boot verification failed`) e recuperação na mesma placa.
7. **(Opcional) `prod`** numa placa dedicada à demonstração (§10.3), só
   depois do passo 6 e da decisão sobre OTA (§9.6). *Checkpoint:* linha
   `[sec] fe=release sb=1 jtag_off=1 dl_off=1`.

## Critérios de aceite

- Passo 0: firmware atual roda sobre Arduino + ESP-IDF em todos os envs
  (ou plano B do §9.5 registrado).
- Todos os vetores do §12 passam no native e, para o HMAC/ECDSA via mbedTLS,
  em placa.
- `SECURITY_PROFILE=none|hmac|asym` compila nos envs de depuração e muda só a
  tag de bloco (0/32/64 B); mensagens de handshake idênticas em tamanho.
- Sem chaves nos perfis `hmac`/`asym` e sem `SECURITY_DEV_FIXED_KEYS`, o
  firmware para com mensagem clara; nenhuma chave aparece em log.
- Mesmo código-fonte e mesma tabela nos envs `dev`, `collector_mock_devsec` e
  `collector_twai_prod`; os dois últimos diferem do env de origem só por
  `board_build.cmake_extra_args` (e a chave de cifra do `-t encrypt`).
- `provision.py` gera DevId/chaves com CSPRNG, imagem `secnvs` cifrada com a
  `nvs_key` (nova ou reaproveitada da placa), `kms_record.json`, e grava a
  placa de desenvolvimento.
- Ensaiado em `devsec` (ou registrado como limitação, §9.5/§10.1) o fluxo com
  Secure Boot V2, *flash encryption*, NVS encryption e eFuses; `prod`
  documentado (§10.3).

## Riscos e decisões em aberto

| Risco / ponto | Impacto | Recomendação |
|---|---|---|
| **[Aberto]** *Build* híbrido Arduino + IDF na plataforma 7.x (agora pré-requisito de toda a fase) | Bloqueia tudo o que vem depois do passo 0 | *Timebox* de ~2 dias; planos B1/B2 (§9.5). |
| **[verificar]** Ajuste do `nvs_flash` para NVS encryption sem FE (§9.3) | Sem ele, `dev` não exercita o caminho cifrado | *Checkpoint* do §9.3; senão plano B1 (`#if CONFIG_NVS_ENCRYPTION`, validação do caminho cifrado no `devsec`). |
| **[verificar]** Nomes das opções de `sdkconfig.*.defaults` no IDF 4.4.7 | Opção errada é ignorada em silêncio e a placa é queimada com outra configuração | Conferir cada linha no `sdkconfig.<env>` gerado **antes** de gravar `devsec`/`prod` (§9.2). |
| `sdkconfig.<env>` velho vence o `*.defaults` editado | Placa queimada com a configuração anterior | Apagar `sdkconfig.<env>` após editar qualquer `*.defaults` (§9.1). |
| `erase_flash` obrigatório ao entrar no passo 0 | Perde reserva de `seq`, `last_cs`, *bonds* das placas de bancada | Uma vez por placa; zerar o estado do dongle no servidor de bancada (spec 09). |
| `nvs_key` gerada pelo próprio dongle antes do provisionamento | `secnvs` cifrada com outra chave → `KEYS source=missing` | `--nvs-key auto/board` no `dev`/`devsec`; em `prod`, gravar tudo antes do 1º boot (§10.3, passo 5). |
| ESP32 com revisão < v3.0 | Sem Secure Boot V2 nem `UART_DOWNLOAD_DIS` | Checar no §10.1; comprar módulos `ESP32-D0WD-V3`/`WROOM-32E`. Para placa antiga: só FE + NVS enc, registrado como limitação. |
| `factory` de 1,8125 MB e o binário híbrido | *Build* falha | Medir no passo 0 (`-t size`); app atual ~630 KB, folga grande. |
| **[Aberto]** Sem OTA (§9.6) | Placa `prod` não é atualizável | Bancada em `dev`/`devsec`; `prod` só demonstração; decidir o *layout* com OTA antes de queimar `prod`. |
| Chave de Secure Boot do ESP32 não é revogável | Placa `devsec` presa à chave de ensaio | Chave de ensaio separada da de produção; placa descartável. |
| Chaves em RAM | Extração por *glitching*/ataque físico avançado | Fora do modelo do documento (aceito em §4.1). *Core dump* desligado em `devsec`/`prod`. |
| `esp_sha` compartilhado entre tarefas | Variância no `sign_us` | Registrar no benchmark; não é falha. |
| Envelope do KMS (resolvido) | Integração Fase 3 | `kms_record.json` no formato BYOK do Transit; `tccelta provisionar importar-registro` consome (§11). |
| Perfil `none` em campo por engano | Blocos sem autenticação | Guarda do §7 (`static_assert` com `CONFIG_SECURE_FLASH_ENCRYPTION_MODE_RELEASE` exigindo `SECURITY_PROFILE == 1`); evento `KEYS profile=none` em vermelho no viewer. |
