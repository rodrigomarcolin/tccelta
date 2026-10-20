# Connectivity

## Canal de dados da coleta (a implementar)

O canal de comandos ELM327 (NUS, `BleConnectivity`) já existe e cobre todos os
`AT#`. Falta o canal **binário** de dados, por onde vão os blocos para o
servidor e a sincronização de hora. A coleta já está pronta para ele; o
contrato é este:

| Sentido | O que fazer | Onde |
|---|---|---|
| App → Dongle | Remontar os quadros (`collector::FrameDecoder`) e pôr cada mensagem em `rtos::DataMessage`: `ACK` (tipo `0x02`) → `ctx.uploadRx`; `TIME_RESP` (`0x04`) → `ctx.sessionRx`. Não verificar nada: as tarefas verificam. | `rtos/CollectorContext.h` |
| Dongle → App | Implementar `IDataChannel::send()`: envolver em quadro (`collector::frameMessage`) e fragmentar pelo MTU. | `IDataChannel.h` |
| Conexão | Ligar `APP_CONNECTED` ao conectar; desligar `APP_CONNECTED`, `INTERNET` e `SESSION_OK` ao desconectar. | `ctx.events` |

O formato das mensagens está em `collector/DataProtocol.h`. Enquanto o canal
não existe, `SerialDataChannel` imprime as mensagens na serial.
