# Roteiro de testes: limite de N blocos no log da flash

Valida em placa real o commit `d02f091` (`feat(firmware): limita o log da flash a N blocos`):
`src/storage/LittleFsBlockStore.cpp`, `src/collector/LogLimit.h`, `[block_log]` no `platformio.ini`.

Todos os comandos são para o **Git Bash** e partem da pasta `esp32-firmware` (veja 0.3).

## 0. Preparação

### 0.1 O que mudou (resumo para conferir contra os eventos)

- `N = (LittleFS.totalBytes() − margem%) / maxRecordBytes(tagLen)`, preso a `[128, 16384]`.
  `-DBLOCK_LOG_MAX_BLOCKS=n` fixa N (também preso a `[128, 16384]`) e vence o cálculo.
- `append`: se `blocos + 1 > N` (ou faltam bytes livres: `bytes + 8 KB`), descarta o **segmento mais antigo inteiro** (até 64 blocos) e emite `SEG_DROP first last count`. Conta em `lostBlocks` e sobe `overflowUpTo`.
- `begin` (boot): se N encolheu, descarta os mais antigos até caber (também com `SEG_DROP`).
- `usagePermille = blocos*1000/N`. O `SyncEngine::tick` (a cada ~1 s) liga o aviso de flash com `usage > 800` e desliga com `usage < 780`.
- O `BOOT` ganhou `log_max=N`. `BLOCK_LOG_LIMIT_BYTES` foi removido.

### 0.2 Valores de N com a partição de 2 MB (`partitions_collector.csv`: `spiffs` 0x200000 = 2097152 B)

`maxRecordBytes(tag) = 1024 (bloco, já com o cabeçalho de 12 B) + tag + 4 (CRC) + 1 (cabeçalho de segmento de 8 B rateado por 64 blocos)` = 1029 / 1061 / 1093 (conferido em `test_log_limit`).

| Perfil (env) | tag | registro máx. | N, margem 10% (utilizável 1887437 B) | N, margem 50% (utilizável 1048576 B) |
|---|---|---|---|---|
| none (`collector_mock_debug_none`) | 0 | 1029 | **1834** | 1019 |
| hmac (`collector_mock_debug_open`, `collector_mock_debug`) | 32 | 1061 | **1778** | 988 |
| asym (`collector_mock_debug_asym`) | 64 | 1093 | **1726** | 959 |

Limiares do aviso de flash (`usage` = `blocos*1000/N`, divisão inteira):

| N | liga (usage > 800) | desliga (usage < 780) |
|---|---|---|
| 200 | blocos ≥ 161 | blocos ≤ 155 (156 a 160 = histerese) |
| 988 | blocos ≥ 791 | blocos ≤ 770 |

### 0.3 Ferramentas e comandos

Abra o Git Bash e prepare a sessão (o `pio` já está no PATH). Repita estes comandos a cada janela nova, ou ponha no `~/.bashrc`:

```bash
cd ~/Projects/tccelta/tccelta/esp32-firmware        # todos os caminhos do roteiro partem daqui
export PYTHONUTF8=1                                  # acentos certos na saída dos scripts Python
conda activate tccelta                               # Python 3.13 com pyserial e bleak
```

Rode o `conda activate tccelta` em toda janela que for usar Python. O env `tccelta` já tem `pyserial` e `bleak`. Fora dele, `python` no Git Bash é só o atalho da Microsoft Store e não roda nada. Confira com `python --version`, que deve mostrar 3.13.

`platformio_local.ini` (fora do git; copie de `platformio_local.ini.example` se ainda não existe). Ele já deve ter a sua porta:

```ini
[base]
upload_port  = COMx
monitor_port = COMx
```

As seções que cada caso pede (`[block_log]` etc.) vão **neste mesmo arquivo**, e valem para todos os envs. Uma opção redefinida no local **substitui** a original (não soma). Por isso:

- `[block_log] build_flags` pode ser redefinido à vontade (a original só tem `-DBLOCK_LOG_MARGIN_PCT=10`).
- Se for redefinir `[base] build_flags` (caso 9, `COLLECTOR_WIPE_ON_BOOT`), repita `-DCONFIG_BT_NIMBLE_MSYS1_BLOCK_COUNT=24`, senão o build perde essa flag do NimBLE.

Comandos (substitua `<env>`):

```bash
pio run -e <env> -t upload                   # compila e grava o firmware (não mexe na partição LittleFS)
winpty pio device monitor -e <env>    # serial a 921600 baud nos envs *_debug; sai com Ctrl+C
```

O monitor do PlatformIO precisa de um console de verdade. No mintty do Git Bash, sem o `winpty`, ele pode não receber as teclas nem o Ctrl+C. Se o seu Git Bash usa o pseudo-console do Windows (opção "Enable experimental support for pseudo consoles" na instalação), `pio device monitor -e <env>` funciona direto.

Se perder o `BOOT` ao abrir o monitor, aperte EN (reset) com o monitor aberto. O monitor e o `capture.py` não dividem a porta: para rodadas longas use um **ou** outro.

Envs usados:

| Env | Uso |
|---|---|
| `collector_mock_debug_open` | Padrão deste roteiro. Mock (sem carro), eventos na serial, perfil hmac, enlace BLE **aberto** (qualquer central conecta e escreve no NUS e no SYNC sem parear). Só de bancada. |
| `collector_mock_debug_none`, `collector_mock_debug_asym` | Só para ler `log_max` do BOOT (caso 2). Enlace com pareamento normal. |

Observação: o log de eventos só existe com `COLLECTOR_EVENTS` (todos os `*_debug`). Os comandos `AT#...` de bancada só funcionam nesses envs.

### 0.4 Captura serial e contagem de blocos

**Não existe evento que informe a contagem de blocos nem o `usagePermille`.** Para saber quantos blocos há no log, conte pelos eventos. Capture a serial com o `capture.py` do repositório (grava só as linhas `EV`, com carimbo do PC; já no env `tccelta`):

```bash
python tools/event-bench/capture.py --port COMx --out logs/limite-caso3.log    # Ctrl+C para parar
```

Deixe a captura numa janela do Git Bash e use uma segunda janela (com a mesma preparação de 0.3) para os comandos `AT#` e para acompanhar o arquivo ao vivo:

```bash
tail -f logs/limite-caso3.log | grep --line-buffered -E ' (BOOT|SEG_NEW|SEG_DROP|SYNC_OVERFLOW|SYNC_FREE|SYNC_NOTICE|HS_ACK) '
grep -c ' FLUSH .*ok=1' logs/limite-caso3.log      # quantos blocos foram gravados até agora
```

`logs/` está no `.gitignore`. Os dois scripts auxiliares estão no repositório:

- `tools/event-bench/contar_blocos.py` reconstrói os segmentos a partir de `SEG_NEW`, `FLUSH ok=1`, `SEG_DROP` e `SYNC_FREE` e imprime blocos, N (`BOOT log_max`) e usage. **Comece a captura logo depois de um wipe** (caso 9), senão o estado inicial é desconhecido. A captura pode seguir aberta através de vários boots. Sai com código 1 se algum `SEG_DROP` não bater com a contagem (`DIVERGE`): a captura começou fora de um wipe ou perdeu linhas (buraco em `n`).
- `tools/sync-bench/nus_at.py` envia um comando `AT#...` pelo BLE (NUS RX `6E400002-...`, resposta no NUS TX `6E400003-...`). **Os `AT#` não entram pela serial**: o firmware não lê a UART. Só funciona no env `*_open` (sem pareamento). Usa o `bleak` do env `tccelta`.

```bash
python tools/event-bench/contar_blocos.py logs/limite-caso3.log | tail -n 5
python tools/sync-bench/nus_at.py 'AT#SYNC?'
```

No `AT#SYNC?`, deve aparecer `CMD txt="AT#SYNC?"` e `RESP ...` na serial. Ponha o comando **entre aspas** (simples ou duplas). Sem aspas, o bash trata o `#` como começo de comentário em alguns contextos e o `?` como curinga de arquivo.

### 0.5 Ritmo de produção de blocos (estimativa, leia antes de planejar)

- Config padrão: 7 PIDs, `flush` = 10000 ms. No log real de bancada `logs/device-monitor-261031-182039.log` (TWAI, perfil asym, **não** mock) o bloco fecha por `reason=full` com `len=1022 recs=202 age≈6,6 s`, ou seja, **~1 bloco a cada 6,6 s**. Não medi o mock; ele usa o mesmo agendador, então espere algo na mesma ordem (confirme com seu `BLK_CLOSE age=`).
- Nesse ritmo: **200 blocos ≈ 22 min**, 128 blocos ≈ 14 min, **1778 blocos ≈ 3 h 15 min** (impraticável como rotina). Por isso os casos de transbordo usam `BLOCK_LOG_MAX_BLOCKS=200`; os casos 2 e 10 só leem `log_max` no BOOT, sem encher.
- Acelerador opcional (não testei na placa): `AT#FLUSH 1000` (mínimo aceito, `kMinFlushIntervalMs`) faz o bloco fechar por prazo a cada 1 s: ~1 bloco/s, 200 blocos ≈ 3,5 min. É gravado na NVS e **sobrevive a reboot e a AT#WIPE**; volte com `AT#FLUSH 10000` ao terminar. Confira com `BLK_CLOSE reason=deadline` e `CONFIG ... flush=1000`. Os tempos abaixo valem para o ritmo padrão; divida por ~6 se usar o acelerador.

---

## Caso 1: testes nativos e builds

**Objetivo.** Garantir que a lógica de N e o código compilam em todos os alvos afetados.

**Config.** Nenhuma (apague/renomeie o `platformio_local.ini` de teste se tiver `BLOCK_LOG_MAX_BLOCKS` ligado).

**Comandos.**

```bash
pio test -e native -f test_log_limit
pio test -e native
for env in collector_mock_debug_open collector_mock_debug_none collector_mock_debug_asym collector_mock; do
    pio run -e "$env" || echo "FALHOU: $env"
done
```

**Esperado.**
- `test_log_limit`: 6 casos aprovados (`test_registro_maximo`, `test_2mb_10pct_tag32`, `test_tag0_e_tag64`, `test_margem_zero`, `test_piso`, `test_teto`). (Rodei só este suíte no PC: 6/6 passaram; o resto dos comandos não rodei.)
- Suíte `native` completa sem falhas; builds sem erro.

**Passa se** todos terminam com código 0. **Falha** em qualquer teste vermelho ou erro de compilação (em especial referência a `BLOCK_LOG_LIMIT_BYTES`/`capacityBytes`).

---

## Caso 2: N padrão por perfil (`log_max` no BOOT)

**Objetivo.** Conferir N calculado com a flash real (`LittleFS.totalBytes()`).

**Config.** `platformio_local.ini` só com a porta (sem `[block_log]`): margem 10%, sem `MAX_BLOCKS`.

**Passos.**
1. `pio run -e collector_mock_debug_open -t upload`, depois `winpty pio device monitor -e collector_mock_debug_open`.
2. Ache a linha `BOOT` (aparece depois de `FORMAT`, `KEYS`, `MARKER`, `SEG_NEW`, `FLUSH kind=boot`).
3. Repita com `collector_mock_debug_none` e `collector_mock_debug_asym`.

**Esperado.**

| Env | `KEYS tag_len` | `BOOT log_max` |
|---|---|---|
| `collector_mock_debug_open` (hmac) | 32 | **1778** |
| `collector_mock_debug_none` | 0 | **1834** |
| `collector_mock_debug_asym` | 64 | **1726** |

Exemplo da linha: `EV .. .. BOOT boot=.. wiped=0 ... lost=.. log_max=1778`.

**Passa se** `log_max` é o valor da tabela em cada perfil. **Falha** se divergir; se for ligeiramente menor, veja se `LittleFS.totalBytes()` não é 2097152 (nesse caso o desvio é da partição, não do cálculo; reporte).
---

## Caso 3: transbordo com `BLOCK_LOG_MAX_BLOCKS=200`

**Objetivo.** `SEG_DROP` no momento certo; contagem oscilando entre 136 e 200; `lost` e `overflowUpTo`.

**Config** (`platformio_local.ini`):

```ini
[block_log]
build_flags =
    -DBLOCK_LOG_MAX_BLOCKS=200
```

**Passos.**
1. `upload` do `collector_mock_debug_open`.
2. Feche o monitor e inicie `capture.py --out logs/limite-caso3.log` (antes do wipe, para o boot de wipe ficar no arquivo).
3. Wipe: `python tools/sync-bench/nus_at.py "AT#WIPE"` (a placa reinicia; alternativa: `COLLECTOR_WIPE_ON_BOOT`, caso 9).
3b. Dica: para não esperar ~22 min, logo depois do wipe mande `python tools/sync-bench/nus_at.py 'AT#FLUSH 1000'` (~3,5 min até 200 blocos; ver 0.5). O valor fica na NVS: volte com `AT#FLUSH 10000` ao terminar.
4. Espere ~22 min (ritmo padrão) até o primeiro `SEG_DROP`, e ~7 min por queda seguinte. Rode `contar_blocos.py` de vez em quando.
5. Opcional, com `python tools/sync-bench/nus_at.py "AT#LIST"`: a última linha traz `LOST n`.

**Esperado.**
- `BOOT ... wiped=1 ... log_max=200 lost=0`; a contagem começa em 1 (o bloco BOOT).
- Primeiro `SEG_DROP` quando entraria o bloco 201 desde o wipe. O BOOT abre o segmento 1 (`SEG_NEW first=1 reason=first`) e os dados seguem nele até 64 blocos, então o esperado é `SEG_DROP first=1 last=64 count=64`. Os blocos passam de 200 para 136 e o bloco novo vira 137.
- Quedas seguintes de 64 em 64 blocos: `first=65 last=128 count=64`, depois `first=129 last=192 count=64` etc. (`first` = `last` anterior + 1; sem lacuna dentro de um boot). A cada 64 blocos novos.
- O `contar_blocos.py` mostra `blocos` indo até 200 e voltando a 136/137; **nunca** acima de 200.
- Logo depois de cada `SEG_DROP` (até ~1 s, no `tick`) sai `SYNC_OVERFLOW upto=<last do SEG_DROP> ack=0`, porque ninguém deu ACK (`overflowUpTo` = `last`).
- `AT#LIST` → `LOST 64` depois da 1ª queda, `LOST 128` depois da 2ª.
- Não aparece `SEG_DROP` antes do 201º bloco, nem `FLUSH ok=0`.

**Passa se** o 1º `SEG_DROP` ocorre com 200 blocos no log (contagem do script) e `count` é o tamanho do segmento removido, a contagem fica sempre em [136, 200] depois da 1ª queda, `upto` do `SYNC_OVERFLOW` = `last` do `SEG_DROP`, `LOST` = soma dos `count`. **Falha** se a queda vier cedo ou tarde, se a contagem passar de 200, ou se faltar `SYNC_OVERFLOW`.

---

## Caso 4: aviso de flash alta (~80% de N) e fim abaixo de 78%

**Objetivo.** Ver o bit `0x04` de `flags` ligar com `usage > 800` e desligar com `usage < 780`.

**Como o aviso aparece (importante).** **Não há evento de serial só porque a flag mudou.** O aviso sai como `SYNC_NOTICE head=.. flags=.. reason=flash` apenas se há central **conectada e inscrita** no SYNC_TX (`sendNotice` retorna sem fazer nada caso contrário). Os avisos seguintes (`reason=flush`, `retry`...) também carregam `flags`. Bits: `0x01` sem handshake na conexão, `0x02` sem handshake no boot, `0x04` flash alta. Sem BLE, só dá para ver o efeito indireto (caso 3).

**Config.** A do caso 3 (`MAX_BLOCKS=200`). Limiares: liga com 161 blocos, desliga com 155 (ver 0.2).

**Passos.**
1. Parta de um log com ~140 blocos (continue o caso 3 após a 1ª queda, ou espere ~15 min após um wipe).
2. Conecte a central de teste e **inscreva-se no SYNC_TX** (`7A3C0003-9B1E-4F2A-8C5D-2E6F1A0B3C4D`, notify) com um app BLE genérico (nRF Connect), ou rode `python tools/sync-bench/sync_bench.py --scenario handshake` e depois `--scenario stream --duration 120` (usa o `bleak` do env `tccelta`; usa as chaves fixas de bancada, que casam com os envs debug sem `secnvs` provisionada). Na serial: `SYNC_SUB on=1` e `SYNC_NOTICE ... reason=subscribe`.
3. Acompanhe os `SYNC_NOTICE` e a contagem do script.
4. Para ler o estado sem depender de aviso: `python tools/sync-bench/nus_at.py 'AT#SYNC?'` imprime `flags=XX` (mesmo campo).
5. Parte B (desligar por exclusão): com a flag ligada, `AT#SYNC?` (anote `head=`) e `AT#ACK <head>` (caso 8).

**Esperado.**
- Ao chegar a **161 blocos**: `SYNC_NOTICE ... flags=05 reason=flash` (bit `0x04` ligado; `0x01` junto enquanto não houve handshake de ACK). Com 160 blocos ainda não liga (`160*1000/200 = 800`, não é `> 800`).
- Blocos 156 a 160 (histerese): a flag permanece como estava.
- Parte A (queda natural): ao chegar a 200 e cair para 137 (`SEG_DROP`), usage = 685 < 780: `SYNC_NOTICE ... reason=flash` sem o bit `0x04` (`flags=01`). Ao subir de novo até 161, liga outra vez. Há um aviso `reason=flash` por transição.
- Parte B: após o `AT#ACK`, `SYNC_FREE`; blocos restantes = os do segmento aberto (≤ 64 → usage ≤ 320) e `SYNC_NOTICE reason=flash` com o bit `0x04` desligado em até ~1 s.

**Passa se** a transição liga ocorre no bloco 161 (não antes) e a transição desliga ocorre abaixo de 780 (≤ 155 blocos), nunca entre 156 e 160. **Falha** se o bit ligar antes de 161, desligar na faixa 156-160, ou nunca desligar após a queda/exclusão.

---

## Caso 5: reduzir N entre boots (200 para 128)

**Objetivo.** `begin` descarta o excedente quando N diminui.

**Config.** Fase A: `MAX_BLOCKS=200` (caso 3). Fase B: troque para `-DBLOCK_LOG_MAX_BLOCKS=128`.

**Passos.**
1. Fase A: rode até o log ter perto de 200 blocos (script, depois de pelo menos 1 queda ou ~190 blocos). Anote `blocos` e os segmentos que o script imprime.
2. Edite o `platformio_local.ini` para 128 e faça `upload` **sem apagar a flash** (nada de `erase`/`uploadfs`). Mantenha a captura aberta (ou reabra apendando).
3. Observe a serial do boot.

**Esperado.**
- Entre `KEYS` e `MARKER`/`BOOT`: um ou mais `SEG_DROP` (sempre do mais antigo, `count` ≤ 64), até restarem ≤ 128 blocos. Exemplo: segmentos 64+64+64+8 = 200 → 2 quedas (136 > 128 ainda) → restam 72; o bloco BOOT vira 73.
- `BOOT ... log_max=128` e `lost` = soma dos `count` das quedas do boot (`lost` é por boot, começa em 0).
- Depois do BOOT, o script mostra `blocos ≤ 128`. O `BOOT log_oldest` é o `first` do segmento mais antigo que sobrou.
- Pode aparecer `SYNC_OVERFLOW upto=.. ack=0` logo após o boot (blocos não confirmados foram descartados).

**Passa se** `blocos` ≤ 128 ao fim do boot, `lost` bate com os `SEG_DROP`, `log_max=128`. **Falha** se sobrar > 128, se o log perder segmento além do necessário (resta ≤ 128 − 64 sem motivo) ou se não houver `SEG_DROP` no boot.

---

## Caso 6: aumentar N entre boots, sem perda

**Objetivo.** Aumentar N não apaga nada.

**Config.** Fase A: `MAX_BLOCKS=128`. Fase B: remova o `[block_log]` do local (volta ao N padrão, 1778) ou use `-DBLOCK_LOG_MAX_BLOCKS=400`.

**Passos.**
1. Fase A: deixe o log passar da 1ª queda (~15 min com N=128) e anote o `last` do último `SEG_DROP` (o mais antigo é `last + 1`) e `blocos` (script).
2. Troque a config, `upload`, e observe o boot.

**Esperado.**
- Nenhum `SEG_DROP` no boot. `BOOT ... lost=0 log_max=1778` (ou `400`).
- `BOOT log_oldest` = `last + 1` anotado. O script mostra `blocos` anterior + 1 (o BOOT).
- Sem `SYNC_OVERFLOW` após o boot.

**Passa se** `lost=0`, sem `SEG_DROP` e `log_oldest` inalterado. **Falha** se algo for descartado.

---

## Caso 7: persistência através de reboot

**Objetivo.** O contador `_blocks` é reconstruído do disco (`begin` → `pushSegment`) e o `usage` fica coerente.

**Config.** `MAX_BLOCKS=200`, `collector_mock_debug_open`, log limpo (caso 9) e captura ligada.

**Passos.**
1. Deixe o log chegar a B blocos, com 140 ≤ B ≤ 190 (use o script; sem queda ainda).
2. Reinicie com o botão EN (não regrave).
3. Continue a captura e deixe rodar até a 1ª `SEG_DROP`.

**Esperado.**
- Boot: sem `SEG_DROP`, sem `SEG_SEAL` (reset limpo). `BOOT lost=0 log_max=200`; `log_oldest` igual ao de antes (1 após um wipe).
- O BOOT abre um segmento novo (`SEG_NEW ... reason=boot`): blocos = **B + 1**.
- A 1ª `SEG_DROP` pós-reboot vem depois de exatamente `200 − B` `FLUSH ok=1` seguintes ao `FLUSH kind=boot` (blocos chegam a 200 e o próximo não cabe). Ela remove o segmento mais antigo (`first=1`, com os blocos 1 a B, que ficou parcial por causa do segmento novo do boot: `count` = o que o script mostrou para esse segmento).
- Verificação extra com BLE inscrito (opcional): se B = 160, o reboot leva a 161 e o `SYNC_NOTICE reason=flash flags=..04..` aparece logo após o primeiro tick depois do BOOT (inscreva-se de novo na reconexão). Se `_blocks` fosse zerado no boot, não apareceria.

**Passa se** a 1ª queda ocorre no bloco previsto pela soma (B + 1 + k = 201) e `count` bate com o tamanho real do segmento. **Falha** se a queda vier com contagem errada (contador não reconstruído), ou se o boot descartar algo com N inalterado.

---

## Caso 8: ACK/sincronização libera espaço

**Objetivo.** `deleteUpTo` remove segmentos inteiros, o `usage` cai e a flag de flash apaga.

**Config.** `MAX_BLOCKS=200`, `collector_mock_debug_open`, log com 165-190 blocos (flag de flash ligada).

**Passos (A: ACK injetado).**
1. `python tools/sync-bench/nus_at.py 'AT#SYNC?'`. Resposta `SYNC conn=.. sub=.. open=.. ack=.. ... head=H tail=T ... flags=XX ...`. Anote `H`.
2. `python tools/sync-bench/nus_at.py "AT#ACK <H>"`. `H` precisa estar em [`ack`, `head`], senão `HS_ACK result=bad_seq`.

**Passos (B: sync completo).** `python tools/sync-bench/sync_bench.py --scenario stream --duration 120`: faz o handshake, lê os blocos (`SYNC_BLOCK`) e dá ACK a cada 10 s (`--tlat`). Imprime `ACK n`.

**Esperado.**
- A: `CMD txt="AT#ACK H"`, `HS_ACK ... result=ok advanced=1`, `SYNC_FREE upto=H segs=k oldest=<first do segmento aberto> dur_us=..`. `deleteUpTo` remove todos os segmentos menos o mais novo (o aberto nunca é apagado, mesmo ≤ ACK). Logo, blocos = tamanho do segmento aberto (1 a 64), usage ≤ 320, e `AT#SYNC?` sem o bit `0x04` em até ~1 s. Se estiver inscrito, `SYNC_NOTICE reason=flash` sem `0x04`.
- Nenhum `SEG_DROP` por causa do ACK. Depois dele, o log enche de novo até 200 só após ~(200 − blocos) × 6,6 s.
- B: vários `SYNC_FREE` ao longo dos ACKs, em lotes de segmentos inteiros; blocos caem em degraus de até 64.
- Um `SEG_DROP` posterior descarta blocos já confirmados? Só se o segmento mais antigo tiver `last ≤ ack`: nesse caso **não** sai `SYNC_OVERFLOW` (`checkOverflow` ignora `up <= ackLocal`). Sai `SYNC_OVERFLOW` se for não confirmado.

**Passa se** `SYNC_FREE.segs` > 0, o script mostra os blocos caindo para o tamanho do segmento aberto e a flag apaga. **Falha** se `segs=0` com ACK válido, se a contagem não cair, ou se o usage continuar alto (flag de flash presa).

---

## Caso 9: wipe (AT#WIPE e COLLECTOR_WIPE_ON_BOOT)

**Objetivo.** Zerar log, contagem e usage.

**Parte A: `AT#WIPE`** (qualquer momento, env `*_open`).
1. `python tools/sync-bench/nus_at.py "AT#WIPE"` → `OK` e a placa reinicia (`requestWipe` grava `wipe_req` na NVS e dá `esp_restart`).
2. No boot: `BOOT ... wiped=1 ... log_oldest=.. log_newest=.. lost=0 log_max=<mesmo N>`; `SEG_NEW first=<seq> reason=first` e `FLUSH kind=boot`; o seq reinicia e `boot` continua crescendo (config e `boot_id` preservados; valores exatos de `seq_next` não verifiquei).
3. Contagem pelo script = 1; usage = 1000/N (5 com N=200).
4. Um reset (EN) seguinte volta a `wiped=0`.

**Parte B: `COLLECTOR_WIPE_ON_BOOT`** (wipe a cada boot; não deixe ligado).

```ini
[base]
upload_port  = COMx
monitor_port = COMx
build_flags =
    -DCONFIG_BT_NIMBLE_MSYS1_BLOCK_COUNT=24
    -DCOLLECTOR_WIPE_ON_BOOT
```

`upload`; cada reset mostra `BOOT wiped=1` com contagem 1. Remova a flag e regrave ao terminar.

**Passa se** `wiped=1`, `lost=0`, `log_max` inalterado, log só com o bloco do BOOT e `LOST 0` no `AT#LIST`. **Falha** se sobrar bloco antigo ou `lost`/`usage` herdarem valor.

---

## Caso 10: margem e clamp (`BLOCK_LOG_MARGIN_PCT`, `BLOCK_LOG_MAX_BLOCKS`)

**Objetivo.** Conferir a fórmula com outra margem, a precedência e o piso/teto. Cada subcaso é só ler `BOOT log_max`; não precisa encher o log.

| Sub | `[block_log] build_flags` | Env | `log_max` esperado |
|---|---|---|---|
| a | `-DBLOCK_LOG_MARGIN_PCT=50` | `collector_mock_debug_open` | **988** (hmac) |
| a2 | idem | `collector_mock_debug_none` / `_asym` | 1019 / 959 |
| b | `-DBLOCK_LOG_MARGIN_PCT=50` e `-DBLOCK_LOG_MAX_BLOCKS=200` | `_open` | **200** (MAX_BLOCKS vence) |
| c | `-DBLOCK_LOG_MAX_BLOCKS=50` | `_open` | **128** (piso, 2 segmentos) |
| d | `-DBLOCK_LOG_MAX_BLOCKS=100000` | `_open` | **16384** (teto, 256 segmentos) |
| e | `-DBLOCK_LOG_MARGIN_PCT=0` | `_open` | 1976 pela fórmula (`2097152/1061`) |

Para cada um: `upload`, ler o `BOOT`. Restaure o `[block_log]` ao fim.

**Obs. sub e.** Só confira o `log_max`; **não** espere enchê-lo: com margem 0 a rede de segurança por bytes (`livre < bytes + 8 KB`) pode descartar antes de N, pois os arquivos do LittleFS ocupam blocos de 4 KB (segmentos pequenos desperdiçam espaço). Também `-DBLOCK_LOG_MARGIN_PCT=100` deve falhar no build (`static_assert`).

**Passa se** todos os `log_max` batem. **Falha** se qualquer um divergir.

---

## Checklist final

| # | Caso | Resultado esperado resumido | OK |
|---|---|---|---|
| 1 | Testes nativos e builds | `test_log_limit` 6/6; suíte `native` e 4 builds sem erro | [ ] |
| 2 | N por perfil | `log_max` = 1778 (hmac), 1834 (none), 1726 (asym) | [ ] |
| 3 | Transbordo N=200 | 1º `SEG_DROP first=1 last=64 count=64` no 201º bloco; blocos em 136-200; `SYNC_OVERFLOW`; `LOST` soma | [ ] |
| 4 | Aviso de flash | `flags` com `0x04` em 161 blocos; apaga em ≤ 155; sem mudança entre 156-160 | [ ] |
| 5 | N 200 para 128 | `SEG_DROP` no boot, blocos ≤ 128, `lost` = soma, `log_max=128` | [ ] |
| 6 | N aumenta | sem `SEG_DROP`, `lost=0`, `log_oldest` igual | [ ] |
| 7 | Persistência | blocos = B + 1 após reboot; 1ª queda após `200 − B` blocos | [ ] |
| 8 | ACK libera | `SYNC_FREE segs>0`, blocos = segmento aberto, flag apaga | [ ] |
| 9 | Wipe | `wiped=1`, `lost=0`, contagem 1, `log_max` igual | [ ] |
| 10 | Margem e clamp | 988 / 200 / 128 / 16384 / 1976 conforme a tabela | [ ] |

## Limites deste roteiro

- Os números de tempo (6,6 s por bloco) vêm de um log TWAI, não do mock; confirme com `BLK_CLOSE age=`.
- A contagem de blocos é reconstruída pelo `contar_blocos.py`; não é um dado emitido pelo firmware.
- `AT#...` e o aviso de flash dependem de BLE; `sync_bench.py` precisa de `bleak`, `capture.py` de `pyserial`.
