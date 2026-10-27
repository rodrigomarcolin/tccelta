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
      -ldflags "-s -w -X main.versao=${VERSAO}" \
      -o /out/tccelta-kms ./cmd/tccelta-kms

# Alvo `ferramentas`: Python + gerador de imagem NVS (usado por `provisionar novo`) + o mesmo binario.
FROM python:3.12-slim AS ferramentas
RUN pip install --no-cache-dir esp-idf-nvs-partition-gen cryptography
COPY --from=build /out/tccelta-kms /tccelta-kms
USER 65532:65532
ENTRYPOINT ["/tccelta-kms"]

# Alvo `kms` (padrao): o servico e a rotacao do JWT, em imagem minima.
FROM gcr.io/distroless/static-debian12:nonroot AS kms
COPY --from=build /out/tccelta-kms /tccelta-kms
USER nonroot:nonroot
EXPOSE 8300 9300
ENTRYPOINT ["/tccelta-kms"]
