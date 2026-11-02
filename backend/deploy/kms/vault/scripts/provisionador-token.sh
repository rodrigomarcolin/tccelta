#!/bin/sh
# Reemite o token do provisionador (24 h) em producao, quando o anterior expirou ou foi revogado.
# Roda no conteiner vault-bootstrap (que monta o volume do provisionador), via:
#   sh deploy/vault-prod.sh provisionador
#
# Gera um token raiz temporario com 3 das 5 chaves de unseal (digitadas sem eco), emite o token de
# provisionamento como ORFAO (sobrevive a revogacao de quem o criou), grava no volume e revoga o raiz temporario.
# As chaves e o token raiz so existem na memoria deste processo.
set -eu
umask 077
: "${VAULT_ADDR:?}"

sai() {
  # se algo falhar no meio, nao deixa a geracao do token raiz pendurada
  [ "${OK:-0}" = 1 ] || vault operator generate-root -cancel >/dev/null 2>&1 || true
}
OK=0
trap sai EXIT

OTP="$(vault operator generate-root -generate-otp)"
NONCE="$(vault operator generate-root -init -otp="$OTP" -format=json | sed -n 's/.*"nonce": *"\([^"]*\)".*/\1/p')"
[ -n "$NONCE" ] || { echo "nao consegui iniciar a geracao do token raiz (ja existe uma em andamento? vault operator generate-root -cancel)" >&2; exit 1; }

ENC=""
n=0
while [ -z "$ENC" ] && [ "$n" -lt 8 ]; do
  printf "Chave de unseal (tentativa %s): " "$((n + 1))"
  read -rs K
  echo
  if R="$(printf '%s' "$K" | vault operator generate-root -nonce="$NONCE" -format=json - 2>/dev/null)"; then
    ENC="$(printf '%s' "$R" | sed -n 's/.*"encoded_token": *"\([^"]*\)".*/\1/p')"
  else
    echo "chave recusada; tente de novo" >&2
  fi
  n=$((n + 1))
done
K=""
[ -n "$ENC" ] || { echo "token raiz nao foi gerado: chaves insuficientes" >&2; exit 1; }

RAIZ="$(vault operator generate-root -decode="$ENC" -otp="$OTP")"
OK=1

DEST=/approle/ferramentas/provisionador_token
rm -f "$DEST"
if ! VAULT_TOKEN="$RAIZ" vault token create -policy=tccelta-provisionador -no-default-policy -orphan -ttl=24h -field=token > "$DEST"; then
  rm -f "$DEST"
  VAULT_TOKEN="$RAIZ" vault token revoke -self >/dev/null 2>&1 || true
  echo "falha ao criar o token do provisionador" >&2
  exit 1
fi
chown 65532:65532 "$DEST"
chmod 0400 "$DEST"
VAULT_TOKEN="$RAIZ" vault token revoke -self >/dev/null
echo "Token do provisionador reemitido (valido por 24 h). Token raiz temporario revogado."
