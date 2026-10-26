# Specs de implementação — segurança dongle ↔ app ↔ servidor

Planos técnicos de implementação derivados dos documentos de alto nível do
projeto. Cada fase é uma pasta com um `README.md` (objetivo, ordem das specs,
critérios de aceite da fase) e um conjunto de specs numeradas, cada uma um
plano arquivo por arquivo pronto para execução.

| Fase | Pasta | Entrega |
|---|---|---|
| 1 | `fase-1-seguranca-telemetria/` | Firmware + app: chaves em NVS encriptada, bloco autenticado (HMAC + CRC32), serviço BLE SYNC (TLV + fragmentação), handshake app↔dongle, cursor de envio, buffer no app, GPS periódico, molde do gatilho de handshake. Fluxos: leitura contínua (Fig. 15) e handshake (Fig. 5, só app↔dongle). |
| 2 | `fase-2-lesc/` | Firmware + app: pareamento BLE LESC Just Works fim a fim, janela de pareamento por botão (GPIO0, 5 s ⇒ 45 s), um bond por vez, derrubada de bonds e conexões. |
| 3 | `fase-3-servidor/` | `backend/` em Docker: API Go (goroutines, sqlc + pgx, migrations), PostgreSQL/TimescaleDB/PostGIS, worker assíncrono, KMS (Vault Transit) com K_auth gerar+verificar e K_rec só verificar, JWT; integração HTTPS no app (desafio/resposta, lotes, ACK, gatilhos de handshake). |

Ordem recomendada: Fase 1 → Fase 3 → Fase 2 (a Fase 2 só troca a camada de
enlace e pode entrar a qualquer momento depois da Fase 1; a Fase 3 depende do
molde de handshake e dos formatos fixados na Fase 1).

## Documentos de referência

- `latex/aux-docs/seguranca-dongle-server/segunraca-dongle-server.tex` — especificação de segurança (protocolo, formatos, tabelas de erros, servidor).
- `latex/aux-docs/firmware-coletor/Investigação-3.pdf` — estratégia de coleta (blocos, seq, boot_id, Cristian).
- `latex/aux-docs/server-implementation/server-specification.md` — DDL de referência, tempo absoluto, worker.
- `latex/assets/aux-docs/plantuml/*.puml` — fontes dos diagramas de sequência e de dados.
- `docs/debug-events.md` — contrato dos eventos de depuração do firmware.
- `docs/plano-coleta.md` — plano da coleta já implementada no firmware.

Mapeamento das figuras do documento de segurança usado nas specs: Fig. 5
handshake; Fig. 6 blocos do handshake; Fig. 7 formato SYNC_RX/SYNC_TX; Fig. 8
bloco na flash; Fig. 9 bloco TLV completo; Fig. 10 área de dados; Fig. 11
registros (PID, GPS, BOOT/LOSS); Fig. 12–13 cursor; Fig. 14 aviso de novo
bloco; Fig. 15 leitura contínua. Tabela 2 parâmetros do handshake; Tabela 3
tipos de mensagem; Tabela 4 flags do aviso; Tabela 5 erros.

## Decisões transversais (valem para as três fases)

Pontos que o documento de segurança deixa implícitos ou em aberto e que foram
fixados aqui para as fases não divergirem entre si.

1. **Endianness.** Todo campo multi-byte é little-endian, em BLE, na flash e
   no servidor, como já faz `esp32-firmware/src/collector/BlockFormat.h`.
2. **Tag de bloco.** `tag = HMAC-SHA256_{K_rec}("MALCONG" || 0x03 || DevId || bloco)`,
   em que `bloco` são os `tamanho` bytes da área de dados (cabeçalho de 12 B +
   registros), não 1024 bytes fixos. Na flash o registro é
   `dados(tamanho) | tag(32) | CRC32(4)`, com o CRC sobre dados + tag.
3. **Tag de handshake.** `HMAC-SHA256_{K_auth}("MALCONG" || tipo || DevId || payload)`
   truncado a 16 bytes; `tipo` é 1 byte (0x01, 0x02, 0x04) e `payload` são
   os campos da mensagem sem a tag. Comparação sempre em tempo constante.
4. **Serviço BLE SYNC.** Service `7A3C0001-9B1E-4F2A-8C5D-2E6F1A0B3C4D`,
   RX `7A3C0002-…` (WRITE, app → dongle, TLV), TX `7A3C0003-…` (NOTIFY,
   dongle → app, byte de controle + TLV). O NUS continua para texto ELM327.
5. **Perfis de segurança do firmware.** Build flag `SECURITY_PROFILE=none|hmac|asym`
   sobre a interface genérica `IMessageAuth`: `none` (tag de 0 B, só bancada),
   `hmac` (produção) e `asym` (assinatura digital, perfil de desenvolvimento
   para benchmark). O servidor prevê o perfil `asym` como desenho.
6. **Eventos de benchmark.** Todo processamento de segurança no firmware
   (HMAC, CRC, fragmentação, desafio/resposta, ACK, leitura e validação de
   bloco) emite evento no padrão de `docs/debug-events.md` com `dur`, para
   comparar builds com e sem segurança.
7. **Marcadores.** BOOT (`0xFE`) e LOSS (`0xFD`) são blocos 0x03 comuns com um
   único registro de 11 B; BOOT substitui a antiga mensagem BATCH; TIME_REQ/
   TIME_RESP/TIME_REPORT somem, a âncora de tempo vem do handshake.
8. **Camada PSK legada do app.** A criptografia de aplicação por PSK/AES-GCM
   no app (`core/crypto`, `encrypted_ble_connection`, `SecurityMode`,
   telas de PSK) não tem contraparte no firmware atual; a Fase 2 decide o
   destino e a Fase 1 não a reutiliza.
9. **Contrato HTTP.** O lado app da Fase 1 expõe um port (gateway de servidor)
   com implementação stub; a Fase 3 define o contrato HTTP byte a byte
   (octet-stream para blocos e desafio; resposta ao desafio carrega ACK em
   base64 junto com o JWT) e implementa o port.
10. **Mesmo firmware em desenvolvimento e produção.** Todo o firmware do
    dongle é compilado como Arduino componente do ESP-IDF
    (`framework = arduino, espidf`, ESP-IDF 4.4.7), com o mesmo código, o
    mesmo `CMakeLists.txt` e a mesma tabela de partições
    (`partitions_collector.csv`: tabela em `0x10000`, `nvs`, `nvs_key`,
    `secnvs`, app, LittleFS de 2 MB, `coredump`) em três perfis de hardware:
    `dev`, `devsec` (ensaio de produção em placa descartável) e `prod`. A
    diferença entre eles é **só** (a) um arquivo de *defaults* do `sdkconfig`
    acrescentado ao *build* (`sdkconfig.devsec.defaults` /
    `sdkconfig.prod.defaults`, via `board_build.cmake_extra_args`) e (b) o
    procedimento de provisionamento e queima de eFuse. A NVS encryption
    (chaves na `secnvs`, estado e *bonds* na `nvs`) fica ligada nos três;
    no `dev` a chave XTS fica legível na flash e não protege nada. A
    migração é o passo 0 da Fase 1 (`fase-1/01` §9).

Cada spec marca com **[Decisão]** as escolhas próprias justificadas e com
**[Aberto]** os pontos que dependem do autor, sempre com recomendação.

## Contratos cruzados entre fases

Reconciliados pelo orquestrador depois que as três fases foram escritas em
paralelo. Quando uma spec de fase disser algo diferente desta tabela, vale a
tabela e a spec deve ser corrigida.

| Contrato | Define | Consome | Onde (lado que define) | Onde (lado que consome) |
|---|---|---|---|---|
| Endianness: little-endian em tudo; os 2 bytes de valor de PID são `A`,`B` crus da resposta OBD-II | Fase 1 | Fase 3 | `fase-1/02` §1, `04` §3 | `fase-3/03` §Pacote `protocolo` |
| `L` do TLV (u16 LE) = comprimento de V = `tamanho + tag_len`; `tamanho` (u16 LE do cabeçalho) = área de dados **com** o cabeçalho de 12 B; dados ≥ 17 B (marcador = 23 B). A Tabela 3 do `.tex` ("55 a 1.056") está incorreta | Fase 1 | Fase 3 | `fase-1/04` §3 | `fase-3/03` §Regras de forma, README |
| Entrada das tags: `"MALCONG"(7) ‖ tipo(1) ‖ DevId(8) ‖ payload`; handshake = campos sem a tag, HMAC `K_auth` truncado a 16 B; bloco = os `tamanho` bytes, HMAC `K_rec` 32 B | Fase 1 | Fase 3 | `fase-1/01` §1, §12 | `fase-3/03` §Entrada do HMAC |
| Perfis `SECURITY_PROFILE=none\|hmac\|asym`: só a tag do **bloco** muda (0/32/64 B); handshake sempre 16 B; `asym` = ECDSA P-256 determinística, `r‖s` 64 B (mbedTLS 2.28 não tem Ed25519) | Fase 1 | Fase 3 | `fase-1/01` §1 | `fase-3/03` §Perfis, `07` §Perfis, `02` (`assinatura` CHECK) |
| Vetores de teste: gerador único `esp32-firmware/tools/provision/vectors.py` → `vectors.json` (DevId `544343454C544101`), consumido por firmware, app e `backend/internal/protocolo/testdata/vetores_firmware.json` | Fase 1 | Fase 1 (app), Fase 3 | `fase-1/01` §12 | `fase-3/03` §Vetores |
| Serviço SYNC: UUIDs `7A3C0001/0002/0003-9B1E-4F2A-8C5D-2E6F1A0B3C4D`, criados dentro de `BleConnectivity`, transporte em `BleSyncLink`; o SYNC **não** vai no anúncio; a Fase 1 não registra `setCustomGapHandler` | Fase 1 | Fase 2 | `fase-1/04` §1, §7–8 | `fase-2/01` §5.4–5.5 |
| Mensagens 0x01–0x07, flags do aviso, erros da Tabela 5, regras do cursor | Fase 1 | Fase 3 (0x01–0x04 no HTTP) | `fase-1/04` §3–5, `05` | `fase-3/03`, `04`, `05` |
| Marcadores no servidor: BOOT aceito com `to = seq − 1` e `from ≤ ACK + 1` (faixa vazia ⇒ sem lacuna); LOSS aceito com `from = ACK + 1` e `from ≤ seq ≤ to`; `seq ≤ ACK` no lote = duplicado sem interromper; `seq = ACK + 1` sempre aceito. BOOT com CRC inválido vira LOSS em pedaços no dongle | Fase 3 (regras) / Fase 1 (geração) | Fase 1 / Fase 3 | `fase-3/05` §Regras de reconciliação | `fase-1/03` §3–4, `05` §4, §6 |
| TAIL efetivo: o dongle relata `TAIL_seq = from_seq` do BOOT mais antigo (CRC válido); o servidor só declara OVERFLOW quando `TAIL_seq > seq_contiguo + 1` | Fase 1 | Fase 3 | `fase-1/05` §4 | `fase-3/04` passo 7f |
| `c_S` emitido = `max(ultimo_c_s + 1, segundos Unix)`; o servidor de bancada do app usa a mesma regra; o dongle persiste `last_cs` e o *wipe* de bancada o zera | Fase 3 | Fase 1 | `fase-3/04` §Queries | `fase-1/03` §7, `09` §7 |
| Contrato HTTP: desafio e resposta em JSON com base64 (`{"desafio"}`, `{"resposta"}` → `{"ack","token","token_expira_em","token_validade_s","t_lat_s"}`); blocos e ACK de lote em `application/octet-stream` (TLVs concatenados); `X-Lote-*`; "corpo é ACK ⇔ octet-stream" | Fase 3 | Fase 1 (app) | `fase-3/03` §Contratos HTTP, `04`, `05` | `fase-1/09` §2 (tabela de mapeamento de bytes) |
| Port do app: `SyncServerGateway` (`requestChallenge`, `submitResponse`, `postBlocks`, `latestConfirmation`), `ChallengeEnvelope`/`HandshakeEnvelope`, tabela de exceções por `codigo`; token em `SyncServerRepository`; dongle em `DongleSyncRepository`; orquestração em `HandshakeRunner`/`SyncCoordinator`. A Fase 3 entrega só `HttpsSyncServerGateway` (`infra/sync/`) e os gatilhos que faltam | Fase 1 | Fase 3 | `fase-1/09` §2–5, `07` §4, §7 | `fase-3/09` §Contrato cruzado |
| Provisionamento: `provision.py --profile dev\|devsec\|prod` (Fase 1) gera `DevId`/`K_auth`/`K_rec`/`k_sig`, `secnvs.csv` (partição `secnvs`, namespace `sec`), imagem NVS sempre cifrada com a `nvs_key` (nova ou reaproveitada da placa) e `kms_record.json` (esquema inalterado) no formato BYOK do Vault Transit (wrapping keys exportadas por `tccelta provisionar exportar-wrapping`); a Fase 3 importa com `importar-registro`; `tccelta provisionar novo` só para dongles de teste | Fase 1 (envelope) / Fase 3 (wrapping) | Fase 3 / Fase 1 | `fase-1/01` §11 | `fase-3/07` §Ferramenta de provisionamento |
| NVS: chaves na `secnvs` (namespace `sec`); estado do protocolo na `nvs` (namespace `collector`: `seq_rsv`, `last_cs`, `wipe_req`); bonds do NimBLE na `nvs` (namespace `nimble_bond`). Nos três perfis de hardware a NVS encryption cifra `secnvs` **e** `nvs` (bonds cobertos, `Preferences` sem mudança); proteção real só em `devsec`/`prod` (chave XTS na `nvs_key` protegida pela *flash encryption*); no `dev` a `nvs_key` é legível | Fase 1 | Fase 2 | `fase-1/01` §2, §7–§9; `03` §1 | `fase-2/01` §6 |
| Build e partições: `framework = arduino, espidf` em todos os envs do firmware; uma só `partitions_collector.csv` (tabela em `0x10000`; `nvs 0x12000/0x5000`, `nvs_key 0x17000/0x1000 encrypted`, `secnvs 0x19000/0x3000`, `factory 0x20000/0x1D0000`, `spiffs 0x1F0000/0x200000`, `coredump 0x3F0000/0x10000`); BT do IDF em Bluedroid com NimBLE do IDF desligado, para os `-DCONFIG_BT_NIMBLE_*` continuarem valendo | Fase 1 | Fase 2 | `fase-1/01` §9 | `fase-2/01` §3, §6; `fase-2/02` §5.1 |
| LESC: NUS e SYNC só para conexão autorizada (bond LESC); `APP_CONNECTED`/`BleSyncLink::onConnected` passam a disparar na autorização | Fase 2 | Fase 1 | `fase-2/01` §5.4 | `fase-1/04` §8, `05` §4 |
| Camada PSK legada do app: a Fase 1 a desliga atrás de `LEGACY_PSK_TRANSPORT` sem apagar nada (estado intermediário); a Fase 2 remove (recomendado, com tag git) ou isola, e retira a flag | Fase 1 → Fase 2 | — | `fase-1/07` §1 | `fase-2/04` §5.7 (A8) |
| NimBLE-Arduino: `platformio.ini` pede 2.5.1, o pacote instalado em `.pio/libdeps` é 2.5.0; as APIs citadas nas specs foram conferidas na 2.5.0 — reconferir após `pio pkg install` | — | Fases 1 e 2 | `esp32-firmware/.pio/libdeps/base/NimBLE-Arduino/library.properties` | `fase-1/04` §1, `fase-2/01` §3 (R6) |

## Correção ao PLAN_PROMPT.md

O pedido original (`PLAN_PROMPT.md`, na raiz) diz "verificar HMAC de 32 bytes
do desafio". Pelo documento de segurança (Tabela 2, Tabela 3 e Fig. 6), a tag
de **todas** as mensagens de handshake (Desafio 0x01, Resposta 0x02, ACK 0x04)
é o HMAC-SHA256 com `K_auth` **truncado a 16 bytes**; só a tag dos blocos
(0x03, com `K_rec`) tem 32 bytes. As três fases seguem o documento. O outro
ponto do prompt que o documento contradiz, "55 a 1.056 bytes" para o bloco, é
um erro da Tabela 3 do próprio documento (ver tabela acima).
