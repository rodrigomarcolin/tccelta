#!/bin/sh
# CA privada + certificado do Vault (ECDSA P-256, 1 ano) em deploy/certs/vault, via conteiner alpine.
set -eu
DIR="$(cd "$(dirname "$0")/.." && pwd)/deploy/certs/vault"
mkdir -p "$DIR"
if [ -f "$DIR/vault.pem" ]; then echo "certs ja existem"; exit 0; fi
MSYS_NO_PATHCONV=1 docker run --rm -e KMS_HOST="${KMS_HOST:-}" -v "$DIR":/out alpine:3.20 sh -c '
  apk add -q --no-cache openssl >/dev/null && cd /out &&
  SAN="DNS:vault,DNS:localhost,IP:127.0.0.1"
  if [ -n "$KMS_HOST" ]; then SAN="$SAN,DNS:$KMS_HOST"; fi
  openssl ecparam -name prime256v1 -genkey -noout -out ca-key.pem &&
  openssl req -x509 -new -key ca-key.pem -days 365 -subj "/CN=tccelta-ca" -out ca.pem &&
  openssl ecparam -name prime256v1 -genkey -noout -out vault-key.pem &&
  openssl req -new -key vault-key.pem -subj "/CN=vault" -out vault.csr &&
  printf "subjectAltName=%s\n" "$SAN" > ext.cnf &&
  openssl x509 -req -in vault.csr -CA ca.pem -CAkey ca-key.pem -CAcreateserial -days 365 -extfile ext.cnf -out vault.pem &&
  chmod 644 *.pem && rm -f vault.csr ext.cnf'
echo "certs em $DIR"
