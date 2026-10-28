export MSYS_NO_PATHCONV = 1
COMPOSE      = docker compose
COMPOSE_PL   = docker compose -f compose.yaml -f compose.prodlike.yaml
COMPOSE_T    = docker compose -f compose.test.yaml
GO_IMG       = golang:1.24-bookworm
SQLC_IMG     = sqlc/sqlc:1.27.0
LINT_IMG     = golangci/golangci-lint:v1.64.8
DOCKER_GO    = docker run --rm -v "$(CURDIR)":/src -w /src -v tccelta_gomod:/go/pkg/mod $(GO_IMG)
FERR         = run --rm ferramentas
KMSF         = run --rm kms-ferramentas
SIM_DB       = --db-url-env SIM_BANCO_URL

# Dongles novos para um cenário do simulador (stack de teste): $(call E2E_NOVOS,<cenario>). O dongle virtual
# sempre recomeça do seq 1, e o servidor guarda o estado por DevId; reusar um DevId que outro cenário já
# avançou dá 422 resposta_inconsistente. Cada cenário recebe 3 dongles recém-criados (KMS, depois banco).
E2E_NOVOS     = ( rm -rf saida/e2e-$(1) saida/dongles-$(1).csv || true ) \
	&& $(COMPOSE_T) --profile ferramentas $(KMSF) provision new --no-material --count 3 --out /saida/e2e-$(1) --csv-out /saida/dongles-$(1).csv \
	&& $(COMPOSE_T) --profile ferramentas $(FERR) provision register --file /saida/e2e-$(1)
E2E_SIMULAR   = $(COMPOSE_T) --profile ferramentas $(FERR) simulator --keys /saida/dongles-$(1).csv --api http://api:8080 $(SIM_DB) --scenario $(1)

.PHONY: env segredos certs build up up-tls up-tunel up-prodlike down limpar logs ps migrar migracao-nova sqlc sqlc-verificar \
	vault-bootstrap vault-unseal vault-unseal-manual saida-dir seed-dev provisionar kms-wrapping kms-importar reprocessar ca-exportar psql \
	teste teste-integracao vetores lint e2e carga cenario build-ferramentas build-kms docker-kms down-tudo limpar-tudo

env:             ; [ -f .env ] || cp .env.example .env
segredos:        ; sh scripts/gerar-segredos.sh
certs:           ; sh scripts/gerar-certs.sh
build:           ; $(COMPOSE) build
up:              env segredos ; $(COMPOSE) up -d --build --wait
up-tls:          env segredos ; $(COMPOSE) --profile tls up -d --build --wait
up-tunel:        env segredos ; $(COMPOSE) --profile tunel up -d --build --wait
up-prodlike:     env segredos certs ; $(COMPOSE_PL) --profile tls up -d --build --wait
down:            ; $(COMPOSE) down
limpar:          ; $(COMPOSE) down -v
logs:            ; $(COMPOSE) logs -f --tail=200 $(S)
ps:              ; $(COMPOSE) ps
migrar:          ; $(COMPOSE) run --rm migrar
migracao-nova:   ; docker run --rm -v "$(CURDIR)/internal/database/migrations":/m -w /m ghcr.io/pressly/goose:v3.22.1 create $(NOME) sql -s
sqlc:            ; docker run --rm -v "$(CURDIR)":/src -w /src $(SQLC_IMG) generate
sqlc-verificar:  ; docker run --rm -v "$(CURDIR)":/src -w /src $(SQLC_IMG) diff
vault-bootstrap: ; $(COMPOSE) run --rm vault-bootstrap
vault-unseal:    ; $(COMPOSE_PL) run --rm vault-unseal
vault-unseal-manual: ; $(COMPOSE_PL) exec -it vault vault operator unseal
seed-dev:        saida-dir
	$(COMPOSE) --profile ferramentas $(KMSF) provision import --file /testdata/test-dongles.csv --out /saida/registros
	$(COMPOSE) --profile ferramentas $(FERR) provision register --file /saida/registros
provisionar:     saida-dir
	$(COMPOSE) --profile ferramentas $(KMSF) provision new --out /saida --count $(or $(N),1) --hw $(or $(HW),dev)
	$(COMPOSE) --profile ferramentas $(FERR) provision register --file /saida
kms-wrapping:    saida-dir ; $(COMPOSE) --profile ferramentas $(KMSF) provision export-wrapping --out /saida/kms_wrapping.json
kms-importar:    saida-dir
	$(COMPOSE) --profile ferramentas $(KMSF) provision import-record --file $(ARQ) --out /saida/registros
	$(COMPOSE) --profile ferramentas $(FERR) provision register --file /saida/registros
saida-dir:       ; mkdir -p saida && chmod a+rwx saida
reprocessar:     ; $(COMPOSE) --profile ferramentas $(FERR) worker reprocess --dongle $(DONGLE) --from-seq $(or $(DESDE),1) $(if $(APLICAR),--apply,)
ca-exportar:     ; $(COMPOSE) cp caddy:/data/caddy/pki/authorities/local/root.crt saida/caddy-root.crt
psql:            ; $(COMPOSE) exec banco psql -U postgres -d tccelta
teste:           ; $(DOCKER_GO) go test -race ./...
teste-integracao:; docker run --rm -v "$(CURDIR)":/src -w /src -v //var/run/docker.sock:/var/run/docker.sock \
                     -e TESTCONTAINERS_HOST_OVERRIDE=host.docker.internal --add-host=host.docker.internal:host-gateway \
                     -v tccelta_gomod:/go/pkg/mod $(GO_IMG) go test -p 1 -tags integration -race ./...
vetores:         ; docker run --rm -v "$(CURDIR)":/src -w /src python:3.12-slim python scripts/gerar_vetores.py \
                   && cp internal/domain/protocol/testdata/vetores.json ../mobile-app/test/fixtures/protocolo_vetores.json
lint:            ; docker run --rm -v "$(CURDIR)":/src -w /src $(LINT_IMG) golangci-lint run
build-ferramentas:
	$(COMPOSE) --profile ferramentas build ferramentas kms-ferramentas
cenario:         saida-dir build-ferramentas
	@[ -n "$(C)" ] || { echo "uso: make cenario C=<e2e|negative-protocol|time|generation|reconnection|...> [N=3] [ARGS='--speedup 120']"; exit 1; }
	-rm -rf saida/cenario-$(C) saida/dongles-$(C).csv
	$(COMPOSE) --profile ferramentas $(KMSF) provision new --no-material --count $(or $(N),3) --out /saida/cenario-$(C) --csv-out /saida/dongles-$(C).csv
	$(COMPOSE) --profile ferramentas $(FERR) provision register --file /saida/cenario-$(C)
	$(COMPOSE) --profile ferramentas $(FERR) simulator --scenario $(C) --keys /saida/dongles-$(C).csv --api http://api:8080 $(SIM_DB) $(ARGS)
e2e:             env segredos saida-dir
	$(COMPOSE_T) up -d --build --wait
	$(COMPOSE_T) --profile ferramentas build ferramentas kms-ferramentas
	$(call E2E_NOVOS,e2e) && $(call E2E_SIMULAR,e2e) \
	 && $(call E2E_NOVOS,negative-protocol) && $(call E2E_SIMULAR,negative-protocol) \
	 && $(call E2E_NOVOS,time) && $(call E2E_SIMULAR,time) --speedup 120 \
	 && $(call E2E_NOVOS,generation) && $(call E2E_SIMULAR,generation); \
	 st=$$?; $(COMPOSE_T) down -v; exit $$st
carga:           saida-dir
	rm -f saida/dongles-carga.csv
	$(COMPOSE) --profile ferramentas $(KMSF) provision new --no-material --count $(or $(N),1000) --out /saida/carga --csv-out /saida/dongles-carga.csv
	$(COMPOSE) --profile ferramentas $(FERR) provision register --file /saida/carga
	$(COMPOSE) --profile ferramentas $(FERR) simulator --scenario load --keys /saida/dongles-carga.csv --api http://api:8080 $(SIM_DB) --report /saida/relatorio.json

# --- KMS (cmd/tccelta-kms): mesmo modulo Go; imagem e processo proprios ---
# `down-tudo` e `limpar-tudo` derrubam todos os servicos, inclusive os de perfis desligados (caddy, ferramentas...)
# e os do overlay prodlike, que o `down` comum deixa para trás com a rede antiga ("network ... not found" no
# próximo `up`).
STACK = docker compose -f compose.yaml -f compose.prodlike.yaml \
        --profile tls --profile tunel --profile ferramentas --profile observabilidade
build-kms:       ; $(DOCKER_GO) go build -trimpath -o /tmp/tccelta-kms ./cmd/tccelta-kms
docker-kms:      ; docker build --target kms -t tccelta-kms:local .
down-tudo:       ; $(STACK) down --remove-orphans
limpar-tudo:     ; $(STACK) down -v --remove-orphans
