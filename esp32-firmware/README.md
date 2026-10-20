# ESP32 OBD2 Dongle Firmware

Firmware do dongle OBD-II do TCCELTA para ESP32 (PlatformIO + FreeRTOS). O
dongle coleta telemetria do barramento CAN do carro (modo Coletor), grava em
blocos na flash e envia ao servidor pelo app. Pelo mesmo canal ELM327, o app
lê os valores coletados e, no modo Diagnóstico (`AT#DIAG 1`), faz leituras
arbitrárias (DTCs, freeze frame, outros PIDs).

Desenho completo: [docs/plano-coleta.md](../docs/plano-coleta.md).
Eventos de depuração: [docs/debug-events.md](../docs/debug-events.md).

## Ambientes

A camada CAN é escolhida pelo ambiente:

| Ambiente | Camada CAN | Eventos de depuração |
|---|---|---|
| `collector_mock` | Simulada (sem hardware CAN) | — |
| `collector_twai` (padrão) | TWAI nativo do ESP32 + SN65HVD230 | — |
| `collector_mcp2515` | MCP2515 via SPI | — |
| `collector_mock_debug` | Simulada | Serial a 921600 baud |
| `collector_twai_debug` | TWAI | Serial a 921600 baud |
| `collector_mcp2515_debug` | MCP2515 | Serial a 921600 baud |

```bash
pio run -e collector_mock_debug -t upload -t monitor   # gravar e acompanhar
pio test -e native                                     # testes no host
```

Para salvar os eventos num arquivo (para o visualizador):
`pio device monitor -e collector_mock_debug --filter log2file`.

## Configuração local

Porta serial e flags de teste ficam fora do git, em `platformio_local.ini`:

```bash
cp platformio_local.ini.example platformio_local.ini
```

- `upload_port` / `monitor_port`: porta do ESP32 nesta máquina (`COM9`,
  `/dev/ttyUSB0`, ...).
- `build_flags = -DCOLLECTOR_WIPE_ON_BOOT`: teste "de fábrica" — a cada boot,
  apaga os blocos da flash e volta o `seq` para 1 (`boot_id` e a configuração
  são mantidos). Não deixe ligado: impede testar a recuperação após reinício.

## Comandos

Além do subconjunto ELM327 (`ATZ`, `ATE0`, `ATH1`, `ATST hh`, `ATSP h`, `01 0C`,
...), o dongle aceita os comandos do projeto:

| Comando | Função |
|---|---|
| `AT#PID pp mmmm` | Adiciona/altera o PID `pp` com período `mmmm` ms |
| `AT#DEL pp` | Remove o PID |
| `AT#LIST` | PIDs, períodos, Tmax, utilização e contadores |
| `AT#DIAG b` | Liga (1) / desliga (0) o modo diagnóstico |
| `AT#NET b` | O app informa se tem Internet |
| `AT#GPS lat lon alt` | Posição atual (graus × 10⁷, mm); no máximo 1/s, ignorada no diagnóstico |
| `AT#TMAX` / `AT#TMAX?` | Mede o Tmax / consulta o resultado |
| `AT#ACK n` | Só nas builds de depuração: simula o ACK do servidor até o bloco `n` |

`ATSP h` (6 a 9) grava o protocolo e reinicia o dongle. O simulador de ECU
(`simulador/`) usa CAN 11 bits a 250 kbps: envie `AT SP 8` uma vez.

## Hardware

| GPIO | Função |
|---|---|
| 21 | TWAI TX |
| 22 | TWAI RX |
| 18 / 19 / 23 | MCP2515 SCK / MISO / MOSI |
| 5 | MCP2515 CS |

Transceiver: SN65HVD230 (TWAI) ou MCP2551 (MCP2515).

## Organização

```
src/
  collector/     núcleo da coleta em C++ puro (testado no host)
  rtos/          adaptadores FreeRTOS, controle, log de eventos
  tasks/         Aquisição, Flush, Upload, Sessão, ELM327
  storage/       LittleFS (blocos) e NVS (config, boot_id, seq)
  can/, obd2/    CAN (TWAI, MCP2515), ISO-TP e OBD-II
  elm327/        interpretador de comandos
  connectivity/  BLE (canal de comandos) e o contrato do canal de dados
  app/           montagem e inicialização (CollectorApp)
```
