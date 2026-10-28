# Fase 1 — Desvios do plano e melhorias possíveis

Referências `arquivo:linha` relativas à raiz do repositório. O "plano" é o
texto original das specs (`git show HEAD:.spec/fase-1-seguranca-telemetria/<arquivo>`).

## Desvios do plano

- **01 §3 (CRC32)** — tabela de 256 entradas `constexpr` (1 KB) → tabela de meio byte, 16 entradas, dois passos por byte (`esp32-firmware/src/crypto/Crc32.cpp:8`). Compila em C++11 e usa menos `.rodata`.
- **01 §11 (`provision.py --nvs-key board`)** — ler a `nvs_key` da placa, decifrar com a chave FE e reaproveitá-la; `auto` escolhia `board` com `--flash` → `board` não existe (aborta com `SystemExit`) e `auto` sempre gera chave nova em `devsec`/`prod` (`esp32-firmware/tools/provision/provision.py:147`).
- **01 §11 e plano de arquivos (`tools/provision/`)** — `README.md` e `verify_block.py` (verificação da assinatura `asym` no host) → ausentes; só `provision.py`, `vectors.py`, `vectors.json`, `requirements.txt`.
- **01 §11 (`--kms-wrapping`)** — arquivo exportado por `tccelta provisionar exportar-wrapping` → o script lê um JSON `{auth, rec}` de PEMs (`provision.py:115`); o formato é consumido do lado da Fase 3.
- **01 plano de arquivos (`IDiagGate`, `ISyncStatePersistence`)** — headers próprios `src/sync/ISyncStatePersistence.h` e `IDiagGate.h` → declarados em `esp32-firmware/src/sync/SyncEngine.h:20-35`.
- **04 §9 / geral (namespace `sync`)** — `sync::` → `datasync::` (colide com `sync()` do POSIX; `esp32-firmware/src/sync/SyncProtocol.h:15`). `Fragmenter` ficou só em header (`src/sync/Fragmenter.h`), sem `.cpp`.
- **02 §11 (`readRecord`)** — `readRecord(src, tagLen, firstSeq, seq, out)` → sem o parâmetro `tagLen`, lido do cabeçalho do segmento (`esp32-firmware/src/collector/SegmentFormat.h:56`). `WriteReport` ganhou o campo `boot` (`BlockWriter.h`).
- **02 §10 (cursor de leitura)** — cache de arquivo aberto para leituras sequenciais, "implementar só se `read_us` médio > 5 ms" → não implementado; `read` varre o segmento a cada chamada (`esp32-firmware/src/storage/LittleFsBlockStore.cpp`).
- **03 Testes (propriedade)** — 10 000 iterações, quedas em pontos aleatórios entre os passos do `BlockWriter` → 400 boots com N = 4, e a "queda" é só `append` falhando (`esp32-firmware/test/test_seq_reserve/test_main.cpp:136-140`).
- **03 plano de arquivos (`-DSEQ_RESERVE_STEP`)** — documentar no `platformio_local.ini.example` → não documentado (`esp32-firmware/platformio_local.ini.example`); o macro existe em `src/collector/SeqReserve.h:6`.
- **04 §1 (propriedades GATT)** — `SYNC_RX` só `WRITE` (sem `WRITE_NR`) e `SYNC_TX` só `NOTIFY` → `WRITE | WRITE_NR | WRITE_ENC` e `NOTIFY | READ_ENC`, por causa do LESC da Fase 2 (`esp32-firmware/src/connectivity/ble/BleSecurity.h:14,19`). O anúncio ganhou dados de fabricante (`BleConnectivity.cpp:36`).
- **04 §7 (`BleSyncLink`)** — buffer `_frag[517 − 3]` e `onRx`/`onNotifyStatus` → `_frag[517]` e callback `onMtuChange` adicional (`esp32-firmware/src/connectivity/ble/BleSyncLink.h:50`).
- **05 §13 (`sync_bench.py`)** — cenários incluem `reboot` e `--repeat` → só `handshake`, `stream`, `replay`, `badtag`, `nosession`, `badack`, `incomplete`, `corrupt` (`esp32-firmware/tools/sync-bench/sync_bench.py:193`).
- **06 §3.1 (`KEYS.source`)** — `mac_derived` → `mac` (`esp32-firmware/src/security/DeviceKeys.h:30`).
- **06 §3.6 (`STACK`)** — tarefas `acquisition, flush, elm327, sync, logger` → também `pairing` (Fase 2).
- **06 (docs)** — `docs/debug-events.md` v2 como único catálogo → coexistem `docs/debug-events.md` (formato v2, inglês) e `esp32-firmware/docs/eventos.md` (catálogo em português, mais recente, inclui os eventos da Fase 2).
- **06 §9 / §9.1 (viewer)** — `tools/event-viewer/core.js` com categorias `link`/`security` para `SYNC_*`/`HS_*`/`KEYS`/`MARKER`, `st.sync`, e painel `bench.html` + `bench-metrics.js` → nada disso existe: `core.js:11` ainda lista `UP_SEND`/`UP_ACK`/`TIME_*` e o gerador de demonstração emite `FORMAT v=1` (`core.js:522`); não há `bench.html`.
- **06 §7.3 (`bench_report.py`)** — B1–B12 → B5 (handshake ponta a ponta) e B7 (overhead de fragmentação) não são calculados; B6 só por lote (`esp32-firmware/tools/event-bench/bench_report.py:58-95`).
- **06 §6.8 (`seq_audit`)** — script de auditoria de `seq` → não existe; a regra é só descrita.
- **07 §1 (PSK)** — camada PSK apagada, `connect(...)` só com `extraChannels` e `mtu` → idem, mais `securingTimeout` (Fase 2) em `mobile-app/lib/src/services/ble/ble_service.dart`.
- **07 §4 (`DongleSyncRepository`)** — sem `linkGeneration` → o port expõe `linkGeneration` (usado pelo coordenador em `start()`, `sync_coordinator.dart:123`).
- **07 §7 (reações do coordenador)** — não prevê buffer cheio nem espera após lote interrompido → buffer cheio ao receber bloco dispara `resync(incompleteMessage)` (`mobile-app/lib/src/application/sync/sync_coordinator.dart:325`); duas interrupções seguidas somam `backoff` em `notBefore` (`:604`); ACK com o link caído fica em `_heldAck` (`:543`).
- **07 Testes** — `test/ui/sync/sync_status_card_test.dart` e `handshake_messages_test.dart`/`send_policy_test.dart` listados → o diretório `test/ui/sync/` não existe e não há `send_policy_test.dart` nem `handshake_messages_test.dart` em `mobile-app/test/domain/sync/`.
- **08 Testes** — `gps_fix_test.dart`, `sync_settings_screen_test.dart` → inexistentes; a conversão de `GpsFix` é testada dentro de `mobile-app/test/application/gps/gps_reporter_test.dart:78`.
- **08 §5 (`fixes`)** — `fixes({interval})` → `fixes({interval, background = false})`; `background` liga o serviço em primeiro plano/iOS (`mobile-app/lib/src/domain/repositories/location_repository.dart`).
- **09 §8 (seleção do servidor)** — padrão `bench` em *debug*, `stub` em *release*, e *release* nunca usa outro → padrão `https` em qualquer build; `bench`/`stub` só em *debug* via `--dart-define=SYNC_SERVER` (`mobile-app/lib/src/ui/sync/sync_providers.dart:26-28`). O gateway HTTPS é da Fase 3.
- **09 §7 (estado do servidor de bancada)** — "só memória" → persistido em `shared_preferences` (`bench_sync_server_state`, `mobile-app/lib/src/infra/sync/bench/bench_state_store.dart:22`).
- **09 §7 (regra do BOOT no lote)** — BOOT aceito com `from == ackSeq + 1` e `to == seq − 1` → aceita também `from < ackSeq + 1` (`mobile-app/lib/src/infra/sync/bench/bench_sync_server.dart:416`), alinhado à ingestão da Fase 3.
- **09 §7 (estatísticas)** — exibidas na seção "Depuração" da tela de configurações → `BenchSyncServer.statsStream` não é consumido por nenhuma tela (`grep statsStream mobile-app/lib`); a seção só tem "Forçar handshake" e "Esquecer estado".
- **09 §2 (`latestConfirmation`)** — mantido no port, não chamado pelo app → idem.

## Melhorias possíveis

- `esp32-firmware/src/storage/KeyStore.cpp` — sem teste (nem nativo nem de placa automatizado) para os caminhos `initPartition`/`load`; extrair a escolha de fonte de chaves para função pura testável.
- `esp32-firmware/src/security/mbed/EcdsaBlockAuth.cpp` — sem teste de assinatura/verificação no host (o vetor SHA-256 da entrada existe, a assinatura não); falha de `sign_det_ext` grava tag zerada em silêncio.
- `esp32-firmware/tools/provision/provision.py` — implementar `--nvs-key board`, ou remover a opção; `kms_record.json` com `alg: none` só avisa em stderr (trocar por falha em `prod`).
- `esp32-firmware/src/storage/LittleFsBlockStore.cpp` — sem teste nativo (só placa); `read` linear por segmento (até ~64 registros) e cursor de leitura em aberto; `kMaxSegments` = 256 fixo.
- `esp32-firmware/src/sync/SyncEngine.cpp` — `ackLocal` só em RAM e `IDiagGate` sempre `nullptr` (`CollectorApp.cpp` passa `nullptr`): `flags_c`/`diag_min` do desafio são ignorados.
- `esp32-firmware/test/test_seq_reserve/test_main.cpp` — propriedade com poucas iterações e sem queda entre reserva NVS e `append`; ampliar para os pontos de falha do `BlockWriter`.
- `esp32-firmware/src/connectivity/README.md` — referencia `docs/debug-events.md` §5.7 e `esp32-firmware/docs/eventos.md` coexistem; unificar os dois catálogos.
- `tools/event-viewer/core.js` — atualizar para v2 (eventos `SYNC_*`, `HS_*`, `KEYS`, `MARKER`) e criar o painel de benchmark; hoje não reconstrói a sessão.
- `esp32-firmware/tools/event-bench/bench_report.py` — sem testes automatizados além da fixture; faltam B5 e B7.
- `esp32-firmware/tools/sync-bench/sync_bench.py` — sem cenário `reboot` e sem `--repeat`.
- `mobile-app/lib/src/application/sync/sync_coordinator.dart` — arquivo de 646 linhas concentra sessão, lote, ressincronização e POST; sem teste de `SyncStatusCard` (`mobile-app/test/ui/sync/` ausente).
- `mobile-app/lib/src/infra/sync/bench/bench_sync_server.dart` — expor as estatísticas (`statsStream`) na tela de depuração; sem verificação ECDSA para blocos `T = 64`.
- `mobile-app/lib/src/application/gps/gps_reporter.dart` — `kGpsMaxAccuracyM` (100 m) e idade máxima (`2 × T_GPS`) fixos no código; sem teste de ciclo de vida (primeiro/segundo plano) com `grantedWhileInUse`.
- `mobile-app/lib/src/data/repositories/dongle_sync_repository_impl.dart` — depende de `DongleRepository` (repository sobre repository), como `Obd2RepositoryImpl`; extrair um port fino de fonte de conexão.
- `mobile-app/lib/src/domain/sync/sync_protocol.dart` — timeouts (`kFragmentTimeout`, `kBatchStallTimeout`, `kHandshakeStepTimeout`) são constantes sem ajuste por configuração ou ambiente.
