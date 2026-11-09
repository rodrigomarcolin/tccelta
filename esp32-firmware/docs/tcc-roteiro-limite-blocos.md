# Roteiro de validação: limite de blocos no log da flash

> **Como usar este arquivo [fora do TCC]**
> As seções com **[fora do TCC]** no título guardam comandos, arquivos de log e diagnóstico.
> Apague-as ao levar o texto para o TCC. O resto (objetivo, procedimento e resultado de cada
> caso) já está escrito para o leitor do TCC. Os trechos de log mostram só o nome do evento e os
> campos que interessam; `…` marca campos omitidos.
> Versão completa dos passos: `esp32-firmware/docs/roteiro-teste-limite-blocos.md` e a Parte 3 do
> artefato de estado do projeto (casos L-00 a L-15).

## Objetivo

O dongle guarda os blocos de telemetria num log circular na flash (partição LittleFS de 2 MB)
até o servidor confirmar o recebimento (ACK). Se o celular ficar muito tempo sem sincronizar, o
log enche. Este roteiro valida que:

1. o log nunca passa de N blocos, com N calculado pelo tamanho da partição e pelo perfil de segurança;
2. ao atingir N, o dongle apaga o segmento mais antigo e declara a perda;
3. o dongle avisa o app quando a flash passa de 80% de N;
4. a contagem sobrevive a reinícios e a mudanças de N entre gravações do firmware;
5. o ACK do servidor e o comando de apagar (wipe) liberam espaço;
6. de ponta a ponta, o servidor registra os blocos descartados como uma lacuna do tipo `OVERFLOW`.

## Montagem

- ESP32 com o firmware coletor no modo simulado (gera leituras OBD-II sem carro), perfil de segurança `hmac`.
- Placa provisionada como um dongle real: identificador (`DevId`) e chaves sorteados no PC, gravados na
  partição protegida, e as mesmas chaves cadastradas no KMS do servidor.
- Nos casos 1 a 5, só o ESP32 ligado ao PC (serial para os eventos, BLE para os comandos `AT#`).
- No caso 6, o app no celular e o servidor rodando localmente (Docker), como em produção.

O dongle escreve um evento por linha na serial. Os eventos usados aqui:

| Evento | Significado |
|---|---|
| `BOOT … lost=… log_max=N` | Fim da inicialização: blocos perdidos neste boot e o limite N em vigor. |
| `SEG_DROP first last count` | O segmento mais antigo (blocos `first..last`) foi apagado. |
| `SYNC_OVERFLOW upto ack` | O descarte atingiu blocos ainda sem ACK: houve perda real. |
| `SYNC_NOTICE … flags reason` | Aviso ao app; o bit `0x04` de `flags` é "flash alta". |
| `SYNC_FREE upto segs oldest` | O ACK chegou até `upto` e `segs` segmentos foram apagados. |
| `HS_RESPONSE tail head` | Resposta do dongle no handshake: bloco mais antigo (`tail`) e mais novo (`head`) do log. |

O firmware não informa quantos blocos há no log. Um script do projeto reconstrói essa contagem a
partir dos eventos acima (`contar_blocos.py`), e é ele que aparece nos resultados como `blocos=`.

## Como N é calculado

O log é gravado em segmentos de até 64 blocos. N reserva 10% da partição e considera o maior
registro possível (1024 B de dados + tag de autenticação + 4 B de CRC + 1 B de cabeçalho de segmento):

| Perfil | Tag | Registro máximo | N = (2 097 152 − 10%) / registro |
|---|---|---|---|
| `none` | 0 B | 1029 B | **1834** |
| `hmac` | 32 B | 1061 B | **1778** |
| `asym` | 64 B | 1093 B | **1726** |

Encher 1778 blocos leva cerca de 3 horas no ritmo normal. Por isso os casos 2 a 6 usam um
firmware de teste com N fixado em **200**.

---

## Caso 1: N calculado por perfil

**Procedimento.** O firmware foi gravado três vezes, uma por perfil (`none`, `hmac`, `asym`), sem
apagar a flash, e o evento `BOOT` de cada inicialização foi lido. Depois, no perfil `hmac`, a margem
foi trocada de 10% para 50% na compilação.

**Resultado.**

```
KEYS profile=hmac … tag_len=32    BOOT … log_max=1778
KEYS profile=none … tag_len=0     BOOT … log_max=1834
KEYS profile=asym … tag_len=64    BOOT … log_max=1726
KEYS profile=hmac … tag_len=32    BOOT … log_max=988     (margem de 50%)
```

Os quatro valores batem com a fórmula, o que mostra que o tamanho real da partição e a margem
chegam ao firmware.

## Caso 2: transbordo com N = 200

**Procedimento.** Com N = 200 e o log recém-apagado, o dongle ficou gravando sem nenhum celular
conectado, ou seja, sem ACK. A contagem foi acompanhada até passar do limite.

**Resultado.** O log enche até 200 blocos. Ao gravar o 201º, o dongle apaga o segmento mais antigo
(blocos 1 a 64) e a contagem cai para 136. Como esses blocos não tinham sido confirmados, ele
declara a perda logo em seguida:

```
FLUSH#200   blocos=200  N=200  segs=[(1,64), (65,64), (129,64), (193,8)]
SEG_DROP first=1 last=64 count=64
SEG_DROP    blocos=136  N=200  segs=[(65,64), (129,64), (193,8)]
SYNC_OVERFLOW upto=64 ack=31
```

A contagem nunca passou de 200 e, depois da primeira queda, ficou entre 136 e 200. O `upto` do
`SYNC_OVERFLOW` é o `last` do segmento apagado.

## Caso 3: aviso de flash alta

**Procedimento.** Com N = 200, uma central BLE se inscreveu no canal de avisos do dongle, sem
confirmar nenhum bloco, e o log foi enchendo.

**Resultado.** O aviso liga quando a ocupação passa de 80% (161 blocos = 805‰) e só desliga abaixo
de 780‰ (155 blocos). A diferença entre os dois limites evita que o aviso fique ligando e
desligando em torno de 80%:

```
blocos=160  usage=800    (sem aviso: 800 não é maior que 800)
blocos=161  usage=805    SYNC_NOTICE … flags=05 reason=flash    (bit 0x04 ligado)
SEG_DROP first=1 last=64 count=64
blocos=137  usage=685    SYNC_NOTICE … flags=01 reason=flash    (bit 0x04 desligado)
```

## Caso 4: contagem preservada entre reinícios e mudanças de N

**Procedimento.**

- (a) Com cerca de 150 blocos no log, a placa foi reiniciada pelo botão EN.
- (b) Com o log perto de 200 blocos, o firmware foi regravado com N = 128, sem apagar a flash.
- (c) Em seguida, regravado com N = 400.

**Resultado.**

```
(a) BOOT … lost=0 log_max=200          SEG_NEW … reason=boot
    → a contagem continua de 151, e a próxima queda vem exatamente no bloco 201
(b) SEG_DROP first=1 last=64 count=64
    SEG_DROP first=65 last=128 count=64
    BOOT … lost=128 log_max=128        → restam 72 blocos (≤ 128)
(c) BOOT … lost=0 log_max=400          → nenhum segmento apagado
```

O reinício não zera a contagem: se zerasse, a queda viria atrasada. Reduzir N apaga só o
necessário para caber no novo limite, e o `lost` do `BOOT` soma o que foi apagado. Aumentar N não
apaga nada.

## Caso 5: ACK e wipe liberam espaço

**Procedimento.** (a) Com o log cheio de blocos não confirmados, o app sincronizou e o servidor
confirmou os blocos. (b) Depois foi enviado o comando de apagar o log (`AT#WIPE`).

**Resultado.**

```
(a) HS_ACK ack=128 … result=ok advanced=1
    SYNC_FREE upto=128 segs=1 oldest=129
    blocos=142 → 78       (o segmento 65–128 foi apagado)

(b) KEYS profile=hmac source=partition dev_id=AA4F51C185A8348E …
    BOOT boot=51 wiped=1 … log_oldest=1 log_newest=1 lost=0 log_max=200
    KEYS profile=hmac source=partition dev_id=AA4F51C185A8348E …   (mesmo DevId)
```

O ACK apaga só segmentos inteiros já confirmados. O wipe esvazia o log (sobra apenas o bloco de
boot) e não toca nas chaves: o `DevId` continua o mesmo.

## Caso 6: ponta a ponta, perda declarada ao servidor

**Procedimento.**

1. Firmware com N = 200 e log apagado; app pareado e sincronizando com o servidor até zerar o atraso.
   O último bloco confirmado (A) foi o 31.
2. Bluetooth do celular desligado. O dongle continuou gravando sem ACK até descartar blocos.
3. Bluetooth religado. O app refez o handshake com o servidor e voltou a sincronizar.
4. Consulta ao banco do servidor.

**Resultado no dongle.**

```
HS_ACK ack=31 … result=ok advanced=1                      (último ACK antes de desligar)
SEG_DROP first=1 last=64 count=64
SYNC_OVERFLOW upto=64 ack=31                              (32 a 64 perdidos sem ACK)
HS_CHALLENGE … ack=31 … result=ok                         (reconexão)
HS_RESPONSE … tail=65 head=205 …                          (o mais antigo agora é o 65)
HS_ACK ack=64 … result=ok advanced=1 opened=1
SYNC_FREE upto=192 segs=1 oldest=193                      (sincronização retomada)
```

**Resultado no servidor.**

| Consulta | Valor |
|---|---|
| `gap` (cause = `OVERFLOW`) | `from_seq = 32`, `to_seq = 64` |
| `raw_block` entre 32 e 64 | 0 linhas |
| `dongle_state.contiguous_seq` | passou de 64 e voltou a acompanhar o `head_seq` |
| `rejected_block` | 0 linhas |

O servidor usou o `tail` do handshake (65) para registrar a faixa 32–64 como perdida por
transbordo, em vez de ficar esperando por ela, e voltou a aceitar os blocos seguintes. O handshake
ter aberto também prova que as chaves cadastradas no KMS são as mesmas gravadas na placa.

---

## Detalhes de execução [fora do TCC]

### Origem dos trechos de log

- Casos 2, 5 e 6: trechos reais de `esp32-firmware/logs/limite-l15.log` (2 nov 2026, DevId
  `AA4F51C185A8348E`, boot 51). A contagem `blocos=` é a saída do
  `python tools/event-bench/contar_blocos.py` sobre esse log a partir do wipe (linha 2310 em diante).
- Casos 1, 3 e 4: os logs do L-06 a L-14 não foram guardados. Os valores (`log_max`, limiares do
  aviso, quedas) são os validados em bancada e registrados no artefato; as linhas seguem o formato
  real dos eventos, mas `head`, `flags=01` etc. são ilustrativos. Se for refazer, troque pelos
  trechos reais.
- Caso 6, servidor: a tabela foi derivada do log (A = 31 pelo `SYNC_OVERFLOW ack=31`, T = 65 pelo
  `HS_RESPONSE tail=65`). Se tiver a saída do `psql` daquele dia, cole no lugar.

### Mapa caso do TCC → caso de bancada

| Caso do TCC | Casos de bancada |
|---|---|
| 1 | L-05 (testes nativos), L-06, L-14 |
| 2 | L-07 |
| 3 | L-08 |
| 4 | L-09, L-10, L-11 |
| 5 | L-12, L-13 |
| 6 | L-01 a L-03 (provisionamento) e L-15 |

### Preparação

```bash
cd ~/Projects/tccelta/tccelta && git switch feat/firmware-limite-blocos && cd esp32-firmware
export PYTHONUTF8=1
conda activate tccelta
```

`platformio_local.ini` (fora do git), para N = 200:

```ini
[block_log]
build_flags =
    -DBLOCK_LOG_MAX_BLOCKS=200
```

Outras variações: `-DBLOCK_LOG_MAX_BLOCKS=128` / `=400` (caso 4), `-DBLOCK_LOG_MARGIN_PCT=50`
(caso 1). Tire a seção `[block_log]` ao terminar.

Provisionamento hmac (L-01 a L-03): exportar a wrapping key do KMS (`kms-ferramentas provision
export-wrapping`), `pio run -e collector_mock_debug_open -t erase`, depois
`python tools/provision/provision.py --profile dev --security hmac --kms-wrapping ../../../backend/saida/kms_wrapping.json --flash --port COMx`,
depois `-t upload`. No backend: `provision import-record` e `provision register`. Comandos completos
na Parte 3 do artefato.

### Comandos por caso

```bash
# captura (uma janela) e contagem (outra)
python tools/event-bench/capture.py --port COMx --out logs/limite-l07.log
python tools/event-bench/contar_blocos.py logs/limite-l07.log | tail -n 5

# comandos AT# pelo NUS (só no env collector_mock_debug_open, sem pareamento)
python tools/sync-bench/nus_at.py 'AT#WIPE'
python tools/sync-bench/nus_at.py 'AT#SYNC?'      # SYNC conn=.. head=.. flags=..
python tools/sync-bench/nus_at.py 'AT#ACK H'      # H entre ack e head
python tools/sync-bench/nus_at.py 'AT#LIST'       # última linha: LOST n
python tools/sync-bench/nus_at.py 'AT#FLUSH 1000' # acelera: 1 bloco/s (volte com 'AT#FLUSH 10000')

# caso 3: central inscrita no SYNC_TX sem ACK
python tools/sync-bench/sync_bench.py --scenario stream --tlat 99999 --duration 1800 --keys tools/provision/out/$DEV/bench_keys.json
```

Caso 6 (env com LESC, `collector_mock_debug`): pareamento e `AT#WIPE` pelo nRF Connect (NUS RX
`6E400002-…`, texto com `\r`), app da Montagem C. Consultas no `make psql`:

```sql
SELECT from_seq, to_seq, cause, c_s FROM gap
 WHERE dongle_id = (SELECT dongle_id FROM dongle WHERE dev_id_hex = lower('<DevId>')) AND cause = 'OVERFLOW';
SELECT e.contiguous_seq, e.head_seq FROM dongle_state e
 WHERE e.dongle_id = (SELECT dongle_id FROM dongle WHERE dev_id_hex = lower('<DevId>'));
SELECT count(*) FROM raw_block WHERE dongle_id = (SELECT dongle_id FROM dongle WHERE dev_id_hex = lower('<DevId>')) AND seq BETWEEN 32 AND 64;
SELECT count(*) FROM rejected_block WHERE dongle_id = (SELECT dongle_id FROM dongle WHERE dev_id_hex = lower('<DevId>'));
```

### Problemas comuns

- `contar_blocos.py` imprime `DIVERGE`: a captura não começou logo depois de um wipe.
- `source=dev_fixed` no `KEYS`: a `secnvs` não foi gravada; refaça erase → provision → upload, nessa ordem.
- `HS_ACK result=bad_seq` no `AT#ACK`: o número precisa estar entre `ack` e `head`.
- Caso 6 com `bad_tag` ou 422 no primeiro handshake: o `kms_record.json` importado é de outra
  execução do `provision.py`.
- Caso 6 com lacuna começando depois de A + 1: o app ainda tinha blocos lidos e não enviados quando
  o Bluetooth caiu; espere o atraso zerar antes de desligar.
- Ao terminar: apague `tools/provision/out/<DevId>/` (chaves em claro).
