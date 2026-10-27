export MSYS_NO_PATHCONV = 1
GO_IMG    = golang:1.24-bookworm
LINT_IMG  = golangci/golangci-lint:v1.64.8
DOCKER_GO = docker run --rm -v "$(CURDIR)":/src -w /src -v tccelta_gomod:/go/pkg/mod $(GO_IMG)

.PHONY: teste lint build certs docker
teste:  ; $(DOCKER_GO) go test -race ./...
lint:   ; docker run --rm -v "$(CURDIR)":/src -w /src $(LINT_IMG) golangci-lint run
build:  ; $(DOCKER_GO) go build -trimpath -o /tmp/tccelta-kms ./cmd/tccelta-kms
certs:  ; sh scripts/gerar-certs.sh
docker: ; docker build --target kms -t tccelta-kms:local .
