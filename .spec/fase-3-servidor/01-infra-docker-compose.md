# 01 — Infraestrutura em Docker Compose

> Fase 3 · Servidor. Depende de: nada (é a primeira a ser executada). É dependência de todas as outras specs.

## Objetivo

Subir, só com Docker, todo o lado servidor do TCCELTA numa máquina: banco (PostgreSQL + TimescaleDB + PostGIS), Redis (estado do rate limit), KMS (HashiCorp Vault), API Go, worker (o mesmo binário em modo worker), job de migrations, proxy (Caddy), túnel com certificado público (Cloudflare Tunnel) e ferramentas (provisionamento e simulador). Há dois perfis de execução:

- **dev**: Vault em modo `-dev` (memória, sem selo), portas publicadas só em `127.0.0.1`, redes não internas, para iterar rápido.
- **prodlike**: Vault com storage persistente (Raft), TLS, `init`/`unseal` e AppRole, redes internas sem portas publicadas, contêineres somente leitura. Serve para testar o que muda quando o servidor for para o VPS e, depois, quando o Vault sair para outro host atrás de uma VPN.

## Escopo

Dentro: `backend/Dockerfile`, os arquivos `compose*.yaml`, `Caddyfile`, a configuração e os scripts do Vault, o script de init do Postgres, o `Makefile` e `scripts/*.sh`, `.env.example`, healthchecks, redes, volumes e segredos.

Fora: o conteúdo das migrations (spec 02), o código da API (spec 03) e as políticas do Vault em detalhe (spec 07). Esta spec só diz onde esses arquivos ficam e como são montados.

## Design

### Serviços

| Serviço | Imagem | Comando | Redes | Perfil compose | Papel |
|---|---|---|---|---|---|
| `banco` | `timescale/timescaledb-ha:pg17` (fixar tag/digest em `.env`) | entrypoint padrão | `dados` | sempre | PostgreSQL 17 + TimescaleDB + PostGIS |
| `vault` | `hashicorp/vault:${VAULT_VERSAO}` (≥ 1.15; ver spec 07) | dev: `server -dev`; prodlike: `server -config=/vault/config/vault.hcl` | `kms` | sempre | KMS (Transit) |
| `vault-bootstrap` | mesma do `vault` | `/vault/scripts/bootstrap.sh` | `kms` | sempre (job) | habilita engines, políticas e AppRole e grava role_id/secret_id |
| `migrar` | `tccelta-backend:local` (build) | `migrar` | `dados` | sempre (job) | `goose up` com migrations embutidas |
| `api` | `tccelta-backend:local` | `api` | `borda`, `dados`, `kms` | sempre | HTTP da API (2 réplicas) |
| `worker` | `tccelta-backend:local` | `worker` | `dados` | sempre | worker assíncrono (spec 06) |
| `redis` | `redis:7.4-alpine` | `redis-server` com a senha lida do segredo (ver snippet) | `dados` | sempre | estado compartilhado do rate limit (spec 03); sem persistência |
| `caddy` | `caddy:2.8-alpine` | padrão | `borda` | `tls`, `tunel` | balanceia as réplicas da API; TLS próprio (CA local do Caddy) no `tls`, HTTP interno atrás do túnel no `tunel` |
| `cloudflared` | `cloudflare/cloudflared:${CLOUDFLARED_VERSAO}` | `tunnel --no-autoupdate run` | `borda` | `tunel` | túnel de saída até a Cloudflare: certificado público, nada a instalar no celular |
| `jwt-rotacao` | `tccelta-backend:local` | `jwt agendar` | `kms` | sempre | rotaciona e expurga a chave do JWT (spec 07, *Rotação da chave do JWT*); credencial própria, sem acesso ao banco |
| `ferramentas` | `tccelta-ferramentas:local` (alvo `ferramentas` do `Dockerfile`: Python + gerador de NVS da Fase 1 + binário) | `sleep infinity` | `dados`, `kms`, `borda` | `ferramentas` | `provisionar` (inclusive a imagem NVS), `simulador`, `worker reprocessar`, `psql` |
| `prometheus` | `prom/prometheus:v2.55.1` | padrão | `borda`, `dados` | `observabilidade` | coleta `/metricas` |
| `grafana` | `grafana/grafana:11.3.0` | padrão | `borda` | `observabilidade` | painéis |

**[Decisão] O worker não entra na rede `kms`.** Ele não assina nem verifica nada: só lê e escreve no banco. Com isso, um worker comprometido não fala com o Vault (privilégio mínimo).

**[Decisão] Um único binário `tccelta` com subcomandos** (`api`, `worker`, `migrar`, `provisionar`, `simulador`, `saude`). Assim há uma imagem só para construir, versionar e escanear. O doc já pede que o worker seja "o mesmo binário".

### Redes

```yaml
networks:
  borda:                     # caddy ↔ api (e cloudflared, prometheus/grafana)
    ipam: { config: [{ subnet: 172.30.10.0/24 }] }   # fixa: PROXIES_CONFIAVEIS=172.30.10.0/24
  dados:   { internal: false }  # api, worker, migrar, ferramentas ↔ banco; api ↔ redis
  kms:     { internal: false }  # api, vault-bootstrap, ferramentas ↔ vault
```

A sub-rede da `borda` é fixa para que `PROXIES_CONFIAVEIS` (spec 03) aceite o `X-Real-IP` vindo do Caddy. Sem isso, todos os celulares chegariam à API com o IP do Caddy, e o `limitarIP` viraria um limite global.

No perfil prodlike, `compose.prodlike.yaml` sobrescreve `dados` e `kms` com `internal: true` e zera as portas publicadas com `ports: !reset []` (precisa do Compose ≥ v2.24). Numa rede `internal`, o contêiner não tem rota para fora nem porta acessível do host. Só o `caddy` publica porta.

**[Decisão] Vault e API compartilham uma rede bridge dedicada (`kms`), separada de `dados`.** Isso reproduz no mesmo host a "rede privada" do diagrama `kms-servidor-app.puml`. O banco não enxerga o Vault, e o Vault não enxerga o banco.

### Volumes

| Volume | Montado em | Conteúdo |
|---|---|---|
| `banco_dados` | `banco:/home/postgres/pgdata` | PGDATA da imagem `-ha` |
| `vault_dados` | `vault:/vault/data` | storage Raft (só no prodlike) |
| `vault_init` | `vault:/vault/init` (0700) | saída do `operator init` (só no prodlike, **só em laboratório**) |
| `vault_approle_api` | `vault-bootstrap:/approle/api` (rw), `api:/run/approle` (ro) | só `role_id` e `secret_id` da API (dono 65532, 0400) |
| `vault_approle_ferramentas` | `vault-bootstrap:/approle/ferramentas` (rw), `ferramentas:/run/approle` (ro) | só o token do provisionador (dono 65532, 0400) |
| `vault_approle_rotacao` | `vault-bootstrap:/approle/rotacao` (rw), `jwt-rotacao:/run/approle` (ro) | só `role_id` e `secret_id` do AppRole `tccelta-rotacao-jwt` (dono 65532, 0400); terceiro volume de credenciais, nunca montado na API |
| `vault_auditoria` | `vault:/vault/logs` | log de auditoria do Vault |
| `caddy_dados` | `caddy:/data` | certificados e CA local do Caddy |
| (nenhum) | `redis` | **[Decisão]** sem volume: `--save "" --appendonly no`. O estado do rate limit é efêmero; perdê-lo só zera as janelas. |
| `prometheus_dados`, `grafana_dados` | — | observabilidade |

### Segredos (Compose `secrets`, arquivos em `backend/deploy/segredos/`, fora do git)

| Segredo | Usado por | Gerado por |
|---|---|---|
| `pg_senha_superusuario` | `banco` (`POSTGRES_PASSWORD_FILE`) | `make segredos` |
| `pg_senha_dono`, `pg_senha_api`, `pg_senha_worker`, `pg_senha_provisionador` | init do banco, `migrar`, `api`, `worker`, `ferramentas` | `make segredos` |
| `vault_dev_root_token` | `vault` (dev), `vault-bootstrap` (dev) | `make segredos` |
| `vault_ca` | `api`, `ferramentas` (prodlike) | `scripts/gerar-certs.sh` |
| `redis_senha` | `redis` (`--requirepass`), `api` (`REDIS_SENHA_ARQUIVO`) | `make segredos` |
| `cloudflared_token` | `cloudflared` (`--token-file`) | **à mão**, copiado do painel Zero Trust da Cloudflare ao criar o túnel; `make segredos` só cria o arquivo vazio com `chmod 600` |

O `make segredos` roda `scripts/gerar-segredos.sh`, que usa `openssl rand -hex 24` dentro de um contêiner `alpine`. Assim não depende de ferramentas do host Windows. O `.gitignore` de `backend/` ignora `deploy/segredos/*` e `deploy/certs/*`, exceto um `.gitkeep`.

**[Decisão, revisão] Dois volumes de credenciais do Vault.** O token do provisionador nunca é montado no contêiner da API. Na primeira versão, um volume único expunha à API um token capaz de importar chaves (e, em dev, de gerar HMAC com `K_rec`). Isso anulava a garantia "a API só verifica com `K_rec`" para quem lesse o disco da API.

### `compose.yaml` (base = perfil dev)

Os pontos obrigatórios estão abaixo. O arquivo final segue esta estrutura.

```yaml
name: tccelta

x-backend: &backend
  image: tccelta-backend:local
  build: { context: ., dockerfile: Dockerfile }
  read_only: true
  tmpfs: [/tmp]
  user: "65532:65532"            # nonroot do distroless
  cap_drop: [ALL]
  security_opt: ["no-new-privileges:true"]
  env_file: [.env]
  restart: unless-stopped

services:
  banco:
    image: ${IMG_TIMESCALE:-timescale/timescaledb-ha:pg17}
    environment:
      POSTGRES_DB: tccelta
      POSTGRES_PASSWORD_FILE: /run/secrets/pg_senha_superusuario
      TS_TUNE_MEMORY: ${BANCO_MEMORIA:-2GB}
      TS_TUNE_NUM_CPUS: ${BANCO_CPUS:-2}
    secrets: [pg_senha_superusuario, pg_senha_dono, pg_senha_api, pg_senha_worker, pg_senha_provisionador]
    volumes:
      - banco_dados:/home/postgres/pgdata
      # Monta SÓ o arquivo, com nome que ordena depois dos scripts de init da própria imagem
      # (instalação da extensão e timescaledb-tune). Montar o diretório inteiro os esconderia.
      - ./deploy/postgres/init/00-papeis.sh:/docker-entrypoint-initdb.d/900-papeis.sh:ro
    command: ["postgres", "-c", "shared_preload_libraries=timescaledb",
              "-c", "max_connections=200", "-c", "log_min_duration_statement=500ms",
              "-c", "timescaledb.telemetry_level=off"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d tccelta -h 127.0.0.1"]
      interval: 5s
      timeout: 3s
      retries: 30
      start_period: 20s
    ports: ["127.0.0.1:5432:5432"]
    networks: [dados]
    shm_size: 512m

  vault:
    image: hashicorp/vault:${VAULT_VERSAO:-1.18}
    cap_add: [IPC_LOCK]
    environment:
      VAULT_DEV_LISTEN_ADDRESS: 0.0.0.0:8200
      VAULT_ADDR: http://127.0.0.1:8200
    secrets: [vault_dev_root_token]
    entrypoint: ["/bin/sh", "-c",
      "VAULT_DEV_ROOT_TOKEN_ID=$$(cat /run/secrets/vault_dev_root_token) exec vault server -dev"]
    healthcheck:
      test: ["CMD", "vault", "status"]
      interval: 5s
      timeout: 3s
      retries: 20
    ports: ["127.0.0.1:8200:8200"]
    networks: [kms]

  vault-bootstrap:
    image: hashicorp/vault:${VAULT_VERSAO:-1.18}
    depends_on: { vault: { condition: service_healthy } }
    environment:
      VAULT_ADDR: http://vault:8200
      PERFIL: dev
    secrets: [vault_dev_root_token]
    volumes:
      - ./deploy/vault/scripts:/vault/scripts:ro
      - ./deploy/vault/politicas:/vault/politicas:ro
      - vault_approle_api:/approle/api
      - vault_approle_ferramentas:/approle/ferramentas
      - vault_approle_rotacao:/approle/rotacao
    user: "0:0"                     # root: o script faz chown para o uid 65532 da distroless
    entrypoint: ["/bin/sh", "/vault/scripts/bootstrap.sh"]
    restart: "no"
    networks: [kms]

  migrar:
    <<: *backend
    command: ["migrar"]
    environment:
      BANCO_URL: postgres://tccelta_dono@banco:5432/tccelta?sslmode=disable
      BANCO_SENHA_ARQUIVO: /run/secrets/pg_senha_dono
    secrets: [pg_senha_dono]
    depends_on: { banco: { condition: service_healthy } }
    restart: "no"
    networks: [dados]

  api:
    <<: *backend
    command: ["api"]
    deploy: { replicas: ${API_REPLICAS:-2} }
    environment:
      BANCO_URL: postgres://tccelta_api@banco:5432/tccelta?sslmode=disable
      BANCO_SENHA_ARQUIVO: /run/secrets/pg_senha_api
      VAULT_ADDR: http://vault:8200
      VAULT_APPROLE_ROLE_ID_ARQUIVO: /run/approle/api_role_id
      VAULT_APPROLE_SECRET_ID_ARQUIVO: /run/approle/api_secret_id
      REDIS_ENDERECO: redis:6379
      REDIS_SENHA_ARQUIVO: /run/secrets/redis_senha
    secrets: [pg_senha_api, redis_senha]
    volumes: [vault_approle_api:/run/approle:ro]
    depends_on:
      migrar: { condition: service_completed_successfully }
      vault-bootstrap: { condition: service_completed_successfully }
      redis: { condition: service_healthy }
    healthcheck:
      test: ["CMD", "/tccelta", "saude", "--url", "http://127.0.0.1:8080/saude/pronta"]
      interval: 10s
      timeout: 3s
      retries: 6
      start_period: 10s
    stop_grace_period: 30s
    ports: ["127.0.0.1:8080-8081:8080"]     # dev: acesso direto sem TLS
    networks: [borda, dados, kms]

  worker:
    <<: *backend
    command: ["worker"]
    environment:
      BANCO_URL: postgres://tccelta_worker@banco:5432/tccelta?sslmode=disable
      BANCO_SENHA_ARQUIVO: /run/secrets/pg_senha_worker
    secrets: [pg_senha_worker]
    depends_on: { migrar: { condition: service_completed_successfully } }
    healthcheck:
      test: ["CMD", "/tccelta", "saude", "--url", "http://127.0.0.1:8090/saude/viva"]
      interval: 15s
      timeout: 3s
      retries: 4
    stop_grace_period: 30s
    networks: [dados]

  redis:
    image: redis:7.4-alpine
    # sem persistência; limite de memória com descarte LRU; senha do segredo
    command: ["/bin/sh", "-c",
      "exec redis-server --save '' --appendonly no --maxmemory 64mb --maxmemory-policy allkeys-lru --requirepass \"$$(cat /run/secrets/redis_senha)\""]
    secrets: [redis_senha]
    read_only: true
    cap_drop: [ALL]
    security_opt: ["no-new-privileges:true"]
    healthcheck:
      test: ["CMD-SHELL", "REDISCLI_AUTH=$$(cat /run/secrets/redis_senha) redis-cli ping | grep -q PONG"]
      interval: 5s
      timeout: 3s
      retries: 20
    networks: [dados]            # nunca publicar a porta 6379

  caddy:
    image: caddy:2.8-alpine
    profiles: [tls, tunel]
    environment:
      API_HOST: ${API_HOST:-localhost}
      API_PORTA_TLS: ${API_PORTA_TLS:-8443}
      CADDY_TLS: ${CADDY_TLS:-internal}       # "internal" (CA local) ou e-mail ACME
    volumes:
      - ./deploy/caddy/Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_dados:/data
    # 8443 (TLS próprio) só no perfil `tls`; no `tunel` o Caddy atende em HTTP na porta 8081, sem publicar nada
    ports: ["${API_PORTA_TLS:-8443}:8443"]
    depends_on: { api: { condition: service_healthy } }
    networks: [borda]

  cloudflared:
    image: cloudflare/cloudflared:${CLOUDFLARED_VERSAO}      # fixar uma tag estável em .env
    profiles: [tunel]
    # túnel gerenciado remotamente: o nome público (TUNEL_HOSTNAME) e a origem http://caddy:8081
    # são configurados no painel da Cloudflare ao criar o túnel; aqui só entra o token.
    command: ["tunnel", "--no-autoupdate", "run", "--token-file", "/run/secrets/cloudflared_token"]   # [verificar] a flag na versão fixada
    secrets: [cloudflared_token]
    read_only: true
    cap_drop: [ALL]
    security_opt: ["no-new-privileges:true"]
    restart: unless-stopped
    depends_on: { caddy: { condition: service_started } }
    networks: [borda]            # só saída; nenhuma porta publicada

  jwt-rotacao:
    <<: *backend
    command: ["jwt", "agendar"]
    environment:
      VAULT_ADDR: http://vault:8200
      VAULT_APPROLE_ROLE_ID_ARQUIVO: /run/approle/rotacao_role_id
      VAULT_APPROLE_SECRET_ID_ARQUIVO: /run/approle/rotacao_secret_id
      JWT_ROTACAO_PERIODO: ${JWT_ROTACAO_PERIODO:-720h}          # rotaciona a cada 30 dias
      JWT_ROTACAO_VERIFICACAO: ${JWT_ROTACAO_VERIFICACAO:-1h}    # intervalo do laço
    volumes: [vault_approle_rotacao:/run/approle:ro]
    depends_on: { vault-bootstrap: { condition: service_completed_successfully } }
    healthcheck:
      test: ["CMD", "/tccelta", "saude", "--url", "http://127.0.0.1:8092/saude/viva"]
      interval: 30s
      timeout: 3s
      retries: 4
    networks: [kms]              # sem banco, sem borda

  ferramentas:
    <<: *backend
    image: tccelta-ferramentas:local
    build: { context: ., dockerfile: Dockerfile, target: ferramentas }
    profiles: [ferramentas]
    entrypoint: ["/tccelta"]
    command: ["ajuda"]
    read_only: false
    environment:
      VAULT_ADDR: http://vault:8200
      VAULT_TOKEN_ARQUIVO: /run/approle/provisionador_token
      BANCO_URL: postgres://tccelta_provisionador@banco:5432/tccelta?sslmode=disable
      BANCO_SENHA_ARQUIVO: /run/secrets/pg_senha_provisionador
      # simulador (verificações só com SELECT); em dev/teste usa o papel dono
      SIM_BANCO_URL: postgres://tccelta_dono@banco:5432/tccelta?sslmode=disable
      SIM_BANCO_SENHA_ARQUIVO: /run/secrets/pg_senha_dono
      # `worker reprocessar` (spec 05) refaz leitura/posicao: precisa de DELETE/INSERT, só o dono tem
      ADMIN_BANCO_URL: postgres://tccelta_dono@banco:5432/tccelta?sslmode=disable
      ADMIN_BANCO_SENHA_ARQUIVO: /run/secrets/pg_senha_dono
    secrets: [pg_senha_provisionador, pg_senha_dono]
    volumes:
      - vault_approle_ferramentas:/run/approle:ro
      - ./testdata:/testdata:ro
      - ./saida:/saida            # material de provisionamento (fora do git)
    networks: [borda, dados, kms]
  # prometheus e grafana: perfil "observabilidade", configs em deploy/observabilidade/

secrets:
  pg_senha_superusuario: { file: ./deploy/segredos/pg_senha_superusuario }
  pg_senha_dono:         { file: ./deploy/segredos/pg_senha_dono }
  pg_senha_api:          { file: ./deploy/segredos/pg_senha_api }
  pg_senha_worker:       { file: ./deploy/segredos/pg_senha_worker }
  pg_senha_provisionador: { file: ./deploy/segredos/pg_senha_provisionador }
  vault_dev_root_token:  { file: ./deploy/segredos/vault_dev_root_token }
  redis_senha:           { file: ./deploy/segredos/redis_senha }
  cloudflared_token:     { file: ./deploy/segredos/cloudflared_token }

volumes: { banco_dados: {}, vault_dados: {}, vault_init: {}, vault_approle_api: {}, vault_approle_ferramentas: {}, vault_approle_rotacao: {}, vault_auditoria: {}, caddy_dados: {}, prometheus_dados: {}, grafana_dados: {} }
```

Notas:

- **Duas réplicas da API e portas.** `deploy.replicas` com `ports` em faixa (`8080-8081`) funciona no Compose v2. Se a faixa causar conflito no Windows, use `API_REPLICAS=1` em dev. O caminho "oficial" é pelo Caddy.
- **`VAULT_DEV_ROOT_TOKEN_ID`.** A imagem não lê o token de um arquivo, por isso o `entrypoint` exporta a variável a partir do segredo. O token de dev nunca vai para o `.env`.
- **`.env`.** O `env_file: [.env]` exige o arquivo. O alvo `make env` copia `.env.example` → `.env` se ele não existir (idempotente) e é dependência de todos os alvos `up*`.
- **Extensão do TimescaleDB.** O `shared_preload_libraries=timescaledb` explícito no `command` garante que o `CREATE EXTENSION` do `900-papeis.sh` funcione mesmo que os scripts da imagem mudem. **Checkpoint 2** confere a lista de scripts da imagem: `docker run --rm --entrypoint ls timescale/timescaledb-ha:pg17 /docker-entrypoint-initdb.d`.
- **Imagem do banco.** A `timescaledb-ha` já traz PostGIS, `timescaledb-tune` e `shared_preload_libraries=timescaledb`. A `timescale/timescaledb` (alpine) não traz PostGIS, por isso não serve.

### `compose.prodlike.yaml` (sobreposição)

```yaml
services:
  banco:
    ports: !reset []
  vault:
    entrypoint: ["vault", "server", "-config=/vault/config/vault.hcl"]
    environment:
      VAULT_ADDR: https://127.0.0.1:8200
      VAULT_CACERT: /vault/certs/ca.pem
    secrets: !reset []
    volumes:
      - ./deploy/vault/config:/vault/config:ro
      - ./deploy/certs/vault:/vault/certs:ro
      - vault_dados:/vault/data
      - vault_init:/vault/init
      - vault_auditoria:/vault/logs
    healthcheck:
      # 200 = ativo; 429 = standby; 473 = perf standby; 501 = não inicializado; 503 = selado
      test: ["CMD-SHELL", "wget -q --no-check-certificate -O /dev/null 'https://127.0.0.1:8200/v1/sys/health?standbyok=true' || exit 1"]
    ports: !reset []
  vault-unseal:
    image: hashicorp/vault:${VAULT_VERSAO:-1.18}
    entrypoint: ["/bin/sh", "/vault/scripts/init-unseal.sh"]
    environment: { VAULT_ADDR: https://vault:8200, VAULT_CACERT: /vault/certs/ca.pem }
    volumes:
      - ./deploy/vault/scripts:/vault/scripts:ro
      - ./deploy/certs/vault/ca.pem:/vault/certs/ca.pem:ro
      - vault_init:/vault/init
    depends_on: { vault: { condition: service_started } }
    restart: "no"
    networks: [kms]
  vault-bootstrap:
    environment: { VAULT_ADDR: https://vault:8200, VAULT_CACERT: /vault/certs/ca.pem, PERFIL: prodlike }
    secrets: !reset []
    volumes:
      - ./deploy/vault/scripts:/vault/scripts:ro
      - ./deploy/vault/politicas:/vault/politicas:ro
      - ./deploy/certs/vault/ca.pem:/vault/certs/ca.pem:ro
      - vault_init:/vault/init:ro       # lê o root token inicial (laboratório)
      - vault_approle_api:/approle/api
      - vault_approle_ferramentas:/approle/ferramentas
      - vault_approle_rotacao:/approle/rotacao
    depends_on: { vault-unseal: { condition: service_completed_successfully } }
  api:
    environment:
      VAULT_ADDR: https://vault:8200
      VAULT_CACERT: /run/secrets/vault_ca
    secrets: [pg_senha_api, redis_senha, vault_ca]
    ports: !reset []
networks:
  dados: { internal: true }
  kms:   { internal: true }
secrets:
  vault_ca: { file: ./deploy/certs/vault/ca.pem }
```

`deploy/vault/config/vault.hcl` (prodlike):

```hcl
ui            = false
disable_mlock = true            # recomendado com Raft
api_addr      = "https://vault:8200"
cluster_addr  = "https://vault:8201"

storage "raft" {
  path    = "/vault/data"
  node_id = "kms1"
}

listener "tcp" {
  address            = "0.0.0.0:8200"
  tls_cert_file      = "/vault/certs/vault.pem"
  tls_key_file       = "/vault/certs/vault-key.pem"
  tls_min_version    = "tls13"
}

# Auditoria habilitada pelo bootstrap: vault audit enable file file_path=/vault/logs/auditoria.log
```

`deploy/vault/scripts/init-unseal.sh`: primeiro espera o Vault aceitar conexões (laço de até 60 s em `vault status`, aceitando os códigos de saída 0 e 2; o código 1 = indisponível continua esperando), porque `service_started` dispara antes de o listener subir. Depois, se `vault status` devolver 2 (selado) e `/vault/init/init.json` não existir, roda `vault operator init -key-shares=1 -key-threshold=1 -format=json > /vault/init/init.json` (chmod 600). Em seguida, extrai a chave com `sed`/`grep`, sem precisar de `jq`, e roda `vault operator unseal <chave>`. **[Decisão] 1 share / threshold 1 é só para o laboratório.** O script imprime um aviso grande e recusa rodar se `AMBIENTE=producao`. Em produção vale a decisão *Unseal manual* (§ Decisões do autor, abaixo).

`scripts/gerar-certs.sh`: roda `openssl` num contêiner `alpine:3.20`. Gera uma CA privada `deploy/certs/vault/ca.pem` e o certificado do servidor `vault.pem` com SAN `DNS:vault,DNS:localhost,IP:127.0.0.1` (e `DNS:${KMS_HOST}` para o cenário de VPN), com validade de 1 ano e chave ECDSA P-256.

### `deploy/postgres/init/00-papeis.sh`

O script roda uma única vez, quando o volume está vazio, como superusuário `postgres`. Ele lê as senhas de `/run/secrets/*`.

```sql
-- Papéis: dono das tabelas (migrations), API, worker, leitura analítica
CREATE ROLE tccelta_dono    LOGIN PASSWORD :'senha_dono';
CREATE ROLE tccelta_api     LOGIN PASSWORD :'senha_api'     CONNECTION LIMIT 120;
CREATE ROLE tccelta_worker  LOGIN PASSWORD :'senha_worker'  CONNECTION LIMIT 20;
CREATE ROLE tccelta_provisionador LOGIN PASSWORD :'senha_provisionador' CONNECTION LIMIT 5;
CREATE ROLE tccelta_leitura NOLOGIN;
ALTER DATABASE tccelta OWNER TO tccelta_dono;
\c tccelta
-- Extensões não confiáveis exigem superusuário: criadas aqui, não na migration
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS postgis;
GRANT ALL ON SCHEMA public TO tccelta_dono;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
```

(O `.sh` chama `psql -v senha_dono="$(cat /run/secrets/pg_senha_dono)" ...`.) Os `GRANT`s por tabela ficam na migration `00008_permissoes.sql` (spec 02), porque as tabelas só existem depois.

### `Dockerfile` (multi-stage)

```dockerfile
# syntax=docker/dockerfile:1.7
ARG GO_VERSAO=1.24
FROM golang:${GO_VERSAO}-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSAO=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags "-s -w -X tccelta/backend/internal/versao.Versao=${VERSAO}" \
      -o /out/tccelta ./cmd/tccelta

# Alvo `ferramentas` (perfil `ferramentas`, `docker build --target ferramentas`): Python + o gerador de
# imagem NVS da Fase 1 (`provisionar novo` o chama por stdin, spec 07) + o mesmo binário. Fica ANTES do
# estágio final para o alvo padrão continuar sendo a imagem distroless da API/worker.
FROM python:3.12-slim AS ferramentas
RUN pip install --no-cache-dir esp-idf-nvs-partition-gen   # fixar a versão em requirements
COPY --from=build /out/tccelta /tccelta
USER 65532:65532
ENTRYPOINT ["/tccelta"]

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tccelta /tccelta
USER nonroot:nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/tccelta"]
```

As migrations vão embutidas no binário (`embed.FS`, spec 02), então a imagem não precisa de arquivos `.sql`. O subcomando `saude` substitui `curl`/`wget`, que não existem na distroless.

### `Caddyfile`

```caddyfile
{
	admin off
	servers { protocols h1 h2 }
}

{$API_HOST}:8443 {
	tls {$CADDY_TLS}
	encode zstd gzip
	request_body { max_size 64KB }
	reverse_proxy {
		dynamic a { name api port 8080 refresh 5s }
		lb_policy round_robin
		health_uri /saude/pronta
		health_interval 10s
		header_up X-Real-IP {remote_host}
	}
	log { output stdout format json }
}

# Atrás do túnel (perfil `tunel`): HTTP puro na rede `borda`, sem porta publicada; só o
# cloudflared alcança. O TLS público termina na Cloudflare. O IP real do celular vem no
# cabeçalho Cf-Connecting-Ip, que só é confiável aqui porque nada além do cloudflared
# chega a esta porta.
:8081 {
	encode zstd gzip
	request_body { max_size 64KB }
	reverse_proxy {
		dynamic a { name api port 8080 refresh 5s }
		lb_policy round_robin
		health_uri /saude/pronta
		health_interval 10s
		header_up X-Real-IP {header.Cf-Connecting-Ip}
	}
	log { output stdout format json }
}
```

- **[Decisão] O caminho padrão do app é o túnel** (`make up-tunel`, perfil `tunel`): certificado público da Cloudflare, nada a instalar no celular. A porta 8443 com `CADDY_TLS=internal` (perfil `tls`) fica como **plano B offline**, sem internet: emite certificado pela CA local do Caddy (funciona para IP de LAN, por exemplo `API_HOST=192.168.0.10`), e `make ca-exportar` copia `/data/caddy/pki/authorities/local/root.crt` para `saida/caddy-root.crt`, para instalar no celular (spec 09).
- Com domínio público, `CADDY_TLS=email@dominio` usa ACME. Nesse caso, publique também a porta 80/443 e use `API_PORTA_TLS=443`.
- `dynamic a` resolve `api` pelo DNS do Docker e distribui entre as réplicas.
- `request_body max_size 64KB` corta corpos absurdos antes da API (o lote máximo tem 32 × 1059 B ≈ 33,9 KB).

### `Makefile` (no Windows, use Git Bash + `make`; cada alvo também existe como `scripts/<alvo>.sh`)

```make
export MSYS_NO_PATHCONV = 1       # Git Bash: não reescrever /var/run/docker.sock, /src etc.
COMPOSE      = docker compose
COMPOSE_PL   = docker compose -f compose.yaml -f compose.prodlike.yaml
COMPOSE_T    = docker compose -f compose.test.yaml
GO_IMG       = golang:1.24-bookworm
SQLC_IMG     = sqlc/sqlc:1.27.0
LINT_IMG     = golangci/golangci-lint:v1.64.8      # ≥ v1.64 é obrigatório para módulos go 1.24
DOCKER_GO    = docker run --rm -v "$(CURDIR)":/src -w /src -v tccelta_gomod:/go/pkg/mod $(GO_IMG)
FERR         = run --rm ferramentas
SIM_DB       = --banco-url-env SIM_BANCO_URL

env:             ; [ -f .env ] || cp .env.example .env
segredos:        ; sh scripts/gerar-segredos.sh
certs:           ; sh scripts/gerar-certs.sh
build:           ; $(COMPOSE) build
up:              env segredos ; $(COMPOSE) up -d --build --wait
up-tls:          env segredos ; $(COMPOSE) --profile tls up -d --build --wait
up-tunel:        env segredos ; $(COMPOSE) --profile tunel up -d --build --wait
up-prodlike:     env segredos certs ; $(COMPOSE_PL) --profile tls up -d --build --wait
down:            ; $(COMPOSE) down
limpar:          ; $(COMPOSE) down -v      # apaga volumes (banco e vault)
logs:            ; $(COMPOSE) logs -f --tail=200 $(S)
ps:              ; $(COMPOSE) ps
migrar:          ; $(COMPOSE) run --rm migrar
migracao-nova:   ; docker run --rm -v "$(CURDIR)/internal/banco/migrations":/m -w /m ghcr.io/pressly/goose:v3.22.1 create $(NOME) sql -s
sqlc:            ; docker run --rm -v "$(CURDIR)":/src -w /src $(SQLC_IMG) generate
sqlc-verificar:  ; docker run --rm -v "$(CURDIR)":/src -w /src $(SQLC_IMG) diff
vault-bootstrap: ; $(COMPOSE) run --rm vault-bootstrap
# produção: unseal manual, 3 de 5 shares (cada operador digita a sua; repetir 3 vezes)
vault-unseal-manual: ; $(COMPOSE_PL) exec -it vault vault operator unseal
seed-dev:        ; $(COMPOSE) --profile ferramentas $(FERR) provisionar importar --arquivo /testdata/dongles-teste.csv
provisionar:     ; $(COMPOSE) --profile ferramentas $(FERR) provisionar novo --saida /saida --quantidade $(or $(N),1) --hw $(or $(HW),dev)
# refaz leitura/posicao a partir de bloco_bruto (spec 05); sem APLICAR=1 é só ensaio
reprocessar:     ; $(COMPOSE) --profile ferramentas $(FERR) worker reprocessar --dongle $(DONGLE) --desde-seq $(or $(DESDE),1) $(if $(APLICAR),--aplicar,)
ca-exportar:     ; $(COMPOSE) cp caddy:/data/caddy/pki/authorities/local/root.crt saida/caddy-root.crt
psql:            ; $(COMPOSE) exec banco psql -U postgres -d tccelta
teste:           ; $(DOCKER_GO) go test -race ./...
# Testcontainers de dentro de um contêiner: socket do Docker + host de saída; -p 1 evita subir
# vários timescaledb-ha (1,5 GB) ao mesmo tempo, um por pacote.
teste-integracao:; docker run --rm -v "$(CURDIR)":/src -w /src -v //var/run/docker.sock:/var/run/docker.sock \
                     -e TESTCONTAINERS_HOST_OVERRIDE=host.docker.internal --add-host=host.docker.internal:host-gateway \
                     -v tccelta_gomod:/go/pkg/mod $(GO_IMG) go test -p 1 -tags integracao -race ./...
vetores:         ; docker run --rm -v "$(CURDIR)":/src -w /src python:3.12-slim python scripts/gerar_vetores.py \
                   && cp internal/protocolo/testdata/vetores.json ../mobile-app/test/fixtures/protocolo_vetores.json
lint:            ; docker run --rm -v "$(CURDIR)":/src -w /src $(LINT_IMG) golangci-lint run
# e2e = stack de teste isolada (compose.test.yaml), seed, 3 cenários, derruba tudo (spec 10)
e2e:             env segredos
	$(COMPOSE_T) up -d --build --wait
	$(COMPOSE_T) --profile ferramentas $(FERR) provisionar importar --arquivo /testdata/dongles-teste.csv
	$(COMPOSE_T) --profile ferramentas $(FERR) simulador --cenario e2e --chaves /testdata/dongles-teste.csv --api http://api:8080 $(SIM_DB) \
	 && $(COMPOSE_T) --profile ferramentas $(FERR) simulador --cenario protocolo-negativo --chaves /testdata/dongles-teste.csv --api http://api:8080 $(SIM_DB) \
	 && $(COMPOSE_T) --profile ferramentas $(FERR) simulador --cenario tempo --acelerar 120 --chaves /testdata/dongles-teste.csv --api http://api:8080 $(SIM_DB) \
	 && $(COMPOSE_T) --profile ferramentas $(FERR) simulador --cenario geracao --chaves /testdata/dongles-teste.csv --api http://api:8080 $(SIM_DB); \
	 st=$$?; $(COMPOSE_T) down -v; exit $$st
carga:           ; $(COMPOSE) --profile ferramentas $(FERR) simulador --cenario carga --provisionar $(or $(N),1000) --api http://api:8080 $(SIM_DB) --relatorio /saida/relatorio.json
```

**[Decisão] Toda ferramenta roda em contêiner** (`go test`, `sqlc`, `goose`, `golangci-lint`, `python` dos vetores). O host só precisa de Docker e `make`/Git Bash. Para quem preferir, `go test ./...` também roda direto no host, porque o código não depende de nada do contêiner.

### `.env.example` (sem segredos)

```dotenv
AMBIENTE=dev                    # dev | prodlike | producao
IMG_TIMESCALE=timescale/timescaledb-ha:pg17
VAULT_VERSAO=1.18
API_REPLICAS=2
API_HOST=localhost              # IP da LAN para testar no celular
API_PORTA_TLS=8443
CADDY_TLS=internal
JWT_ROTACAO_PERIODO=720h       # rotação da chave do JWT (serviço jwt-rotacao); expurgo = JWT_VALIDADE + folga após a rotação
CLOUDFLARED_VERSAO=            # fixar a tag estável do cloudflared (perfil tunel)
TUNEL_HOSTNAME=                # ex.: api.seudominio.com.br — o app usa https://$TUNEL_HOSTNAME
LOG_NIVEL=info
LOG_FORMATO=json
# demais variáveis da API/worker: ver spec 03 (tabela de configuração)
```

### KMS no mesmo host agora, via VPN depois

Hoje, a API fala com o Vault por `http://vault:8200` (dev) ou `https://vault:8200` (prodlike), na rede bridge `kms`. Quando o Vault for para outro host atrás de uma VPN (WireGuard), mudam os itens abaixo. **Nenhum deles exige mudar código**: todos são configuração, porque o cliente do Vault (spec 07) já lê endereço, CA, timeouts e credenciais de variáveis de ambiente.

| Aspecto | Mesmo host (hoje) | Outro host via VPN |
|---|---|---|
| Endereço | `VAULT_ADDR=https://vault:8200` (DNS do Docker) | `VAULT_ADDR=https://kms.tccelta.interno:8200` (IP da interface `wg0` do host KMS, em `/etc/hosts` ou DNS interno). A rede `kms` some do compose da API; a API sai pela rota da VPN do host. |
| TLS | Opcional em dev; obrigatório no prodlike com CA privada | Obrigatório (TLS 1.3), com `VAULT_CACERT` apontando para a CA privada. Recomendado: **mTLS** (`tls_require_and_verify_client_cert=true` no listener, com certificado de cliente da API em `VAULT_CLIENT_CERT`/`VAULT_CLIENT_KEY`). A VPN cifra o enlace, mas o TLS protege contra outro nó comprometido dentro da VPN. |
| Autenticação | AppRole com `secret_id` num volume compartilhado | AppRole com `secret_id_bound_cidrs` e `token_bound_cidrs` = IP da API na VPN. O `secret_id` é entregue por *response wrapping* (`-wrap-ttl=5m`) e desembrulhado no primeiro boot. Alternativa: método `cert` (mTLS como identidade). |
| Exposição | Porta 8200 só na rede `kms` | Listener só no IP da `wg0`; firewall do host KMS aceita 8200/tcp apenas dos peers da API; nada exposto à internet. |
| Timeouts | `VAULT_TIMEOUT=2s`, 1 retentativa | `VAULT_TIMEOUT=5s`, 2 retentativas com backoff 100–400 ms, *circuit breaker* abre após 5 falhas seguidas por 10 s (a API responde 503 + `Retry-After`). Pool HTTP com keep-alive (`MaxIdleConnsPerHost=64`) para não pagar handshake TLS por chamada. |
| Latência no caminho quente | ~0,3–1 ms por chamada | ~5–30 ms por chamada. O lote faz 2 chamadas (spec 05), então o custo é por lote, não por bloco. |
| Operação | `docker compose` único | Dois composes: `backend/compose.yaml` sem o Vault (`VAULT_EXTERNO=1`) e `backend/deploy/kms/compose.kms.yaml` no host KMS. Backup do Raft com `vault operator raft snapshot save`. |

## Plano arquivo por arquivo

| Arquivo | Conteúdo |
|---|---|
| `backend/Dockerfile` | multi-stage acima |
| `backend/.dockerignore` | `saida/`, `deploy/segredos/`, `deploy/certs/`, `**/*_test.go` não (testes precisam) |
| `backend/compose.yaml` | base/dev |
| `backend/compose.prodlike.yaml` | sobreposição |
| `backend/compose.test.yaml` | stack mínima para e2e em CI: banco + redis + vault dev + migrar + api (sem caddy, sem túnel) |
| `backend/.env.example` | variáveis não secretas |
| `backend/Makefile` | alvos acima |
| `backend/scripts/gerar-segredos.sh` | gera os arquivos de `deploy/segredos/` se não existirem (idempotente) |
| `backend/scripts/gerar-certs.sh` | CA privada + certificado do Vault (+ cliente da API para mTLS) |
| `backend/deploy/postgres/init/00-papeis.sh` | papéis e extensões |
| `backend/deploy/vault/config/vault.hcl` | servidor prodlike |
| `backend/deploy/vault/scripts/bootstrap.sh` | spec 07 |
| `backend/deploy/vault/scripts/init-unseal.sh` | init/unseal de laboratório |
| `backend/deploy/vault/politicas/*.hcl` | spec 07 |
| `backend/deploy/caddy/Caddyfile` | proxy: TLS próprio na 8443 (plano B) e HTTP interno na 8081 (túnel) |
| `backend/deploy/redis/` | (vazio; a configuração vai na linha de comando do serviço) |
| `backend/deploy/observabilidade/prometheus.yml` | *scrape* de `api:9090` e `worker:9091` |
| `backend/deploy/kms/compose.kms.yaml` | Vault isolado para o cenário de VPN (só documentado e testado manualmente) |
| `backend/.gitignore` | `saida/`, `deploy/segredos/*`, `deploy/certs/*`, `.env` |

## Testes

- `make up` termina com `--wait` sem erro: todos os healthchecks ficam verdes e os jobs `migrar` e `vault-bootstrap` saem com código 0.
- `docker compose exec banco psql -U postgres -d tccelta -c "\dx"` lista `timescaledb` e `postgis`.
- `docker compose exec redis sh -c 'REDISCLI_AUTH=$(cat /run/secrets/redis_senha) redis-cli ping'` responde `PONG`; da máquina host, `redis-cli -h 127.0.0.1 ping` não conecta (porta não publicada).
- Perfil `tunel`: `curl https://$TUNEL_HOSTNAME/saude/pronta` responde 200 com certificado público válido (sem `--cacert`); `docker compose --profile tunel ps` mostra `cloudflared` ativo e nenhuma porta publicada por ele.
- `docker compose run --rm ferramentas saude --url http://api:8080/saude/pronta` devolve 0.
- No prodlike: `docker compose -f compose.yaml -f compose.prodlike.yaml exec api sh` falha (a distroless não tem shell). `docker network inspect tccelta_dados` mostra `"Internal": true`. `curl http://127.0.0.1:5432` no host falha, porque a porta não é publicada.
- Reinício a frio no prodlike: `docker compose restart vault` deixa o Vault selado. O `vault-unseal` (`make vault-unseal`) o desela, e a API volta a ficar pronta sozinha (relogin AppRole, spec 07).

## Passos ordenados com checkpoints

1. Criar `backend/` com `go.mod` mínimo e `cmd/tccelta/main.go` que só responde `saude` e `api` (servidor com `/saude/viva`). **Checkpoint:** `docker build` gera a imagem, e `docker run tccelta-backend:local saude --help` funciona.
2. `compose.yaml` com `banco` + `00-papeis.sh`. **Checkpoint:** `make up` e `\dx` mostra as extensões; `\du` mostra os papéis.
3. Adicionar `vault` dev + `vault-bootstrap` (por enquanto só habilita os engines). **Checkpoint:** `vault secrets list` mostra `transit-auth/`, `transit-rec/` e `transit-jwt/`.
4. Adicionar `migrar`, `api` e `worker` com `depends_on` condicionais. **Checkpoint:** `make up` sobe tudo na ordem certa; derrubar o banco deixa a API "não pronta" (healthcheck vermelho) sem ela cair.
5. Perfil `tunel` com Caddy + `cloudflared` (caminho padrão do app) e, como plano B offline, o perfil `tls`. **Checkpoint:** `curl https://$TUNEL_HOSTNAME/saude/pronta` dá 200; `curl --cacert saida/caddy-root.crt https://localhost:8443/saude/pronta` dá 200 no `tls`.
6. Perfil prodlike (certs, Raft, init/unseal, redes internas). **Checkpoint:** os testes do prodlike acima passam.
7. Observabilidade (opcional). **Checkpoint:** o Prometheus mostra os alvos `api` e `worker` como *up*.

## Critérios de aceite

- [ ] `make up` sobe a stack dev do zero numa máquina só com Docker (Windows + Docker Desktop e Linux).
- [ ] `make up-prodlike` sobe com Vault persistente e TLS; os dados do Vault e do banco sobrevivem a `docker compose down` (sem `-v`).
- [ ] O worker não alcança o Vault (`docker compose exec worker /tccelta saude --url http://vault:8200/v1/sys/health` falha por DNS/rede).
- [ ] O Redis só é alcançável da rede `dados` e com senha; a API segue de pé (limite local) quando ele cai.
- [ ] O `jwt-rotacao` só enxerga a rede `kms` (sem banco) e sua credencial não está montada na API; o token da API segue sem poder rotacionar (spec 07, testes de política).
- [ ] Com `make up-tunel`, o app fala com `https://$TUNEL_HOSTNAME` sem instalar certificado no celular.
- [ ] Nenhum segredo em `compose*.yaml`, `.env.example` ou no git.
- [ ] O documento descreve e valida (manualmente) o cenário de Vault externo com `VAULT_ADDR`/`VAULT_CACERT`.

## Riscos

| Risco | Mitigação |
|---|---|
| `timescaledb-ha` é uma imagem grande (~1,5 GB) | aceitável no TCC; fixar o digest para não baixar de novo |
| `deploy.replicas` com porta publicada conflita no Windows | `API_REPLICAS=1` em dev; o acesso multi-réplica é pelo Caddy |
| Volume `vault_init` guarda a chave de unseal junto com os dados | só no laboratório, com aviso no script e recusa em `AMBIENTE=producao` |
| `!reset` exige Compose ≥ 2.24 | `make` verifica `docker compose version` e falha com mensagem clara |
| Relógio do host sem NTP afeta S1/S2 (âncoras) | checklist do README: `w32tm /query /status` (Windows) ou `timedatectl` no VPS; o contêiner usa o relógio do kernel |
| O túnel passa o tráfego por um terceiro: a Cloudflare termina o TLS e **vê em claro** telemetria e GPS (os blocos continuam autenticados por HMAC, mas não são secretos) | Aceito no TCC e registrado como limitação; o plano B `tls` (CA local) mantém o tráfego só entre app e servidor; para produção, TLS de ponta a ponta (domínio próprio + ACME no Caddy) |
| Depende de conta Cloudflare, de um domínio e da internet no momento do teste | Plano B `tls` na LAN; o túnel é de saída (nenhuma porta aberta no roteador) |
| `Cf-Connecting-Ip` forjável se a 8081 for alcançável por outro caminho | A 8081 não é publicada nem roteada além da rede `borda`; só o `cloudflared` está nela com o Caddy |
| Redis fora do ar | A API cai para o limitador local por réplica (spec 03), com métrica e log; nunca bloqueia o tráfego por causa disso |

## Decisões do autor (antes em aberto)

- **[Decisão] Unseal manual 3 de 5; auto-unseal só documentado.** Em produção o Vault é inicializado com `vault operator init -key-shares=5 -key-threshold=3` e as 5 shares vão para pessoas diferentes; após cada reinício do Vault, três operadores rodam `make vault-unseal-manual` (uma share cada). O 1/1 do `init-unseal.sh` fica só no laboratório. *Transit auto-unseal* com um segundo Vault mínimo, ou o KMS do provedor do VPS, ficam **apenas descritos** no texto do TCC (sem infraestrutura extra); a consequência aceita é que reiniciar o Vault exige intervenção humana. Vault em HA (réplicas Raft) também só documentado.
- **[Decisão] Backup só documentado, não automatizado.** No texto do TCC: pgBackRest para o banco (o doc cita; `pgbackrest backup --type=full` + arquivamento de WAL) e `vault operator raft snapshot save` para o Vault (sem o snapshot, perder o volume do Vault é perder `K_auth`/`K_rec` de todos os dongles). Nenhum serviço, cron ou teste de restauração entra no Compose desta fase.
- **[Decisão] Túnel com certificado público desde já** (Cloudflare Tunnel; "ou similar" = qualquer túnel de saída com TLS público). Perfil `tunel`, `make up-tunel`; o app usa `https://$TUNEL_HOSTNAME` e não instala CA no celular. A CA do Caddy (`tls`) fica como plano B offline. Limitação registrada: o provedor do túnel termina o TLS (ver Riscos).
- **[Decisão] Redis para o rate limit** (spec 03): serviço `redis` na rede `dados`, sem persistência, com senha; a API degrada para o limitador local se ele falhar.
- **[Decisão] HTTP/3 não habilitado** no Caddy (`protocols h1 h2`; o `dio` do app usa HTTP 1.1/2; sem porta UDP).
