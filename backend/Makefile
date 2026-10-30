export MSYS_NO_PATHCONV = 1
COMPOSE      = docker compose
COMPOSE_PL   = docker compose -f compose.yaml -f compose.prodlike.yaml
COMPOSE_T    = docker compose -f compose.test.yaml
GO_IMG       = golang:1.26-bookworm
SQLC_IMG     = sqlc/sqlc:1.27.0
LINT_IMG     = golangci/golangci-lint:v1.64.8
DOCKER_GO    = docker run --rm -v "$(CURDIR)":/src -w /src -v tccelta_gomod:/go/pkg/mod $(GO_IMG)
TOOLS        = run --rm ferramentas
KMS_TOOLS    = run --rm kms-ferramentas
SIM_DB       = --db-url-env SIM_BANCO_URL

# Dongles novos para um cenário do simulador (stack de teste): $(call E2E_NEW_DONGLES,<scenario>). O dongle virtual
# sempre recomeça do seq 1, e o servidor guarda o estado por DevId; reusar um DevId que outro cenário já
# avançou dá 422 resposta_inconsistente. Cada cenário recebe 3 dongles recém-criados (KMS, depois banco).
E2E_NEW_DONGLES = ( rm -rf saida/e2e-$(1) saida/dongles-$(1).csv || true ) \
	&& $(COMPOSE_T) --profile ferramentas $(KMS_TOOLS) provision new --no-material --count 3 --out /saida/e2e-$(1) --csv-out /saida/dongles-$(1).csv \
	&& $(COMPOSE_T) --profile ferramentas $(TOOLS) provision register --file /saida/e2e-$(1)
E2E_SIMULATE    = $(COMPOSE_T) --profile ferramentas $(TOOLS) simulator --keys /saida/dongles-$(1).csv --api http://api:8080 $(SIM_DB) --scenario $(1)

.PHONY: env secrets certs build up up-tls up-tunnel up-prodlike down clean logs ps migrate new-migration sqlc sqlc-check \
	vault-bootstrap vault-unseal vault-unseal-manual output-dir seed-dev provision kms-wrapping kms-import reprocess ca-export psql \
	test test-integration vectors lint e2e load-test scenario build-tools build-kms docker-kms down-all clean-all

env:             ; [ -f .env ] || cp .env.example .env
secrets:         ; sh scripts/gerar-segredos.sh
certs:           ; sh scripts/gerar-certs.sh
build:           ; $(COMPOSE) build
up:              env secrets ; $(COMPOSE) up -d --build --wait
up-tls:          env secrets ; $(COMPOSE) --profile tls up -d --build --wait
up-tunnel:       env secrets ; $(COMPOSE) --profile tunel up -d --build --wait
up-prodlike:     env secrets certs ; $(COMPOSE_PL) --profile tls up -d --build --wait
down:            ; $(COMPOSE) down
clean:           ; $(COMPOSE) down -v
logs:            ; $(COMPOSE) logs -f --tail=200 $(S)
ps:              ; $(COMPOSE) ps
migrate:         ; $(COMPOSE) run --rm migrar
new-migration:   ; docker run --rm -v "$(CURDIR)/internal/database/migrations":/m -w /m ghcr.io/pressly/goose:v3.22.1 create $(NAME) sql -s
sqlc:            ; docker run --rm -v "$(CURDIR)":/src -w /src $(SQLC_IMG) generate
sqlc-check:      ; docker run --rm -v "$(CURDIR)":/src -w /src $(SQLC_IMG) diff
vault-bootstrap: ; $(COMPOSE) run --rm vault-bootstrap
vault-unseal:    ; $(COMPOSE_PL) run --rm vault-unseal
vault-unseal-manual: ; $(COMPOSE_PL) exec -it vault vault operator unseal
seed-dev:        output-dir
	$(COMPOSE) --profile ferramentas $(KMS_TOOLS) provision import --file /testdata/test-dongles.csv --out /saida/registros
	$(COMPOSE) --profile ferramentas $(TOOLS) provision register --file /saida/registros
provision:       output-dir
	$(COMPOSE) --profile ferramentas $(KMS_TOOLS) provision new --out /saida --count $(or $(N),1) --hw $(or $(HW),dev)
	$(COMPOSE) --profile ferramentas $(TOOLS) provision register --file /saida
kms-wrapping:    output-dir ; $(COMPOSE) --profile ferramentas $(KMS_TOOLS) provision export-wrapping --out /saida/kms_wrapping.json
kms-import:      output-dir
	$(COMPOSE) --profile ferramentas $(KMS_TOOLS) provision import-record --file $(FILE) --out /saida/registros
	$(COMPOSE) --profile ferramentas $(TOOLS) provision register --file /saida/registros
output-dir:      ; mkdir -p saida && chmod a+rwx saida
reprocess:       ; $(COMPOSE) --profile ferramentas $(TOOLS) worker reprocess --dongle $(DONGLE) --from-seq $(or $(FROM),1) $(if $(APPLY),--apply,)
ca-export:       ; $(COMPOSE) cp caddy:/data/caddy/pki/authorities/local/root.crt saida/caddy-root.crt
psql:            ; $(COMPOSE) exec banco psql -U postgres -d tccelta
test:            ; $(DOCKER_GO) go test -race ./...
test-integration: ; docker run --rm -v "$(CURDIR)":/src -w /src -v //var/run/docker.sock:/var/run/docker.sock \
                     -e TESTCONTAINERS_HOST_OVERRIDE=host.docker.internal --add-host=host.docker.internal:host-gateway \
                     -v tccelta_gomod:/go/pkg/mod $(GO_IMG) go test -p 1 -tags integration -race ./...
vectors:         ; docker run --rm -v "$(CURDIR)":/src -w /src python:3.12-slim python scripts/gerar_vetores.py \
                   && cp internal/domain/protocol/testdata/vetores.json ../mobile-app/test/fixtures/protocolo_vetores.json
lint:            ; docker run --rm -v "$(CURDIR)":/src -w /src $(LINT_IMG) golangci-lint run
build-tools:
	$(COMPOSE) --profile ferramentas build ferramentas kms-ferramentas
scenario:        output-dir build-tools
	@[ -n "$(C)" ] || { echo "uso: make scenario C=<e2e|negative-protocol|time|generation|reconnection|...> [N=3] [ARGS='--speedup 120']"; exit 1; }
	-rm -rf saida/cenario-$(C) saida/dongles-$(C).csv
	$(COMPOSE) --profile ferramentas $(KMS_TOOLS) provision new --no-material --count $(or $(N),3) --out /saida/cenario-$(C) --csv-out /saida/dongles-$(C).csv
	$(COMPOSE) --profile ferramentas $(TOOLS) provision register --file /saida/cenario-$(C)
	$(COMPOSE) --profile ferramentas $(TOOLS) simulator --scenario $(C) --keys /saida/dongles-$(C).csv --api http://api:8080 $(SIM_DB) $(ARGS)
e2e:             env secrets output-dir
	chmod a+r deploy/segredos/*
	$(COMPOSE_T) up -d --build --wait
	$(COMPOSE_T) --profile ferramentas build ferramentas kms-ferramentas
	$(call E2E_NEW_DONGLES,e2e) && $(call E2E_SIMULATE,e2e) \
	 && $(call E2E_NEW_DONGLES,negative-protocol) && $(call E2E_SIMULATE,negative-protocol) \
	 && $(call E2E_NEW_DONGLES,time) && $(call E2E_SIMULATE,time) --speedup 120 \
	 && $(call E2E_NEW_DONGLES,generation) && $(call E2E_SIMULATE,generation); \
	 st=$$?; $(COMPOSE_T) down -v; exit $$st
load-test:       output-dir
	rm -f saida/dongles-carga.csv
	$(COMPOSE) --profile ferramentas $(KMS_TOOLS) provision new --no-material --count $(or $(N),1000) --out /saida/carga --csv-out /saida/dongles-carga.csv
	$(COMPOSE) --profile ferramentas $(TOOLS) provision register --file /saida/carga
	$(COMPOSE) --profile ferramentas $(TOOLS) simulator --scenario load --keys /saida/dongles-carga.csv --api http://api:8080 $(SIM_DB) --report /saida/relatorio.json

# --- KMS (cmd/tccelta-kms): mesmo modulo Go; imagem e processo proprios ---
# `down-all` e `clean-all` derrubam todos os servicos, inclusive os de perfis desligados (caddy, ferramentas...)
# e os do overlay prodlike, que o `down` comum deixa para trás com a rede antiga ("network ... not found" no
# próximo `up`).
STACK = docker compose -f compose.yaml -f compose.prodlike.yaml \
        --profile tls --profile tunel --profile ferramentas --profile observabilidade
build-kms:       ; $(DOCKER_GO) go build -trimpath -o /tmp/tccelta-kms ./cmd/tccelta-kms
docker-kms:      ; docker build --target kms -t tccelta-kms:local .
down-all:        ; $(STACK) down --remove-orphans
clean-all:       ; $(STACK) down -v --remove-orphans
