#!/bin/sh
# A PKI (CA, Vault, KMS e cliente da API) e do modulo kms: delega para ../kms/scripts/gerar-certs.sh.
set -eu
exec sh "$(cd "$(dirname "$0")/.." && pwd)/../kms/scripts/gerar-certs.sh" "$@"
