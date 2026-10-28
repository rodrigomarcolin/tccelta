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

# Mantem o secret_id gravado no volume so se o login com ele funcionar (um Vault dev recriado perde os IDs
# antigos, e o volume sobrevive). Caso contrario, gera outro. Os arquivos sao 0400: remove antes de gravar.
garantir_secret_id() {
  role="$1"; rid="$2"; sid="$3"
  if [ -s "$sid" ] && VAULT_TOKEN= vault write -field=token auth/approle/login role_id="$(cat "$rid")" secret_id="$(cat "$sid")" >/dev/null 2>&1; then
    return 0
  fi
  rm -f "$sid"
  vault write -f -field=secret_id "auth/approle/role/$role/secret-id" > "$sid"
}

montar() { vault secrets list | grep -q "^$1/" || vault secrets enable -path="$1" transit; }
montar transit-auth; montar transit-rec; montar transit-jwt

vault read transit-jwt/keys/jwt >/dev/null 2>&1 || \
  vault write -f transit-jwt/keys/jwt type=ed25519 exportable=false allow_plaintext_backup=false

vault policy write tccelta-kms           /vault/policies/tccelta-kms.hcl
vault policy write tccelta-provisionador /vault/policies/tccelta-provisioner.hcl
vault policy write tccelta-rotacao-jwt   /vault/policies/tccelta-jwt-rotation.hcl
if [ "$PERFIL" = dev ]; then vault policy write tccelta-teste-dourado /vault/policies/tccelta-golden-test.hcl; fi

vault auth list | grep -q '^approle/' || vault auth enable approle
vault write auth/approle/role/tccelta-kms token_policies=tccelta-kms token_no_default_policy=true \
  token_type=service token_ttl=1h token_max_ttl=24h secret_id_ttl=0 secret_id_num_uses=0
vault read -field=role_id auth/approle/role/tccelta-kms/role-id > /approle/kms/kms_role_id
garantir_secret_id tccelta-kms /approle/kms/kms_role_id /approle/kms/kms_secret_id

# AppRole do servico de rotacao do JWT: volume proprio, so no conteiner jwt-rotacao.
vault write auth/approle/role/tccelta-rotacao-jwt token_policies=tccelta-rotacao-jwt token_no_default_policy=true \
  token_type=service token_ttl=1h token_max_ttl=24h secret_id_ttl=0 secret_id_num_uses=0
vault read -field=role_id auth/approle/role/tccelta-rotacao-jwt/role-id > /approle/rotacao/rotacao_role_id
garantir_secret_id tccelta-rotacao-jwt /approle/rotacao/rotacao_role_id /approle/rotacao/rotacao_secret_id

# Token do provisionador: volume SEPARADO, montado so em `ferramentas` (nunca na API).
POL="-policy=tccelta-provisionador"
if [ "$PERFIL" = dev ]; then POL="$POL -policy=tccelta-teste-dourado"; fi
# os arquivos sao 0400: remove o anterior antes de gravar o novo
rm -f /approle/ferramentas/provisionador_token
vault token create $POL -no-default-policy -ttl=24h -field=token > /approle/ferramentas/provisionador_token

if [ "$PERFIL" != dev ]; then
  vault audit list | grep -q '^file/' || vault audit enable file file_path=/vault/logs/auditoria.log
fi

chown 65532:65532 /approle/kms/* /approle/ferramentas/* /approle/rotacao/*
chmod 0400 /approle/kms/* /approle/ferramentas/* /approle/rotacao/*
echo "bootstrap do Vault concluido (perfil=$PERFIL)"
