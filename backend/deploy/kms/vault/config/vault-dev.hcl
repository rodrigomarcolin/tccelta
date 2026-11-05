# Vault do dev (make up): modo servidor com raft em volume (sobrevive a reinicios), HTTP, um no.
# Mesmo esquema do prodlike (vault_dados + vault_init, node_id kms1), so sem TLS: os dois modos compartilham os volumes.
ui            = false
disable_mlock = true
api_addr      = "http://vault:8200"
cluster_addr  = "http://vault:8201"

storage "raft" {
  path    = "/vault/data"
  node_id = "kms1"
}

listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = true
}
