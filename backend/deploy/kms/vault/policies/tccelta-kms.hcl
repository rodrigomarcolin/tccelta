# Politica do servico KMS (unico cliente do Vault em runtime; a API nao tem credencial do Vault).
# K_auth: gerar HMAC (Desafio 0x01 e ACK 0x04) e verificar a Resposta 0x02
# (o KMS devolve o HMAC completo e a API compara os 16 primeiros bytes em tempo constante).
path "transit-auth/hmac/*" {
  capabilities = ["update"]
}

# K_rec: SOMENTE verificar a tag de 32 bytes dos blocos 0x03.
path "transit-rec/verify/*" {
  capabilities = ["update"]
}

# JWT: assinar e ler a chave publica.
path "transit-jwt/sign/jwt" {
  capabilities = ["update"]
}
path "transit-jwt/keys/jwt" {
  capabilities = ["read"]
}

# Ciclo de vida do proprio token (a role usa token_no_default_policy=true).
path "auth/token/lookup-self" { capabilities = ["read"] }
path "auth/token/renew-self"  { capabilities = ["update"] }
path "auth/token/revoke-self" { capabilities = ["update"] }

# ---- Negacoes explicitas: documentam a intencao. A garantia real e a role ter UMA unica
# politica, esta politica nao conceder esses caminhos e os testes negativos no CI. ----
path "transit-rec/hmac/*"         { capabilities = ["deny"] }
path "transit-rec/keys/*"         { capabilities = ["deny"] }
path "transit-rec/export/*"       { capabilities = ["deny"] }
path "transit-rec/backup/*"       { capabilities = ["deny"] }
path "transit-rec/restore*"       { capabilities = ["deny"] }
path "transit-rec/byok-export/*"  { capabilities = ["deny"] }
path "transit-auth/keys/*"        { capabilities = ["deny"] }
path "transit-auth/export/*"      { capabilities = ["deny"] }
path "transit-auth/backup/*"      { capabilities = ["deny"] }
path "transit-auth/byok-export/*" { capabilities = ["deny"] }
path "transit-jwt/keys/jwt/*"     { capabilities = ["deny"] }
path "transit-jwt/export/*"       { capabilities = ["deny"] }
path "transit-jwt/backup/*"       { capabilities = ["deny"] }
path "sys/*"                      { capabilities = ["deny"] }
