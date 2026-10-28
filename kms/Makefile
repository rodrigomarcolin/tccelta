export MSYS_NO_PATHCONV = 1
GO_IMG    = golang:1.24-bookworm
LINT_IMG  = golangci/golangci-lint:v1.64.8
DOCKER_GO = docker run --rm -v "$(CURDIR)":/src -w /src -v tccelta_gomod:/go/pkg/mod $(GO_IMG)

# A stack Docker vive em ../backend. Estes alvos derrubam todos os serviços, inclusive os de perfis
# desligados (caddy, ferramentas...) e os do overlay prodlike, que o `down` comum deixa para trás
# com a rede antiga ("network ... not found" no próximo `up`).
STACK = docker compose -f ../backend/compose.yaml -f ../backend/compose.prodlike.yaml \
        --profile tls --profile tunel --profile ferramentas --profile observabilidade

.PHONY: teste lint build certs docker down limpar
teste:  ; $(DOCKER_GO) go test -race ./...
lint:   ; docker run --rm -v "$(CURDIR)":/src -w /src $(LINT_IMG) golangci-lint run
build:  ; $(DOCKER_GO) go build -trimpath -o /tmp/tccelta-kms ./cmd/tccelta-kms
certs:  ; sh scripts/gerar-certs.sh
docker: ; docker build --target kms -t tccelta-kms:local .
down:   ; $(STACK) down --remove-orphans
limpar: ; $(STACK) down -v --remove-orphans
