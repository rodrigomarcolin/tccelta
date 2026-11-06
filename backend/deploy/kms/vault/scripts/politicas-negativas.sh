#!/bin/sh
# Testes de politica no Vault REAL (CI, `make e2e`): confere o que cada papel pode e NAO pode fazer com o KV de clientes
# (kv-clientes) e com a chave cli do Transit. Usa a API crua do Vault (read/write/delete/list por caminho), porque o
# `vault kv` consulta caminhos internos que o papel kms nao tem e falharia por outro motivo.
# Roda dentro da imagem hashicorp/vault (sh), com os volumes dos AppRoles montados em leitura.
set -u
: "${VAULT_ADDR:?}"
falhas=0

login() { VAULT_TOKEN= vault write -field=token auth/approle/login role_id="$(cat "$1/$2_role_id")" secret_id="$(cat "$1/$2_secret_id")"; }
KMS_TOKEN="$(login /approle/kms kms)" || { echo "login do papel kms falhou"; exit 1; }
ROT_TOKEN="$(login /approle/rotacao rotacao)" || { echo "login do papel rotacao falhou"; exit 1; }
PROV_TOKEN="$(cat /approle/ferramentas/provisionador_token)"

# permitir <descricao> <token> <comando...>: o comando precisa dar certo.
permitir() {
  desc="$1"; tok="$2"; shift 2
  if saida="$(VAULT_TOKEN="$tok" "$@" 2>&1)"; then echo "OK      (permitido) $desc"; else echo "FALHOU  (deveria ser permitido) $desc: $saida"; falhas=$((falhas + 1)); fi
}
# negar <descricao> <token> <comando...>: o comando precisa falhar COM "permission denied" (e nao por outro motivo).
negar() {
  desc="$1"; tok="$2"; shift 2
  if saida="$(VAULT_TOKEN="$tok" "$@" 2>&1)"; then
    echo "FALHOU  (deveria ser negado) $desc"; falhas=$((falhas + 1))
  elif echo "$saida" | grep -q "permission denied"; then
    echo "OK      (negado)    $desc"
  else
    echo "FALHOU  (falhou por outro motivo) $desc: $saida"; falhas=$((falhas + 1))
  fi
}

CORPO_NOVO='{"data":{"algo":"sha256-v1","status":"active","created_at":"2026-01-01T00:00:00Z"},"options":{"cas":0}}'
CORPO_ALTERA='{"data":{"algo":"sha256-v1","status":"revoked"},"options":{"cas":1}}'
echo "$CORPO_NOVO" > /tmp/novo.json
echo "$CORPO_ALTERA" > /tmp/altera.json
REG=kv-clientes/data/clients/e2e-politica

echo "--- provisionador: cria e le o registro, nao apaga"
permitir "provisionador cria o registro (cas=0)" "$PROV_TOKEN" vault write "$REG" @/tmp/novo.json
permitir "provisionador le o registro" "$PROV_TOKEN" vault read "$REG"
permitir "provisionador atualiza o registro (cas)" "$PROV_TOKEN" vault write "$REG" @/tmp/altera.json
negar "provisionador nao apaga o registro" "$PROV_TOKEN" vault delete "$REG"
negar "provisionador nao apaga o historico" "$PROV_TOKEN" vault delete kv-clientes/metadata/clients/e2e-politica
negar "provisionador nao assina com a cli" "$PROV_TOKEN" vault write transit-jwt/sign/cli input=YWJj

echo "--- kms: le o registro e assina com a cli; nao escreve, nao le metadata, nao rotaciona"
permitir "kms le o registro do cliente" "$KMS_TOKEN" vault read "$REG"
permitir "kms assina com a chave cli" "$KMS_TOKEN" vault write transit-jwt/sign/cli input=YWJj
permitir "kms le as chaves publicas da cli" "$KMS_TOKEN" vault read transit-jwt/keys/cli
negar "kms nao escreve no KV (atualizar)" "$KMS_TOKEN" vault write "$REG" @/tmp/altera.json
negar "kms nao escreve no KV (criar outro)" "$KMS_TOKEN" vault write kv-clientes/data/clients/e2e-outro @/tmp/novo.json
negar "kms nao le a metadata do KV" "$KMS_TOKEN" vault read kv-clientes/metadata/clients/e2e-politica
negar "kms nao lista os clientes" "$KMS_TOKEN" vault list kv-clientes/metadata/clients
negar "kms nao apaga o registro" "$KMS_TOKEN" vault delete "$REG"
negar "kms nao rotaciona a cli" "$KMS_TOKEN" vault write -f transit-jwt/keys/cli/rotate
negar "kms nao altera min_decryption_version da cli" "$KMS_TOKEN" vault write transit-jwt/keys/cli/config min_decryption_version=1
negar "kms nao exporta a chave cli" "$KMS_TOKEN" vault read transit-jwt/export/signing-key/cli

echo "--- rotacao: gira a cli, nao le o KV nem assina"
permitir "rotacao le a chave cli" "$ROT_TOKEN" vault read transit-jwt/keys/cli
negar "rotacao nao le o registro de cliente" "$ROT_TOKEN" vault read "$REG"
negar "rotacao nao assina com a cli" "$ROT_TOKEN" vault write transit-jwt/sign/cli input=YWJj

if [ "$falhas" -gt 0 ]; then echo "$falhas verificacao(oes) de politica falharam"; exit 1; fi
echo "politicas do Vault conferidas"
