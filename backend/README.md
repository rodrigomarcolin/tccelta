# Backend TCCELTA: guia do Makefile

O `backend/` é a API Go (`tccelta`), o worker e as ferramentas de linha de comando, e também o serviço KMS
(`tccelta-kms`), que guarda as chaves dos dongles e fala com o Vault. É um único módulo Go, mas o KMS é outro
binário, outra imagem e outro processo ([`docs/kms.md`](docs/kms.md)). A API só conversa com o KMS por HTTP
(contrato em [`docs/kms-http-contract.md`](docs/kms-http-contract.md)).

O que cada cenário do simulador e cada verificação do `make e2e` fazem está em
[`docs/cenarios-simulador.md`](docs/cenarios-simulador.md).

Este documento descreve o `Makefile` como ele está hoje. Tudo foi escrito a partir do próprio `Makefile`,
dos `compose*.yaml`, dos scripts e do código dos binários, e **não** a partir das specs em `.spec/`, que
ainda descrevem o desenho anterior (a API falando direto com o Vault).

> **Estado de verificação.** Os alvos abaixo foram lidos, mas não foram reexecutados depois do refactor em
> camadas e da incorporação do KMS ao `backend/` (a máquina em que o refactor foi feito não tinha o daemon do Docker).
> Antes do refactor, a stack subiu e os cenários do simulador passaram. Depois dele, o autor do projeto
> executou `make up`, `make seed-dev` e `make e2e`; o `make lint` (regra `depguard` nova) e os cenários
> `reconnection`, `load` e `knee` ainda não foram executados.

## Estrutura do código

Um módulo Go (`tccelta/backend`), dois binários: `cmd/tccelta` (API, worker e ferramentas) e `cmd/tccelta-kms` (KMS).

```
cmd/tccelta/            raiz de composição da API, do worker e das ferramentas (fina)
cmd/tccelta-kms/        raiz de composição do KMS
internal/domain/        regras puras (protocol, pid, dongle, handshake, ingestion, occurrence, timeline)
internal/port/          TODAS as portas: repositórios + transação, KMS, tokens, métricas, relógio, erros (+ porttest)
internal/usecase/      catalog, handshake, ingestion, session: dependem só de domain e port
internal/adapter/      implementações das portas: postgres, cache (decorator), memory (testes), kmsclient,
                       jwt, prometheus
internal/api/          transporte HTTP (+ api/ratelimit: rate limit e concorrência)
internal/worker/       processamento assíncrono (+ worker/reprocess)
internal/tools/        ferramentas de linha de comando: provisioning (cadastro no banco), simulator
internal/database/     infraestrutura de banco: pool, transação, listen, migrações, queries e dbgen (sqlc)
internal/kms/          serviço KMS: server, vault, provisioning, config, metrics (isolado, veja abaixo)
internal/platform/     utilitários sem dependências do projeto (breaker, usado pelo kmsclient e pelo KMS)
internal/config/ version/   configuração da API e versão
deploy/                 caddy, postgres, observability, segredos e deploy/kms (Vault, certs, compose do host separado)
```

As camadas são impostas pelo `depguard` (`.golangci.yml`, rodado por `make lint`): `domain` é puro; `usecase` só
enxerga `domain` e `port`; `api` só enxerga `usecase`, `port` e `domain`; os adaptadores implementam `port` e não
conhecem `usecase` nem `api`; `internal/kms` não importa nenhum outro pacote do projeto (só `platform/breaker`) e
nada fora dele o importa. Exceção conhecida: `worker`, `worker/reprocess`, `tools/provisioning` e `tools/simulator`
usam `database`/`dbgen` direto, sem passar por portas.

## Pré-requisitos

- Docker (no Windows, Docker Desktop) com o daemon ligado. **Nada de Go, Python ou sqlc é instalado no
  host**: cada ferramenta roda em um contêiner descartável.
- `make`. No Windows, use o **Git Bash**: os alvos usam `sh`, `cp`, `mkdir` e `chmod`.
- O `Makefile` exporta `MSYS_NO_PATHCONV=1` para o Git Bash não reescrever caminhos como `/src` ou `/saida`
  dentro dos comandos do Docker.

## Uso rápido

```
make up                # sobe a stack de desenvolvimento
make seed-dev          # cadastra os 3 dongles de teste (KMS e banco)
make ps                # confere o estado dos serviços
make psql              # abre o psql no banco
make logs S=api        # acompanha o log de um serviço
make limpar            # derruba tudo e apaga os volumes (banco e Vault incluídos)
```

Todos os comandos rodam a partir de `backend/`.

## Como o Makefile está organizado

**Variáveis internas**

| Variável | Valor | Para quê |
|---|---|---|
| `COMPOSE` | `docker compose` | Stack de desenvolvimento (`compose.yaml`, projeto `tccelta`) |
| `COMPOSE_PL` | `docker compose -f compose.yaml -f compose.prodlike.yaml` | Mesma stack com o ajuste `prodlike` por cima |
| `COMPOSE_T` | `docker compose -f compose.test.yaml` | Stack isolada de teste (projeto `tccelta-teste`), usada pelo `e2e` |
| `DOCKER_GO` | `docker run --rm -v "$(CURDIR)":/src -w /src -v tccelta_gomod:/go/pkg/mod golang:1.24-bookworm` | Roda comandos Go; o volume `tccelta_gomod` guarda o cache dos módulos |
| `FERR` | `run --rm ferramentas` | Serviço `ferramentas`: o binário `tccelta` (banco e API) |
| `KMSF` | `run --rm kms-ferramentas` | Serviço `kms-ferramentas`: o binário `tccelta-kms` (Vault) |
| `SIM_DB` | `--db-url-env SIM_BANCO_URL` | Aponta o simulador para a variável com a URL do banco |

**Imagens de ferramenta (versões fixas)**: Go `golang:1.24-bookworm`, sqlc `sqlc/sqlc:1.27.0`,
golangci-lint `golangci/golangci-lint:v1.64.8`, goose `ghcr.io/pressly/goose:v3.22.1`,
Python `python:3.12-slim`.

**Parâmetros de linha de comando.** Alguns alvos leem variáveis que você passa depois do nome:

| Parâmetro | Usado por | Significado |
|---|---|---|
| `S=<serviço>` | `logs` | Serviço do compose (vazio = todos) |
| `NOME=<nome>` | `migracao-nova` | Nome da migration |
| `N=<n>`, `HW=dev\|devsec` | `provisionar` | Quantidade de dongles novos (padrão 1) e perfil de hardware (padrão `dev`) |
| `N=<n>` | `carga` | Quantidade de dongles virtuais (padrão 1000) |
| `ARQ=<caminho>` | `kms-importar` | `kms_record.json` gerado pelo `provision.py` do firmware |
| `C=<cenário>`, `N=<n>`, `ARGS='…'` | `cenario` | Cenário do simulador, quantidade de dongles novos (padrão 3) e opções extras repassadas ao simulador |
| `DONGLE=<16 hex>`, `DESDE=<seq>`, `APLICAR=1` | `reprocessar` | DevId, seq inicial (padrão 1) e se o reprocessamento é aplicado (sem `APLICAR`, é só um ensaio) |

## Alvos

### Subir e derrubar

| Alvo | O que faz | Observações |
|---|---|---|
| `env` | Copia `.env.example` para `.env` se ainda não existir | Não sobrescreve um `.env` seu |
| `segredos` | Roda `scripts/gerar-segredos.sh`: gera, em `deploy/segredos/`, as senhas do Postgres (superusuário, dono, API, worker, provisionador), o token de desenvolvimento do Vault e a senha do Redis | Idempotente: só cria o que falta. Usa um contêiner `alpine` com `openssl`. Cria também um `cloudflared_token` vazio |
| `certs` | Roda `scripts/gerar-certs.sh`: PKI de laboratório em `deploy/kms/certs/` (CA, certificado do Vault, do KMS e certificado de cliente da API; ECDSA P-256, 1 ano) | Idempotente. A variável `KMS_HOST` acrescenta um nome extra aos certificados do Vault e do KMS |
| `build` | `docker compose build` | Constrói as imagens sem subir |
| `up` | `env` + `segredos`, depois `up -d --build --wait` | Sobe banco, Redis, Vault (modo dev), `vault-bootstrap`, `migrar`, KMS, API, worker e `jwt-rotacao`. `--wait` só retorna quando os healthchecks ficam verdes |
| `up-tls` | Igual ao `up`, com o perfil `tls` | Acrescenta o Caddy na porta `API_PORTA_TLS` (8443). É o caminho para o celular alcançar a API na rede local |
| `up-tunel` | Igual ao `up`, com o perfil `tunel` | Acrescenta Caddy e `cloudflared`. O token do túnel vai em `deploy/segredos/cloudflared_token` |
| `up-prodlike` | `env` + `segredos` + `certs`, depois sobe com `compose.prodlike.yaml` e o perfil `tls` | Vault em modo servidor (raft, TLS), mTLS entre API e KMS. Veja [prodlike](#prodlike) |
| `down` | `docker compose down` | Para e remove os contêineres; **mantém** os volumes |
| `limpar` | `docker compose down -v` | Para tudo e **apaga os volumes** (banco, Vault, Redis). Começa do zero |
| `ps` | Lista os serviços e a saúde | |
| `logs` | `logs -f --tail=200 $(S)` | Segue o log. Ex.: `make logs S=kms` |

Se você subiu com um perfil (`tls`, `tunel`), o `make down` pode não derrubar os serviços desse perfil, porque
o Compose só considera os perfis ativos. Nesse caso use `docker compose --profile tls down`. Eu não verifiquei
esse comportamento.

### Banco de dados

| Alvo | O que faz |
|---|---|
| `migrar` | Roda o serviço `migrar` (`tccelta migrate`, equivalente a `up`). O `make up` já faz isso uma vez |
| `migracao-nova NOME=x` | Cria um arquivo de migration SQL numerado em `internal/database/migrations/`, usando o goose |
| `sqlc` | Regenera o código de acesso ao banco em `internal/database/dbgen/` a partir das migrations e das queries (`internal/database/queries/`) |
| `sqlc-verificar` | `sqlc diff`: falha se o código gerado estiver desatualizado. É a checagem que a CI usa |
| `psql` | Abre o `psql` no contêiner `banco` (usuário `postgres`, banco `tccelta`) |

### Vault

| Alvo | O que faz |
|---|---|
| `vault-bootstrap` | Reexecuta o serviço `vault-bootstrap`: habilita os motores Transit `transit-auth`, `transit-rec` e `transit-jwt`, cria a chave Ed25519 do JWT (não exportável), grava as políticas `tccelta-kms`, `tccelta-provisionador` e `tccelta-rotacao-jwt` (mais uma de teste no perfil dev) e cria os AppRoles correspondentes. O `make up` já o executa; o script é idempotente |
| `vault-unseal` | **Só prodlike.** Inicializa e abre o Vault (`init-unseal.sh`), guardando o material de inicialização no volume `vault_init` |
| `vault-unseal-manual` | **Só prodlike.** `vault operator unseal` interativo, para abrir o Vault digitando as chaves |

### Dongles (provisionamento em dois passos)

O provisionamento acontece sempre em **duas etapas**, nesta ordem: primeiro o KMS (importa K_auth e K_rec
no Vault e grava um `record.json`), depois o banco (`tccelta provision register` cadastra o dongle). Os
dois passos são idempotentes. O formato do `record.json` está em
[`docs/kms-dongle-record.md`](docs/kms-dongle-record.md). Os arquivos ficam em `backend/saida/`
(ignorada pelo git).

| Alvo | O que faz | Resultado |
|---|---|---|
| `seed-dev` | Importa o `testdata/test-dongles.csv` e cadastra os dongles | 3 dongles de teste (`0011223344556677`, `…78`, `…79`, perfil `hmac`). **Chaves públicas e fixas**: nunca use em dongle real |
| `provisionar [N=] [HW=]` | `tccelta-kms provision new` e depois `provision register` | Dongles novos com chaves aleatórias, em `saida/<devid>/`, incluindo o material para gravar no firmware |
| `kms-wrapping` | `provision export-wrapping` | Exporta as chaves públicas de embrulho para `saida/kms_wrapping.json`. É o que o `provision.py --kms-wrapping` do firmware usa no perfil `prod` |
| `kms-importar ARQ=…` | `provision import-record` e depois `provision register` | Importa o `kms_record.json` gerado pelo `provision.py` do firmware |
| `saida-dir` | Cria `saida/` com permissão aberta | Dependência dos alvos acima. O contêiner roda com usuário sem privilégio e precisa escrever lá |

O dongle de bancada do firmware (DevId `544343454C544101`) **não** está no `test-dongles.csv`. Para
cadastrá-lo, monte um CSV com o mesmo formato (`dev_id_hex,k_auth_hex,k_rec_hex,perfil`) e rode os dois passos
à mão, como mostrado no roteiro de testes.

### Dados e certificados

| Alvo | O que faz | Observações |
|---|---|---|
| `reprocessar DONGLE=… [DESDE=n] [APLICAR=1]` | `tccelta worker reprocess`: refaz as tabelas `reading` e `position` a partir dos blocos brutos | Sem `APLICAR=1` é só um ensaio. Processa em janelas de 200 blocos |
| `ca-exportar` | Copia a CA raiz do Caddy para `saida/caddy-root.crt` | Precisa do Caddy rodando (`up-tls`/`up-tunel`) com `CADDY_TLS=internal`. É o arquivo para instalar no celular |

### Testes e qualidade

| Alvo | O que faz | Observações |
|---|---|---|
| `teste` | `go test -race ./...` em contêiner | Todo o módulo, inclusive o KMS |
| `teste-integracao` | `go test -p 1 -tags integration -race ./...`, com o socket do Docker montado para o testcontainers | Hoje **nenhum arquivo** tem a tag `integration`, então o alvo só reexecuta os testes de unidade |
| `vetores` | Roda `scripts/gerar_vetores.py`, que gera `internal/domain/protocol/testdata/vetores.json` e o copia para `../mobile-app/test/fixtures/protocolo_vetores.json` | O script foi escrito de forma independente do código Go, a partir da spec, para servir de referência cruzada. Os vetores do firmware (`vetores_firmware.json`) vêm de outro gerador, o `vectors.py` |
| `lint` | `golangci-lint run` (v1.64.8) com `.golangci.yml` | Inclui a regra `depguard` que impede a API e o KMS de importarem um ao outro. Ainda não foi executado |
| `build-ferramentas` | Reconstrói as imagens `ferramentas` e `kms-ferramentas` | O `docker compose run` só constrói uma imagem que **não existe**; depois de mudar o código, uma imagem antiga continua rodando o binário antigo. O `cenario` e o `e2e` já chamam este passo |
| `cenario C=<nome> [N=3] [ARGS='…']` | Cria `N` dongles **novos** (KMS, depois banco), grava o CSV em `saida/dongles-<nome>.csv` e roda esse cenário do simulador sobre eles, na stack de desenvolvimento | Ex.: `make cenario C=negative-protocol` ou `make cenario C=time ARGS='--speedup 120'`. Cenários: `e2e`, `negative-protocol`, `time`, `generation`, `reconnection`, `load`, `knee` |
| `e2e` | Sobe a stack isolada `compose.test.yaml`, reconstrói as imagens de ferramentas e roda quatro cenários do simulador em sequência: `e2e`, `negative-protocol`, `time --speedup 120` e `generation`. **Cada cenário recebe 3 dongles novos** | Derruba a stack com `down -v` no fim, passando ou não, e devolve o código do primeiro cenário que falhou. Os comandos foram validados com `make -n` e `bash -n`, e o alvo foi executado depois pelo autor do projeto (resultado não registrado aqui) |
| `carga [N=]` | Cria `N` dongles virtuais (só com as chaves, sem material de NVS), registra no banco e roda o cenário `load`, com relatório em `saida/relatorio.json` | O projeto decidiu não executar testes de carga por ora, só documentá-los. As metas M1 a M6 não foram medidas |

## dev, prodlike e prod: o que muda

São três níveis de segurança sobre o mesmo código e as mesmas imagens: **dev** (`make up`), **prodlike**
(`make up-prodlike`, com HTTPS e mTLS em toda a cadeia) e **prod** (`AMBIENTE=producao`, que proíbe os atalhos de
teste). A comparação completa, o que falta para uma produção de verdade e como trocar de um nível para outro estão
em [docs/ambientes.md](docs/ambientes.md).

## Stacks e perfis

O `compose.yaml` define o projeto `tccelta`. Sem perfil, ele sobe:

| Serviço | Papel | Porta no host |
|---|---|---|
| `banco` | PostgreSQL com TimescaleDB e PostGIS (`IMG_TIMESCALE`) | `127.0.0.1:5432` |
| `redis` | Limites de taxa compartilhados entre réplicas | nenhuma |
| `vault` | Vault em modo dev | `127.0.0.1:8200` |
| `vault-bootstrap` | Política e AppRoles (executa e sai) | nenhuma |
| `migrar` | Migrations (executa e sai) | nenhuma |
| `kms` | Serviço KMS (HTTP na 8300 e métricas/saúde na 9300, dentro da rede) | nenhuma |
| `api` | API (`API_REPLICAS` réplicas, padrão 2) | `127.0.0.1:8080-8081` |
| `worker` | Âncoras de tempo e expiração de desafios | nenhuma |
| `jwt-rotacao` | Rotação periódica da chave do JWT (`tccelta-kms jwt agendar`) | nenhuma |

Perfis opcionais:

| Perfil | Serviços | Como ativar |
|---|---|---|
| `tls` | `caddy` (HTTPS na `API_PORTA_TLS`, 8443) | `make up-tls` |
| `tunel` | `caddy` (HTTP na 8081 da rede interna) e `cloudflared` | `make up-tunel` |
| `ferramentas` | `ferramentas` (`tccelta`) e `kms-ferramentas` (`tccelta-kms`) | Os alvos de provisionamento e do simulador já passam `--profile ferramentas` |
| `observabilidade` | `prometheus` (`127.0.0.1:9099`) e `grafana` (`127.0.0.1:3000`) | `docker compose --profile observabilidade up -d` |

**Redes.** `borda` (Caddy, API, ferramentas), `dados` (banco, Redis, API, worker), `kms` (somente API ↔ KMS) e
`cofre` (somente KMS ↔ Vault). A API não tem credencial do Vault e não está na rede `cofre`.

As métricas Prometheus da API e do worker estão descritas em [docs/metricas.md](docs/metricas.md).

A porta do Caddy (`API_PORTA_TLS`) é a única publicada em todas as interfaces do host; as outras escutam só em
`127.0.0.1`. Por isso, para um celular alcançar a API, é preciso o perfil `tls` (ou o túnel).

### prodlike

`make up-prodlike` aplica o `compose.prodlike.yaml` sobre a stack: Vault em modo servidor, mTLS entre a API e o KMS
e nada publicado no host além do Caddy. O que muda em relação ao dev, linha a linha, está em
[docs/ambientes.md](docs/ambientes.md).

### Stack de teste

O `compose.test.yaml` é autossuficiente e usa o projeto `tccelta-teste`, separado do de desenvolvimento
(volumes e redes próprios). Sobe banco, Redis, Vault em modo dev, KMS, `migrar` e API, sem Caddy nem túnel.
É a stack do `make e2e`.

## Variáveis do `.env`

O `make env` cria o `.env` a partir de `.env.example`. As variáveis, e onde cada uma é lida:

| Variável | Padrão | Para quê |
|---|---|---|
| `AMBIENTE` | `dev` | `dev`, `prodlike` ou `producao`. Fora de `dev` o KMS exige mTLS |
| `IMG_TIMESCALE` | `timescale/timescaledb-ha:pg17` | Imagem do banco |
| `VAULT_VERSAO` | `1.18` | Versão da imagem do Vault |
| `API_REPLICAS` | `2` | Réplicas da API. Cada uma publica uma porta do intervalo `8080-8081` |
| `API_HOST` | `localhost` | Endereço do site no `Caddyfile` e nome do certificado. Para o celular, ponha o IP do PC na rede |
| `API_PORTA_TLS` | `8443` | Porta do Caddy no host |
| `CADDY_TLS` | `internal` | Diretiva `tls` do Caddy: `internal` usa a CA local dele |
| `JWT_ROTACAO_PERIODO` | `720h` | Período da rotação da chave do JWT |
| `JWT_VALIDADE` | `1h` | Validade do token emitido |
| `JWT_FOLGA` | `30s` | Folga de relógio na validação |
| `PROXIES_CONFIAVEIS` | `172.30.10.0/24` | Redes de onde a API aceita o cabeçalho com o IP real do cliente (a rede `borda`) |
| `LOG_NIVEL` / `LOG_FORMATO` | `info` / `json` | Log da API |
| `CLOUDFLARED_VERSAO` | `2024.12.2` | Versão do `cloudflared` |
| `TUNEL_HOSTNAME` | vazio | Está no exemplo, mas não encontrei nenhum uso dele no `compose.yaml` nem no `Caddyfile` |

## Arquivos gerados (ficam fora do git)

| Caminho | Origem |
|---|---|
| `.env` | `make env` |
| `deploy/segredos/*` | `make segredos` (o `.gitkeep` é versionado) |
| `saida/` | Provisionamento, `ca-exportar`, `carga` (registros dos dongles, `kms_wrapping.json`, `caddy-root.crt`, `relatorio.json`) |
| `deploy/kms/certs/` | `make certs` |

Os arquivos de `saida/` contêm material de chave dos dongles. Apague-os quando não precisar mais.

## Os binários que o Makefile chama

**`tccelta`** (serviços `api`, `worker`, `migrar`, `ferramentas`):

```
api                              servidor HTTP
worker                           worker de tempo; "worker reprocess" refaz reading/position
migrate [up|down-to N|status]    migrations
provision register --file        cadastra no banco um dongle provisionado pelo KMS
simulator                        dongles virtuais e cenários
health --url <url>               healthcheck (sai 0/1)
```

Cenários do simulador (`--scenario`): `e2e`, `negative-protocol`, `time`, `generation`, `load`, `reconnection` e
`knee`. Outras opções: `--api`, `--keys` (CSV), `--dongles`, `--duration`, `--speedup`, `--tlat`,
`--accumulated`, `--ramp`, `--report`, `--drift`, `--db-url-env`.

**`tccelta-kms`** (serviços `kms`, `jwt-rotacao`, `kms-ferramentas`):

```
serve                                serviço KMS
jwt <rotate [--now]|schedule>        rotação da chave do JWT
provision <new|import|import-record|export-wrapping|verify>
health --url <url>                   healthcheck (sai 0/1)
```

`provision new` aceita `--count`, `--profile hmac|asym|none`, `--hw dev|devsec`, `--no-material` e
`--csv-out`. A API só tem `provision register`; os subcomandos antigos de importação e de JWT saíram do
binário `tccelta`.

## Problemas comuns

- **Caminho do Docker alterado no Windows.** Use o Git Bash. O `Makefile` já define `MSYS_NO_PATHCONV=1`; se
  você rodar um `docker run` à mão, exporte a mesma variável.
- **Porta já em uso.** Com `API_REPLICAS=2`, a API publica as portas 8080 e 8081. Num teste anterior, isso
  conflitou no Windows e foi contornado com `API_REPLICAS=1` no `.env`. Não reproduzi esse conflito.
- **O celular não alcança a API.** A API escuta só em `127.0.0.1`. Suba com `make up-tls`, ponha o IP do PC
  em `API_HOST`, libere a porta 8443 no firewall e instale a CA exportada pelo `make ca-exportar`.
- **Recomeçar do zero.** `make limpar` e depois `make up`. Isso apaga o banco e o Vault; os dongles
  precisam ser cadastrados de novo (`make seed-dev`).
- **Cenário do simulador devolve `422 resposta_inconsistente` no handshake.** Um dongle simulado recomeça do
  seq 1, e o servidor recusa se o mesmo DevId já tem um HEAD maior. Nada no simulador limpa o estado entre
  execuções, então reaproveitar os dongles de um cenário que já rodou (por exemplo, os três do
  `test-dongles.csv`) falha. Use `make cenario C=<nome>`, que cria dongles novos a cada execução, ou zere o
  banco (`make limpar`, `make up`, `make seed-dev`). O `make e2e` já cria dongles novos para cada um dos seus
  quatro cenários. Rodar o simulador à mão com `--keys /testdata/test-dongles.csv` só serve na primeira vez
  depois de um banco limpo.
- **`role_id: open : no such file or directory` (ou outro comportamento que não bate com o código atual) ao
  usar as ferramentas.** A imagem `tccelta-ferramentas:local` é antiga: o `docker compose run` não reconstrói
  imagens que já existem. Rode `make build-ferramentas` e repita.

## Lacunas conhecidas

- Nenhum alvo foi reexecutado depois do refactor (veja o aviso no topo).
- `teste-integracao` não tem testes com a tag `integration`.
- `lint` e a regra de `depguard` nunca rodaram.
- `carga` e as metas M1 a M6 não foram medidas.
- O túnel, o perfil `observabilidade` e o `prodlike` nunca foram exercitados.
