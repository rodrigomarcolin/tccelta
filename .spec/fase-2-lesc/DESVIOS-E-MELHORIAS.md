# Fase 2 — Desvios do plano e melhorias possíveis

Referências `arquivo:linha` relativas à raiz do repositório. O "plano" é o
texto original das specs (`git show HEAD:.spec/fase-2-lesc/<arquivo>`).

## Desvios do plano

- **02 §7 (`ILinkControl::clearAuthorized`)** — `clearAuthorized()` sem argumento → `clearAuthorized(uint16_t conn)`, só desliga se `conn` é a autorizada (`esp32-firmware/src/connectivity/pairing/ILinkControl.h:22`); `Actions` ganhou `deauthorizeConn` (`PairingPolicy.h:52`).
- **02 §7 (`Actions.reject`)** — `Reject reject[kMaxLinks]` → `reject[kMaxLinks + 1]`, para a conexão que chega sem slot na tabela (`esp32-firmware/src/connectivity/pairing/PairingPolicy.h:48`); `Link` ganhou `rejectHci`; `EncryptionInfo` ganhou valores padrão (`lesc = true`, `persisted = true`).
- **02 §7 (`PairingController::execute`)** — `execute(Actions)` por valor → `execute(const Actions&)` e, após `deleteAllBonds`, chama `onBondsDeleted` e executa recursivamente as novas ações, descartando o resto (`esp32-firmware/src/connectivity/pairing/PairingController.cpp:93-105`).
- **01 §5.4 (estado de `BleConnectivity`)** — `std::atomic<bool> _nusSubscribed` e tabela `connHandle → idAddress` → `_nusSubscribedConn` (`atomic<uint16_t>`, só vale se igual à autorizada) e `_clocks` para `dur`; `deleteBond(conn)` usa `_server->getPeerInfoByHandle` (`esp32-firmware/src/connectivity/ble/BleConnectivity.h:96`, `BleConnectivity.cpp:222`).
- **01 §5.4 (SYNC)** — `onSubscribe` do SYNC TX repassado sempre → só repassado se a conexão é autorizada (`BleConnectivity.cpp:292`).
- **01 §5.6 (`BleSecurity`)** — API listada sem `clearEncStatus` → acrescentada (`esp32-firmware/src/connectivity/ble/BleSecurity.h:60`); `BOND_DELETE reason=recover` é emitido dentro de `eraseBondStoreAndRestart`.
- **02 §6 (`platformio_local.ini.example`)** — exemplo com `PAIR_LED_PIN=2` (A6) → o exemplo só traz `PAIR_BUTTON_HOLD_MS`/`PAIR_WINDOW_MS` (`esp32-firmware/platformio_local.ini.example:14-20`).
- **01 §7 / 02 §7 (docs)** — `docs/rtos-freertos.md` com a linha da `PairingTask` → sem menção (`docs/rtos-freertos.md`); `src/tasks/README.md` e `src/connectivity/README.md` foram atualizados.
- **03 §3 (testes)** — POL-01…31 → 32 casos mais `POL-27b` (`esp32-firmware/test/test_pairing/test_main.cpp:794-799`); POL-25 tem 10 000 passos.
- **03 §2.2 (`docs/debug-events.md`)** — catálogo da seção 5.9 → coexistem `docs/debug-events.md` e `esp32-firmware/docs/eventos.md` §3.9 (mais recente, em português).
- **03 §4 / A7 (artefatos de bancada)** — logs em `docs/bancada/fase-2-lesc/` → a pasta não existe no repositório; nenhum log de B1–B16 versionado.
- **03 A7 (`.gitignore`)** — acrescentar `*.pcapng` → a linha está colada ao final de outra regra (`!latex/aux-docs/firmware-coletor/Investigação-3.pdf*.pcapng`, `.gitignore:7`), sem efeito como regra própria.
- **04 §5.6 (recusa × janela fechada)** — decidir pela última dica do anúncio → `_resolveRefusal` espera um anúncio novo por `refusalProbe` = 4 s e usa `windowOpen` dele (`mobile-app/lib/src/ui/connection/view_model/connecting_view_model.dart:316`).
- **04 §5.6 (`secured` → falha)** — queda após `ready` leva a `removingBond`/`forgetOnIos` → vai a `failed` com `linkError`; o tratamento é do botão "Parear novamente" (`repair()`: Android desconecta e remove o bond; iOS apaga `LastDongle` e vai a `forgetOnIos`) (`connecting_view_model.dart:464`).
- **04 §5.6 (`ConnectingState`)** — campos `security`, `linkError`, `dongleSeen` → também `watching` e `canManageBonds` (`connecting_view_model.dart:71-99`).
- **04 §5.6 (`waitingButton`, 120 s)** — "tela mostra Tentar de novo" → idem, com `onDone` do stream de anúncios definindo `dongleSeen=false` e `watching=false` (`connecting_view_model.dart:376`).
- **04 §7 (testes)** — casos `connection_guard_test` e `forget_dongle_action` com diálogo iOS listados → existem `test/ui/connection/widgets/connection_guard_test.dart` e o diálogo em `mobile-app/lib/src/ui/connection/actions/forget_dongle_action.dart:32`, sem teste específico do diálogo de "esquecer" no iOS.

## Melhorias possíveis

- `esp32-firmware/src/connectivity/ble/BleConnectivity.cpp` e `BleSecurity.cpp` — sem teste automatizado (NimBLE só em placa); a lógica de autorização está coberta apenas pela `PairingPolicy`. Considerar um fake de `ILinkControl` + `PairingController` nativo.
- `esp32-firmware/src/connectivity/pairing/PairingController.cpp` — o mutex é segurado durante a emissão de eventos da política (mutex do logger dentro do lock); se a bancada mostrar latência no host do NimBLE, acumular as linhas de evento nas `Actions` e emitir fora do lock.
- `esp32-firmware/src/connectivity/pairing/PairingController.cpp:93-105` — `kDeleteAttempts` = 2 fixo; falha persistente leva a reinício (`recoverStore`) sem contador de reinícios consecutivos.
- `esp32-firmware/src/connectivity/pairing/PairingConfig.h` — `PAIR_AUTH_TIMEOUT_MS` (35 s) e o prazo do app (30 s, `mobile-app/lib/src/data/datasources/dongle_datasource.dart:99`) são constantes independentes; um teste ou comentário cruzado evitaria divergência.
- `esp32-firmware/src/connectivity/ble/BleSecurity.cpp` — declaração à mão de `ble_gatts_set_clt_cfg_perm_flags` (API privada da NimBLE): sem teste que detecte regressão ao trocar a versão da NimBLE-Arduino (o gate de aplicação cobre o efeito).
- `esp32-firmware/src/connectivity/ble/BleConnectivity.cpp:36-43` — anúncio do dongle expõe o bit "há bond" (decisão A1) e usa endereço público (A3): rastreabilidade (T9) segue como limitação.
- `esp32-firmware/platformio_local.ini.example` — documentar `PAIR_LED_PIN=2` para placas DOIT.
- `docs/rtos-freertos.md` — acrescentar a `PairingTask` (núcleo 0, prioridade 2, 4096 B) à tabela de tarefas.
- `.gitignore:7` — separar a regra `*.pcapng` da linha anterior.
- `mobile-app/lib/src/infra/ble/ble_link_error_classifier.dart` — os códigos de erro por fabricante/versão do Android e iOS (marcados "confirmar em bancada") não têm registro de cobertura em aparelhos; manter uma tabela dos aparelhos testados.
- `mobile-app/lib/src/ui/connection/view_model/connecting_view_model.dart` — `refusalProbe` (4 s) e `hintTimeout` (120 s) são constantes estáticas; sem teste do ciclo de vida do `watchPairingHint` ao cancelar com o app em segundo plano.
- `mobile-app/test/ui/connection/` — sem teste do diálogo "esquecer dispositivo" do iOS (`forget_dongle_action.dart`).
