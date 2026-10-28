path "transit-auth/wrapping_key"     { capabilities = ["read"] }
path "transit-rec/wrapping_key"      { capabilities = ["read"] }
path "transit-auth/keys/+/import" {
  capabilities        = ["update"]
  denied_parameters   = { "exportable" = [true, "true"], "allow_plaintext_backup" = [true, "true"],
                          "allow_rotation" = [true, "true"], "derived" = [true, "true"] }
  required_parameters = ["ciphertext", "type"]
  allowed_parameters  = { "type" = ["hmac"], "*" = [] }
}
path "transit-rec/keys/+/import" {
  capabilities        = ["update"]
  denied_parameters   = { "exportable" = [true, "true"], "allow_plaintext_backup" = [true, "true"],
                          "allow_rotation" = [true, "true"], "derived" = [true, "true"] }
  required_parameters = ["ciphertext", "type"]
  allowed_parameters  = { "type" = ["hmac"], "*" = [] }
}
path "transit-auth/keys/+"                { capabilities = ["read"] }
path "transit-rec/keys/+"                 { capabilities = ["read"] }
path "transit-auth/keys/+/import_version" { capabilities = ["deny"] }
path "transit-rec/keys/+/import_version"  { capabilities = ["deny"] }
path "transit-rec/hmac/*"                 { capabilities = ["deny"] }
path "auth/token/lookup-self"             { capabilities = ["read"] }
