# Documentação técnica — segurança dongle ↔ app ↔ servidor

Documentação do estado implementado de cada fase (o código é a fonte de
verdade; formatos, máquinas de estado e fluxos descritos aqui batem com o
código testado). Cada fase é uma pasta com um `README.md` (visão geral,
arquitetura, índice), documentos numerados e, nas fases 1 e 2, um
`DESVIOS-E-MELHORIAS.md` com o que divergiu do plano original e o que ainda
pode melhorar. A Fase 4 está implementada no código (specs por entrega em `fase-4-ecdsa/`);
falta consolidá-las no estado final, como nas fases 1 a 3, e rodar o roteiro de
bancada com a placa.

| Fase | Pasta | Entrega |
|---|---|---|
| 1 | `fase-1-seguranca-telemetria/` | Firmware + app: build único Arduino + ESP-IDF, chaves na partição `secnvs`, bloco autenticado (HMAC/ECDSA + CRC32) em segmentos v2, reserva de `seq` + BOOT, serviço BLE SYNC (TLV + fragmentação), handshake app↔dongle, cursor de envio, buffer e `SyncCoordinator` no app, GPS periódico, port do servidor com stub e servidor de bancada. Fluxos: leitura contínua (Fig. 15) e handshake (Fig. 5, perna app↔dongle). |
| 2 | `fase-2-lesc/` | Firmware + app: pareamento BLE LESC Just Works fim a fim, janela de pareamento por botão (GPIO0, 5 s ⇒ 45 s), um bond por vez, derrubada de bonds e conexões. |
| 3 | `fase-3-servidor/` | `backend/` (um módulo Go, dois binários): API (`tccelta api`) com handshake, ingestão de lotes e confirmação; worker de tempo e reprocessamento; KMS próprio (`tccelta-kms`, único cliente do Vault Transit: `K_auth` gerar+verificar, `K_rec` só verificar, JWT Ed25519 com rotação); PostgreSQL/TimescaleDB/PostGIS; Redis, Caddy/túnel, Docker Compose (dev, prodlike, teste); simulador e `make e2e`. App: `HttpsSyncServerGateway` atrás do port da Fase 1. |
| 4 | `fase-4-ecdsa/` | **Implementada (falta a bancada com a placa).** Perfil `asym` fim a fim: verificação ECDSA P-256 local na ingestão (despacho por perfil, `none` incluso), `provision new/import` com `k_sig`, simulador e cenário `asym` no `make e2e`, vetor de assinatura RFC 6979 cruzado firmware ↔ servidor, falha de assinatura explícita no firmware. |

A Fase 2 só troca a camada de enlace; a Fase 3 depende dos formatos e do port
de servidor da Fase 1; a Fase 4 depende das Fases 1 e 3.

## Documentos de referência

- `latex/aux-docs/seguranca-dongle-server/segunraca-dongle-server.tex` — especificação de segurança (protocolo, formatos, tabelas de erros, servidor).
- `latex/aux-docs/firmware-coletor/Investigação-3.pdf` — estratégia de coleta (blocos, seq, boot_id, Cristian).
- `latex/aux-docs/server-implementation/server-specification.md` — DDL de referência, tempo absoluto, worker.
- `latex/assets/aux-docs/plantuml/*.puml` — fontes dos diagramas de sequência e de dados.
- `esp32-firmware/docs/eventos.md` — catálogo dos eventos de depuração do firmware; `docs/debug-events.md` — especificação do formato v2.
- `docs/plano-coleta.md` — plano da coleta no firmware.
- `backend/README.md` e `backend/docs/{ambientes,kms,kms-http-contract,kms-dongle-record,metricas,cenarios-simulador}.md` — operação do backend, KMS, métricas e simulador.

Mapeamento das figuras do documento de segurança: Fig. 5 handshake; Fig. 6
blocos do handshake; Fig. 7 formato SYNC_RX/SYNC_TX; Fig. 8 bloco na flash;
Fig. 9 bloco TLV completo; Fig. 10 área de dados; Fig. 11 registros (PID,
GPS, BOOT/LOSS); Fig. 12–13 cursor; Fig. 14 aviso de novo bloco; Fig. 15
leitura contínua. Tabela 2 parâmetros do handshake; Tabela 3 tipos de
mensagem; Tabela 4 flags do aviso; Tabela 5 erros.

## Invariantes transversais

1. **Endianness.** Todo campo multi-byte é little-endian (BLE, flash e servidor), como em `esp32-firmware/src/collector/BlockFormat.h`. Exceção: `r‖s` da assinatura ECDSA são inteiros big-endian (tags são cadeias de bytes).
2. **Tag de bloco.** `tag = HMAC-SHA256_{K_rec}("MALCONG" ‖ 0x03 ‖ DevId ‖ bloco)` (32 B); `bloco` são os `tamanho` bytes da área de dados (cabeçalho de 12 B + registros), não 1024 fixos. Na flash o registro é `dados(tamanho) ‖ tag ‖ CRC32(dados ‖ tag)`.
3. **Tag de handshake.** `HMAC-SHA256_{K_auth}("MALCONG" ‖ tipo ‖ DevId ‖ payload)` truncado a 16 B nos três perfis; `tipo` é 1 byte (0x01, 0x02, 0x04) e `payload` são os campos da mensagem sem a tag. Comparação sempre em tempo constante.
4. **Serviço BLE SYNC.** Service `7A3C0001-9B1E-4F2A-8C5D-2E6F1A0B3C4D`, RX `7A3C0002-…` (app → dongle, TLV, até 64 B por escrita), TX `7A3C0003-…` (NOTIFY, dongle → app, byte de controle + TLV). O NUS continua para texto ELM327. O SYNC não vai no anúncio.
5. **Perfis de segurança do firmware.** `SECURITY_PROFILE=none|hmac|asym` sobre `IMessageAuth`: só a tag do **bloco** muda (0 / 32 / 64 B; `asym` = ECDSA P-256 determinística, `r‖s`). Handshake sempre HMAC de 16 B.
6. **Eventos de benchmark.** Todo processamento de segurança/sincronização no firmware emite evento com duração `_us` (microssegundos); `dur` continua em ms.
7. **Marcadores.** BOOT (`0xFE`) e LOSS (`0xFD`) são blocos 0x03 com um único registro de 11 B (`tamanho` = 23). Todo boot grava um BOOT (inclusive o primeiro, faixa vazia `to = from − 1`); o LOSS é gerado no envio e não é gravado.
8. **Sem PSK no app.** A criptografia de aplicação por PSK/AES-GCM foi removida; a segurança do enlace é LESC (Fase 2) e a dos dados é a tag (Fase 1). Histórico na tag `versao-custom-secure-ble`.
9. **Contrato do servidor no app.** O app fala com o servidor pelo port `SyncServerGateway`: desafio e resposta ao desafio em JSON com os bytes em base64 (a resposta devolve o ACK em base64 junto com o JWT); blocos e o ACK do lote em `application/octet-stream` (TLVs concatenados). O gateway HTTPS é da Fase 3; stub e servidor de bancada embutido (só *debug*) são da Fase 1.
10. **Mesmo firmware em todos os perfis de hardware.** Todo o firmware é Arduino como componente do ESP-IDF 4.4.7 (`framework = arduino, espidf`), com o mesmo código, o mesmo `CMakeLists.txt` e a mesma `partitions_collector.csv` em `dev`, `devsec` (ensaio de produção em placa descartável) e `prod`. A diferença é só o arquivo de *defaults* do `sdkconfig` (`sdkconfig.devsec.defaults` / `sdkconfig.prod.defaults`, via `board_build.cmake_extra_args`) e o procedimento de provisionamento/eFuse. A NVS encryption (cobre `secnvs` e `nvs`, inclusive os bonds do NimBLE) está ligada em `devsec`/`prod`; no `dev` a NVS fica em claro (`KeyStore` escolhe por `#if CONFIG_NVS_ENCRYPTION`).
11. **Enlace.** O dongle só entrega dados do NUS e do SYNC à conexão autorizada (cifrada + bond LESC com `sc == 1`, chave de 16 B); `APP_CONNECTED`/`CONN up` significam "sessão autorizada".
12. **KMS isolado.** A API não tem credencial do Vault: todo HMAC `K_auth`, verificação `K_rec` e assinatura do JWT passam pelo serviço `tccelta-kms` (HTTP, mTLS fora do dev), único cliente do Vault Transit em runtime. `K_rec` só é usada para verificar; as chaves entram no Vault por BYOK (`kms_record.json` do firmware ou `provision new`/`import` de teste). Identificadores do backend, esquema do banco, CLI e métricas estão em inglês; rotas, JSON do handshake, `codigo` dos erros, `X-Lote-*`, variáveis de ambiente e serviços do compose continuam em português (contrato de fio e de operação); os alvos do Makefile estão em inglês.

## Contratos cruzados entre fases

Quando uma doc de fase disser algo diferente desta tabela, vale a tabela e a
doc deve ser corrigida.

| Contrato | Define | Consome | Onde (lado que define) | Onde (lado que consome) |
|---|---|---|---|---|
| Endianness: little-endian em tudo; os 2 bytes de valor de PID são `A`,`B` crus da resposta OBD-II | Fase 1 | Fase 3 | `fase-1/02` | `fase-3/03` §Pacote `domain/protocol` |
| Δt dos registros (PID, GPS, marcador): **u16 LE sem sinal**, em ms a partir do `t_ms` inicial do bloco (0…65 535); registro anterior ao `t_ms` inicial fecha o bloco e abre outro, então `t_ms` inicial não é crescente entre blocos consecutivos e a Fase 3 agrega o tempo do boot por `min`/`max` | Fase 1 | Fase 3 | `fase-1/02` | `fase-3/03` §Pacote `domain/protocol`, `05` |
| `L` do TLV (u16 LE) = comprimento de V = `tamanho + tag_len`; `tamanho` (u16 LE do cabeçalho) = área de dados **com** o cabeçalho de 12 B; dados ≥ 17 B (marcador = 23 B). A Tabela 3 do `.tex` ("55 a 1.056") está incorreta | Fase 1 | Fase 3 | `fase-1/04` | `fase-3/03` §Pacote `domain/protocol` |
| Entrada das tags: `"MALCONG"(7) ‖ tipo(1) ‖ DevId(8) ‖ payload`; handshake = campos sem a tag, HMAC `K_auth` truncado a 16 B; bloco = os `tamanho` bytes, HMAC `K_rec` 32 B | Fase 1 | Fase 3 | `fase-1/01` | `fase-3/03` (`TagInput`) |
| Perfis `SECURITY_PROFILE=none\|hmac\|asym`: só a tag do **bloco** muda (0/32/64 B); `asym` = ECDSA P-256 determinística, `r‖s` 64 B (mbedTLS 2.28 não tem Ed25519) | Fase 1 | Fase 3, Fase 4 | `fase-1/01` | `fase-3/03`, `07`, `02` (`dongle.security_profile`); `fase-4/01`, `02` |
| Vetores de teste: gerador único `esp32-firmware/tools/provision/vectors.py` → `vectors.json` (DevId `544343454C544101`), consumido por firmware (`test/support/vectors.h`), app (`test/support/sync_vectors.dart`) e backend (`backend/internal/domain/protocol/testdata/vetores_firmware.json`) | Fase 1 | Fase 1 (app), Fase 3 | `fase-1/01` | `fase-3/03` §Pacote `domain/protocol` |
| Serviço SYNC: UUIDs `7A3C0001/0002/0003-9B1E-4F2A-8C5D-2E6F1A0B3C4D`, criados dentro de `BleConnectivity`, transporte em `BleSyncLink`; o SYNC não vai no anúncio; o único listener de GAP é o do `BleSecurity` | Fase 1 | Fase 2 | `fase-1/04` | `fase-2/01` |
| Mensagens 0x01–0x07, flags do aviso, erros da Tabela 5, regras do cursor | Fase 1 | Fase 3 (0x01–0x04 no HTTP) | `fase-1/04`, `05` | `fase-3/03`, `04`, `05` |
| Marcadores no servidor: BOOT aceito com `to = seq − 1` e `from ≤ ACK + 1` (faixa vazia ⇒ sem lacuna); LOSS aceito com `from = ACK + 1` e `from ≤ seq ≤ to`; `seq ≤ ACK` no lote = duplicado sem interromper; `seq = ACK + 1` sempre aceito. BOOT com CRC inválido vira LOSS em pedaços no dongle | Fase 3 (regras) / Fase 1 (geração) | Fase 1 / Fase 3 | `fase-3/05` §Regras de aceitação | `fase-1/03`, `05` |
| TAIL efetivo: o dongle relata `TAIL_seq = from_seq` do BOOT mais antigo (CRC válido); o servidor só declara OVERFLOW quando `TAIL_seq > seq_contiguo + 1` | Fase 1 | Fase 3 | `fase-1/05` | `fase-3/04` §Resposta (`ApplyTail`) |
| `c_S` emitido = `max(ultimo_c_s + 1, segundos Unix)`; o servidor de bancada do app usa a mesma regra; o dongle persiste `last_cs` e o *wipe* de bancada o zera | Fase 3 | Fase 1 | `fase-3/04` §Emissão do desafio | `fase-1/03`, `09` |
| Contrato HTTP: desafio e resposta em JSON com base64 (`{"desafio"}`, `{"resposta"}` → `{"ack","token","token_expira_em","token_validade_s","t_lat_s"}`); blocos e ACK de lote em `application/octet-stream` (TLVs concatenados); `X-Lote-*`; "corpo é ACK ⇔ octet-stream" | Fase 3 | Fase 1 (app) | `fase-3/03` §HTTP, `04`, `05` | `fase-1/09` (tabela de mapeamento de bytes) |
| Port do app: `SyncServerGateway` (`requestChallenge`, `submitResponse`, `postBlocks`, `latestConfirmation`), `ChallengeEnvelope`/`HandshakeEnvelope`, exceções por `codigo`; token em `SyncServerRepository`; dongle em `DongleSyncRepository`; orquestração em `HandshakeRunner`/`SyncCoordinator`. A Fase 3 entrega `HttpsSyncServerGateway` (`infra/sync/`) | Fase 1 | Fase 3 | `fase-1/07`, `09` | `fase-3/09` |
| Provisionamento: `provision.py --profile dev\|devsec\|prod` gera `DevId`/`K_auth`/`K_rec`/`k_sig`, `secnvs.csv`/`secnvs.bin` (partição `secnvs`, namespace `sec`; cifrada com uma `nvs_key` nova em `devsec`/`prod`, em claro no `dev`) e `kms_record.json` (esquema `tccelta.kms.v1`) no formato BYOK do Vault Transit (wrapping keys exportadas por `tccelta-kms provision export-wrapping`); a Fase 3 importa com `import-record`; `tccelta-kms provision new` só para dongles de teste | Fase 1 (envelope) / Fase 3 (wrapping) | Fase 3 / Fase 1 | `fase-1/01` | `fase-3/07` §Provisionamento |
| NVS: chaves na `secnvs` (namespace `sec`); estado do protocolo na `nvs` (namespace `collector`: `seq_rsv`, `last_cs`, `wipe_req`); bonds do NimBLE na `nvs` (namespace `nimble_bond`). Em `devsec`/`prod` a NVS encryption cifra `secnvs` **e** `nvs`; no `dev` fica em claro | Fase 1 | Fase 2 | `fase-1/01`, `03` | `fase-2/01` |
| Build e partições: `framework = arduino, espidf` em todos os envs; uma só `partitions_collector.csv` (tabela em `0x10000`; `nvs 0x12000/0x5000`, `nvs_key 0x17000/0x1000 encrypted`, `phy_init 0x18000/0x1000`, `secnvs 0x19000/0x3000`, `factory 0x20000/0x1D0000`, `spiffs 0x1F0000/0x200000`, `coredump 0x3F0000/0x10000`); BT do IDF em Bluedroid com o NimBLE do IDF desligado, para os `-DCONFIG_BT_NIMBLE_*` continuarem valendo | Fase 1 | Fase 2 | `fase-1/01` | `fase-2/01`, `02` |
| LESC: NUS e SYNC só para conexão autorizada (bond LESC); `APP_CONNECTED`/`BleSyncLink::onConnected` disparam na autorização | Fase 2 | Fase 1 | `fase-2/01` | `fase-1/04`, `05` |
