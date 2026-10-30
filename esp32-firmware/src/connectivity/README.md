# Connectivity

Dois serviços GATT no mesmo servidor NimBLE (`ble/BleConnectivity`):

| Serviço | UUID | Uso |
|---|---|---|
| NUS | `6E400001-B5A3-F393-E0A9-E50E24DCCA9E` | Comandos de texto ELM327 e `AT#` (RX `...0002` escrita, TX `...0003` notificação). É o único anunciado. |
| SYNC | `7A3C0001-9B1E-4F2A-8C5D-2E6F1A0B3C4D` | Protocolo binário de segurança e telemetria: RX `7A3C0002-...` (`WRITE`, app para dongle, um TLV por escrita de até 64 B) e TX `7A3C0003-...` (`NOTIFY`, dongle para app). O app o descobre na conexão. |

## Formato do SYNC

- TLV `T (1) | L (2, LE) | V`. Tipos: `0x01` desafio, `0x02` resposta, `0x03` bloco,
  `0x04` ACK, `0x05` pedido de lote, `0x06` erro, `0x07` aviso de novo bloco.
  Layouts byte a byte em `sync/SyncProtocol.h` (spec `fase-1/04`).
- No SYNC_TX cada notificação é `C | pedaço do TLV`; `C` tem o bit 7 ligado no
  primeiro fragmento e o contador (módulo 128) nos bits 6..0. O pedaço tem
  `MTU - 4` bytes (`sync/Fragmenter.h`).
- `BleSyncLink` controla o fluxo por créditos de notificação (janela derivada do MTU)
  e só enfileira em `syncRx` o que chega: nenhum trabalho no callback do NimBLE.
  A `SyncTask` consome a fila e roda o `SyncEngine` (handshake, cursor, lotes).
- A cada conexão `ctx.connections` incrementa (geração); `APP_CONNECTED` liga ao
  conectar e `APP_CONNECTED`/`INTERNET` desligam ao desconectar.

Eventos de depuração do SYNC e do handshake: `docs/debug-events.md` (§5.7).

## Segurança de enlace (Fase 2)

- O dongle só aceita enlaces BLE LESC Just Works com bond, e **só entrega dados do NUS e do
  SYNC à conexão autorizada** (cifrada + bond LESC válido). Escritas de quem não está
  autorizado são descartadas (`WRITE_DROP`); notificações vão sempre para a conexão autorizada.
- `APP_CONNECTED`/`CONN up=1` significam "sessão autorizada", não "conexão crua".
- A decisão é de `pairing::PairingPolicy` (`pairing/`), ligada à NimBLE por
  `PairingController` e `BleConnectivity` (`ILinkControl`); `BleSecurity` guarda as chamadas
  à pilha. O botão BOOT (5 s) abre a janela de 45 s (`[pairing]` no `platformio.ini`).
- Eventos: `docs/debug-events.md` §5.9.
