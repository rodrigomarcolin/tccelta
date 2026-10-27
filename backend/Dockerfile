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

# Alvo `ferramentas`: o mesmo binario, para `provisionar registrar`, simulador e reprocessar.
# (A parte Vault do provisionamento e o gerador de imagem NVS ficam no alvo `ferramentas` do modulo ../kms.)
FROM gcr.io/distroless/static-debian12:nonroot AS ferramentas
COPY --from=build /out/tccelta /tccelta
USER nonroot:nonroot
ENTRYPOINT ["/tccelta"]

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tccelta /tccelta
USER nonroot:nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/tccelta"]
