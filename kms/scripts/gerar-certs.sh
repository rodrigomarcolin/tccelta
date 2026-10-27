#!/bin/sh
# PKI de laboratorio do KMS (ECDSA P-256, 1 ano) em deploy/certs, via conteiner alpine:
#   ca.pem / ca-key.pem                  CA privada (assina todos os certificados abaixo)
#   vault.pem / vault-key.pem            servidor Vault (TLS)
#   kms.pem / kms-key.pem                servidor KMS (TLS; a API confere o nome)
#   api-cliente.pem / api-cliente-key.pem  cliente da API (mTLS: o KMS so aceita quem tem este certificado)
# Variaveis: KMS_HOST (SAN extra do Vault e do KMS, ex.: IP da VPN). Idempotente.
set -eu
DIR="$(cd "$(dirname "$0")/.." && pwd)/deploy/certs"
mkdir -p "$DIR"
if [ -f "$DIR/kms.pem" ] && [ -f "$DIR/api-cliente.pem" ]; then echo "certs ja existem"; exit 0; fi
MSYS_NO_PATHCONV=1 docker run --rm -e KMS_HOST="${KMS_HOST:-}" -v "$DIR":/out alpine:3.20 sh -c '
  apk add -q --no-cache openssl >/dev/null && cd /out &&
  EXTRA=""
  if [ -n "$KMS_HOST" ]; then EXTRA=",DNS:$KMS_HOST"; fi
  if [ ! -f ca.pem ]; then
    openssl ecparam -name prime256v1 -genkey -noout -out ca-key.pem &&
    openssl req -x509 -new -key ca-key.pem -days 365 -subj "/CN=tccelta-ca" -out ca.pem
  fi
  emitir() { # nome CN SAN EKU
    openssl ecparam -name prime256v1 -genkey -noout -out "$1-key.pem" &&
    openssl req -new -key "$1-key.pem" -subj "/CN=$2" -out "$1.csr" &&
    printf "subjectAltName=%s\nextendedKeyUsage=%s\n" "$3" "$4" > "$1.cnf" &&
    openssl x509 -req -in "$1.csr" -CA ca.pem -CAkey ca-key.pem -CAcreateserial -days 365 -extfile "$1.cnf" -out "$1.pem" &&
    rm -f "$1.csr" "$1.cnf"
  }
  [ -f vault.pem ] || emitir vault vault "DNS:vault,DNS:localhost,IP:127.0.0.1$EXTRA" serverAuth
  [ -f kms.pem ] || emitir kms kms "DNS:kms,DNS:localhost,IP:127.0.0.1$EXTRA" serverAuth
  [ -f api-cliente.pem ] || emitir api-cliente tccelta-api "DNS:api" clientAuth
  chmod 644 *.pem && rm -f ca.srl'
echo "certs em $DIR"
