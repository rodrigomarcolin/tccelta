# 02 — Firmware: formato do bloco, marcadores e log autenticado na flash

## Objetivo

Adequar o bloco às Figuras 8–11 do documento de segurança: área de dados com
cabeçalho de 12 B (Fig. 10), três formas de registro (PID, GPS e marcador
BOOT/LOSS, Fig. 11), e gravação na flash como `dados | tag | CRC32` (Fig. 8),
com a tag calculada pela interface genérica `IMessageAuth` (spec 01) e o CRC32
sobre dados + tag. O log em LittleFS ganha um formato de arquivo versionado
que carrega o tamanho da tag, para os três perfis (`none`/`hmac`/`asym`)
conviverem, e a API de leitura que o cursor de envio (spec 05) precisa para
detectar corrompimento (LOSS), lacunas de boot (BOOT) e descarte por flash
cheia (OVERFLOW).

## Escopo

**Dentro:** `BlockFormat.h` (constantes de marcador, `Block` com tag e CRC),
construção/parse de blocos marcadores, codec puro de segmento e registro,
`BlockWriter` (assinar + CRC + gravar), `IBlockStore` v2,
`LittleFsBlockStore` v2 com migração, validação de PID reservado, eventos de
gravação (detalhados na spec 06), testes.

**Fora:** reserva de `seq` na NVS e geração do BOOT no boot (spec 03 — aqui só
o formato e a gravação); geração do LOSS no envio e cursor (spec 05);
transporte BLE (spec 04).

## Estado atual (verificado no código)

| Peça | Onde | O que faz hoje |
|---|---|---|
| Layout | `src/collector/BlockFormat.h` | Cabeçalho `seq u32 @0 \| boot_id u16 @4 \| t_ms inicial u32 @6 \| tamanho u16 @10`; PID 5 B (`tipo=PID \| Δt i16 \| valor 2 B`); GPS 15 B (`0xFF \| Δt i16 \| lat i32 \| lon i32 \| alt i32`); `kBlockSize = 1024`; `struct Block { uint8_t bytes[1024]; size_t length; }`; `stampHeader(seq, bootId)`. |
| Montagem | `src/collector/BlockBuilder.{h,cpp}` | Abre no 1º registro; hoje o Δt é `i16` **com sinal** (GPS pode ser anterior ao `t_ms` inicial); fecha preenchendo `t_ms` inicial e `tamanho`. Esta fase muda para `u16` sem sinal (**[Decisão]** abaixo): `deltaFor` recusa `tMs < _tStartMs` e `tMs − _tStartMs > 65 535`. |
| Gravação | `src/collector/BlockWriter.{h,cpp}` | `stampHeader` → `store.append` (2 tentativas) → `persistence.saveLastSeq(seq)` → `SeqCounter::commit()`. Falha dupla: bloco descartado, `seq` não consumido. |
| Interface | `src/collector/IBlockStore.h` | `append`, `read(seq, Block&) → bool`, `deleteUpTo`, `empty`, `oldestSeq`, `newestSeq`, `lostBlocks`; `ISeqPersistence::saveLastSeq`. |
| Log | `src/storage/LittleFsBlockStore.{h,cpp}` | Diretório `/log`, arquivos `/log/<firstSeq %010lu>.seg` com até 64 blocos **só com os dados** (sem tag/CRC/cabeçalho de arquivo); índice em RAM de até 256 segmentos; `ensureSpace` descarta o segmento mais antigo (`SEG_DROP`); `deleteUpTo` só remove segmentos inteiros e mantém sempre o mais novo; mutex interno. |
| Dublê | `test/support/Fakes.h` | `MemoryBlockStore` (vetor de `Block`), `FakeSeqPersistence`. |
| Evento | `src/tasks/FlushTask.cpp` | `FLUSH buf seq boot len ok attempts dur hex` (linha de 2400 B estática). |

Conferência com o documento:

| Figura | Documento | Código atual | Ação |
|---|---|---|---|
| Fig. 10 | `seq(4) \| boot_id(2) \| t_ms inicial(4) \| tamanho(2)` + registros até 1012 B | Idêntico (offsets 0/4/6/10; `tamanho` inclui o cabeçalho) | Nenhuma. |
| Fig. 11 PID | `tipo \| Δt(2) \| valor` = 5 B | Idêntico no tamanho; Δt é `i16` no código | **[Decisão]** Δt **sem sinal (`u16`), como no documento** ("deslocamento a partir do `t_ms` inicial"): o código troca `int16_t` por `uint16_t` em `BlockBuilder::deltaFor` e `BlockFormat.h`, o servidor (Fase 3) passa a lê-lo como `u16`. Os bytes de Δt ≥ 0 são idênticos, então os vetores de teste não mudam. Custo: um GPS cujo `tMs` é anterior ao `t_ms` inicial do bloco aberto **deixa de ser representável nele**; ver o tratamento abaixo. |
| Fig. 11 marcador | `0xFE`/`0xFD \| Δt(2) \| from_seq(4) \| to_seq(4)` = 11 B | Não existe | Criar (§2–3). PIDs `0xFD`/`0xFE` passam a ser reservados. |
| Fig. 8 | `dados(1024) \| tag(32) \| CRC32(4)` = 1060 B fixos | Só `dados(tamanho)` | **[Decisão do orquestrador]** tag e CRC sobre os `tamanho` bytes; registro na flash com `tamanho + T + 4` bytes (§5). |
| Fig. 9 | `0x03 \| L(2) \| dados \| tag(32)` | Não existe | Montado no envio (spec 04/05); `L = tamanho + T`. |

## Design

### 1. Área de dados (Fig. 10) — sem mudança

| Offset | Campo | Tipo | Tamanho | Observação |
|---|---|---|---|---|
| 0 | `seq` | u32 LE | 4 | Carimbado pela Flush; nos marcadores, ver §3 |
| 4 | `boot_id` | u16 LE | 2 | 0 no LOSS |
| 6 | `t_ms` inicial | u32 LE | 4 | ms desde o boot do 1º registro; 0 no LOSS |
| 10 | `tamanho` | u16 LE | 2 | Bytes da área de dados, **cabeçalho incluído** (12…1024) |
| 12 | registros | — | `tamanho − 12` | Até 1012 B |

### 2. Registros (Fig. 11)

| Registro | Tipo (byte 0) | Layout | Tamanho |
|---|---|---|---|
| Leitura de PID | `0x00`–`0xFC` (= código do PID) | `tipo u8 \| Δt u16 LE \| A u8 \| B u8` (B = 0 em PID de 1 byte) | 5 |
| Marcador LOSS | `0xFD` | `0xFD \| Δt u16 LE \| from_seq u32 LE \| to_seq u32 LE` | 11 |
| Marcador BOOT | `0xFE` | `0xFE \| Δt u16 LE \| from_seq u32 LE \| to_seq u32 LE` | 11 |
| Posição GPS | `0xFF` | `0xFF \| Δt u16 LE \| lat i32 LE (°×10⁷) \| lon i32 LE (°×10⁷) \| alt i32 LE (mm)` | 15 |

**[Decisão]** `validate(CollectorConfig)` passa a rejeitar PID ≥ `0xFD`
(`ConfigError::InvalidPid`, resposta `?` ao `AT#PID`), para nenhum registro de
leitura colidir com marcador/GPS. Nenhum PID do Serviço 01 usado no projeto
chega perto disso.

**[Decisão] Δt sem sinal (`u16`) e GPS anterior ao bloco aberto.** O
deslocamento é `tMs − t_ms inicial`, em `0…65 535` ms (o bloco fecha em até
`CONFIG.flush`, bem abaixo disso). `BlockBuilder::deltaFor` recusa `tMs`
anterior ao `t_ms` inicial (o GPS é marcado ao chegar o comando e pode ter
`tMs` menor que o do 1º registro do bloco) e deslocamentos acima de 65 535;
`addPid`/`addGps` devolvem `false` e o `Collector::append` existente
**fecha o bloco** (`BLK_CLOSE reason=full`) e abre outro cujo `t_ms` inicial é o
`tMs` do registro recusado — nenhum campo novo, nenhum registro reordenado.
Consequência para a Fase 3: o `t_ms` inicial **não é crescente** entre blocos
consecutivos (o bloco novo pode começar antes do fim do anterior), então o
servidor trata o intervalo de cada bloco como `[t0, t0 + Δt_max]` e agrega
`t_ms_primeiro`/`t_ms_ultimo` do boot por `min`/`max`, não pelo primeiro e
pelo último bloco. Teste nativo: GPS com `tMs = t0 − 50` fecha o bloco e abre
outro com `t0 − 50`; `Δt = 65 536` também.

### 3. Blocos marcadores

Bloco 0x03 comum cuja área de dados tem **um único** registro de 11 B
(`tamanho = 23`). Ocupa um `seq`, recebe tag como qualquer bloco.

| Offset | Campo | BOOT | LOSS |
|---|---|---|---|
| 0 | `seq` u32 | `R` (reserva lida da NVS, spec 03) | `seq` do 1º bloco corrompido (= `from_seq`) |
| 4 | `boot_id` u16 | `boot_id` do boot atual | `0` |
| 6 | `t_ms` u32 | `millis()` ao montar | `0` |
| 10 | `tamanho` u16 | `23` | `23` |
| 12 | tipo u8 | `0xFE` | `0xFD` |
| 13 | Δt u16 | `0` | `0` |
| 15 | `from_seq` u32 | último `seq` na flash + 1 (spec 03) | 1º `seq` da faixa perdida |
| 19 | `to_seq` u32 | `R − 1` | último `seq` da faixa perdida |
| — | Na flash? | **Sim**, gravado como bloco comum | **Não**; montado e assinado a cada envio (spec 05) |

Exemplo (perfil `hmac`, chaves fixas da spec 01): BOOT `seq=192`,
`boot_id=4`, `t_ms=812`, `[131,191]` → dados
`C0000000 0400 2C030000 1700 FE 0000 83000000 BF000000`, tag
`E55ECDF1…AC333D8D`, CRC32(dados‖tag) = `B294D366` (gravado `66 D3 94 B2`).
Perfil `none` (sem tag): CRC32(dados) = `8D81DCB2`.

API pura (`src/collector/MarkerBlock.{h,cpp}`):

```cpp
namespace collector {
constexpr uint8_t kLossRecordType = 0xFD;
constexpr uint8_t kBootRecordType = 0xFE;
constexpr size_t  kMarkerRecordSize = 11;
constexpr size_t  kMarkerBlockLength = kHeaderSize + kMarkerRecordSize;  // 23

enum class MarkerKind : uint8_t { None, Boot, Loss };
struct MarkerInfo { MarkerKind kind; uint32_t fromSeq; uint32_t toSeq; };

/** Monta a área de dados (sem tag). `block.length = 23`; não mexe em tag/crc. */
void buildBootMarker(Block& block, uint32_t seq, uint16_t bootId, uint32_t tMs,
                     uint32_t fromSeq, uint32_t toSeq);
void buildLossMarker(Block& block, uint32_t fromSeq, uint32_t toSeq);  // seq = fromSeq, boot/t_ms = 0

/** Lê um bloco e diz se é marcador (tamanho 23 e tipo 0xFD/0xFE). */
MarkerInfo parseMarker(const Block& block);
}
```

### 4. `Block` v2

```cpp
constexpr size_t kMaxTagSize = 64;   // maior tag entre os perfis (ECDSA r‖s)
constexpr size_t kCrcSize    = 4;

struct Block {
    uint8_t  bytes[kBlockSize];   // área de dados
    size_t   length;              // = tamanho
    uint8_t  tag[kMaxTagSize];    // só os primeiros tagLength bytes valem
    uint8_t  tagLength;           // 0, 32 ou 64
    uint32_t crc;                 // CRC32(bytes[0..length) ‖ tag[0..tagLength))
};
```

Custo: +72 B por `Block` (≈ 1,1 KB cada). Instâncias: 2 no pool
(`CollectorContext::blocks`), 1 de leitura e 1 de LOSS no `SyncEngine`
(spec 05), 1 do BOOT no `BlockWriter`. Todas estáticas, nunca na pilha.

### 5. Registro na flash (Fig. 8 ajustada)

| Offset | Campo | Tamanho | Observação |
|---|---|---|---|
| 0 | dados | `tamanho` (12…1024) | Área de dados exata (o próprio `tamanho` está em `dados[10..12)`) |
| `tamanho` | tag | `T` = tag do segmento (0/32/64) | Saída de `IMessageAuth::sign(dados, tamanho)` |
| `tamanho + T` | CRC32 | 4, u32 LE | `crc32(dados ‖ tag)`; com `T = 0`, só sobre os dados |

Tamanho do registro: `tamanho + T + 4` → 27…1092 B (perfil `asym`), 59…1060 B
(`hmac`), 27…1028 B (`none`).

### 6. Arquivo de segmento v2

Diretório `/log2`, arquivos `/log2/<firstSeq %010lu>.seg`.

| Offset | Campo | Tipo | Valor |
|---|---|---|---|
| 0 | magia | 4 B | `54 43 53 32` (`"TCS2"`) |
| 4 | versão | u8 | `2` |
| 5 | `tag_len` | u8 | 0, 32 ou 64 — vale para todos os registros do arquivo |
| 6 | reservado | u16 LE | `0` |
| 8 | registros | — | concatenados (§5) |

Invariantes (garantidas na escrita, conferidas na varredura do boot):
- **I1** dentro de um segmento os `seq` são contíguos: o k-ésimo registro
  tem `seq = firstSeq + k`. Por isso o BOOT abre **sempre** um segmento novo
  (`startNewSegment()` antes de gravá-lo — o salto de `seq` de um boot fica
  entre segmentos) e uma gravação que falha "sela" o segmento (a nova
  tentativa vai para outro arquivo).
- **I2** um segmento tem um único `tag_len`; trocar de perfil abre segmento
  novo.
- **I3** no máximo 64 registros por segmento (`kBlocksPerSegment`, mantido).

**[Decisão] Versionar por diretório + cabeçalho.** `/log` (v1, sem tag) é
apagado inteiro no primeiro boot do firmware novo (evento `LOG_MIGRATE`): são
dados de bancada que não podem ser autenticados. Nenhum dongle de campo tem
log v1. Na prática o caminho é defensivo: o passo 0 da fase (spec 01 §9.5,
tabela única) exige `erase_flash` em toda placa, então o `/log` v1 não chega
ao firmware novo; o teste em placa "boot com `/log` antigo" passa a usar um
`/log` preparado à mão (imagem LittleFS com `data/log/…` gravada por
`pio run -t uploadfs`).

### 7. Efeito do tamanho da tag (`tagSize()`) em cada camada

| Camada | `none` (T=0) | `hmac` (T=32) | `asym` (T=64) | Quem lê T |
|---|---|---|---|---|
| Registro na flash | `tamanho + 4` | `tamanho + 36` | `tamanho + 68` | Cabeçalho do segmento |
| `V` do TLV 0x03 (spec 04) | `tamanho` | `tamanho + 32` | `tamanho + 64` | `L` do TLV |
| TLV completo máximo | 1027 B | 1059 B | 1091 B | — |
| Fragmentos no MTU 247 (`MTU − 4 = 243`) | 5 | 5 | 5 | — |
| Fragmentos no MTU 23 (19 B/frag.) | 55 | 56 | 58 | — |
| App | Não interpreta a tag: lê `seq` em `V[0..4)` e, nos marcadores, `V[12..23)`; guarda e repassa `V` inteiro | idem | idem | — |
| Servidor (Fase 3) | — | `L = tamanho + 32` | Desenho (perfil de bancada) | `L` e perfil do dongle |
| Autonomia da flash | +3,4% blocos | linha de base | −3% | — |

O app aceita `L ≤ 1088` (= 1024 + 64), o que cobre os três perfis sem mudança.

### 8. Gravação (`BlockWriter` v2)

```cpp
namespace collector {
struct WriteReport {
    uint32_t seq;
    bool     ok;
    int      attempts;
    bool     reserved;       // gravou nova reserva de seq na NVS antes (spec 03)
    uint32_t reserveUs;      // custo da escrita da reserva
    uint32_t signUs;         // IMessageAuth::sign
    uint32_t crcUs;          // crc32(dados ‖ tag)
    uint32_t writeUs;        // append (todas as tentativas)
};

class BlockWriter {
public:
    BlockWriter(IBlockStore& store, ISeqReservePersistence& persistence, IMessageAuth& auth,
                const IClock& clock, SeqReserve reserve, uint16_t bootId);

    /** Bloco de dados fechado pela Aquisição. Se o BOOT ainda está pendente, tenta-o antes. */
    bool write(Block& block, WriteReport& report);

    /** Monta e grava o BOOT deste boot com seq = R. Chamado uma vez no startCollector;
     *  se falhar, fica pendente e é refeito no próximo write(). */
    bool writeBootMarker(uint32_t fromSeq, uint32_t toSeq, WriteReport& report);

    bool     bootPending() const;
    uint32_t nextSeq() const;
    uint32_t failedBlocks() const;
};
}
```

Ordem de `write` (cada passo medido com `clock.nowUs()`, spec 06):

1. Se `bootPending()`: `writeBootMarker(...)` com os parâmetros guardados; se
   continuar pendente, o bloco de dados **falha** (contado em
   `failedBlocks`), porque nenhum dado pode receber `seq` antes do BOOT.
2. `seq = reserve.peek()`; se `reserve.needsReserve(seq)` →
   `persistence.saveSeqReserve(seq + N)` **antes** da flash (spec 03). Falha
   da NVS → não grava (o invariante de reserva quebraria), conta falha.
3. `stampHeader(block, seq, bootId)`.
4. `auth.sign(block.bytes, block.length, block.tag)`; `block.tagLength = auth.tagSize()`.
5. `block.crc = crc32(block.tag, tagLength, crc32(block.bytes, length))`.
6. `store.append(block)`; até 2 tentativas (a 2ª vai para segmento novo, I1).
7. Sucesso → `reserve.commit()`. Falha dupla → `seq` não consumido.

**[Decisão]** Assinar e calcular o CRC **uma vez**, antes das tentativas: a
nova tentativa grava os mesmos bytes (mesmo `seq`).

`ISeqPersistence::saveLastSeq` some (substituída por
`ISeqReservePersistence::saveSeqReserve`, spec 03): a NVS deixa de ser
escrita a cada bloco.

### 9. `IBlockStore` v2

```cpp
namespace collector {
enum class ReadStatus : uint8_t {
    Ok,        // registro lido inteiro (dados, tag, crc); o CRC NÃO foi conferido
    NotFound,  // seq fora de todo segmento do índice
    Corrupt,   // o índice diz que existe, mas a varredura não chega nele / E/S falhou
};

class IBlockStore {
public:
    virtual ~IBlockStore() = default;
    virtual bool       append(const Block& block) = 0;            // dados|tag|crc; true só após sincronizar
    virtual ReadStatus read(uint32_t seq, Block& out) = 0;
    virtual uint32_t   nextStoredSeq(uint32_t fromSeq) const = 0; // menor seq guardado ≥ fromSeq; 0 = nenhum
    virtual size_t     deleteUpTo(uint32_t upTo) = 0;             // segmentos inteiros com lastSeq ≤ upTo; devolve quantos
    virtual void       startNewSegment() = 0;                     // próximo append abre arquivo novo
    virtual bool       empty() const = 0;
    virtual uint32_t   oldestSeq() const = 0;                     // TAIL na flash (0 se vazio)
    virtual uint32_t   newestSeq() const = 0;                     // HEAD (0 se vazio)
    virtual uint32_t   lostBlocks() const = 0;                    // descartados por falta de espaço
    virtual uint32_t   overflowUpTo() const = 0;                  // maior seq já descartado por falta de espaço (0 = nunca)
    virtual uint16_t   usagePermille() const = 0;                 // ocupação do log (0–1000), em cache
    virtual bool       corrupt(uint32_t /*seq*/) { return false; } // bancada (AT#CORRUPT), só COLLECTOR_EVENTS
};
}
```

**[Decisão]** O CRC é conferido por quem lê (o `SyncEngine`), não pelo store:
o motor mede `crc_us` e decide entre enviar e gerar LOSS; o store só
distingue "não existe" de "estrutura quebrada".

### 10. `LittleFsBlockStore` v2

Índice em RAM (até 256 segmentos, como hoje):

```cpp
struct Segment {
    uint32_t firstSeq, lastSeq;   // I1: lastSeq = firstSeq + count − 1
    uint16_t count;
    uint32_t bytes;               // cabeçalho + registros válidos
    uint8_t  tagLen;
    bool     sealed;              // não recebe mais appends (cauda inválida ou falha de escrita)
};
```

| Operação | Algoritmo |
|---|---|
| `begin(wipe)` | Monta LittleFS; se existe `/log` → apaga tudo nele (`LOG_MIGRATE removed=n`); `wipe` → formata. Lista `/log2/*.seg`, ordena por `firstSeq`, varre cada um com `SegmentScanner` (§11): cabeçalho inválido ou 0 registros válidos → apaga o arquivo; varredura parou antes do fim do arquivo → `sealed = true`; `count == 64` → `sealed = true`. |
| `append(b)` | Sob mutex. Abre segmento novo se: índice vazio, mais novo cheio/selado, `tagLen` diferente de `b.tagLength`, ou `_forceNew` (de `startNewSegment`). `ensureSpace(recordSize + (novo ? 8 : 0))`. Escreve (novo: cabeçalho) + dados + tag + CRC LE com `File::write` e `close()` (sincroniza). Se `written != esperado` → `sealed = true` no segmento usado e `false`. Senão atualiza índice e `_usagePermille`. |
| `read(seq, out)` | Sob mutex. Acha o segmento com `firstSeq ≤ seq ≤ lastSeq` (senão `NotFound`). Abre, pula o cabeçalho, varre `seq − firstSeq` registros pelo `tamanho` de cada um, lê dados, tag (`tagLen`) e CRC. Qualquer incoerência → `Corrupt`. |
| `nextStoredSeq(s)` | Primeiro segmento com `lastSeq ≥ s` → `max(s, firstSeq)`; nenhum → 0. |
| `deleteUpTo(u)` | Como hoje (segmentos inteiros, mantém o mais novo); devolve quantos removeu; atualiza `_usagePermille`. |
| `discardOldest()` | Como hoje (`SEG_DROP`) e `_overflowUpTo = max(_overflowUpTo, oldest.lastSeq)` (`std::atomic<uint32_t>`). |
| `ensureSpace(n)` | `while (_count > 1 && livre() < n + kReserveBytes) discardOldest();` com `livre()` = espaço do LittleFS ou, com `BLOCK_LOG_LIMIT_BYTES` (bancada), `limite − Σ bytes`. |
| `usagePermille()` | Valor em cache (`std::atomic<uint16_t>`), recalculado em `append`/`deleteUpTo`/`discard`: `usedBytes × 1000 / totalBytes` (ou `Σ bytes × 1000 / limite`). |
| `corrupt(seq)` | Só com `COLLECTOR_EVENTS`: localiza o registro, abre `"r+"`, inverte o byte `dados[12]` (1º byte de registro; cabeçalho intacto, varredura do boot continua válida). |

Custo de E/S: `read` de um bloco no fim de um segmento cheio lê ~64
cabeçalhos de 12 B com `seek` entre eles; o `SyncEngine` lê em ordem, então
**[Decisão]** o store guarda um cursor de leitura (arquivo aberto + posição
do último registro lido) e reaproveita se o próximo `read` for `seq + 1` no
mesmo segmento; qualquer `append`/`delete` invalida o cursor. Opcional,
medido pelo `read_us` do evento `SYNC_BLOCK` (spec 06); implementar só se o
`read_us` médio passar de 5 ms.

### 11. Codec e varredura puros (`src/collector/SegmentFormat.{h,cpp}`)

```cpp
namespace collector {
constexpr size_t   kSegmentHeaderSize = 8;
constexpr uint8_t  kSegmentVersion    = 2;
constexpr uint16_t kBlocksPerSegment  = 64;

void encodeSegmentHeader(uint8_t tagLen, uint8_t out[kSegmentHeaderSize]);
bool decodeSegmentHeader(const uint8_t in[kSegmentHeaderSize], uint8_t& tagLen);  // magia, versão, tagLen ∈ {0,32,64}

constexpr size_t recordSize(size_t length, size_t tagLen) { return length + tagLen + kCrcSize; }
uint32_t computeCrc(const Block& b);                // crc32(dados ‖ tag)
bool     crcMatches(const Block& b);                // computeCrc(b) == b.crc

/** Fonte de bytes sequencial (File no ESP32, buffer em memória no native). */
class IByteSource {
public:
    virtual ~IByteSource() = default;
    virtual size_t read(uint8_t* out, size_t n) = 0;
    virtual bool   skip(size_t n) = 0;
    virtual size_t remaining() const = 0;
};

struct ScanResult { uint32_t firstSeq, lastSeq; uint16_t count; uint32_t bytes; uint8_t tagLen; bool trailingGarbage; };

/** Varre um segmento: para no 1º registro com tamanho fora de [12,1024],
 *  seq ≠ firstSeq + k, ou bytes insuficientes para dados+tag+crc. */
bool scanSegment(IByteSource& src, uint32_t expectedFirstSeq, ScanResult& out);

/** Lê o registro `seq` (varrendo desde o início). */
ReadStatus readRecord(IByteSource& src, uint8_t tagLen, uint32_t firstSeq, uint32_t seq, Block& out);
}
```

`expectedFirstSeq` vem do nome do arquivo; o 1º registro precisa ter esse
`seq` (protege contra arquivo renomeado/corrompido).

## Plano arquivo por arquivo

| Ação | Arquivo | Mudança |
|---|---|---|
| Alterar | `src/collector/BlockFormat.h` | `kMaxTagSize`, `kCrcSize`, `Block` v2 (§4); comentário do layout com os 4 tipos de registro; `kPidRecordTypeMax = 0xFC`. |
| Criar | `src/collector/MarkerBlock.{h,cpp}` | §3. |
| Criar | `src/collector/SegmentFormat.{h,cpp}` | §11. |
| Alterar | `src/collector/IBlockStore.h` | §9; remove `ISeqPersistence` (vira `ISeqReservePersistence` em `SeqReserve.h`, spec 03). |
| Alterar | `src/collector/BlockWriter.{h,cpp}` | §8. |
| Remover | `src/collector/SeqCounter.h` | Substituído por `SeqReserve` (spec 03). |
| Alterar | `src/collector/CollectorConfig.{h,cpp}` | `ConfigError::InvalidPid`; `validate` e `setPid` rejeitam PID ≥ `0xFD`. |
| Alterar | `src/storage/LittleFsBlockStore.{h,cpp}` | §10; adaptador `FileByteSource : IByteSource`. |
| Alterar | `src/tasks/FlushTask.{h,cpp}` | Usa `WriteReport`; evento `FLUSH` v2 (spec 06); posta `SyncInput{Flushed, seq}` (spec 04); pilha por perfil: 8 KB (`none`/`hmac`), 12 KB (`asym`). |
| Alterar | `src/collector/Events.h` | `kLargeCapacity` 2400 → 2600 (FLUSH com 1 KB de dados + 64 B de tag em hexa). |
| Alterar | `test/support/Fakes.h` | `MemoryBlockStore` v2 (vetor de `Block` com tag/crc, `failAppends`, `corruptSeqs`, `overflowUpTo` controlável, `usagePermille` controlável, `startNewSegment` registrado), `FakeSeqReservePersistence`, `FakeAuth` (tag = primeiros T bytes de um contador, para conferir chamadas). |
| Criar | `test/test_block_format/test_main.cpp` | Ver Testes. |
| Alterar | `test/test_persistence/test_main.cpp` | Remove os casos de `SeqCounter::recover` e `saveLastSeq` (migrados para `test_seq_reserve`, spec 03) e os de `UploadCursor`/`DataProtocol`/`TimeSync` (spec 05). |
| Alterar | `docs/debug-events.md` §7 | Formato v2 (spec 06). |

## Testes

Nativos (`pio test -e native -f test_block_format`):
- `buildBootMarker`/`buildLossMarker` produzem exatamente os bytes do §3;
  `parseMarker` reconhece BOOT/LOSS e devolve `None` para bloco de dados e
  para bloco de 23 B cujo 1º registro é PID.
- Com `HmacBlockAuth` (chaves fixas): tag e CRC do BOOT de exemplo
  (`E55E…3D8D`, `B294D366`); perfil `NoAuth`: CRC `8D81DCB2`.
- `encode/decodeSegmentHeader`: ida e volta; rejeita magia, versão e `tagLen ∉ {0,32,64}`.
- `scanSegment` sobre buffer: segmento íntegro (T = 0/32/64); cauda truncada
  no meio da tag (`trailingGarbage`, `count` correto); `seq` fora de ordem
  (para no anterior); `tamanho` = 11 e 1025 (para); arquivo só com cabeçalho
  (`count = 0`).
- `readRecord`: 1º, meio e último; `seq` além do fim → `Corrupt`.
- `BlockWriter`: assina com `length` exato (o `FakeAuth` registra o `len`);
  CRC sobre dados‖tag; 2ª tentativa após falha produz os mesmos bytes; falha
  dupla não consome `seq`; BOOT pendente bloqueia dados até ser gravado;
  `WriteReport` com durações do `FakeClock::nowUs`.
- `validate` rejeita `AT#PID FD …`, `FE`, `FF`.

Em placa (`collector_mock_debug`):
- Boot com `/log` antigo (preparado por `uploadfs`, §6) → `LOG_MIGRATE`; depois só `/log2`.
- 200 blocos gravados, reboot: índice reconstruído, `log_newest` igual.
- `AT#CORRUPT n` (spec 05) e reboot: varredura continua válida (o byte
  invertido é de registro, não de cabeçalho).
- Corte de energia durante `FLUSH` (desligar a USB em rajada, 20 vezes):
  boot sempre sobe, segmentos com cauda inválida ficam `sealed`, nenhum
  `seq` repetido no log (conferir com `FLUSH` do log de eventos).

## Passos de implementação

1. `BlockFormat.h` v2 + `MarkerBlock` + testes de bytes. *Checkpoint:* testes de marcador verdes.
2. `SegmentFormat` (codec + varredura) + testes. *Checkpoint:* todos os casos de varredura verdes.
3. `IBlockStore` v2 + `MemoryBlockStore` v2 + `BlockWriter` v2 (com
   `SeqReserve` da spec 03 já disponível — fazer o passo 1 da spec 03 antes
   deste). *Checkpoint:* `test_block_format` e `test_seq_reserve` verdes.
4. `LittleFsBlockStore` v2 + migração. *Checkpoint:* em placa, 200 blocos
   gravados e relidos após reboot com tag e CRC conferidos por um teste
   temporário.
5. `FlushTask` + evento `FLUSH` v2. *Checkpoint:* log de bancada com
   `FLUSH … tag_len=32 sign_us=… crc_us=…` e `hex` + `tag` decodificáveis.

## Critérios de aceite

- Registro na flash = `dados(tamanho) | tag(T) | CRC32 LE` com `T` do perfil;
  CRC sobre dados‖tag; tag sobre os `tamanho` bytes com o separador de contexto.
- BOOT e LOSS com 23 B de dados no layout do §3, reconhecidos por `parseMarker`.
- Segmentos v2 com cabeçalho; seq contíguo por segmento; boot sempre abre
  segmento novo; falha de escrita sela o segmento.
- `read` distingue `Ok`/`NotFound`/`Corrupt`; `nextStoredSeq`,
  `overflowUpTo`, `usagePermille` funcionam (testes nativos com
  `MemoryBlockStore` e em placa com `LittleFsBlockStore`).
- PID ≥ `0xFD` recusado na configuração.
- Nenhuma escrita de NVS por bloco (só a reserva a cada N, spec 03).

## Riscos e decisões em aberto

| Risco / ponto | Impacto | Recomendação |
|---|---|---|
| `read` linear dentro do segmento | Até ~64 `seek` por bloco; latência no envio | Cursor de leitura do §10 se `read_us` médio > 5 ms. |
| Escrita parcial deixa lixo no fim do segmento | Varredura para antes; blocos seguintes no mesmo arquivo ficariam invisíveis | Selar o segmento na falha (I1) e nunca mais escrever nele. |
| Corrompimento do campo `tamanho` | Varredura de boot perde o resto do segmento | Os `seq` faltantes viram LOSS no envio (spec 05) — perda declarada, não silenciosa. |
| Troca de perfil numa placa com log | Segmentos com `tag_len` diferente convivem; o servidor de bancada só verifica um perfil | Documentado: trocar de perfil = `COLLECTOR_WIPE_ON_BOOT` uma vez + reset do servidor de bancada. |
| `kLargeCapacity` 2600 B estático por tarefa que emite FLUSH | +200 B de RAM | Aceitável; só com `COLLECTOR_EVENTS`. |
| **[Decisão]** Δt sem sinal (`u16`), como no documento | GPS anterior ao `t_ms` inicial não cabe no bloco aberto; Fase 3 lê `u16` | `Collector::append` fecha o bloco (`BLK_CLOSE reason=full`) e abre outro com o `tMs` do GPS como `t_ms` inicial (§2); a Fase 3 não pode supor `t_ms` inicial crescente entre blocos. |
