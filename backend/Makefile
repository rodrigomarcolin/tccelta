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
SIM_DB       = --banco-url-env SIM_BANCO_URL

# Dongles novos para um cenário do simulador (stack de teste): $(call E2E_NOVOS,<cenario>). O dongle virtual
# sempre recomeça do seq 1, e o servidor guarda o estado por DevId; reusar um DevId que outro cenário já
# avançou dá 422 resposta_inconsistente. Cada cenário recebe 3 dongles recém-criados (KMS, depois banco).
E2E_NOVOS     = ( rm -rf saida/e2e-$(1) saida/dongles-$(1).csv || true ) \
	&& $(COMPOSE_T) --profile ferramentas $(KMSF) provisionar novo --sem-material --quantidade 3 --saida /saida/e2e-$(1) --csv-saida /saida/dongles-$(1).csv \
	&& $(COMPOSE_T) --profile ferramentas $(FERR) provisionar registrar --arquivo /saida/e2e-$(1)
E2E_SIMULAR   = $(COMPOSE_T) --profile ferramentas $(FERR) simulador --chaves /saida/dongles-$(1).csv --api http://api:8080 $(SIM_DB) --cenario $(1)

.PHONY: env segredos certs build up up-tls up-tunel up-prodlike down limpar logs ps migrar migracao-nova sqlc sqlc-verificar \
	vault-bootstrap vault-unseal vault-unseal-manual saida-dir seed-dev provisionar kms-wrapping kms-importar reprocessar ca-exportar psql \
	teste teste-integracao vetores lint e2e carga cenario build-ferramentas

env:             ; [ -f .env ] || cp .env.example .env
segredos:        ; sh scripts/gerar-segredos.sh
certs:           ; sh ../kms/scripts/gerar-certs.sh
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
migracao-nova:   ; docker run --rm -v "$(CURDIR)/internal/banco/migrations":/m -w /m ghcr.io/pressly/goose:v3.22.1 create $(NOME) sql -s
sqlc:            ; docker run --rm -v "$(CURDIR)":/src -w /src $(SQLC_IMG) generate
sqlc-verificar:  ; docker run --rm -v "$(CURDIR)":/src -w /src $(SQLC_IMG) diff
vault-bootstrap: ; $(COMPOSE) run --rm vault-bootstrap
vault-unseal:    ; $(COMPOSE_PL) run --rm vault-unseal
vault-unseal-manual: ; $(COMPOSE_PL) exec -it vault vault operator unseal
seed-dev:        saida-dir
	$(COMPOSE) --profile ferramentas $(KMSF) provisionar importar --arquivo /testdata/dongles-teste.csv --saida /saida/registros
	$(COMPOSE) --profile ferramentas $(FERR) provisionar registrar --arquivo /saida/registros
provisionar:     saida-dir
	$(COMPOSE) --profile ferramentas $(KMSF) provisionar novo --saida /saida --quantidade $(or $(N),1) --hw $(or $(HW),dev)
	$(COMPOSE) --profile ferramentas $(FERR) provisionar registrar --arquivo /saida
kms-wrapping:    saida-dir ; $(COMPOSE) --profile ferramentas $(KMSF) provisionar exportar-wrapping --saida /saida/kms_wrapping.json
kms-importar:    saida-dir
	$(COMPOSE) --profile ferramentas $(KMSF) provisionar importar-registro --arquivo $(ARQ) --saida /saida/registros
	$(COMPOSE) --profile ferramentas $(FERR) provisionar registrar --arquivo /saida/registros
saida-dir:       ; mkdir -p saida && chmod a+rwx saida
reprocessar:     ; $(COMPOSE) --profile ferramentas $(FERR) worker reprocessar --dongle $(DONGLE) --desde-seq $(or $(DESDE),1) $(if $(APLICAR),--aplicar,)
ca-exportar:     ; $(COMPOSE) cp caddy:/data/caddy/pki/authorities/local/root.crt saida/caddy-root.crt
psql:            ; $(COMPOSE) exec banco psql -U postgres -d tccelta
teste:           ; $(DOCKER_GO) go test -race ./...
teste-integracao:; docker run --rm -v "$(CURDIR)":/src -w /src -v //var/run/docker.sock:/var/run/docker.sock \
                     -e TESTCONTAINERS_HOST_OVERRIDE=host.docker.internal --add-host=host.docker.internal:host-gateway \
                     -v tccelta_gomod:/go/pkg/mod $(GO_IMG) go test -p 1 -tags integracao -race ./...
vetores:         ; docker run --rm -v "$(CURDIR)":/src -w /src python:3.12-slim python scripts/gerar_vetores.py \
                   && cp internal/dominio/protocolo/testdata/vetores.json ../mobile-app/test/fixtures/protocolo_vetores.json
lint:            ; docker run --rm -v "$(CURDIR)":/src -w /src $(LINT_IMG) golangci-lint run
build-ferramentas:
	$(COMPOSE) --profile ferramentas build ferramentas kms-ferramentas
cenario:         saida-dir build-ferramentas
	@[ -n "$(C)" ] || { echo "uso: make cenario C=<e2e|protocolo-negativo|tempo|geracao|reconexao|...> [N=3] [ARGS='--acelerar 120']"; exit 1; }
	-rm -rf saida/cenario-$(C) saida/dongles-$(C).csv
	$(COMPOSE) --profile ferramentas $(KMSF) provisionar novo --sem-material --quantidade $(or $(N),3) --saida /saida/cenario-$(C) --csv-saida /saida/dongles-$(C).csv
	$(COMPOSE) --profile ferramentas $(FERR) provisionar registrar --arquivo /saida/cenario-$(C)
	$(COMPOSE) --profile ferramentas $(FERR) simulador --cenario $(C) --chaves /saida/dongles-$(C).csv --api http://api:8080 $(SIM_DB) $(ARGS)
e2e:             env segredos saida-dir
	$(COMPOSE_T) up -d --build --wait
	$(COMPOSE_T) --profile ferramentas build ferramentas kms-ferramentas
	$(call E2E_NOVOS,e2e) && $(call E2E_SIMULAR,e2e) \
	 && $(call E2E_NOVOS,protocolo-negativo) && $(call E2E_SIMULAR,protocolo-negativo) \
	 && $(call E2E_NOVOS,tempo) && $(call E2E_SIMULAR,tempo) --acelerar 120 \
	 && $(call E2E_NOVOS,geracao) && $(call E2E_SIMULAR,geracao); \
	 st=$$?; $(COMPOSE_T) down -v; exit $$st
carga:           saida-dir
	rm -f saida/dongles-carga.csv
	$(COMPOSE) --profile ferramentas $(KMSF) provisionar novo --sem-material --quantidade $(or $(N),1000) --saida /saida/carga --csv-saida /saida/dongles-carga.csv
	$(COMPOSE) --profile ferramentas $(FERR) provisionar registrar --arquivo /saida/carga
	$(COMPOSE) --profile ferramentas $(FERR) simulador --cenario carga --chaves /saida/dongles-carga.csv --api http://api:8080 $(SIM_DB) --relatorio /saida/relatorio.json
