# Fase 3 — Servidor (API Go + PostgreSQL + KMS) e integração HTTPS do app

Plano de implementação do lado servidor do TCCELTA e da ponte HTTPS no app, escrito para ser executado sem reler os documentos de origem. Ele se baseia em:

- `latex/aux-docs/seguranca-dongle-server/segunraca-dongle-server.tex`: seções 4 (chaves, KMS, handshake, HMAC), 5 (mensagens, Tabela 3, Tabela 5, buffer) e 6 (implementação do servidor);
- `latex/aux-docs/server-implementation/server-specification.md`: DDL de referência, tempo absoluto, worker, implantação;
- PlantUML em `latex/assets/aux-docs/plantuml/`: `ingestao-lote-servidor`, `handshake-servidor-{desafio,resposta,worker}`, `kms-servidor-app`, `modelo-dados-{bronze,contexto,silver}`, `handshake-sequencia`, `aquisicao-lote-sequencia`;
- `Investigação-3.pdf`: Cristian (p. 6–7), registros (p. 9), formato em blocos (p. 16);
- `esp32-firmware/src/collector/BlockFormat.h` e `BlockBuilder.cpp` (layout real, little-endian);
- `mobile-app/CLAUDE.md` (arquitetura do app).

## Objetivo

Colocar no ar, **sempre via Docker**, o servidor que:

1. responde ao handshake (desafio 0x01 → resposta 0x02 → ACK 0x04 + JWT), com âncora de tempo de Cristian;
2. ingere lotes de blocos 0x03 autenticados por HMAC `K_rec`, em Go-Back-N estrito com marcadores BOOT/LOSS, gravando bronze e silver numa transação por lote;
3. guarda `K_auth`/`K_rec` num KMS (Vault Transit) que deixa a API gerar MAC só com `K_auth` e **apenas verificar** com `K_rec`;
4. recalcula, num worker assíncrono, a hora real de cada boot.

Do lado do app Flutter, a fase implementa a ponte HTTPS que substitui o *stub* deixado pela Fase 1.

## Entregas

| Entrega | Onde |
|---|---|
| Pasta `backend/` (módulo Go `tccelta/backend`, binário único `tccelta`) | raiz do monorepo |
| Docker Compose dev e prodlike, Redis (rate limit), Caddy, túnel com certificado público (Cloudflare Tunnel), Vault dev e Raft, serviço `jwt-rotacao`, Makefile | `backend/compose*.yaml`, `backend/deploy/`, `backend/Makefile` |
| Migrations goose (bronze, contexto, silver, cadastro, rejeitados, alertas, permissões) | `backend/internal/banco/migrations/` |
| API: `POST /v1/dongles/{id}/desafio`, `POST /v1/dongles/{id}/desafio/resposta`, `POST /v1/dongles/blocos`, `GET /v1/dongles/{id}/confirmacao` | `backend/internal/httpapi`, `handshake`, `ingestao` |
| Pacote `protocolo` + vetores de teste (Python independente + firmware) | `backend/internal/protocolo/` |
| Worker de tempo + expiração de desafios | `backend/internal/worker`, `tempo` |
| KMS: políticas, AppRole, bootstrap, rotação automática da chave do JWT, provisionamento de dongles (imagem NVS direta + import BYOK) | `backend/deploy/vault/`, `backend/internal/kms`, `provisionamento` |
| Reprocessamento de `leitura`/`posicao` a partir de `bloco_bruto` | `backend/internal/ingestao/reprocessar.go` (`tccelta worker reprocessar`) |
| Simulador de dongles, testes de integração, e2e, CI; testes de carga **documentados (execução adiada)** | `backend/internal/simulador`, `testeinfra`, `.github/workflows/backend-ci.yml` |
| App: ambiente/URL, `dio`, DTOs, datasource → repository (port da Fase 1) → use cases, gatilhos, reenvio | `mobile-app/lib/src/...` (spec 09) |

## Ordem das specs e dependências

| # | Spec | Depende de | Pode rodar em paralelo com |
|---|---|---|---|
| 01 | [Infraestrutura Docker Compose](01-infra-docker-compose.md) | — | — |
| 02 | [Banco e migrations](02-banco-migrations.md) | 01 | 03 (pacote `protocolo`) |
| 03 | [Arquitetura da API Go e pacote `protocolo`](03-api-go-arquitetura.md) | 01, 02 (sqlc) | 02, 07 |
| 07 | [KMS Vault](07-kms-vault.md) | 01, 02 | 03 |
| 08 | [JWT e autenticação](08-jwt-e-autenticacao-app.md) | 03, 07 | 04 |
| 04 | [Endpoints do handshake](04-endpoints-handshake.md) | 02, 03, 07, 08 | 05 (depois do estado comum) |
| 05 | [Ingestão de blocos](05-endpoint-ingestao-blocos.md) | 02, 03, 04, 07, 08 | 06 |
| 06 | [Worker assíncrono](06-worker-assincrono.md) | 02, 04, 05 | 09 |
| 09 | [App: integração HTTPS](09-app-integracao-https.md) | 03/04/05/08 (contratos), **Fase 1** | 06, 10 |
| 10 | [Simulador e testes de carga](10-simulador-e-testes-de-carga.md) | 03–07 | 09 |

Ordem sugerida de execução: **01 → 02 + 03 (protocolo) → 07 (checkpoint dourado do Vault) → 08 → 04 → 05 → 06 → 10 → 09**. O app (09) pode começar assim que os contratos HTTP estiverem estáveis (fim da 04/05), usando o simulador como servidor de verdade.

### Relação com as outras fases

- **Fase 1 (em paralelo)** define o firmware (HMAC, CRC, NVS, SYNC BLE, handshake no dongle) e o app até o *molde* do gatilho de handshake. Esta fase **não** redefine nada do lado BLE: ela implementa a ponte HTTPS atrás do port deixado pela Fase 1 e consome o material de provisionamento.
- **Fase 2 (LESC)** não toca o servidor.
- **Perfis de segurança da Fase 1** (`SECURITY_PROFILE=none|hmac|asym`): o servidor implementa `hmac`, aceita `none` só em bancada (`PERMITIR_PERFIL_NONE=true`, nunca em `producao`) e deixa `asym` para **depois do caminho HMAC estável** (decisão do autor; spec 07), sem afetar o caminho HMAC. **Reconciliado:** o perfil só muda a tag do bloco (0/32/64 B); a tag do handshake é HMAC `K_auth` de 16 B nos três perfis; `asym` = ECDSA P-256 determinística, `r‖s` 64 B.

### Contratos cruzados com a Fase 1 (reconciliados pelo orquestrador; ver `.spec/README.md`)

| Contrato | Valor definido aqui | Spec |
|---|---|---|
| Ordem de bytes | todos os inteiros little-endian, **exceto** os 2 bytes de valor do PID, que são `A`, `B` crus da resposta OBD-II (bruto = `A` ou `256·A + B`) | 03 |
| Envelope trocado com o servidor | TLV completo `T(1) \| L(u16 LE) \| V`, idêntico ao do SYNC sem o byte de controle | 03, 09 |
| `L` de cada tipo | 0x01: 45/47; 0x02: 50; 0x03: `tamanho + 32`; 0x04: 25 | 03 |
| `tamanho` do bloco | total da área de dados **com** o cabeçalho de 12 B (como no `BlockFormat.h`), 17..1024 | 03 |
| Entrada do HMAC | `"MALCONG"`(7 B ASCII) `\|\| tipo(1) \|\| DevId(8 B crus) \|\| payload`; tag = 16 primeiros bytes (K_auth) ou 32 B (K_rec, sobre os `tamanho` bytes de dados) | 03 |
| Marcadores | BOOT 0xFE / LOSS 0xFD, 11 B, registro único, `tamanho = 23`; LOSS com `boot_id = 0`, `t0 = 0` | 03, 05 |
| Aceitação de marcadores no servidor | BOOT: `to = seq − 1` e `from ≤ ACK+1` (estende o doc para BOOT após OVERFLOW; BOOT de faixa vazia aceito sem lacuna); LOSS: `from = ACK+1` e `from ≤ seq ≤ to` (estende o doc). **Confirmado com a Fase 1:** BOOT com CRC inválido vira LOSS em pedaços (`[ACK+1, seq−1]` e `[seq, seq]`), aceitos por essa regra; blocos `seq ≤ ACK` no lote são duplicados sem interromper; `seq = ACK+1` sempre aceito | 05 |
| Tamanho mínimo do bloco | `tamanho ≥ 17` (1 leitura de PID: V = 49 B) nas duas fases. A Tabela 3 do `.tex` diz "55 a 1.056" e está incorreta (55 seria o marcador: 23 + 32; o mínimo real é 49) | 03 |
| `c_S` emitido | `c_S = max(ultimo_c_s + 1, segundos Unix)`: o servidor de bancada do app (Fase 1) usa a mesma regra, e o dongle persiste `last_cs` | 04 |
| Dongle com contadores zerados (NVS/flash apagadas) | é tratado como **novo DevId** (reprovisionar com o `provision.py` da Fase 1 + `importar-registro`): o servidor recusa `HEAD_seq` menor que o salvo, como manda o doc | 04 |
| Vetores de teste | fonte única: `esp32-firmware/tools/provision/vectors.py` → `vectors.json` da Fase 1 (DevId `544343454C544101`), copiado para `testdata/vetores_firmware.json`; `vetores.json` (Python desta fase) é a segunda implementação independente | 03 |
| Material de provisionamento | dongle real: `provision.py` (Fase 1) gera `secnvs.csv`/`secnvs.bin` (partição `secnvs`, namespace `sec`) e o envelope `kms_record.json` (BYOK do Transit), que `tccelta provisionar importar-registro` consome; `tccelta provisionar novo` só para dongles de teste e **grava a imagem `secnvs.bin` direto** (o CSV existe só na memória e passa por stdin ao gerador da Fase 1; `--hw dev` em claro, plano B1, ou `devsec` cifrada) | 07 |
| Δt dos registros | `u16` sem sinal (decisão da Fase 1); `t0` pode recuar entre blocos consecutivos | 03, 05 |
| Identidade do boot | `(dongle_id, geracao_boot, boot_id)`: a volta do `boot_id` (u16) abre nova geração | 02, 04, 05 |
| Port no app | nomes da Fase 1: `SyncServerGateway` (`requestChallenge`, `submitResponse`, `postBlocks`, `latestConfirmation`; esta fase implementa `HttpsSyncServerGateway` em `infra/sync/`), token em `SyncServerRepository` (`needsHandshake`/`clearToken`), port do dongle `DongleSyncRepository`, orquestração em `HandshakeRunner`/`SyncCoordinator` | 09 |
| Ações do app por resposta | `X-Lote-Acao: nenhuma \| reler (Erro 0x07 ao dongle) \| aguardar`; tabela de gatilhos de handshake | 05, 09 |

## Principais decisões (detalhes e justificativas nas specs)

| Decisão | Spec |
|---|---|
| Binário único com subcomandos; worker sem acesso ao Vault; redes `borda`/`dados`/`kms` separadas | 01 |
| `goose` (SQL embutido, um arquivo por migration, lido pelo sqlc) | 02 |
| `bloco_bruto` como tabela comum (não hypertable) com PK `(dongle_id, seq)`, **particionada por hash de `dongle_id` (16 partições) desde já**, append-only por `GRANT` + trigger; `leitura`/`posicao` como hypertables por `recebido_em` (1 dia, compressão aos 7 dias) | 02 |
| `boot` identificado por `(dongle_id, geracao_boot, boot_id)` desde já; `boot.config` sempre nula; `dongle.t_lat_s` criada mas sem uso; grupos de âncoras persistidos em `grupo_ancora` | 02, 04, 06 |
| Rate limit global entre réplicas em **Redis** (com *fallback* local); HTTP/3 não habilitado | 01, 03 |
| Caminho padrão do app: **túnel com certificado público** (Cloudflare Tunnel), CA do Caddy como plano B offline; Vault com unseal manual 3 de 5 em produção; backups só documentados | 01, 09 |
| `ServeMux` da stdlib; `pgx/v5` + sqlc; `slog`; Prometheus; geography por EWKB em COPY binário | 03 |
| Handshake em JSON (bytes em base64, porque leva JWT e metadados); blocos e ACK de lote em `octet-stream` puro | 03, 04, 05 |
| Pré-checagem barata do desafio antes de chamar o KMS; consistência sob o advisory lock; JWT assinado fora e ACK dentro da transação | 04 |
| Lote interrompido = 200 + ACK; 422 só sem progresso por tag inválida; `X-Lote-Acao` orienta o buffer do app | 05 |
| Decodificação **síncrona** na transação do lote (como o doc); modo assíncrono desenhado em [Aberto] | 05 |
| KMS: `verify` com `batch_input` de todo o lote em paralelo com a pré-assinatura do ACK provável (1 ida e volta, 0 chamadas sob o lock em regime), duplicados pulados, ACK reaproveitado | 05, 07 |
| **Sem saída manual** para fila travada por bloco autêntico (`admin saltar` descartado): a fila do dongle fica travada até corrigir firmware/chave; nenhuma perda é declarada sem prova do dongle | 05 |
| Reprocessamento: `tccelta worker reprocessar` (ensaio por padrão, `--aplicar` refaz `leitura`/`posicao` do bronze, inclusive em chunks comprimidos) | 05 |
| Rotação da chave do JWT automática (`jwt-rotacao`, credencial própria; expurgo coordenado com a validade do token) | 07 |
| Mounts `transit-auth`/`transit-rec`/`transit-jwt`, chaves tipo `hmac` importadas por BYOK; `deny` explícito em tudo que geraria, exportaria ou reconfiguraria `K_rec` | 07 |
| JWT EdDSA (Ed25519 no Transit), 1 h, validação local, sem revogação | 08 |
| Grupos de âncoras com janela contada da primeira âncora; vizinhos = boots ancorados mais próximos; limite superior extra pelo `recebido_em` | 06 |
| App: token só em memória (o handshake é obrigatório a cada conexão BLE); expiração pelo relógio monotônico + `token_validade_s` | 09 |

## Critérios de aceite da fase

- [ ] `make up` (dev) e `make up-prodlike` sobem a stack do zero só com Docker; healthchecks verdes.
- [ ] Teste dourado do Vault: HMAC do Transit com a chave importada = HMAC local. O token da API **não** consegue gerar, exportar nem reconfigurar `K_rec`.
- [ ] O pacote `protocolo` passa nos vetores Python e, quando entregues, nos do firmware.
- [ ] Handshake completo via HTTP: `c_S` crescente, desafio de uso único, regra do TAIL (OVERFLOW), âncora só com S2 − S1 ≤ 2 s, `flags_a` bit 0 quando não houver âncora, JWT com `jti` gravado.
- [ ] Ingestão: Go-Back-N estrito, BOOT/LOSS pelas regras do doc, uma transação e ≤ 2 chamadas ao KMS por lote, idempotente no reenvio.
- [ ] Worker: reproduz o exemplo numérico do doc (±200 ms); boots sem âncora ganham janela por vizinhos; alertas de deriva e interseção vazia.
- [ ] `make e2e` verde (BOOT, LOSS, OVERFLOW, âncora, vizinhos, rejeição, duplicados, reconexão).
- [ ] Metas de carga M1–M6 (spec 10) documentadas e cenários de carga implementados; **execução e relatório adiados** (decisão do autor).
- [ ] App: os bytes servidor ⇄ SYNC são preservados; gatilhos e política de reenvio cobertos por testes; `format`/`analyze`/`test` limpos.
- [ ] Roteiro ponta a ponta abaixo executado com o simulador **e** com um dongle real.

## Roteiro de validação ponta a ponta

### A. Simulador (sem hardware)

1. `cd backend && make up` (sobe banco, Vault dev, bootstrap, migrations, API e worker).
2. `make seed-dev` (importa os 3 dongles de teste no Vault e no banco e roda o teste dourado).
3. `make e2e`. Saída esperada: todas as verificações `OK` e código 0.
4. Inspeção manual (`make psql`):

   ```sql
   SELECT d.dev_id_hex, e.seq_contiguo, e.head_seq, e.c_conf FROM dongle d JOIN dongle_estado e USING (dongle_id);
   SELECT causa, from_seq, to_seq FROM lacuna ORDER BY dongle_id, from_seq;
   SELECT dongle_id, geracao_boot, boot_id, origem_tempo, round(b*1e6, 1) AS ppm, incerteza_base_ms FROM boot ORDER BY 1, 2, 3;
   SELECT count(*) FROM leitura; SELECT count(*) FROM posicao;
   ```
5. Prometheus (`--profile observabilidade`): `tccelta_blocos_total{resultado="aceito"}` cresce; `tccelta_kms_chamadas_total / tccelta_lotes_total ≤ 2`.

### B. App com servidor real e dongle simulado no lado do port (sem BLE)

1. **Caminho padrão:** `make up-tunel` (Cloudflare Tunnel; `TUNEL_HOSTNAME` e o token do túnel em `deploy/segredos/cloudflared_token`) — certificado público, **nada a instalar no celular**. *Plano B offline (LAN):* `make up-tls API_HOST=<IP da LAN>` → `make ca-exportar` → instalar `saida/caddy-root.crt` no aparelho.
2. Rodar o app com `--dart-define-from-file=env/dev.json` apontando para `https://<TUNEL_HOSTNAME>` (plano B: `env/lan.json`, `https://<IP>:8443`), usando o `FakeDongle` da Fase 1 (`test/support/fake_dongle.dart`, implementa `DongleSyncRepository`) com as chaves de teste.
3. Esperado: handshake 200, lotes 200 completos e `seq_contiguo` subindo no banco.

### C. Dongle real ⇒ app ⇒ API ⇒ banco

1. `make kms-wrapping` → na estação, `provision.py --profile dev|devsec --kms-wrapping kms_wrapping.json --flash` (Fase 1, spec 01 §11; no perfil `prod` não há `--flash`: a gravação segue o procedimento da spec 01 §10.3) → `make kms-importar ARQ=out/<hex>/kms_record.json`. (Alternativa só de bancada: `make provisionar N=1` → `saida/<hex>/secnvs.bin` (imagem pronta, sem CSV em disco) → gravar com `esptool.py write_flash 0x19000` → `shred -u` no `bench_keys.json`.)
2. Dongle no carro ou na bancada com o simulador de ECU (`simulador/`); app pareado (LESC, Fase 2) e com o SYNC da Fase 1.
3. Ligar o dongle. Esperado:
   - o aviso 0x07 tem bit 0/1 ⇒ o app faz o handshake;
   - `desafio.resultado = 'aceito'` e uma linha em `ancora_tempo` (se o RTT ≤ 2 s);
   - `boot.origem_tempo = 'ancoras'` em segundos.
4. Dirigir ou simular 10 min. Esperado: lotes a cada `T_lat`; `leitura` com os 7 PIDs; `posicao` com GPS do app; `bloco_rejeitado` vazio.
5. Tirar a energia do dongle e religar. Esperado: lacuna `BOOT` (ou BOOT sem furo) e novo boot com âncora.
6. Desligar a internet do celular por 15 min e religar. Esperado: drenagem com lotes de 32 e nenhuma perda (`seq_contiguo = HEAD`).
7. Conferir o relógio: `SELECT to_timestamp((a_ms + (1+b)*t_ms)/1000) …` de uma leitura recente contra o relógio do celular (diferença dentro de `incerteza_base_ms` + RTT).

**Pré-requisito em todos os roteiros:** o host da API precisa de NTP ativo (`w32tm /query /status` no Windows; `timedatectl` no VPS), porque S1/S2 vêm do relógio da API.

## Mapa de pastas de `backend/`

```
backend/
├── cmd/tccelta/                    # main + subcomandos: api, worker (e worker reprocessar), migrar, provisionar, jwt (rotacionar, agendar), simulador, saude
├── internal/
│   ├── config/                     # env vars (+ *_ARQUIVO para segredos)
│   ├── versao/
│   ├── protocolo/                  # TLV, 0x01–0x04, bloco, registros, entrada do HMAC, perfis
│   │   └── testdata/               # vetores.json (Python), vetores_firmware.json (Fase 1)
│   ├── banco/
│   │   ├── migrations/             # 00001…00008 (goose, embutidas)
│   │   ├── queries/                # *.sql (fonte do sqlc)
│   │   ├── consultas/              # GERADO pelo sqlc
│   │   ├── pool.go  tx.go  escuta.go  geo.go  migracoes.go
│   ├── dongles/                    # cache DevId → dongle
│   ├── pids/                       # catálogo de PIDs + LISTEN pid_alterado
│   ├── kms/                        # interfaces + Vault (+ rotacao.go do JWT, + kmsteste/ para testes)
│   ├── token/                      # JWT: emissor e validador
│   ├── handshake/                  # desafio e resposta
│   ├── ingestao/                   # lote, plano, aplicação, decodificador
│   ├── tempo/                      # âncoras, reta, vizinhos, desembrulhar t_ms
│   ├── worker/                     # laço LISTEN/SKIP LOCKED, expiração
│   ├── httpapi/                    # rotas, handlers, middlewares, problem+json
│   ├── limites/                    # rate limit (local + Redis), semáforo
│   ├── metricas/
│   ├── provisionamento/            # DevId/chaves, BYOK (RSA-OAEP + AES-KWP), material NVS
│   ├── simulador/                  # dongle virtual + app virtual + cenários
│   └── testeinfra/                 # testcontainers (build tag integracao)
├── deploy/
│   ├── caddy/Caddyfile
│   ├── postgres/init/00-papeis.sh
│   ├── vault/{config/vault.hcl, politicas/*.hcl, scripts/{bootstrap,init-unseal}.sh}
│   ├── kms/compose.kms.yaml        # Vault em outro host (cenário VPN)
│   ├── observabilidade/prometheus.yml
│   ├── segredos/   (gitignored)    # senhas, token dev
│   └── certs/      (gitignored)    # CA privada, certificado do Vault
├── scripts/                        # gerar-segredos.sh, gerar-certs.sh, gerar_vetores.py
├── testdata/dongles-teste.csv      # 3 dongles de TESTE (chaves públicas por definição)
├── docs/relatorios/                # relatórios de carga
├── saida/          (gitignored)    # material de provisionamento, CA do Caddy exportada
├── compose.yaml  compose.prodlike.yaml  compose.test.yaml
├── Dockerfile  .dockerignore  .env.example  .gitignore  .golangci.yml
├── Makefile  sqlc.yaml  go.mod  go.sum
```

## Pontos em aberto e decisões do autor (consolidados)

Todos os pontos que estavam em aberto foram decididos pelo autor (painel de decisões); a coluna "Decisão" é o que vale. Só ficam em aberto a decodificação assíncrona (só se a carga, quando medida, exigir) e o `flutter_secure_storage` do token.

| Ponto | Decisão | Spec |
|---|---|---|
| Auto-unseal e HA do Vault | **Unseal manual 3 de 5 em produção; auto-unseal e HA só documentados** | 01, 07 |
| Backup do banco e do Vault | **Só documentar** pgBackRest e `vault operator raft snapshot` | 01 |
| Túnel com certificado público | **Cloudflare Tunnel (ou similar) desde já**; CA do Caddy como plano B | 01, 09 |
| Particionar `bloco_bruto` | **Por hash de `dongle_id` (16 partições) já** | 02 |
| Encadeamento por hash dos blocos (AD5) | **Fora do escopo mínimo, só documentado** | 02 |
| `boot.config` não vem no protocolo | **Sempre nula** (a Fase 1 não inclui registro de configuração) | 02 |
| Volta do `boot_id` (u16) | **`geracao_boot` na PK já** | 02, 04, 05, 06 |
| Camada gold | **Fase posterior**; só o gancho `pendente_gold` | 02, 06 |
| Modo diagnóstico (`flags_c` bit 0) | **Adiado**: formato suportado, emissão desligada (`flags_c = 0`) | 04 |
| `T_lat` por dongle | **Coluna `dongle.t_lat_s` já**, mas o servidor não a usa (handshake segue com `T_LAT_SUGERIDO` global) | 02, 04 |
| `t_lat_s` sugerido ao app | **Teto `min(local, servidor)`** (aplicado pelo app, Fase 1) | 09 |
| Fila travada por bloco autêntico | **Sem saída manual** (`admin saltar` descartado) | 05 |
| Reprocessamento de `leitura` | **`tccelta worker reprocessar` implementado nesta fase** | 05 |
| Limite de âncoras por boot | **Sem limite**; limitar só se a fila crescer | 06 |
| Persistir os grupos de âncoras | **Persistir** (`grupo_ancora`) | 02, 06 |
| Calibrar `DERIVA_MAX_PPM` | **Manter 40 ppm sem medir**; procedimento de medição documentado | 06, 10 |
| Perfil `asym` no servidor | **Depois do caminho HMAC estável** (ECDSA P-256, verificação local) | 07 |
| NVS cifrada na ferramenta `novo` | **Sim, junto com a Fase 1** (imagem direta, CSV só em memória) | 07 |
| Rotação da chave do JWT | **Automatizada** (`jwt-rotacao`) | 07, 08 |
| Rate limit distribuído | **Redis**, com *fallback* local | 01, 03 |
| HTTP/3 no Caddy | **Não habilitar** | 01, 03 |
| Identidade de usuário do app | **Fora do escopo do TCC** | 08 |
| URL base em Configurações | **Só se trocar o IP virar atrito** | 09 |
| Simulador próprio × k6/vegeta | **Simulador próprio em Go**; testes de carga só documentados | 10 |
| Decodificação assíncrona | em aberto: só se o teste de carga (adiado) exigir; desenho pronto | 05 |
| Token em `flutter_secure_storage` | em aberto: só se o buffer for persistido | 09 |

## Revisão crítica (advisor) e o que mudou

Um arquiteto backend sênior (subagente revisor) confrontou o rascunho com o `.tex`, o `server-specification.md`, os PlantUML e o firmware (`BlockFormat.h`, `BlockBuilder.cpp`, `LittleFsBlockStore.cpp`). Ele confirmou como corretos e **inalterados**:

- formatos byte a byte;
- entrada do HMAC e truncamento;
- regra do TAIL/OVERFLOW;
- disciplina de locks (advisory por dongle, `UPDATE … RETURNING`, consistência sob o lock);
- idempotência;
- premissa do tipo de chave `hmac` importada por BYOK e formato de wrapping;
- política "só verify" para `K_rec` (com a ressalva abaixo);
- decisões do app.

Achados incorporados:

| # | Severidade | Achado | Mudança | Spec |
|---|---|---|---|---|
| 1 | Crítico | BOOT após OVERFLOW (o firmware descarta flash por segmento) seria `marcador_invalido` para sempre, travando a fila | BOOT aceito com `to = seq − 1` e `from ≤ exp`; lacuna só na parte não coberta; casos de teste 16–17 e cenário e2e | 05, 10 |
| 2 | Alto | Montar o diretório `/docker-entrypoint-initdb.d` escondia os scripts de init da `timescaledb-ha` | Monta só `900-papeis.sh`; `shared_preload_libraries` explícito; checkpoint de inspeção da imagem | 01 |
| 3 | Alto | Deadlock possível worker × API: o `UPDATE` de vizinhos trava várias linhas de `boot` | TX2 do worker toma o mesmo `pg_advisory_xact_lock` do dongle; `EmTransacao` repete 1× em `40P01`/`40001` e depois 503 | 03, 04, 05, 06 |
| 4 | Alto | Sem saída para fila travada por bloco autêntico (malformado, chave errada, BOOT corrompido) | Proposta original: `tccelta admin saltar` (lacuna `OPERADOR`) — **descartada pelo autor** (sem saída manual; a fila do dongle fica travada até corrigir firmware/chave). Mantido: LOSS estendido (`from ≤ seq ≤ to`) + pedido à Fase 1 para BOOT corrompido | 02, 05, 09 |
| 5 | Médio | A premissa "deny vence qualquer allow futuro" está errada (no Vault, vence o caminho mais específico) | Nota corrigida; a garantia vem de política única + testes negativos no CI; `denied_parameters`/`allowed_parameters` no import do provisionador | 07 |
| 6 | Médio | O token do provisionador ficava no mesmo volume montado na API | Volumes `vault_approle_api` e `vault_approle_ferramentas` separados; teste de isolamento | 01, 07 |
| 7 | Médio | `maior_seq_visto` ficava morto (sempre igual a `seq_contiguo`) | O lote inteiro (acima do snapshot) é verificado num só `batch_input`; `maior_seq_visto`/`head_seq` refletem blocos autênticos acima do furo; some o motivo `concorrencia`; todas as tags inválidas vão para rejeitados | 05 |
| 8 | Médio | Mínimos quadrados com grupos próximos dava `b` absurdo e alerta falso | `b ≠ 0` só com span ≥ 1 h e `|b| > σ_b`; alerta só se `|b| − 2σ_b > 40 ppm`; teste novo | 06 |
| 9 | Médio | `t_ms` das âncoras não era desembrulhado como o das leituras | Handshake lê a referência do boot sob o lock e grava o `t_ms` desembrulhado | 04, 06 |
| 10 | Médio | Assinatura do ACK no Vault dentro da transação, segurando lock e conexão | ACK provável pré-assinado em paralelo com o `verify`; reassina na transação só se divergir; métrica `fase=pre/tx` | 03, 05, 10 |
| 11 | Médio | Itens que quebravam `make up`/`e2e`/integração | alvo `env`; caminho de `migracao-nova`; `TESTCONTAINERS_HOST_OVERRIDE` + `-p 1` + `MSYS_NO_PATHCONV`; variáveis e segredo do provisionador no `ferramentas`; `e2e` unificado com `compose.test.yaml`; golangci-lint ≥ v1.64; goose `NewProvider` + `WithSessionLocker`; espera no `init-unseal.sh`; variável inócua removida | 01, 02, 10 |
| 12 | Baixo | Semáforo antes do JWT permitia que tráfego sem token esgotasse as vagas | Ordem `limitarIP → JWT → DevId → semáforo`; `limitarIP` na rota de blocos | 03 |
| 13 | Baixo | Armadilhas do JWT (`kid` não string, `cs` como float) | *Type assertion* segura; `json.Number`; signing string explicitada | 08 |
| 14 | Baixo | sqlc/pgx: `unnest` múltiplo, `ALTER … SET (compress)`, codec binário de `geography` no `CopyFrom` | Alternativas `ROWS FROM`/`DO $$`; teste com `CopyFrom` real | 02, 03 |
| 15 | Baixo | `X-Real-IP` ignorado com sub-rede dinâmica ⇒ rate limit global por IP | Sub-rede fixa da `borda` + `PROXIES_CONFIAVEIS` | 01, 03 |
| 16 | Baixo | Dongle com contadores zerados ⇒ 422 eterno sem procedimento | Documentado: novo DevId | 04, README |
| 17 | Baixo | 422 de lote com corpo binário, diferente dos demais 422 | Regra "o corpo é ACK ⇔ `octet-stream`" na API e no app | 03, 05, 09 |
| 18 | Baixo | Tamanho mínimo do bloco diverge da Tabela 3 | Registrado como contrato cruzado | README |
| 19 | Baixo | Volume de `leitura` sem compressão enche o disco no teste de carga | Intervalo de compressão configurável; carga roda com 1 dia | 02, 10 |

Desvios conscientes em relação ao doc, todos justificados nas specs:

1. handshake em JSON + base64;
2. consistência checada sob o lock;
3. regras estendidas de BOOT/LOSS;
4. limite superior de tempo por `recebido_em`;
5. `b = 0` quando o ajuste não é significativo;
6. grupos de âncoras persistidos (`grupo_ancora`) e `geracao_boot` na identidade do boot (escolhas do autor).
