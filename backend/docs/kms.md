# tccelta-kms

Serviço KMS do TCC Elta: único cliente do Vault (Transit) em runtime. É um binário próprio (`cmd/tccelta-kms`) no mesmo módulo Go do `backend/`, com pacotes isolados por regra do depguard
(`internal/kms`) que pode ser implantado em outro host; a API (`backend/`) fala com ele só por HTTP
([`kms-http-contract.md`](kms-http-contract.md)). Nenhum pacote da API importa o KMS e vice-versa.

```
app/firmware ──HTTPS──▶ API (backend/) ──HTTP(S) + mTLS──▶ tccelta-kms ──▶ Vault (Transit)
                          │                                   │
                          └── PostgreSQL                      └── K_auth, K_rec, chave Ed25519 do JWT
```

A API não tem nenhuma credencial do Vault. O AppRole que ela usava (`tccelta-api`) agora é do KMS
(`tccelta-kms`), com as mesmas políticas.

## Comandos

```
tccelta-kms serve                                    # serviço HTTP (:8300) + métricas/saúde (:9300)
tccelta-kms jwt rotate [--now]                    # rotaciona/expurga a chave do JWT uma vez
tccelta-kms jwt schedule                            # idem, em laço (serviço jwt-rotacao)
tccelta-kms provision new --count N [--csv-out FILE]
tccelta-kms provision import --file dongles.csv
tccelta-kms provision import-record --file kms_record.json
tccelta-kms provision export-wrapping --out kms_wrapping.json
tccelta-kms provision verify --file dongles.csv
tccelta-kms health --url http://127.0.0.1:9300/saude/pronta
```

`provision` grava `<out>/<devid>/record.json` ([`kms-dongle-record.md`](kms-dongle-record.md));
o cadastro no banco é `tccelta provision register --file …` no backend.

## Configuração (variáveis de ambiente)

| Variável | Padrão | Uso |
|---|---|---|
| `AMBIENTE` | `dev` | `dev`, `prodlike` ou `producao`; fora de `dev`, mTLS é obrigatório |
| `KMS_ENDERECO` | `:8300` | endereço do serviço |
| `KMS_METRICAS_ENDERECO` | `:9300` | `/metricas` e `/saude/*` em HTTP simples |
| `KMS_TLS_CERT`, `KMS_TLS_KEY` | – | certificado e chave do servidor |
| `KMS_TLS_CLIENT_CA` | – | CA que assina o certificado de cliente da API |
| `KMS_TLS_CLIENTE_CN` | `tccelta-api` | CN exigido no certificado de cliente (outro CN da mesma CA recebe 403) |
| `VAULT_ADDR`, `VAULT_CACERT`, `VAULT_CLIENT_CERT`, `VAULT_CLIENT_KEY` | – | conexão com o Vault |
| `VAULT_APPROLE_ROLE_ID_ARQUIVO`, `VAULT_APPROLE_SECRET_ID_ARQUIVO` | – | login AppRole (`tccelta-kms`) |
| `VAULT_TOKEN_ARQUIVO` | – | token estático (provisionamento) |
| `VAULT_TIMEOUT`, `VAULT_RETENTATIVAS`, `VAULT_MAX_SIMULTANEAS` | `2s`, `1`, `32` | chamadas ao Vault |
| `VAULT_MOUNT_AUTH`, `VAULT_MOUNT_REC`, `VAULT_MOUNT_JWT`, `VAULT_CHAVE_JWT` | `transit-auth`, `transit-rec`, `transit-jwt`, `jwt` | montagens do Transit |
| `JWT_VALIDADE`, `JWT_FOLGA` | `1h`, `30s` | só para a rotação decidir quando expurgar uma versão |
| `JWT_ROTACAO_PERIODO`, `JWT_ROTACAO_VERIFICACAO` | `720h`, `1h` | ciclo de vida da chave do JWT |

## Desenvolvimento

```
make teste     # go test -race ./... (em contêiner; todo o módulo)
make lint      # golangci-lint
make certs     # PKI de laboratório em deploy/kms/certs (CA, Vault, KMS, cliente da API)
make docker-kms # imagem tccelta-kms:local
```

Para subir tudo junto (Postgres, Vault, KMS, API, worker) use o `backend/compose.yaml`; para o KMS num host
separado, `deploy/kms/compose.yaml`.

## Layout

```
cmd/tccelta-kms/        serve | jwt | provision | health
internal/kms/config/        variáveis de ambiente
internal/kms/vault/         cliente do Vault, operações do Transit, rotação do JWT (+ vaulttest: Vault falso)
internal/kms/breaker/       disjuntor das chamadas ao Vault
internal/kms/server/        um arquivo por endpoint, mTLS, problem+json
internal/kms/provisioning/  importação BYOK, teste dourado, material NVS, record.json
internal/kms/metrics/        séries tccelta_kmssvc_*
deploy/kms/             Vault (config, políticas, scripts), compose do host separado, certs
docs/kms-*.md        contrato HTTP e registro do dongle
```
