ui            = false
disable_mlock = true
api_addr      = "https://vault:8200"
cluster_addr  = "https://vault:8201"

storage "raft" {
  path    = "/vault/data"
  node_id = "kms1"
}

listener "tcp" {
  address         = "0.0.0.0:8200"
  tls_cert_file   = "/vault/certs/vault.pem"
  tls_key_file    = "/vault/certs/vault-key.pem"
  tls_min_version = "tls13"
}
