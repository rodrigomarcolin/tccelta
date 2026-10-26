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
  em claro no perfil de desenvolvimento e encriptada no de produção; chaves
  fixas de bancada por *build flag*.
- Perfis de *build*: desenvolvimento (Arduino puro, sem eFuse) e produção
  (Arduino como componente do ESP-IDF, irreversível), tabelas de partição e
  passo a passo com `espsecure.py`/`espefuse.py`/`nvs_partition_gen.py`.
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
| Plataforma | `platformio.ini` | `platform = espressif32@7.1.3`, `framework = arduino`, `board = esp32dev`, NimBLE-Arduino 2.5.1. |
| Núcleo Arduino instalado | `~/.platformio/packages/framework-arduinoespressif32/package.json` | `3.20017…` = **Arduino-ESP32 2.0.17 sobre ESP-IDF 4.4.7** (`esp_idf_version.h`: 4.4.7). |

Fatos do núcleo Arduino que determinam o desenho (conferidos com `nm` e no
`sdkconfig` pré-compilado em `tools/sdk/esp32/`):

| Recurso | Arduino puro (envs atuais) | Consequência |
|---|---|---|
| Secure Boot V2 | `# CONFIG_SECURE_BOOT is not set`; *bootloader* pré-compilado (`tools/sdk/esp32/bin/bootloader_*.elf`) sem verificação de assinatura | Impossível sem compilar o *bootloader* pelo ESP-IDF. |
| *Flash encryption* | `# CONFIG_SECURE_FLASH_ENC_ENABLED is not set` | O *bootloader* não cifra no primeiro boot; só daria para cifrar manualmente no host, sem modo *release*. |
| NVS encryption | `libnvs_flash.a` **não exporta** `nvs_flash_secure_init`, `nvs_flash_secure_init_partition`, `nvs_flash_generate_keys`, `nvs_flash_read_security_cfg` (compilada sem `CONFIG_NVS_ENCRYPTION`) | Impossível no Arduino puro. |
| HMAC-SHA256 | `mbedtls_md_hmac_*` presentes; `CONFIG_MBEDTLS_HARDWARE_SHA=y` | Suportado, com SHA em hardware. |
| ECDSA P-256 | `mbedtls_ecdsa_sign_det_ext`, `CONFIG_MBEDTLS_ECP_DP_SECP256R1_ENABLED=y`, `CONFIG_MBEDTLS_HARDWARE_MPI=y` | Suportado. |
| Ed25519 | Não existe no mbedTLS 2.28 | Perfil `asym` usa **ECDSA P-256**. |
| eFuse | `libefuse.a` presente; `espefuse.py` no `tool-esptoolpy` | Leitura/queima pelo host funciona; o *release mode* automático depende do *bootloader* do IDF. |

Conclusão: **o perfil de produção exige trocar o *build* para Arduino como
componente do ESP-IDF** (`framework = arduino, espidf`), que compila o
*bootloader* e a NVS com o `sdkconfig` do projeto. O perfil de
desenvolvimento continua no Arduino puro e cobre tudo menos as camadas de
hardware.

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
(apagar esses dois faz o servidor rejeitar o dongle); (b) a imagem de fábrica
contém só chaves; (c) no perfil de produção as duas partições são cifradas
com a mesma chave XTS de `nvs_keys`.

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

Algoritmo de `load`:
1. Produção (`SECURITY_NVS_ENCRYPTION` definido): `esp_partition_find_first(DATA, NVS_KEYS)`
   → `nvs_flash_read_security_cfg(part, &cfg)` → `nvs_flash_secure_init_partition("secnvs", &cfg)`.
   Desenvolvimento: `nvs_flash_init_partition("secnvs")`.
2. `nvs_open_from_partition("secnvs", "sec", NVS_READONLY, &h)`; ler `prov_ver`
   e os blobs com `nvs_get_blob`, conferindo o tamanho exato de cada um.
3. Exigências por perfil: `hmac` → `dev_id`, `k_auth`, `k_rec`; `asym` →
   `dev_id`, `k_auth`, `k_sig`; `none` → só `dev_id` (se ausente,
   `DevId = 0x00 0x00 ‖ MAC Wi-Fi (6 B)`, `source = MacDerived`).
4. Se faltar algo e `SECURITY_DEV_FIXED_KEYS` estiver definido → copiar as
   chaves fixas (§2), `source = DevFixed`. Senão `source = Missing`, `false`.
5. `nvs_close`; `crypto::secureZero` em todo buffer temporário.

**[Decisão]** O `KeyStore` usa a API C da NVS, não `Preferences`: o
`Preferences::begin(name, ro, label)` chama `nvs_flash_init_partition`, que
não sabe abrir partição cifrada.

Sem chaves (`Missing`) nos perfis `hmac`/`asym`, `startCollector` falha com
`[keys] não provisionado` na serial e o `main` para, como já faz com as
outras falhas de inicialização. **[Decisão]** não há modo degradado: um bloco
sem tag válida não serve para nada no servidor, e coletar sem poder assinar
apenas enche a flash.

Higiene: `DeviceKeys` é `static` em `CollectorApp.cpp` (nunca na pilha); o
evento `KEYS` (spec 06) mostra só `DevId`, perfil e origem; nenhum log imprime
chave. No perfil de produção o *core dump* fica desligado (§10), porque um
dump de RAM conteria as chaves.

### 8. Camadas de proteção e o que cada perfil de *build* oferece

| Camada (§4.1 do documento) | Desenvolvimento (`collector_*`, Arduino puro) | Produção (`collector_twai_prod`, Arduino + ESP-IDF) | Como |
|---|---|---|---|
| NVS encryption (chaves e estado) | Não (NVS em claro) | Sim: `secnvs` e `nvs` cifradas (XTS-AES) com chaves da partição `nvs_keys` | `CONFIG_NVS_ENCRYPTION=y` + `nvs_keys` marcada `encrypted` |
| *Flash encryption* (cifra `nvs_keys`, app, *bootloader*, tabela) | Não | Sim, modo *release* | `CONFIG_SECURE_FLASH_ENC_ENABLED=y`, `CONFIG_SECURE_FLASH_ENCRYPTION_MODE_RELEASE=y`; chave gerada no chip no 1º boot (BLOCK1, leitura/escrita bloqueadas) |
| Secure Boot V2 | Não | Sim (exige ESP32 **rev. v3.0+**) | `CONFIG_SECURE_BOOT_V2_ENABLED=y`; imagens assinadas RSA-3072; *digest* em BLOCK2 |
| eFuse: JTAG | Livre | Desabilitado (`JTAG_DISABLE`) | Queimado pelo *bootloader* com *secure boot*/FE *release* |
| eFuse: UART download | Livre | Desabilitado (`UART_DOWNLOAD_DIS`) | `CONFIG_SECURE_DISABLE_ROM_DL_MODE=y` (+ `CONFIG_ESP32_REV_MIN_3=y`) |
| eFuse: cache/encrypt/decrypt via download | — | `DISABLE_DL_ENCRYPT`, `DISABLE_DL_DECRYPT`, `DISABLE_DL_CACHE` | Automático no modo *release* |
| LittleFS (blocos) | Em claro | **Em claro** (integridade pela tag + CRC) | **[Decisão]** ver abaixo |

**[Decisão] O log de blocos (LittleFS) não é cifrado**, nem em produção: o
`esp_littlefs` escreve em granularidade arbitrária e partições `encrypted`
exigem escrita alinhada em 16 B (`esp_flash_write_encrypted`); a propriedade
exigida dos blocos é integridade/autenticidade, garantida pela tag `K_rec`.
Limitação documentada: um atacante com acesso físico lê a telemetria
armazenada (privacidade), mas não a altera sem ser detectado.

### 9. Tabelas de partição

Desenvolvimento — `partitions_collector.csv` (alterada):

```
# Name,   Type, SubType,  Offset,   Size,     Flags
nvs,      data, nvs,      0x9000,   0x5000,
phy_init, data, phy,      0xe000,   0x1000,
factory,  app,  factory,  0x10000,  0x1E0000,
spiffs,   data, spiffs,   0x1F0000, 0x1FD000,
secnvs,   data, nvs,      0x3ED000, 0x3000,
coredump, data, coredump, 0x3F0000, 0x10000,
```

**[Decisão]** `secnvs` (12 KB, o mínimo de 3 páginas que o
`nvs_partition_gen` aceita) é tirada do fim do LittleFS (2 MB − 12 KB): o
começo da tabela fica idêntico, então placas de desenvolvimento não perdem
`nvs` (config, `boot_id`, reserva de `seq`) ao atualizar. O LittleFS muda de
tamanho e é reformatado no primeiro boot — aceitável, porque o formato da
flash muda de qualquer forma (spec 02).

Produção — `partitions_collector_prod.csv` (nova), tabela em `0x10000`
(`CONFIG_PARTITION_TABLE_OFFSET=0x10000`, porque o *bootloader* com Secure
Boot V2 + *flash encryption* não cabe nos 28 KB antes de `0x8000`):

```
# Name,   Type, SubType,  Offset,   Size,     Flags
nvs,      data, nvs,      0x11000,  0x6000,
nvs_key,  data, nvs_keys, 0x17000,  0x1000,   encrypted
phy_init, data, phy,      0x18000,  0x1000,
secnvs,   data, nvs,      0x19000,  0x3000,
factory,  app,  factory,  0x20000,  0x1D0000,
spiffs,   data, spiffs,   0x1F0000, 0x200000,
```

- `factory` cai de 1,875 MB para 1,8125 MB (início alinhado a 64 KB) e
  recebe o bloco de assinatura do Secure Boot (4 KB). Conferir com
  `pio run -e collector_twai_prod -t size` antes de fixar.
- Sem `coredump` (dump de RAM com chaves). 64 KB livres no fim (`0x3F0000`).
- `nvs_key` é a única partição de dados com `encrypted`; `nvs`/`secnvs` são
  protegidas pela NVS encryption (que não usa a *flash encryption* de
  hardware), e a `spiffs` fica em claro.

### 10. Perfil de produção (irreversível)

> **AVISO.** Os passos abaixo queimam eFuses de forma permanente. Depois do
> primeiro boot: a placa só executa firmware assinado com a chave de Secure
> Boot; não aceita mais *download* pela UART (não dá para regravar nem ler a
> flash pela serial, nem recuperar a placa); a chave de *flash encryption*
> nunca sai do chip; JTAG fica morto. Ensaiar primeiro numa placa
> descartável. Perder `secure_boot_signing_key.pem` = não conseguir gerar
> firmware para nenhum dongle já produzido.

**Ambiente PlatformIO** (`platformio.ini`):

```ini
[env:collector_twai_prod]
platform  = espressif32@7.1.3
board     = esp32dev
framework = arduino, espidf
; IDF da mesma linha do Arduino 2.0.17 (conferir o pacote exato ao migrar)
platform_packages = framework-espidf @ ~3.40407.0
board_build.partitions = partitions_collector_prod.csv
board_build.filesystem = littlefs
board_build.cmake_extra_args = -DSDKCONFIG_DEFAULTS="sdkconfig.defaults;sdkconfig.prod.defaults"
build_flags =
    -DUSE_TWAI
    -DSECURITY_PROFILE=1
    -DSECURITY_NVS_ENCRYPTION
build_src_filter = +<*> -<can/bus/mcp2515/> -<obd2/mock/>
lib_deps = h2zero/NimBLE-Arduino@2.5.1
```

Arquivos exigidos pelo *build* híbrido: `CMakeLists.txt` na raiz
(`cmake_minimum_required(3.16)`, `include($ENV{IDF_PATH}/tools/cmake/project.cmake)`,
`project(tccelta_dongle)`), `src/CMakeLists.txt`
(`FILE(GLOB_RECURSE app_sources ${CMAKE_SOURCE_DIR}/src/*.*)` +
`idf_component_register(SRCS ${app_sources})`), `sdkconfig.defaults` com o
que o Arduino exige como componente (`CONFIG_FREERTOS_HZ=1000`,
`CONFIG_AUTOSTART_ARDUINO=y`, `CONFIG_BT_ENABLED=y` com controlador BLE só,
`CONFIG_ESP32_SPIRAM_SUPPORT` desligado) e `sdkconfig.prod.defaults`:

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
CONFIG_PARTITION_TABLE_CUSTOM=y
CONFIG_PARTITION_TABLE_OFFSET=0x10000
CONFIG_ESP_COREDUMP_ENABLE_TO_NONE=y
CONFIG_BOOTLOADER_LOG_LEVEL_WARN=y
```

**[Aberto]** A migração para `framework = arduino, espidf` é o item de maior
risco da fase (compatibilidade das versões do pacote `framework-espidf` com o
Arduino 2.0.17 na plataforma 7.x, e opções de BLE do NimBLE-Arduino quando o
IDF também tem NimBLE). **Recomendação:** fazer num ambiente separado, sem
tocar nos envs de desenvolvimento; se não fechar em ~2 dias de trabalho,
entregar a Fase 1 com o perfil de desenvolvimento e o passo a passo validado
numa placa descartável via `idf.py` (projeto IDF mínimo só para gerar o
*bootloader* assinado e o `sdkconfig`), registrando a limitação no TCC.

**Passo a passo de fábrica** (por dongle; comandos com o `esptool` 4.x do
PlatformIO, `~/.platformio/packages/tool-esptoolpy/`):

| # | Ação | Comando | Verificação |
|---|---|---|---|
| 0 | (uma vez) Gerar a chave de Secure Boot | `espsecure.py generate_signing_key --version 2 --scheme rsa3072 secure_boot_signing_key.pem` | Guardar offline (cofre/HSM); nunca no repositório (`.gitignore`). |
| 1 | (por versão) Compilar assinado | `pio run -e collector_twai_prod` | `espsecure.py verify_signature --version 2 --keyfile secure_boot_signing_key.pem .pio/build/collector_twai_prod/firmware.bin` |
| 2 | Conferir o chip | `esptool.py --port COMx chip_id`; `espefuse.py --port COMx summary` | "revision v3.0" ou maior; `FLASH_CRYPT_CNT = 0`; `ABS_DONE_1 = 0`. Se < v3.0: parar (ver Riscos). |
| 3 | Provisionar | `python tools/provision/provision.py --profile prod --kms-wrapping kms_wrapping.json --out out/` (`kms_wrapping.json` vem de `tccelta provisionar exportar-wrapping`, Fase 3) | Gera `out/<DevId>/{secnvs.csv, secnvs.bin, nvs_keys.bin, kms_record.json}` (§11). |
| 4 | Apagar a flash | `esptool.py --port COMx erase_flash` | NVS principal em branco. |
| 5 | Gravar em claro | `esptool.py --port COMx --chip esp32 write_flash 0x1000 bootloader.bin 0x10000 partition-table.bin 0x17000 out/<DevId>/nvs_keys.bin 0x19000 out/<DevId>/secnvs.bin 0x20000 firmware.bin` | O *bootloader* com Secure Boot não é gravado pelo `pio run -t upload`; usar sempre o comando explícito. |
| 6 | Primeiro boot, **sem desligar** (até ~1 min) | `pio device monitor -b 115200` | O *bootloader* gera a chave FE, cifra *bootloader*/tabela/app/`nvs_key` no lugar, queima `FLASH_CRYPT_CNT` e as proteções do modo *release*; o Secure Boot grava o *digest* e `ABS_DONE_1`; o app desabilita o *download* da ROM. Log mostra o evento `KEYS source=partition`. |
| 7 | Conferir eFuses **pelo próprio firmware** (o `espefuse.py` precisa do modo *download*, que o passo 6 desabilitou) | Linha de boot `[sec] fe=… sb=… jtag_off=… dl_off=… crypt_cnt=…` impressa por `KeyStore` com `esp_flash_encryption_enabled()`, `esp_secure_boot_enabled()` e `esp_efuse_read_field_bit(ESP_EFUSE_JTAG_DISABLE / ESP_EFUSE_UART_DOWNLOAD_DIS)` (só no env de produção) | `fe=1 sb=1 jtag_off=1 dl_off=1`, `crypt_cnt` ímpar. Na placa de ensaio, rodar `espefuse.py summary` **antes** do passo 6 para ver o estado inicial e comparar. |
| 8 | Registrar no KMS | `tccelta provisionar importar-registro --arquivo kms_record.json` (Fase 3, spec 07) | Dongle no banco e chaves importadas no Vault; teste dourado OK. |
| 9 | Destruir material local | Apagar com sobrescrita `out/<DevId>/` | A estação não guarda chaves (documento §4.1). |

**[Decisão]** Os `nvs_keys` (chaves XTS da NVS) são gerados **na estação**
(passo 3) e gravados em claro antes do primeiro boot; o *bootloader* os cifra
no lugar com a chave de *flash encryption* gerada no chip. Isso permite gravar
`secnvs.bin` já cifrada sem que a chave FE saia do chip, ao custo de as chaves
XTS existirem por instantes na estação — o mesmo modelo de confiança que o
documento aceita para `K_auth`/`K_rec`.

### 11. Provisionamento (`esp32-firmware/tools/provision/`)

`provision.py` (Python 3.11, dependências em `requirements.txt`:
`esp-idf-nvs-partition-gen`, `cryptography`, `esptool`):

```
provision.py --profile {dev,prod} [--security {hmac,asym,none}] [--port COMx]
             [--kms-wrapping kms_wrapping.json] [--dev-id HEX16] [--out DIR] [--flash]
             [--import secnvs.csv]     # bancada: usa chaves geradas por `tccelta provisionar novo` (Fase 3)
```

| Passo | Detalhe |
|---|---|
| Entropia | `secrets.token_bytes(n)` (CSPRNG do SO: `BCryptGenRandom`/`getrandom`). `DevId` aleatório de 8 B, rejeitando tudo-zero e tudo-FF; `--dev-id` só para reprovisionar a mesma placa. `k_sig`: `ec.generate_private_key(SECP256R1())`, exportado como escalar de 32 B. |
| CSV | `secnvs.csv` no formato do gerador: `key,type,encoding,value` / `sec,namespace,,` / `prov_ver,data,u8,1` / `dev_id,data,hex2bin,<16 hex>` / `k_auth,data,hex2bin,<64 hex>` / `k_rec,…` / `k_sig,…` (só `asym`) / `prof,data,u8,<0-2>`. |
| Imagem dev | `python -m esp_idf_nvs_partition_gen generate secnvs.csv secnvs.bin 0x3000` |
| Imagem prod | `python -m esp_idf_nvs_partition_gen generate-key --keyfile nvs_keys.bin --outdir out/<DevId>` e `python -m esp_idf_nvs_partition_gen encrypt secnvs.csv secnvs.bin 0x3000 --inputkey out/<DevId>/keys/nvs_keys.bin` |
| Gravação dev (`--flash`) | `esptool.py --port COMx write_flash 0x3ED000 secnvs.bin` (offset lido de `partitions_collector.csv`, não fixo no script). |
| Registro KMS | `kms_record.json` (ver abaixo). Com `--kms-wrapping kms_wrapping.json` (exportado por `tccelta provisionar exportar-wrapping`, Fase 3 spec 07: as *wrapping keys* RSA-4096 dos mounts `transit-auth` e `transit-rec`), `k_auth` e `k_rec` vão no formato BYOK do Vault Transit (`alg: "vault-byok-rsa-oaep-sha256-aes-kwp"`: chave AES-256 efêmera cifrada com RSA-OAEP-SHA256 pela wrapping key do mount, seguida do material embrulhado com AES-KWP, RFC 5649), cada uma com o `kid` do seu mount; sem ela (só `--profile dev`) vão em claro com `"alg":"none"` e o script imprime aviso. A opção `--kms-pubkey` não existe mais. |
| Saída de bancada | `--profile dev` também grava `out/<DevId>/bench_keys.json` (`dev_id`, `k_auth`, `k_rec` em hex) para o servidor de bancada do app (spec 09). |

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
| Criar | `src/storage/KeyStore.{h,cpp}` | §7. |
| Alterar | `src/app/CollectorApp.cpp` | Passo 0.5 do boot: `KeyStore::load` → evento `KEYS`; criar as instâncias da §6 conforme `security::kProfile`. |
| Alterar | `platformio.ini` | `env:native` ganha `+<crypto/> +<security/> -<security/mbed/>`; `[debug]` ganha `-DSECURITY_DEV_FIXED_KEYS`; seções `[sec_none]`, `[sec_asym]` e envs de benchmark (spec 06); `env:collector_twai_prod`. |
| Alterar | `partitions_collector.csv` | §9 (desenvolvimento). |
| Criar | `partitions_collector_prod.csv`, `sdkconfig.defaults`, `sdkconfig.prod.defaults`, `CMakeLists.txt`, `src/CMakeLists.txt` | §9–10 (só usados pelo env de produção). |
| Alterar | `.gitignore` (firmware) | `secure_boot_signing_key.pem`, `tools/provision/out/`. |
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

Em placa (`collector_mock_debug`):
- Boot com `secnvs` vazia e chaves fixas: `KEYS source=dev_fixed`.
- `provision.py --profile dev --flash` e reboot: `KEYS source=partition dev_id=<o gerado>`.
- Perfil `asym` (`collector_mock_debug_asym`): `FLUSH tag_len=64`; a
  assinatura de um bloco verifica com a chave pública impressa pelo
  `provision.py` (script `tools/provision/verify_block.py <hex do FLUSH>`).
- Ensaio de produção numa placa descartável: tabela do §10 inteira.

## Passos de implementação

1. **Primitivas** (`crypto/`) + `test_crypto`. *Checkpoint:* `pio test -e native -f test_crypto` verde.
2. **Perfis e interfaces** (`security/` puro: `SecurityProfile.h`,
   `DeviceKeys.h`, `BlockAuth`, `HandshakeAuth`) + testes com os vetores do
   §12. *Checkpoint:* vetores passam no native.
3. **Adaptadores mbedTLS** e `KeyStore`; ligar em `CollectorApp` (sem usar
   ainda nas specs 02/05). *Checkpoint:* `pio run -e collector_mock_debug`
   compila; boot imprime `KEYS source=dev_fixed`; um teste de placa
   temporário compara `MbedHmacSha256` com `PortableHmacSha256` no vetor RFC 4231.
4. **Partições e provisionamento dev.** *Checkpoint:* `provision.py --flash`
   numa placa e `KEYS source=partition` com o `DevId` gerado.
5. **Perfil `asym`.** *Checkpoint:* assinatura de bloco verificada no host.
6. **Perfil de produção** (env híbrido) e ensaio numa placa descartável.
   *Checkpoint:* passos 0–9 do §10 executados e linha `[sec]` com todas as
   proteções ligadas. Para provar a recusa de firmware não assinado, gravar
   (passo 5) um `firmware.bin` com assinatura inválida numa segunda placa
   descartável **com** `CONFIG_SECURE_DISABLE_ROM_DL_MODE` desligado nesse
   ensaio e conferir `secure boot verification failed` no log do
   *bootloader*.

## Critérios de aceite

- Todos os vetores do §12 passam no native e, para o HMAC/ECDSA via mbedTLS,
  em placa.
- `SECURITY_PROFILE=none|hmac|asym` compila nos envs de depuração e muda só a
  tag de bloco (0/32/64 B); mensagens de handshake idênticas em tamanho.
- Sem chaves nos perfis `hmac`/`asym` e sem `SECURITY_DEV_FIXED_KEYS`, o
  firmware para com mensagem clara; nenhuma chave aparece em log.
- `provision.py` gera DevId/chaves com CSPRNG, imagem NVS em claro (dev) ou
  cifrada (prod), `kms_record.json`, e grava a placa de desenvolvimento.
- Documentado e ensaiado (ou registrado como limitação, ver [Aberto] do §10)
  o fluxo de produção com Secure Boot V2, *flash encryption release*, NVS
  encryption e eFuses.

## Riscos e decisões em aberto

| Risco / ponto | Impacto | Recomendação |
|---|---|---|
| **[Aberto]** *Build* híbrido Arduino + IDF na plataforma 7.x | Bloqueia o perfil de produção | Ambiente separado; *timebox*; plano B com projeto IDF mínimo para o *bootloader* (§10). |
| ESP32 com revisão < v3.0 | Sem Secure Boot V2 nem `UART_DOWNLOAD_DIS` | Checar no passo 2; comprar módulos `ESP32-D0WD-V3`/`WROOM-32E`. Para placa antiga: só FE + NVS enc, registrado como limitação. |
| `factory` de 1,8125 MB pode não comportar o binário de produção | *Build* falha | Medir no passo 3; se faltar, reduzir `spiffs` em 64 KB (recalcular `t_mem` da Tabela 14 da Investigação). |
| Chaves em RAM | Extração por *glitching*/ataque físico avançado | Fora do modelo do documento (aceito em §4.1). *Core dump* desligado em produção. |
| `esp_sha` compartilhado entre tarefas | Variância no `sign_us` | Registrar no benchmark; não é falha. |
| Envelope do KMS (resolvido) | Integração Fase 3 | `kms_record.json` no formato BYOK do Transit; `tccelta provisionar importar-registro` consome (§11). |
| Perfil `none` em campo por engano | Blocos sem autenticação | `static_assert` em `collector_twai_prod` exigindo `SECURITY_PROFILE == 1`; evento `KEYS profile=none` em vermelho no viewer. |
