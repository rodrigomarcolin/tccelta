# infra/ble

Adapter BLE sobre `flutter_blue_plus` (única camada que importa a lib).

## Fluxo de conexão (LESC Just Works)

1. `connecting`: `connect` ao dongle.
2. `securing`: o enlace precisa ser cifrado com um bond LESC.
   - Android: se o SO ainda não tem bond, o adapter chama `createBond` logo após
     conectar; com bond, o *Security Request* do dongle dispara a cifra.
   - iOS: não há API; o alerta do sistema aparece na primeira operação que exige
     cifra (a inscrição no TX), por isso ela usa o prazo do pareamento.
3. `optimizingLink` (Android), `discovering`, `enablingNotify`, `ready`.

Falhas são classificadas por `classifyLinkError` (função pura, tabelada em teste)
em `BleLinkError`, exposto em `BleConnection.lastError`. Erros de segurança não
são repetidos na reconexão; só `busy` tem novas tentativas (2 s, 4 s, 8 s).
