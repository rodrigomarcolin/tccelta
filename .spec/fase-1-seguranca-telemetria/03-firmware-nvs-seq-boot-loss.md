# 03 — Firmware: reserva de `seq` na NVS, bloco BOOT e estado persistente do protocolo

## Objetivo

Implementar a recuperação de `seq` após perda de energia descrita no
documento (§4.3 e Fig. 4 "reserva-seq-boot"): a NVS guarda uma **reserva**
`R` gravada como `seq + N` a cada `N` blocos (N parametrizável, padrão 64); no
boot o contador retoma de `R` e o primeiro bloco gravado é um **BOOT** com
`seq = R` e `[from_seq, to_seq] = [último seq na flash + 1, R − 1]`, com o
novo `boot_id` no cabeçalho. Persistir também o último `c_S` aceito (proteção
contra *replay* de desafios, spec 05) e manter o `boot_id`. Documentar a
migração da semântica atual (`last_seq` gravado a cada bloco).

O bloco **LOSS** (corrompimento) não é persistido: é gerado no envio pela
spec 05 com o construtor da spec 02. Esta spec só garante que os `seq`
nunca se repetem e que todo "furo" na sequência é declarado.

## Escopo

**Dentro:** `SeqReserve` (puro), `NvsStore` (reserva, `last_cs`, `boot_id`
sem o valor 0, migração), ordem de inicialização em `startCollector` para
gravar o BOOT, `COLLECTOR_WIPE_ON_BOOT`, testes de recuperação com quedas
simuladas.

**Fora:** formato do BOOT (spec 02), uso do `last_cs` no handshake (spec 05),
chaves (spec 01, partição `secnvs`).

## Estado atual (verificado no código)

| Peça | Onde | Hoje |
|---|---|---|
| Contador | `src/collector/SeqCounter.h` | `SeqCounter { peek(); commit(); static recover(lastSeqInNvs, newestSeqInLog) → max + 1 }` |
| Persistência | `src/storage/NvsStore.{h,cpp}` | Namespace `collector` na `nvs`: `config` (blob), `config_ver` (u8 = 2), `boot_id` (u16, `nextBootId()` incrementa e grava), `last_seq` (u32, `saveLastSeq` **a cada bloco**). |
| Gravação | `src/collector/BlockWriter.cpp` | `append` → `saveLastSeq(seq)` → `commit()`. |
| Boot | `src/app/CollectorApp.cpp:53–71` | `nvs.begin()` → `loadConfig()` → `nextBootId()` → `store.begin(wipe)` → `if (wipe) saveLastSeq(0)` → `SeqCounter::recover(loadLastSeq(), store.newestSeq())` → evento `BOOT`. Nenhum bloco é gravado no boot. |
| Semântica | `docs/plano-coleta.md` §5.5 | "A NVS guarda o **último `seq` gravado** (não o próximo). No boot: `próximo = max(NVS, último seq no log) + 1`." Lacunas reais só pelo `oldestSeq` do BATCH. |

O que muda de semântica: `last_seq` (último gravado, escrito por bloco) dá
lugar a `seq_rsv` (limite superior de tudo que já foi gravado, escrito a cada
N blocos); o boot deixa de "colar" a sequência e passa a **declarar** o
intervalo `[último na flash + 1, R − 1]` num bloco BOOT autenticado, que o
servidor aceita mesmo com `seq > ACK_seq + 1`.

## Design

### 1. Conteúdo da NVS (`nvs`, namespace `collector`)

| Chave | Tipo | Quem escreve | Quando | Conteúdo |
|---|---|---|---|---|
| `config`, `config_ver` | blob, u8 | `Elm327Task` (via `CollectorControl`) | `AT#PID`, `AT ST`, `AT SP`… | Sem mudança. |
| `boot_id` | u16 | `startCollector` | Uma vez por boot | Incrementa; **[Decisão]** pula o 0 (`0xFFFF → 1`), porque `boot_id = 0` identifica o LOSS. |
| `seq_rsv` | u32 | `FlushTask` (`BlockWriter`) | Quando `seq ≥ reserva` (no boot e a cada N blocos) | `R`: todo `seq` já gravado na flash é `< R`. Ausente = nunca gravado. |
| `last_cs` | u32 | `SyncTask` (`SyncEngine`, spec 05) | A cada desafio aceito, **antes** de responder | Maior `c_S` aceito. Ausente = 0. |
| `last_seq` | u32 | — | — | **Legado**: lido uma vez na migração e apagado. |
| `wipe_req` | u8 | `Elm327Task` (`AT#WIPE`, só builds de depuração) | Antes do reinício | Pedido de *wipe* no próximo boot (§7). |

Chaves ficam na partição `secnvs` (spec 01). A NVS do ESP-IDF serializa o
acesso internamente; o mesmo objeto `NvsStore` (`Preferences` aberto uma vez)
é usado pelas três tarefas sem mutex próprio, como hoje.

### 2. `SeqReserve` (`src/collector/SeqReserve.h`, puro)

```cpp
namespace collector {

#ifndef SEQ_RESERVE_STEP
#define SEQ_RESERVE_STEP 64
#endif
constexpr uint32_t kSeqReserveStep = SEQ_RESERVE_STEP;
static_assert(kSeqReserveStep >= 1 && kSeqReserveStep <= 65536, "SEQ_RESERVE_STEP fora da faixa");

/** Onde a Flush grava a reserva de seq (NVS no ESP32). */
class ISeqReservePersistence {
public:
    virtual ~ISeqReservePersistence() = default;
    /** Grava e confirma (commit). false = a NVS falhou. */
    virtual bool saveSeqReserve(uint32_t reserve) = 0;
};

/** Resultado da recuperação no boot (Fig. 4 do documento). */
struct SeqRecovery {
    uint32_t bootSeq;     // R: seq do bloco BOOT
    uint32_t fromSeq;     // último seq na flash + 1 (R se a flash está vazia)
    uint32_t toSeq;       // R − 1
    uint32_t reserveRead; // valor lido de seq_rsv (0 = ausente)
    bool     migrated;    // R veio do legado (last_seq/log), não de seq_rsv
};

/**
 * Dono do seq. `peek()` é o próximo seq; só avança em `commit()` (bloco
 * gravado). Antes de gravar `seq`, se `needsReserve(seq)`, a Flush grava
 * `reserveFor(seq)` na NVS e chama `reserved(...)`.
 * Invariante: todo seq gravado na flash é < reserva persistida.
 */
class SeqReserve {
public:
    SeqReserve(uint32_t next, uint32_t reserve, uint32_t step = kSeqReserveStep);

    static SeqRecovery recover(uint32_t reserveInNvs, uint32_t legacyLastSeq, uint32_t newestInLog);
    /** Contador pronto para gravar o BOOT: next = R, reserva em memória = 0 (força gravar R + N). */
    static SeqReserve afterRecovery(const SeqRecovery& recovery, uint32_t step = kSeqReserveStep);

    uint32_t peek() const { return _next; }
    void     commit() { ++_next; }
    bool     needsReserve(uint32_t seq) const { return seq >= _reserve; }
    uint32_t reserveFor(uint32_t seq) const { return seq + _step; }
    void     reserved(uint32_t value) { _reserve = value; }
    uint32_t reserve() const { return _reserve; }

private:
    uint32_t _next, _reserve, _step;
};
}  // namespace collector
```

`recover`:

```
se reserveInNvs > 0:   R = reserveInNvs;            migrated = false
senão:                 R = max(legacyLastSeq, newestInLog) + 1; migrated = true   // 1º boot: R = 1
R = max(R, newestInLog + 1)                                                        // defensivo: nunca reusar seq
fromSeq = (newestInLog == 0) ? R : newestInLog + 1
toSeq   = R − 1
```

Sequência de reservas: o BOOT (`seq = R`) dispara `saveSeqReserve(R + N)`;
os blocos `R+1 … R+N−1` não gravam NVS; o bloco `R + N` dispara
`saveSeqReserve(R + 2N)`, e assim por diante — exatamente "seq + N a cada N
blocos" do documento, com a reserva gravada **antes** do bloco que a
consome.

### 3. Boot (`startCollector`, ordem nova)

| Passo | Ação | Observação |
|---|---|---|
| 0 | Eventos (`SerialEventLog`) | Inalterado. |
| 0.5 | `KeyStore::load` + instâncias de autenticação (spec 01) | Necessário antes do BOOT (assinado). |
| 1 | `nvs.begin()`, `loadConfig()`, `bootId = nextBootId()` | `boot_id` pula 0. |
| 2 | `store.begin(wipe)` (v2, com migração `/log`, spec 02) | `wipe` = `COLLECTOR_WIPE_ON_BOOT` ou `wipe_req` na NVS: `nvs.clearSeqState()` (remove `seq_rsv`, `last_seq`, `last_cs` e `wipe_req`). |
| 3 | `rec = SeqReserve::recover(nvs.loadSeqReserve(), nvs.loadLegacyLastSeq(), store.newestSeq())` | — |
| 4 | `writer = BlockWriter(store, nvs, flushAuth, clock, SeqReserve::afterRecovery(rec), bootId)` | — |
| 5 | `store.startNewSegment()`; `writer.writeBootMarker(rec.fromSeq, rec.toSeq, report)` | Grava `seq_rsv = R + N` e depois o BOOT (`FLUSH kind=boot`). Se falhar, fica pendente (spec 02 §8). |
| 6 | Se `rec.migrated` e o BOOT foi gravado: `nvs.removeLegacyLastSeq()` | Migração concluída. |
| 7 | Evento `BOOT` (campos na spec 06) | `seq_next = writer.nextSeq()`. |
| 8 | Barramento, contexto, objetos, BLE, tarefas | O `BlockWriter` é entregue à `FlushTask`, como hoje. |

**[Decisão] Todo boot grava um BOOT, inclusive o primeiro** (`R = 1`,
`[1, 0]`, faixa vazia) e quando não há furo (`newest = R − 1` → `[R, R − 1]`).
O servidor aceita pela mesma regra (`from = ACK + 1 = R` e `to = seq − 1 = R − 1`)
e ganha, de graça, a fronteira de cada boot com o `boot_id` no cabeçalho.
Faixa vazia ≡ `to_seq = from_seq − 1`; a Fase 3 registra lacuna só quando
`to_seq ≥ from_seq`.

### 4. Quedas de energia — por que nunca há `seq` repetido nem furo silencioso

Notação: `n` = maior `seq` no log; `Rnvs` = reserva na NVS.

| Queda em… | NVS após a queda | Log após a queda | Próximo boot |
|---|---|---|---|
| Boot, antes de gravar `R + N` | `Rnvs = R` | `n < R`, sem BOOT | `R` de novo; BOOT `[n+1, R−1]` (o BOOT anterior nunca existiu) |
| Boot, entre gravar `R + N` e o BOOT | `R + N` | `n < R` | `R' = R + N`; BOOT `[n+1, R+N−1]` cobre o `R` não usado |
| Operação normal, bloco `s` gravado | `R + kN > s` | `n = s` | BOOT `[s+1, Rnvs−1]` |
| Durante o `append` de `s` | `> s` | `n = s − 1` (registro parcial descartado pela varredura, segmento selado) | BOOT `[s, Rnvs−1]` declara o `s` |
| Durante a escrita de `seq_rsv` | Valor antigo **ou** novo (entrada NVS é atômica) | `n` < valor antigo | Correto nos dois casos: a reserva só é gravada antes de o `seq` correspondente ir para a flash |
| Bloco `s` já lido pelo app e não confirmado | `> s` | `n ≥ s` | `from = n + 1 > s`: `s` continua na flash e é reenviado normalmente |

Invariantes verificados pelos testes de propriedade (Testes, abaixo):
- **R1** todo `seq` na flash < reserva persistida;
- **R2** os `seq` do log são estritamente crescentes entre boots;
- **R3** a união dos `seq` no log com as faixas dos BOOT cobre todos os
  inteiros de 1 até o último `seq` (sem furo não declarado).

**Furo com a flash vazia.** Se a varredura de boot não encontrar nenhum
bloco (`newestInLog = 0`) mas a reserva indicar `R > 1` (log v1 apagado na
migração, ou único segmento descartado por cabeçalho inválido), o BOOT sai
com faixa vazia `[R, R − 1]` e os `seq` entre o ACK do servidor e `R − 1`
**não** são declarados pelo BOOT. Não há travamento: no envio, o cursor em
`ACK + 1` cai no caso D da spec 05 §6 e gera LOSS (em pedaços de até 64)
até `R − 1`. A perda chega declarada, com causa LOSS em vez de BOOT; o
`seq_audit` (spec 06 §6.8) aceita faixas de `MARKER kind=loss` como
cobertura (R3).

Limite conhecido: os registros em RAM (bloco aberto, até 10 s) perdidos numa
queda não têm `seq` — é a perda de até `T_flush` aceita na Investigação
(Tabela 7, "Intervalo máximo de flush").

### 5. Desgaste da NVS

| Escrita | Antes (`last_seq`) | Depois |
|---|---|---|
| Por bloco | 1 | 0 |
| Reserva | — | 1 a cada N = 64 blocos + 1 por boot |
| `last_cs` | — | 1 por handshake aceito |

Com 0,1–0,17 bloco/s (Tabela 14 da Investigação) e 2 h de uso por dia:
720–1 224 blocos/dia → 12–20 reservas + ~4 boots + ~10 handshakes ≈ 35
escritas/dia (antes: ~1 200). Uma página NVS (4 KB, 126 entradas) é apagada a
cada ~3,6 dias; desgaste irrelevante. N maior reduz escritas e aumenta só o
tamanho cosmético das faixas BOOT; N menor que 8 não traz ganho.

### 6. `NvsStore` v2

```cpp
class NvsStore : public collector::ISeqReservePersistence,
                 public sync::ISyncStatePersistence {      // interface da spec 05
public:
    bool begin();
    collector::CollectorConfig loadConfig();
    void saveConfig(const collector::CollectorConfig& config);
    uint16_t nextBootId();                       // pula 0

    uint32_t loadSeqReserve();                   // 0 = ausente
    bool     saveSeqReserve(uint32_t reserve) override;   // putUInt; true se gravou 4 bytes
    uint32_t loadLegacyLastSeq();                // 0 = ausente
    void     removeLegacyLastSeq();
    void     clearSeqState();                    // wipe de bancada: remove seq_rsv, last_seq, last_cs, wipe_req
    bool     wipeRequested();                    // AT#WIPE gravou wipe_req = 1
    void     requestWipe();

    uint32_t loadLastAcceptedCs() override;      // 0 = ausente
    bool     saveLastAcceptedCs(uint32_t cs) override;
};
```

`Preferences::putUInt` devolve o número de bytes gravados (0 em falha) e já
faz `nvs_commit`. As durações dessas escritas entram nos eventos
`FLUSH reserve_us` e `HS_RESPONSE nvs_us` (spec 06).

### 7. `COLLECTOR_WIPE_ON_BOOT` (bancada)

Formata o LittleFS e remove `seq_rsv`, `last_seq` **e `last_cs`** → o
próximo BOOT tem `seq = 1` e qualquer `c_S` volta a ser aceito. Também
disparável em tempo de execução por `AT#WIPE` (spec 05 §12, chave
`wipe_req`). **[Decisão]** zerar `last_cs` no *wipe*: o servidor de bancada
emite `c_S` a partir da época Unix (~1,7·10⁹, spec 09 §7); sem zerar, uma
placa que falou com ele recusaria (erro 0x02) todo desafio de um servidor
que conte `c_S` a partir de 1. **Consequência documentada:** o
estado do dongle no servidor (ACK, HEAD) precisa ser zerado junto — no
servidor de bancada do app há o botão "Esquecer estado do dongle" (spec 09);
na Fase 3, um comando administrativo.

## Plano arquivo por arquivo

| Ação | Arquivo | Mudança |
|---|---|---|
| Criar | `src/collector/SeqReserve.h` (+ `.cpp` para `recover`) | §2. |
| Remover | `src/collector/SeqCounter.h` | Substituído. |
| Alterar | `src/collector/BlockWriter.{h,cpp}` | Recebe `SeqReserve` e `ISeqReservePersistence` (spec 02 §8, passo 2). |
| Alterar | `src/storage/NvsStore.{h,cpp}` | §6; constantes `kKeySeqReserve = "seq_rsv"`, `kKeyLastCs = "last_cs"`, `kKeyLegacyLastSeq = "last_seq"`. |
| Alterar | `src/app/CollectorApp.cpp` | Ordem do §3; remove `SeqCounter::recover` e `saveLastSeq(0)`. |
| Alterar | `platformio.ini` | Comentário documentando `-DSEQ_RESERVE_STEP=<n>` (padrão 64) no `platformio_local.ini.example`. |
| Alterar | `docs/plano-coleta.md` §5.5 | Nota "substituído pela reserva de seq + BOOT (Fase 1 de segurança)". |
| Criar | `test/test_seq_reserve/test_main.cpp` | Ver Testes. |
| Alterar | `test/support/Fakes.h` | `FakeSeqReservePersistence { std::vector<uint32_t> saved; bool fail; }`. |

## Testes

Nativos (`pio test -e native -f test_seq_reserve`):

| Caso | Entrada (`reserveInNvs`, `legacyLast`, `newestInLog`) | Esperado (`bootSeq`, `from`, `to`, `migrated`) |
|---|---|---|
| Primeiro boot | (0, 0, 0) | (1, 1, 0, true) |
| Reboot normal | (192, 0, 130) | (192, 131, 191, false) |
| Reboot sem furo | (131, 0, 130) | (131, 131, 130, false) |
| Migração v1 com log | (0, 127, 127) | (128, 128, 127, true) |
| Migração v1 sem log (log v1 apagado) | (0, 500, 0) | (501, 501, 500, true) |
| Reserva inconsistente (defensivo) | (100, 0, 150) | (151, 151, 150, false) |
| Wipe | (0, 0, 0) após `clearSeqState` | (1, 1, 0, true) |

- `BlockWriter` + `FakeSeqReservePersistence` + `MemoryBlockStore`, N = 4:
  BOOT em `R = 10` grava 14; blocos 11–13 não gravam; o 14 grava 18; a
  reserva é gravada antes do `append` (ordem registrada pelos dublês).
- `saveSeqReserve` falhando: o bloco não é gravado e o `seq` não avança.
- **Propriedade** (10 000 iterações, semente fixa): sequência aleatória de
  {gravar bloco, gravar com falha de `append`, queda em ponto aleatório
  entre os passos 2–7 da spec 02 §8, reboot}; após cada reboot, recuperar
  a partir do estado dos dublês e conferir R1, R2 e R3.

Em placa (`collector_mock_debug`):
- Ligar, gravar ~10 blocos, desligar a USB, religar: `BOOT rsv=<R> from=<n+1> to=<R−1>`
  e `FLUSH kind=boot seq=<R>` como primeiro `FLUSH`.
- 30 ciclos de liga/desliga em instantes aleatórios: conferir R2/R3 no log
  de eventos com o script de análise da spec 06 (`seq_audit`).

## Passos de implementação

1. `SeqReserve` + tabela de testes. *Checkpoint:* `test_seq_reserve` verde (exceto o de propriedade).
2. `BlockWriter` com reserva (junto com o passo 3 da spec 02) + teste de propriedade.
   *Checkpoint:* propriedade verde com 10 000 iterações.
3. `NvsStore` v2 e ordem de boot. *Checkpoint:* em placa, primeiro `FLUSH`
   de cada boot é `kind=boot` com a faixa correta; `last_seq` some da NVS
   após a migração (conferir com `AT#SYNC?`, spec 05, que mostra `rsv`).
4. Ensaio de 30 quedas. *Checkpoint:* `seq_audit` sem violação.

## Critérios de aceite

- NVS escrita só a cada N blocos (+ boot + handshake); N configurável por
  `-DSEQ_RESERVE_STEP`.
- Todo boot começa com um BOOT autenticado `seq = R`, `[último na flash + 1, R − 1]`,
  `boot_id` novo no cabeçalho, antes de qualquer bloco de dados.
- Invariantes R1–R3 verdadeiros no teste de propriedade e no ensaio de quedas.
- `last_cs` persistido e lido pelo `SyncEngine` (spec 05).
- Migração de `last_seq` funciona uma única vez.

## Riscos e decisões em aberto

| Risco / ponto | Impacto | Recomendação |
|---|---|---|
| Falha persistente da NVS | Nenhum bloco é gravado (invariante preservado, coleta perdida) | Contar em `failedBlocks`; aparece no `AT#LIST`. Melhor perder do que repetir `seq`. |
| BOOT não gravado (flash cheia/erro) | Dados bloqueados até gravar o BOOT | Pendente com nova tentativa a cada `write` (spec 02 §8). |
| `boot_id` dá a volta (65 535 boots) | Ambiguidade no servidor | Limite do documento (2¹⁶ boots); pular o 0 evita colisão com LOSS. |
| Reflash com NVS apagada (`erase_flash`) em placa já registrada | `seq` recomeça; servidor rejeita (HEAD menor que o salvo) | Procedimento: reprovisionar e zerar o estado do dongle no servidor; nunca `erase_flash` em placa de campo. |
| **[Aberto]** Faixa vazia `[R, R−1]` no BOOT | A Fase 3 precisa tratar como "sem lacuna" | Combinado no §3; confirmar com a spec de ingestão da Fase 3. |
| **[Aberto]** `c_S` a partir da época no servidor de bancada × servidor real | Placa usada na bancada recusa desafios do servidor real (0x02) | *Wipe* zera `last_cs` (§7); recomendar à Fase 3 emitir `c_S = max(c_S_banco + 1, segundos desde a época)`, o que também cobre placas reaproveitadas. |
