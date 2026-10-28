# 10 — Simulador de dongles, testes de integração e de carga

Estado implementado do simulador (`tccelta simulator`, `internal/tools/simulator/`),
dos testes do backend, do `make e2e` e da CI (`.github/workflows/backend-ci.yml`).
Descrição verificação a verificação em `backend/docs/cenarios-simulador.md`.

## Componentes

```mermaid
flowchart LR
    SC[scenarios.go<br/>e2e, negative-protocol, time, generation,<br/>reconnection, load, knee] --> APP[AppVirtual app.go<br/>contrato HTTP real]
    APP --> DV[DongleVirtual dongle.go<br/>flash em memória, assina com K_rec]
    APP -->|HTTP| API[(API real)]
    SC -->|port.Inspector: SELECT, papel owner| DB|SELECT, papel owner| DB[(PostgreSQL)]
    SC --> REP[Report report.go<br/>latências por rota, JSON]
    DV --> P[domain/protocol<br/>mesmo pacote da API]
```

- **Dongle virtual** (`DongleVirtual`): identidade e chaves do CSV (`dev_id_hex,k_auth_hex,k_rec_hex,perfil`), `seq`, `boot_id`, último ACK, flash em memória (capacidade configurável, padrão 1 milhão de blocos). Cada bloco de dados tem 61 registros (20 amostras de 3 PIDs 0x0C, 0x0D e 0x04 a 40 ms, mais 1 GPS) ⇒ 60 linhas em `reading` e 1 em `position`. Operações: `GenerateBlocks`, `LosePower` (novo BOOT com faixa perdida), `Corrupt(seq)` (vira LOSS; BOOT corrompido vira LOSS em pedaços), `DiscardSegment(n)`, flash pequena (OVERFLOW), `DriftPPM`, `InitialBoot`; processa desafio (0x01 → 0x02) e ACK (0x04) com a lógica do firmware (`ProcessChallenge`, `ProcessAck`, `ReadForSending`).
- **App virtual** (`AppVirtual`): `RequestChallenge`, `SendResponse`, `Handshake` (até 3 tentativas se a âncora falhar), `SendBatch`, `Drain` (lote `K` começa em 32; dobra com progresso; metade em 429/503 respeitando `Retry-After` até 3 s; 401 refaz o handshake; renova o token a menos de 5 min; 3 lotes sem progresso viram erro).
- **Relatório** (`--report`): `scenario`, `params`, `duration_s`, `requests_per_s`, `routes.<rota>` (`n`, `p50_ms`, `p95_ms`, `p99_ms`, `status`), extras (`dongles`, `responses_5xx`, `drain_s`, `step_<n>`, `dongle_capacity`).

## Cenários

| Cenário | O que prova | Falha quando |
|---|---|---|
| `e2e` | por dongle: handshake; 200 blocos; queda de energia (BOOT); bloco corrompido (LOSS); offline com flash cheia (OVERFLOW); no último dongle, OVERFLOW que engole o segmento antes do BOOT e BOOT corrompido | alguma das 6 verificações falha: `contiguous_seq`/`head_seq` = HEAD do dongle; lacunas BOOT, LOSS e OVERFLOW presentes; contagem **exata** de `reading`/`position`; `rejected_block` vazio; ≥ 1 âncora; boot com tempo calculado |
| `negative-protocol` | tag adulterada, salto, lote de 33, token de outro dongle, resposta repetida (409), rajada de desafios (429), token lixo (401); dados já aceitos não mudam | alguma das 11 verificações falha |
| `time` | 4 rodadas ⇒ 4 âncoras; `time_source = anchors`; `base_uncertainty_ms ≤ 2000`; hora reconstruída a menos de 10 s do relógio (tempo real; `--speedup` não afeta) | alguma das 4 verificações |
| `generation` | `boot_id` de 65534 a 2 (volta do u16): `boot_generation = 1`, pares (geração, boot) esperados, alerta `boot_generation_advanced` uma vez, `reprocess` preserva as leituras | alguma das 4 verificações |
| `reconnection` | `--accumulated` blocos (500) offline por dongle drenados por inteiro, com rampa `--ramp` | algum dongle termina com `contiguous_seq` ≠ HEAD |
| `load` | `N` dongles em regime por `--duration`; latências por rota | só com resposta 5xx |
| `knee` | degraus de 100, 250, 500, 1000, 2000 e 4000 dongles; para quando p99 de `POST /blocos` > 1000 ms ou 503 > 1 % | nunca falha; reporta `dongle_capacity` |

Opções: `--scenario`, `--api` (`http://api:8080`), `--keys` (CSV), `--dongles`, `--duration` (2m), `--speedup`, `--tlat` (60s), `--accumulated` (500), `--ramp` (30s), `--db-url-env` (`SIM_BANCO_URL`), `--report`, `--drift`.

Todos os dongles partem do seq 1; o servidor guarda o estado por DevId, então cada execução usa dongles novos (`make cenario` e `make e2e` os criam: KMS primeiro, banco depois, CSV em `saida/dongles-<nome>.csv`). `load` e `knee` saem de um único IP e sofrem o limite por IP da API (`LIMITE_IP_MIN`).

## `make e2e`

Sobe a stack isolada `compose.test.yaml` (projeto `tccelta-teste`), reconstrói as imagens `ferramentas` e `kms-ferramentas` e roda `e2e`, `negative-protocol`, `time --speedup 120` e `generation`, cada um com 3 dongles novos; derruba a stack com `down -v` ao fim e devolve o código do primeiro cenário que falhou. Não cobre `reconnection`, `load` nem `knee`.

## Testes do backend

`go test -race ./...` (`make teste`, em contêiner) cobre, por pacote, `domain/{protocol,pid,handshake,ingestion,timeline}`, `usecase/{handshake,ingestion,session,catalog,boottime,reprocess,provisioning}` (com `adapter/memory` e `port/porttest`), `api`, `http/ratelimit`, `adapter/{jwt,kmsclient}`, `config`, `platform/breaker`, `tools/provisioning`, `kms/{server,vault,provisioning,config}`. Vetores: `domain/protocol/testdata/vetores.json` (gerado por `scripts/gerar_vetores.py`) e `vetores_firmware.json` (do `vectors.py` da Fase 1). Não há testes dos adaptadores `postgres`/`database` nem do loop do `worker` (a lógica está em `usecase/boottime`).

## CI (`.github/workflows/backend-ci.yml`, `working-directory: backend`)

| Job | Passos |
|---|---|
| `lint` | `gofmt`, `go vet`, `sqlc diff` (sqlc 1.27.0), `golangci-lint` |
| teste | `go test -race ./...` |
| `integracao` | `go test -p 1 -tags integration -race ./...` (nenhum arquivo tem a tag; reexecuta os testes de unidade) |
| `e2e` | `make e2e` |
| `vuln` | `govulncheck ./...` |

## Metas de carga (documentadas, não medidas)

M1: 1 000 dongles a 0,17 bloco/s, p99 `POST /blocos` < 300 ms, 0 respostas 5xx. M2: o mesmo com `T_lat` = 10 s. M3: reconexão de 200 dongles × 2 000 blocos drenada em < 10 min. M4: `knee` reporta a capacidade. M5: p95 do handshake < 150 ms. M6: `tccelta_kms_calls_total / tccelta_batches_total ≤ 2` e ≥ 95 % dos lotes sem chamada ao KMS dentro da transação (`phase="tx"`). `make carga [N=]` cria `N` dongles virtuais e roda `load` com relatório em `saida/relatorio.json`; nenhuma meta foi medida.
