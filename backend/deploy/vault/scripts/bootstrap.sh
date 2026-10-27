#!/bin/sh
# Bootstrap idempotente do Vault (executa na imagem hashicorp/vault, /bin/sh).
set -eu
: "${VAULT_ADDR:?}" "${PERFIL:?}"
if [ "$PERFIL" = dev ]; then
  VAULT_TOKEN="$(cat /run/secrets/vault_dev_root_token)"
else
  VAULT_TOKEN="$(sed -n 's/.*"root_token": *"\([^"]*\)".*/\1/p' /vault/init/init.json)"
fi
export VAULT_TOKEN

montar() { vault secrets list | grep -q "^$1/" || vault secrets enable -path="$1" transit; }
montar transit-auth; montar transit-rec; montar transit-jwt

vault read transit-jwt/keys/jwt >/dev/null 2>&1 || \
  vault write -f transit-jwt/keys/jwt type=ed25519 exportable=false allow_plaintext_backup=false

vault policy write tccelta-api           /vault/politicas/tccelta-api.hcl
vault policy write tccelta-provisionador /vault/politicas/tccelta-provisionador.hcl
vault policy write tccelta-rotacao-jwt   /vault/politicas/tccelta-rotacao-jwt.hcl
if [ "$PERFIL" = dev ]; then vault policy write tccelta-teste-dourado /vault/politicas/tccelta-teste-dourado.hcl; fi

vault auth list | grep -q '^approle/' || vault auth enable approle
vault write auth/approle/role/tccelta-api token_policies=tccelta-api token_no_default_policy=true \
  token_type=service token_ttl=1h token_max_ttl=24h secret_id_ttl=0 secret_id_num_uses=0
vault read -field=role_id auth/approle/role/tccelta-api/role-id > /approle/api/api_role_id
[ -s /approle/api/api_secret_id ] || \
  vault write -f -field=secret_id auth/approle/role/tccelta-api/secret-id > /approle/api/api_secret_id

# AppRole do servico de rotacao do JWT: volume proprio, so no conteiner jwt-rotacao.
vault write auth/approle/role/tccelta-rotacao-jwt token_policies=tccelta-rotacao-jwt token_no_default_policy=true \
  token_type=service token_ttl=1h token_max_ttl=24h secret_id_ttl=0 secret_id_num_uses=0
vault read -field=role_id auth/approle/role/tccelta-rotacao-jwt/role-id > /approle/rotacao/rotacao_role_id
[ -s /approle/rotacao/rotacao_secret_id ] || \
  vault write -f -field=secret_id auth/approle/role/tccelta-rotacao-jwt/secret-id > /approle/rotacao/rotacao_secret_id

# Token do provisionador: volume SEPARADO, montado so em `ferramentas` (nunca na API).
POL="-policy=tccelta-provisionador"
if [ "$PERFIL" = dev ]; then POL="$POL -policy=tccelta-teste-dourado"; fi
# os arquivos sao 0400: remove o anterior antes de gravar o novo
rm -f /approle/ferramentas/provisionador_token
vault token create $POL -no-default-policy -ttl=24h -field=token > /approle/ferramentas/provisionador_token

if [ "$PERFIL" != dev ]; then
  vault audit list | grep -q '^file/' || vault audit enable file file_path=/vault/logs/auditoria.log
fi

chown 65532:65532 /approle/api/* /approle/ferramentas/* /approle/rotacao/*
chmod 0400 /approle/api/* /approle/ferramentas/* /approle/rotacao/*
echo "bootstrap do Vault concluido (perfil=$PERFIL)"
