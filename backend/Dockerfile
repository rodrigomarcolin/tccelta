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
      -ldflags "-s -w -X tccelta/backend/internal/version.Version=${VERSAO}" \
      -o /out/tccelta ./cmd/tccelta
# O KMS e outro binario, outra imagem e outro processo (unico cliente do Vault); so compartilha o modulo Go.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSAO}" \
      -o /out/tccelta-kms ./cmd/tccelta-kms

# Alvo `ferramentas`: o binario da API, para `provision register`, simulator e reprocess.
# (A parte Vault do provisionamento e o gerador de imagem NVS ficam no alvo `kms-tools`.)
FROM gcr.io/distroless/static-debian12:nonroot AS ferramentas
COPY --from=build /out/tccelta /tccelta
USER nonroot:nonroot
ENTRYPOINT ["/tccelta"]

# Alvo `kms-tools`: Python + gerador de imagem NVS (usado por `provision new`) + o binario do KMS.
FROM python:3.12-slim AS kms-tools
RUN pip install --no-cache-dir esp-idf-nvs-partition-gen cryptography
COPY --from=build /out/tccelta-kms /tccelta-kms
USER 65532:65532
ENTRYPOINT ["/tccelta-kms"]

# Alvo `kms`: o servico e a rotacao do JWT, em imagem minima.
FROM gcr.io/distroless/static-debian12:nonroot AS kms
COPY --from=build /out/tccelta-kms /tccelta-kms
USER nonroot:nonroot
EXPOSE 8300 9300
ENTRYPOINT ["/tccelta-kms"]

# Alvo padrao (ultimo): a API.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tccelta /tccelta
USER nonroot:nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/tccelta"]
