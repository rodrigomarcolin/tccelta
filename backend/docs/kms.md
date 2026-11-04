# tccelta-kms

Serviço KMS do TCC Elta: único cliente do Vault (Transit) em runtime. É um binário próprio (`cmd/tccelta-kms`) no mesmo módulo Go do `backend/`, com pacotes isolados por regra do depguard
(`internal/kms`) que pode ser implantado em outro host; a API (`backend/`) fala com ele só por HTTP
([`kms-http-contract.md`](kms-http-contract.md)). Nenhum pacote da API importa o KMS e o KMS só importa o `internal/platform/breaker` (sem dependências).

```
app/firmware ──HTTPS──▶ API (backend/) ──HTTP(S) + mTLS──▶ tccelta-kms ──▶ Vault (Transit)
                          │                                   │
                          └── PostgreSQL                      └── K_auth, K_rec, chave Ed25519 do JWT, chave `cli` e hash dos clientes
```

A API não tem nenhuma credencial do Vault. O AppRole que ela usava (`tccelta-api`) agora é do KMS
(`tccelta-kms`), com as mesmas políticas.

## Comandos

```
tccelta-kms serve                                    # serviço HTTP (:8300) + métricas/saúde (:9300)
tccelta-kms jwt rotate [--now]                    # rotaciona/expurga a chave do JWT uma vez
tccelta-kms jwt schedule                            # idem, em laço (serviço jwt-rotacao)
tccelta-kms clientjwt rotate [--now]              # idem, para a chave cli (JWT dos clientes)
tccelta-kms clientjwt schedule                      # idem, em laço (serviço jwt-cli-rotacao)
tccelta-kms client create --id ID [--out FILE]      # cria o cliente e mostra o segredo UMA vez (ou grava em FILE, modo 0400)
tccelta-kms client rotate --id ID [--out FILE]      # novo segredo e novo sal; o antigo deixa de valer na hora
tccelta-kms client revoke --id ID                   # marca revogado e apaga hash e sal (idempotente)
tccelta-kms provision new --count N [--profile hmac|asym|none] [--csv-out FILE]
tccelta-kms provision import --file dongles.csv
tccelta-kms provision import-record --file kms_record.json
tccelta-kms provision export-wrapping --out kms_wrapping.json
tccelta-kms provision verify --file dongles.csv
tccelta-kms health --url http://127.0.0.1:9300/saude/pronta
```

### Clientes de client credentials

O servidor do Next.js obtém o token com `POST /v1/auth/token` (API). O KMS guarda só o hash do segredo no KV v2 do Vault e o
verifica (`POST /v1/clients/verify`); a chave Ed25519 `cli` assina os tokens (`POST /v1/client-jwt/sign`). Os comandos `client`
usam o token do provisionador (`VAULT_TOKEN_ARQUIVO`), como `provision`. O segredo (32 bytes aleatórios, base64url sem padding,
43 caracteres) nunca vai a log, métrica nem mensagem de erro, e `create` repetido falha (check-and-set `cas=0`).

| Ambiente | Como provisionar |
|---|---|
| dev | `make seed-client`: sobe o bootstrap, cria `next-dev` e grava o segredo em `deploy/segredos/analise_cliente_secret` (fora do git). Idempotente: se o arquivo existe, não recria |
| prodlike | `docker compose -f compose.yaml -f compose.prodlike.yaml --profile tls --profile ferramentas run --rm -T kms-ferramentas client create --id next-prodlike > arquivo` (executado: o cliente obtém token em `POST /v1/auth/token` e o JWKS responde). O `compose.prodlike.yaml` ajusta `VAULT_ADDR` (HTTPS) e `VAULT_CACERT` do `kms-ferramentas`; só o comando `client` foi exercitado nesse ambiente |
| produção | `client create --id ID --out ARQUIVO` no host do KMS, com o token do provisionador; o operador entrega o segredo ao Next.js como variável de ambiente de runtime (`ANALISE_CLIENT_SECRET`) |

Troca do segredo: `client rotate` e reinício do Next.js com o valor novo (tokens já emitidos valem até `exp`). Revogação:
`client revoke` (nenhum token novo; os emitidos valem até `exp`). Desbloqueio manual do bloqueio progressivo da API:
`redis-cli --scan --pattern 'tccelta:tok:*' | xargs redis-cli DEL`.

`provision` grava `<out>/<devid>/record.json` ([`kms-dongle-record.md`](kms-dongle-record.md));
o cadastro no banco é `tccelta provision register --file …` no backend.

### CSV de chaves de teste

`provision import --file`, `provision verify --file`, `--csv-out` e o `--keys` do simulador usam o mesmo CSV:

```
dev_id_hex,k_auth_hex,k_rec_hex,perfil[,k_sig_hex]
```

A 5ª coluna é opcional. No perfil `asym` ela é **obrigatória** e traz o escalar P-256 de 32 B (big-endian);
a `public_key` do `record.json` é derivada dela. Nos outros perfis ela é proibida (vazia é aceita). Arquivos de
4 colunas continuam válidos. `provision new --profile asym` gera o par, grava `k_sig` na NVS
(`secnvs.bin`), inclui `k_sig` e `k_sig_public` no `bench_keys.json` e a `k_sig` na 5ª coluna do `--csv-out`.
`K_auth` e `K_rec` são importadas no Transit em todos os perfis; no `asym` a `K_rec` não é usada nos blocos.

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
| `VAULT_MOUNT_CLIENTES`, `VAULT_CHAVE_CLI` | `kv-clientes`, `cli` | KV v2 com o hash dos segredos dos clientes e nome da chave Ed25519 dos tokens de cliente (no mount do JWT) |
| `JWT_VALIDADE`, `JWT_FOLGA` | `1h`, `30s` | só para a rotação decidir quando expurgar uma versão |
| `JWT_ROTACAO_PERIODO`, `JWT_ROTACAO_VERIFICACAO` | `720h`, `1h` | ciclo de vida da chave do JWT |
| `CLI_JWT_VALIDADE` | `30m` | validade do token de cliente; no KMS só decide quando expurgar uma versão da chave `cli` (`CLI_JWT_VALIDADE + JWT_FOLGA + 10 min`) |
| `CLI_JWT_ROTACAO_PERIODO` | `720h` | rotação da chave `cli` (`clientjwt`); a verificação usa `JWT_ROTACAO_VERIFICACAO` |
| `KMS_SAUDE_ROTACAO` | `:8092` | saúde dos serviços de rotação (`jwt-rotacao`; o `jwt-cli-rotacao` usa `:8093`) |

## Desenvolvimento

```
make test      # go test -race ./... (em contêiner; todo o módulo)
make lint      # golangci-lint
make certs     # PKI de laboratório em deploy/kms/certs (CA, Vault, KMS, cliente da API)
make docker-kms # imagem tccelta-kms:local
```

Para subir tudo junto (Postgres, Vault, KMS, API, worker) use o `backend/compose.yaml`; para o KMS num host
separado, `deploy/kms/compose.yaml`.

## Layout

```
cmd/tccelta-kms/        serve | jwt | clientjwt | client | provision | health
internal/kms/config/    variáveis de ambiente
internal/kms/vault/     cliente do Vault, operações do Transit, rotação do JWT, registro de clientes no KV (+ vaulttest: Vault falso)
internal/kms/server/    um arquivo por endpoint, mTLS, problem+json
internal/kms/provisioning/  importação BYOK, teste dourado, material NVS, record.json, ciclo de vida dos clientes (create, rotate, revoke)
internal/kms/metrics/   séries tccelta_kmssvc_*
internal/platform/breaker/  disjuntor das chamadas ao Vault (compartilhado com o cliente do KMS da API)
deploy/kms/             Vault (config, policies, scripts), compose do host separado, certs
docs/kms-*.md           contrato HTTP e registro do dongle
```
