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

# Alvo `ferramentas`: Python + gerador de imagem NVS (usado por `provisionar novo`) + o mesmo binario.
FROM python:3.12-slim AS ferramentas
RUN pip install --no-cache-dir esp-idf-nvs-partition-gen cryptography
COPY --from=build /out/tccelta /tccelta
USER 65532:65532
ENTRYPOINT ["/tccelta"]

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tccelta /tccelta
USER nonroot:nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/tccelta"]
