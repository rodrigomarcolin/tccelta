# SO dev/CI: o teste dourado compara o HMAC do Vault com o local nas chaves de teste.
# O bootstrap recusa aplicar esta politica com PERFIL != dev.
path "transit-auth/hmac/*"   { capabilities = ["update"] }
path "transit-rec/hmac/*"    { capabilities = ["update"] }
path "transit-rec/verify/*"  { capabilities = ["update"] }
