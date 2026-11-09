# Roteiro de validação: adaptação do Tmax à latência da ECU

> **Como usar este arquivo [fora do TCC]**
> As seções com **[fora do TCC]** no título guardam comandos, arquivos de log e diagnóstico.
> Apague-as ao levar o texto para o TCC. Os trechos de log mostram só o nome do evento e os campos
> que interessam; `…` marca campos omitidos.
>
> **O que já foi executado e o que falta.** Os casos 1, 2 e 3 (parte) usam logs reais da bancada de
> 2 nov 2026 (casos J-01, J-07, J-09 e J-10 da Parte 4 do artefato). **A aplicação do Tmax
> (`AT ST`) nunca foi executada**: nenhum log tem `CONFIG reason=apply` com Tmax diferente de 50.
> Os blocos marcados com **[esperado: substituir pelo log real]** foram calculados a partir do
> código (`TmaxMeter.cpp`, `Utilization.cpp`, `CollectorConfig.h`) e precisam ser rodados antes de
> ir para o TCC.

## Objetivo

O **Tmax** é o tempo máximo que o dongle espera pela resposta de uma ECU a um pedido OBD-II. Ele
define duas coisas:

- quando uma leitura é dada como perdida (sem resposta até o Tmax);
- quanto do barramento o escalonador reserva para cada leitura. A ocupação planejada é
  U = Σ Tmax / Pᵢ, em que Pᵢ é o período de leitura de cada PID, e o firmware só aceita
  configurações com U ≤ 80%.

Uma ECU real pode responder em 1 ms ou em dezenas de ms, e o valor padrão (50 ms) pode sobrar ou
faltar. Este roteiro valida que o firmware:

1. mede a latência real da ECU e calcula um Tmax adequado para ela;
2. calcula valores de Tmax diferentes para latências diferentes;
3. recusa um Tmax que tornaria o escalonamento inviável;
4. passa a ler normalmente uma ECU que antes estourava o prazo, depois de aplicado o novo Tmax;
5. aproveita uma ECU rápida para ler os PIDs com mais frequência.

## Montagem

- ESP32 com o firmware coletor ligado ao barramento CAN (controlador TWAI interno e transceptor
  SN65HVD230), a 500 kbit/s.
- Simulador de ECU (ECUSim: Arduino Uno com controlador CAN MCP2515) no mesmo barramento, com
  terminação de 120 Ω nas duas pontas. O simulador atrasa cada resposta por um tempo configurável:
  um valor fixo mais um desvio aleatório (uniforme, normal ou exponencial), para todos os PIDs ou
  para um PID específico.
- PC com a serial do Uno (para configurar a latência) e a serial do ESP32 (para capturar os eventos).
- Celular pareado ao dongle por BLE, para enviar os comandos do console (`AT#TMAX`, `AT ST`, `AT#PID`).

O dongle lê 7 PIDs com períodos Pᵢ = kᵢ · Tmax. Com o Tmax padrão de 50 ms:

| PID | Grandeza | kᵢ | Pᵢ com Tmax = 50 |
|---|---|---|---|
| `0C`, `0D`, `49` | rotação, velocidade, pedal | 5 | 250 ms |
| `04`, `10` | carga, MAF | 12 | 600 ms |
| `05` | temperatura do motor | 100 | 5 000 ms |
| `2F` | combustível | 200 | 10 000 ms |

U = 3/5 + 2/12 + 1/100 + 1/200 = 0,782, que aparece no log como `u=782`.

Eventos usados:

| Evento | Significado |
|---|---|
| `CONFIG … tmax u` | Configuração em vigor (no boot ou depois de uma mudança, `reason=apply`). |
| `READ pid ok dur` | Uma leitura: `ok=0` é sem resposta; `dur` é o tempo até a resposta, em ms. |
| `CAN_STATUS` | Emitido depois de cada leitura sem resposta, com o estado do barramento. |
| `TMAX_START`, `TMAX_SAMPLE`, `TMAX_DONE tmax` | Medição do Tmax: início, cada amostra e o valor calculado. |

## Como o firmware calcula o Tmax

O comando `AT#TMAX` faz 15 pedidos a cada PID, intercalados, e mede a latência de cada um. Nessa
medição o dongle espera até 1 s por resposta, e não o Tmax em vigor: assim consegue medir uma ECU
mais lenta que o Tmax atual. Depois:

1. toma o p99 das amostras de cada PID (com 15 amostras, o maior valor);
2. pega o pior PID e multiplica por 1,3 (margem de 30%);
3. arredonda para cima ao múltiplo de 4 ms (o comando `AT ST hh` do ELM327 usa passos de 4 ms);
4. aplica o piso de 20 ms.

O resultado é só uma sugestão. Ele passa a valer com o comando `AT ST hh` (Tmax = hh × 4 ms), que o
firmware só aceita se a nova ocupação U ficar em até 80%. O valor aceito fica gravado na NVS.

---

## Caso 1: linha de base

**Procedimento.** Simulador sem latência, captura de 3 minutos com a configuração padrão.

**Resultado.**

```
CONFIG reason=boot proto=6 tmax=50 … u=782 pids=0C,0D,49,04,10,05,2F
0C  n=1534  min=1  p50=1  p99=1  max=2  média=1.01  dp=0.09  sem resposta=0/1534
CAN_STATUS emitidos: 0
```

A ECU simulada responde em ~1 ms, muito abaixo do Tmax de 50 ms.

## Caso 2: exemplo de latência vista pelo dongle

**Procedimento.** No simulador foi configurada a latência de 20 ms mais um desvio normal de média
10 ms e desvio padrão 3 ms (`LAT.ALL=20:N:10:3`), para todos os PIDs. Captura de 5 minutos.

**Resultado.** Tempo de resposta (`dur`) das leituras de rotação:

```
0C  n=1857  min=20  p50=31  p95=36  p99=39  max=42  média=30.61  dp=3.17  sem resposta=0/1857
```

O esperado era média de 30,5 ms (20 + 10 + ~0,5 ms de medição) e desvio de ~3 ms. O dongle enxerga
a latência configurada no simulador, e é essa latência que o Tmax precisa cobrir.

## Caso 3: Tmax calculado para latências diferentes

**Procedimento.** Para cada latência configurada no simulador, foi enviado `AT#TMAX` pelo console
do dongle e lido o `TMAX_DONE`.

**Resultado.**

```
Sem latência
TMAX_START pids=0C,0D,49,04,10,05,2F samples=15
TMAX_DONE tmax=20

20 ms + normal(10, 3)
TMAX_START pids=0C,0D,49,04,10,05,2F samples=15
TMAX_DONE tmax=52

Fixa 55 ms                                   [esperado: substituir pelo log real]
TMAX_DONE tmax=76

Fixa 110 ms                                  [esperado: substituir pelo log real]
TMAX_DONE tmax=148
```

| Latência no simulador | Pior amostra | × 1,3 | Múltiplo de 4 / piso | Tmax |
|---|---|---|---|---|
| nenhuma | 1 ms | 1,3 | piso de 20 | **20** |
| 20 + normal(10, 3) | 38 ms | 49,4 | 52 | **52** |
| fixa 55 ms | 56 ms | 72,8 | 76 | **76** |
| fixa 110 ms | 111 ms | 144,3 | 148 | **148** |

O Tmax acompanha a latência da ECU. Com a latência aleatória, ele depende do pior caso das 15
amostras e por isso varia um pouco de uma medição para outra. Nos dois últimos casos a ECU está
acima do Tmax em vigor (50 ms) e mesmo assim é medida, porque a medição espera até 1 s.

## Caso 4: adaptação a uma ECU lenta

**Procedimento.**

1. Simulador com latência fixa de 55 ms, dongle com o Tmax padrão de 50 ms.
2. `AT#TMAX` pelo console: Tmax sugerido de 76 ms.
3. Tentativa de aplicar direto: `AT ST 13` (0x13 = 19, 19 × 4 = 76 ms).
4. Ajuste dos períodos para Pᵢ = kᵢ · 76 com `AT#PID` (0C, 0D e 49 em 380 ms; 04 e 10 em 912 ms;
   05 em 7 600 ms; 2F em 15 200 ms) e nova tentativa de `AT ST 13`.
5. Captura de 3 minutos.

**Resultado antes da adaptação** (Tmax = 50):

```
READ pid=0C ok=0 val=- … dur=56
CAN_STATUS state=running tec=0 rec=0 … bus_err=0
3940 leituras, 3940 sem resposta
```

Todas as leituras estouram o prazo. O barramento está saudável (`tec=0 rec=0`): é a ECU que é lenta.

**Resultado da adaptação** [esperado: substituir pelo log real]:

```
CMD txt="AT ST 13"      RESP txt="?\r>"                    (recusado: U = 76 × 0,01563 = 119%)
CMD txt="AT#PID 0C 380" … CONFIG reason=apply tmax=50 …    (um por PID)
CMD txt="AT ST 13"      RESP txt="OK\r>"
CONFIG reason=apply proto=6 tmax=76 … u=782 pids=0C,0D,49,04,10,05,2F
0C  n≈470  min=55  média≈55.5  max=56  sem resposta=0
CAN_STATUS emitidos: 0
```

Com os períodos padrão, um Tmax de 76 ms ocuparia 119% do barramento, e o firmware recusa a mudança.
Com os períodos ajustados na mesma proporção, a ocupação volta a 78,2% e o Tmax é aceito. A partir
daí, todas as leituras voltam a ter resposta. O custo é ler cada PID com menos frequência (rotação a
cada 380 ms em vez de 250 ms), que é o máximo que uma ECU com essa latência permite.

## Caso 5: adaptação a uma ECU rápida

**Procedimento.** A latência do simulador foi zerada (`LAT.CLEAR=ALL`). Depois: `AT#TMAX`,
`AT ST 05` (5 × 4 = 20 ms) e períodos Pᵢ = kᵢ · 20 (0C, 0D e 49 em 100 ms; 04 e 10 em 240 ms; 05 em
2 000 ms; 2F em 4 000 ms). Por fim, a placa foi reiniciada.

**Resultado** [esperado: substituir pelo log real]:

```
TMAX_DONE tmax=20
CONFIG reason=apply proto=6 tmax=20 … u=782 pids=0C,0D,49,04,10,05,2F
0C  ~10 leituras/s (antes: 4 com Tmax 50; 2,6 com Tmax 76)    sem resposta=0
… reinício …
CONFIG reason=boot proto=6 tmax=20 … u=782                    (mantido na NVS)
```

Com a ECU rápida, o Tmax cai para o piso de 20 ms e os PIDs principais passam a ser lidos 2,5 vezes
mais rápido que na configuração padrão, com a mesma ocupação do barramento. A configuração sobrevive
ao reinício.

## Limitações

- O firmware mede e sugere o Tmax, mas não o aplica sozinho: a aplicação (`AT ST`) e o ajuste dos
  períodos (`AT#PID`) são comandos separados.
- Com a ECU acima do Tmax em vigor, o prazo de espera se estende quando chegam respostas atrasadas
  de pedidos anteriores (timeouts de até ~100 ms com a ECU a 110 ms), e uma resposta atrasada do
  mesmo PID pode ser aceita como resposta do pedido novo: o OBD-II não identifica a qual pedido a
  resposta pertence. As duas situações deixam de ocorrer depois que o Tmax é adaptado.
- O Tmax calculado depende das 15 amostras por PID; com latência aleatória, uma cauda rara (como na
  distribuição exponencial) pode não aparecer na amostra.

---

## Detalhes de execução [fora do TCC]

### Origem dos números

| Caso | Fonte | Observação |
|---|---|---|
| 1 | `esp32-firmware/logs/lat-j01.log`, cortado em t < 200 s | depois de 200 s a latência já tinha sido mudada |
| 2 | `lat-j07.log`, cortado em t < 310 s | idem, no fim do log |
| 3, sem latência | `lat-j10.log` | real |
| 3, normal(10, 3) | artefato, J-10 (pior amostra 38, `tmax=52`) | o log dessa rodada não foi guardado |
| 3, 55 e 110 ms | calculado | **a executar** |
| 4, antes | `lat-j09-55.log` (log inteiro) | real |
| 4, adaptação | calculado | **a executar** |
| 5 | calculado | **a executar** |

Corte: `awk -F'\t' '$1<200' esp32-firmware/logs/lat-j01.log > j01.log`, depois
`python simulador/analisar_latencia.py j01.log --desde 5`.

### Contas usadas nos valores esperados

- Σ 1/Pᵢ com os períodos padrão = 3/250 + 2/600 + 1/5000 + 1/10000 = 0,015633 por ms de Tmax.
  Maior Tmax aceito sem mexer nos períodos: 0,8 / 0,015633 = 51,2 → **48 ms** (múltiplo de 4).
  Ou seja, nem o 52 do caso 3 é aceito direto: precisa ajustar os períodos antes.
- Com Pᵢ = kᵢ · Tmax, U = Σ 1/kᵢ = 0,782 para qualquer Tmax (por isso `u=782` nos casos 4 e 5).
- Período mínimo aceito: 100 ms (`kMinPeriodMs`), o que limita k · Tmax nos PIDs rápidos a Tmax ≥ 20.
- Ordem dos comandos: subindo o Tmax, ajuste os períodos **antes** do `AT ST` (cada `AT#PID` só reduz
  U). Descendo, faça o `AT ST` **antes** e reduza os períodos depois.
- `AT ST hh` é hexadecimal: 76 ms → `13`; 52 ms → `0D`; 20 ms → `05`; 148 ms → `25`.
- Para 148 ms, os períodos seriam 740 / 1 776 / 14 800 / 29 600 ms.

### Comandos

```bash
cd ~/Projects/tccelta/tccelta && git switch feat/simulador-latencia-variavel
conda activate tccelta && export PYTHONUTF8=1
cd esp32-firmware && pio run -e collector_twai_debug -t upload && cd ..
cd simulador && pio run -t upload --upload-port COMu && cd ..
python simulador/send_payload.py --port COMu          # sessão fica aberta até o fim
# dentro da sessão:
RPM=3000;SPEED=80;PEDAL_D=40;ENGINE_LOAD=50;MAF=10;COOLANT_TEMP=90;FUEL_LEVEL=60
LAT.ALL=55                                            # ou LAT.ALL=20:N:10:3, LAT.ALL=110, LAT.CLEAR=ALL

# captura e análise
python esp32-firmware/tools/event-bench/capture.py --port COMd --out logs/tmax-55.log
python simulador/analisar_latencia.py logs/tmax-55.log --desde 5
grep -E ' (CMD|RESP|CONFIG|TMAX_START|TMAX_DONE) ' logs/tmax-55.log
```

Console do dongle: nRF Connect no celular pareado (o `collector_twai_debug` exige LESC), NUS RX
`6E400002-B5A3-F393-E0A9-E50E24DCCA9E`, cada linha terminada em CR. Sequência do caso 4:

```
AT#TMAX
AT#TMAX?            → uma linha por PID ("0C P99 56 TO 0") e "TMAX 76"
AT ST 13            → ?
AT#PID 0C 380
AT#PID 0D 380
AT#PID 49 380
AT#PID 04 912
AT#PID 10 912
AT#PID 05 7600
AT#PID 2F 15200
AT ST 13            → OK
AT#LIST             → confere "TMAX 76 U 0.78"
```

Caso 5: `LAT.CLEAR=ALL` no simulador; no console `AT#TMAX`, `AT ST 05`, depois `AT#PID 0C 100`,
`0D 100`, `49 100`, `04 240`, `10 240`, `05 2000`, `2F 4000`; reinicie pelo botão EN com a captura
aberta. Para contar a taxa de leitura: `grep -c ' READ pid=0C ' logs/…` dividido pela duração.

**Ao terminar**, não há comando que volte exatamente ao padrão: 50 ms não é múltiplo de 4, então o
`AT ST` não chega nele, e o `ATD` não mexe na configuração da coleta. Duas saídas:

- ficar perto do padrão: `AT ST 0C` (48 ms) e os períodos padrão de volta (`AT#PID 0C 250`, `0D 250`,
  `49 250`, `04 600`, `10 600`, `05 5000`, `2F 10000`; U = 0,750). Se o Tmax estiver acima de 48,
  faça o `AT ST` antes dos `AT#PID`;
- voltar ao padrão exato: `pio run -e collector_twai_debug -t erase` e regravar. Isso apaga também o
  bond BLE e o log de blocos (pareie de novo depois).

Confirme o que vale com `AT#LIST` (`TMAX … U …`).

### Cuidados

- Abrir a serial do Uno reinicia o ECUSim e apaga a latência: configure com a sessão do
  `send_payload.py` aberta e mantenha-a aberta durante a captura.
- Pare a captura antes de trocar a latência; senão a rodada seguinte entra no mesmo arquivo.
- A configuração (Tmax e períodos) fica na NVS e sobrevive a reinício e a upload do firmware.
- Pendências conhecidas do dongle com ECU acima do Tmax: `docs/followups-latencia-ecusim.md`.
