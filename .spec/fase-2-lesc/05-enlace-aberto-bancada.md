# 05 — Enlace aberto para bancada (`collector_mock_debug_open`)

## Motivo

Desde a Fase 2 o firmware só entrega NUS e SYNC a uma conexão cifrada com bond
LESC; escritas de não autorizados são descartadas (`WRITE_DROP`). O
`tools/sync-bench/sync_bench.py` (bleak, no PC) faz o papel do app mas não
pareia, então só funcionava no firmware da Fase 1.

## Decisão

Flag `-DLINK_SECURITY_OPEN` e env `collector_mock_debug_open` (estende
`collector_mock_debug`). Só o **enlace** BLE fica aberto; o perfil de segurança
do protocolo continua `hmac` (tags, handshake, K_auth/K_rec inalterados).
Sem o flag, o comportamento dos demais envs é idêntico ao anterior.

Com o flag:

- `BleSecurity::configure()` não liga LESC nem bond nem as permissões de CCCD cifradas;
  RX/TX do NUS e do SYNC perdem `*_ENC`.
- `onConnect` chama `setAuthorized` direto (`CONN up=1` logo após `LINK_UP`);
  `onDisconnect` chama `clearAuthorized` e religa o anúncio `Normal`.
- `CollectorApp` não cria `PairingController` nem `PairingTask` e não chama
  `sanitizeBondsAtBoot` (a tarefa `pairing` some do evento `STACK`).
- Evento `LINK_OPEN` no boot (docs/debug-events.md §5.9).

## Proteções de build (`LinkSecurityMode.h`)

`#error` se o flag aparecer sem `COLLECTOR_EVENTS` ou junto de
`CONFIG_SECURE_BOOT` / `CONFIG_SECURE_FLASH_ENC_ENABLED` (devsec/prod).

## Uso

`pio run -e collector_mock_debug_open -t upload`, depois
`python tools/sync-bench/sync_bench.py --scenario handshake` (`pip install bleak`).
Qualquer central conecta e escreve no NUS, incluindo `AT#WIPE`: só bancada.
O app Flutter não é objetivo neste env.
