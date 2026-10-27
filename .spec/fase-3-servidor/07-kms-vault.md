# 07 — KMS com HashiCorp Vault (Transit)

> Fase 3 · Servidor. Depende de: 01 (contêiner `vault`, `vault-bootstrap`, redes), 02 (tabela `dongle`). É dependência de: 04, 05 (HMAC e verificação), 08 (assinatura do JWT), 10 (chaves de teste), e da Fase 1 (material de provisionamento do firmware).

## Objetivo

Implementar o `S_KMS` do doc (§4.1 e `kms-servidor-app.puml`) com o Vault **Transit**:

- `K_auth` e `K_rec` por dongle, sorteadas na fabricação e importadas no Vault cifradas com a chave pública do KMS (*wrapping key*);
- políticas em que a API pode **gerar** HMAC com `K_auth` e **apenas verificar** com `K_rec`;
- chave de assinatura do JWT dentro do Vault;
- AppRole para a API;
- scripts de bootstrap;
- ferramenta de provisionamento que gera DevId e as duas chaves e produz (a) o material do firmware e (b) a importação no Vault.

## Escopo

Dentro: engines, nomes de chaves, tipos, políticas HCL, AppRole, bootstrap, wrapping/import, cliente Go (`internal/kms`) com batching, limites, timeouts e *circuit breaker*, ferramenta `provisionar`, testes de política. Fora: emissão e validação do JWT em si (08); gravação na NVS do dongle (Fase 1).

## Design

### Engines, nomes e tipos de chave

**[Decisão] Três mounts Transit separados, com a chave nomeada pelo DevId:**

| Mount | Chave | Tipo | Origem | Operações permitidas à API |
|---|---|---|---|---|
| `transit-auth/` | `<dev_id_hex>` (ex.: `0011223344556677`) | `hmac` | importada (BYOK) | `hmac` |
| `transit-rec/` | `<dev_id_hex>` | `hmac` | importada (BYOK) | `verify` |
| `transit-jwt/` | `jwt` | `ed25519` | gerada no Vault, `exportable=false` | `sign` + leitura da chave pública |

Por que não `dongle/<DevId>/k_auth` (nome sugerido):

1. Nomes de chave do Transit aceitam só `[A-Za-z0-9_.-]`; não aceitam `/`.
2. **Separar por mount torna a política à prova de erro:** `path "transit-rec/hmac/*"` simplesmente não está na política da API, e fica com `deny` explícito. Não depende de *globs* sobre sufixos de nome (`*-k_rec`), que um nome mal formado poderia contornar.
3. Cada mount pode ter auditoria, *rate limit quota* e *lease* próprios.

O DevId hex já é único (`dongle.dev_id_hex UNIQUE`). `dongle.kms_chave_auth`/`kms_chave_rec` guardam o nome, para permitir uma migração futura de esquema de nomes sem mudar código.

**[Decisão] Tipo `hmac` (e não `aes256-gcm96`).** Numa chave `aes256-gcm96`, o endpoint `hmac` usa uma **chave HMAC interna sorteada pelo Vault**, não o material importado. O resultado não bateria com o HMAC calculado pelo dongle. Com o tipo `hmac`, o material importado (os 32 bytes de `K_auth`/`K_rec`) é a própria chave do HMAC. **Isso é verificado no bootstrap** (teste dourado, abaixo), porque é a premissa crítica desta spec. Versão mínima: Vault ≥ 1.15 (tipo `hmac` com import BYOK). Fixar `VAULT_VERSAO` ≥ 1.15; os exemplos usam 1.18.

**Chave do JWT:** `ed25519` (EdDSA, RFC 8037). O Transit devolve a assinatura crua de 64 bytes, que vira diretamente a assinatura JWS. Com ECDSA seria preciso `marshaling_algorithm=jws`. A chave pública é lida pela API e cacheada para validar tokens localmente, sem chamar o Vault por requisição.

### Políticas

`deploy/vault/politicas/tccelta-api.hcl`:

```hcl
# K_auth: gerar HMAC (Desafio 0x01 e ACK 0x04) e verificar a Resposta 0x02
# (a API gera o HMAC completo e compara os 16 primeiros bytes em tempo constante).
path "transit-auth/hmac/*" {
  capabilities = ["update"]
}

# K_rec: SOMENTE verificar a tag de 32 bytes dos blocos 0x03.
path "transit-rec/verify/*" {
  capabilities = ["update"]
}

# JWT: assinar e ler a chave pública.
path "transit-jwt/sign/jwt" {
  capabilities = ["update"]
}
path "transit-jwt/keys/jwt" {
  capabilities = ["read"]
}

# Ciclo de vida do próprio token (a role usa token_no_default_policy=true).
path "auth/token/lookup-self" { capabilities = ["read"] }
path "auth/token/renew-self"  { capabilities = ["update"] }
path "auth/token/revoke-self" { capabilities = ["update"] }

# ---- Negações explícitas: documentam a intenção (ver nota sobre precedência abaixo) ----
# Nada que produza MAC, exporte, faça backup, reconfigure ou rotacione K_rec.
path "transit-rec/hmac/*"         { capabilities = ["deny"] }
path "transit-rec/keys/*"         { capabilities = ["deny"] }   # inclui /config (exportable), /rotate, /trim, /import*
path "transit-rec/export/*"       { capabilities = ["deny"] }
path "transit-rec/backup/*"       { capabilities = ["deny"] }
path "transit-rec/restore*"       { capabilities = ["deny"] }
path "transit-rec/byok-export/*"  { capabilities = ["deny"] }
# Mesmo para K_auth e para a chave do JWT: só as operações acima.
path "transit-auth/keys/*"        { capabilities = ["deny"] }
path "transit-auth/export/*"      { capabilities = ["deny"] }
path "transit-auth/backup/*"      { capabilities = ["deny"] }
path "transit-auth/byok-export/*" { capabilities = ["deny"] }
path "transit-jwt/keys/jwt/*"     { capabilities = ["deny"] }   # rotate, config, trim
path "transit-jwt/export/*"       { capabilities = ["deny"] }
path "transit-jwt/backup/*"       { capabilities = ["deny"] }
path "sys/*"                      { capabilities = ["deny"] }
```

Por que a política garante "só verify" para `K_rec`:

- O Vault nega tudo por padrão. Os únicos caminhos de `transit-rec/` liberados são `verify/*`, que devolve só `valid: true|false`, nunca o MAC.
- Os caminhos que **produziriam** um MAC (`hmac`), **revelariam** a chave (`export`, `backup`, `byok-export`) ou **mudariam** essa garantia (`keys/<n>/config` com `exportable=true`, `rotate`, `import_version`) têm `deny` explícito.
- **Precedência (correção da revisão).** No ACL do Vault, vence o caminho **mais específico** (exato > glob mais longo > glob mais curto). O `deny` só prevalece sobre outras capacidades **no mesmo caminho**. Uma política futura com `path "transit-rec/keys/<hex>/config"` (mais específico) sobreporia o `transit-rec/keys/*` negado. Portanto, os `deny` documentam a intenção, mas **a garantia real** é:
  1. a role da API ter **uma única** política (`token_policies=tccelta-api`, `token_no_default_policy=true`);
  2. essa política não conceder esses caminhos;
  3. os testes negativos (Testes, item 3) rodarem no CI e falharem se alguém acrescentar uma concessão.

  Toda mudança em `deploy/vault/politicas/` exige revisão de segurança no PR.
- As chaves são importadas com `exportable=false` e `allow_plaintext_backup=false`, que são irreversíveis no Vault.
- A role usa `token_no_default_policy=true`, então a política `default` (que dá acesso a cubbyhole, wrapping etc.) não entra.
- Verificar como oráculo não ajuda um atacante: a tag tem 32 bytes (sem truncamento), então forjar exigiria 2²⁵⁶ tentativas.

`deploy/vault/politicas/tccelta-provisionador.hcl`:

```hcl
path "transit-auth/wrapping_key"     { capabilities = ["read"] }
path "transit-rec/wrapping_key"      { capabilities = ["read"] }
path "transit-auth/keys/+/import"    {                               # cria; não há import_version
  capabilities      = ["update"]
  # O token do provisionador não pode criar chave exportável/rotacionável/derivada:
  # a ferramenta envia os valores seguros por padrão e qualquer tentativa de mudá-los é negada.
  denied_parameters = { "exportable" = [true, "true"], "allow_plaintext_backup" = [true, "true"],
                        "allow_rotation" = [true, "true"], "derived" = [true, "true"] }
  required_parameters = ["ciphertext", "type"]
  allowed_parameters  = { "type" = ["hmac"], "*" = [] }
}
path "transit-rec/keys/+/import"     {
  capabilities      = ["update"]
  denied_parameters = { "exportable" = [true, "true"], "allow_plaintext_backup" = [true, "true"],
                        "allow_rotation" = [true, "true"], "derived" = [true, "true"] }
  required_parameters = ["ciphertext", "type"]
  allowed_parameters  = { "type" = ["hmac"], "*" = [] }
}
path "transit-auth/keys/+"           { capabilities = ["read"] }     # metadados (existe?), nunca material
path "transit-rec/keys/+"            { capabilities = ["read"] }
path "transit-auth/keys/+/import_version" { capabilities = ["deny"] }
path "transit-rec/keys/+/import_version"  { capabilities = ["deny"] }
path "transit-rec/hmac/*"            { capabilities = ["deny"] }
# testes dourados só no perfil dev (ver abaixo): política extra tccelta-teste-dourado
```

`tccelta-teste-dourado.hcl` (só dev/CI): `transit-auth/hmac/*` e `transit-rec/hmac/*` com `update`, para o teste dourado comparar o HMAC do Vault com o local nas **chaves de teste**. O bootstrap recusa aplicar esta política com `PERFIL != dev`.

### AppRole da API

```sh
vault auth enable approle
vault write auth/approle/role/tccelta-api \
  token_policies=tccelta-api token_no_default_policy=true \
  token_type=service token_ttl=1h token_max_ttl=24h \
  secret_id_ttl=0 secret_id_num_uses=0
  # produção/VPN: secret_id_bound_cidrs=<IP da API na VPN>/32 token_bound_cidrs=<idem> secret_id_ttl=720h
```

- A API faz login no boot (`role_id`/`secret_id` lidos de arquivos), renova com `api.NewLifetimeWatcher` e, se a renovação falhar ou o `token_max_ttl` vencer, refaz o login.
- Enquanto não houver token válido, `/saude/pronta` = 503.
- Em produção, o `secret_id` é entregue por *response wrapping* (`vault write -wrap-ttl=5m -f auth/approle/role/tccelta-api/secret-id`). A API desembrulha no primeiro boot (`VAULT_APPROLE_SECRET_ID_EMBRULHADO=true`).

### Bootstrap (`deploy/vault/scripts/bootstrap.sh`, idempotente, `/bin/sh` da imagem do Vault)

```sh
#!/bin/sh
set -eu
: "${VAULT_ADDR:?}" "${PERFIL:?}"
if [ "$PERFIL" = dev ]; then
  VAULT_TOKEN="$(cat /run/secrets/vault_dev_root_token)"
else
  # laboratório prodlike: root token inicial do init (spec 01). Produção: token admin efêmero.
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
[ "$PERFIL" = dev ] && vault policy write tccelta-teste-dourado /vault/politicas/tccelta-teste-dourado.hcl

vault auth list | grep -q '^approle/' || vault auth enable approle
vault write auth/approle/role/tccelta-api token_policies=tccelta-api token_no_default_policy=true \
  token_type=service token_ttl=1h token_max_ttl=24h secret_id_ttl=0 secret_id_num_uses=0
vault read -field=role_id auth/approle/role/tccelta-api/role-id > /approle/api/api_role_id
[ -s /approle/api/api_secret_id ] || \
  vault write -f -field=secret_id auth/approle/role/tccelta-api/secret-id > /approle/api/api_secret_id

# AppRole do serviço de rotação do JWT: terceiro volume de credenciais, só no contêiner `jwt-rotacao`.
vault write auth/approle/role/tccelta-rotacao-jwt token_policies=tccelta-rotacao-jwt token_no_default_policy=true \
  token_type=service token_ttl=1h token_max_ttl=24h secret_id_ttl=0 secret_id_num_uses=0
vault read -field=role_id auth/approle/role/tccelta-rotacao-jwt/role-id > /approle/rotacao/rotacao_role_id
[ -s /approle/rotacao/rotacao_secret_id ] || \
  vault write -f -field=secret_id auth/approle/role/tccelta-rotacao-jwt/secret-id > /approle/rotacao/rotacao_secret_id

# Token do provisionador (curta duração; renovado a cada execução do bootstrap).
# Vai para um volume SEPARADO, montado só no contêiner "ferramentas" (nunca na API).
POL=tccelta-provisionador; [ "$PERFIL" = dev ] && POL="$POL,tccelta-teste-dourado"
vault token create -policy="$POL" -no-default-policy -ttl=24h -field=token > /approle/ferramentas/provisionador_token

[ "$PERFIL" != dev ] && { vault audit list | grep -q '^file/' || vault audit enable file file_path=/vault/logs/auditoria.log; }

# O job roda como root (user: "0:0" no compose) para poder entregar os arquivos ao uid da distroless.
chown 65532:65532 /approle/api/* /approle/ferramentas/* /approle/rotacao/*; chmod 0400 /approle/api/* /approle/ferramentas/* /approle/rotacao/*
echo "bootstrap do Vault concluído (perfil=$PERFIL)"
```

Em `prodlike`, depois do bootstrap: `make vault-revogar-root` (`vault token revoke <root>`) e apagar `root_token` de `init.json` (documentado como passo manual de laboratório).

### Wrapping e importação (a "chave pública fornecida por `S_KMS`")

O doc pede que as chaves trafeguem "criptografadas com uma chave pública fornecida por `S_KMS`". O BYOK do Transit faz exatamente isso:

1. `GET transit-auth/wrapping_key` → chave pública RSA-4096 (PEM) do Vault. A chave privada nunca sai do Vault. O mesmo vale para `transit-rec/wrapping_key` (cada mount tem a sua).
2. Na estação de fabricação: gerar uma chave efêmera AES-256 `K_e` (`crypto/rand`).
3. `c1 = RSA-OAEP(SHA-256, wrapping_pub, K_e)` (512 B).
4. `c2 = AES-KWP(K_e, K_material)` (RFC 5649; 40 B para 32 B de chave).
5. `ciphertext = base64(c1 || c2)`.
6. `POST transit-auth/keys/<dev_id_hex>/import` com `{"ciphertext": ..., "hash_function": "SHA256", "type": "hmac", "exportable": false, "allow_plaintext_backup": false, "allow_rotation": false}`.
7. Zerar `K_e` e o material em memória (melhor esforço em Go: sobrescrever os `[]byte` depois do uso).

A chave em claro existe só na memória da ferramenta por poucos milissegundos e no arquivo de material do firmware (ver abaixo). Isso é o "fica em claro apenas por pouco tempo na memória da estação de fabricação" do doc.

`internal/provisionamento/embrulho.go`:

```go
// Embrulhar implementa o formato BYOK do Vault Transit: RSA-OAEP-SHA256(K_e) || AES-KWP(K_e, chave).
func Embrulhar(wrappingPub *rsa.PublicKey, chave []byte) (ciphertextB64 string, err error)
// aesKWP: RFC 5649 (key wrap with padding), ~60 linhas com crypto/aes; testado com os vetores do RFC 5649 §6.
func aesKWP(kek, chave []byte) ([]byte, error)
```

**[Decisão] Implementar o embrulho em Go**, em vez de usar `vault transit import` do CLI. A ferramenta precisa, na mesma operação atômica, gerar as chaves, importá-las nos dois mounts, gravar `dongle`/`dongle_estado` e emitir o material do firmware. Com o CLI, a chave em claro passaria por argumento ou arquivo intermediário. O teste dourado de integração compara o resultado com o comportamento real do Vault.

### Ferramenta de provisionamento (`tccelta provisionar`)

```
tccelta provisionar exportar-wrapping --saida kms_wrapping.json            # chaves de embrulho dos mounts (para o provision.py da Fase 1)
tccelta provisionar importar-registro --arquivo kms_record.json [--descricao "..."]   # dongle REAL, provisionado pela Fase 1
tccelta provisionar novo     --quantidade N --saida /saida [--perfil hmac|asym] [--hw dev|devsec] [--descricao "..."]   # só dongles de TESTE/simulador (AMBIENTE != producao); grava a imagem NVS direto, sem CSV em disco
tccelta jwt rotacionar       [--agora]                                    # rotação e expurgo da chave do JWT (também roda em laço no serviço `jwt-rotacao`)
tccelta provisionar importar --arquivo /testdata/dongles-teste.csv      # só AMBIENTE != producao
tccelta provisionar verificar --dev-id <hex>                              # teste dourado (dev)
```

**[Decisão, reconciliação com a Fase 1] Dois caminhos de provisionamento, um contrato.** Dongles reais são provisionados na estação de fabricação pelo `esp32-firmware/tools/provision/provision.py` da Fase 1 (spec 01 §11), que gera `DevId`/`K_auth`/`K_rec` (e `k_sig` no perfil `asym`), a imagem da partição `secnvs` e o envelope **`kms_record.json`**. Esta fase consome o envelope com `importar-registro`; as chaves nunca saem da estação em claro. O subcomando `novo` fica restrito a dongles de teste e ao simulador (geração no servidor), recusado em `AMBIENTE=producao`.

Fluxo com dongle real:

```
1. make kms-wrapping           → tccelta provisionar exportar-wrapping: lê transit-auth/wrapping_key e
                                 transit-rec/wrapping_key (RSA-4096 públicas) e grava kms_wrapping.json
                                 {"schema":"tccelta.kms.wrapping.v1","auth":{"kid":"<sha256 8 B hex>","pem":"..."},"rec":{...}}
2. Estação (Fase 1)            → provision.py --profile prod --kms-wrapping kms_wrapping.json → kms_record.json
3. make kms-importar ARQ=…     → tccelta provisionar importar-registro --arquivo kms_record.json:
     a) valida schema, dev_id (16 hex), security_profile, kid de cada chave == kid do mount atual
     b) BEGIN; InserirDongle(dev_id, perfil, chave_publica = k_sig_public se asym); InserirDongleEstado
     c) transit-auth/keys/<hex>/import {ciphertext: k_auth.ciphertext_b64, type: hmac, allow_rotation: false, exportable: false}
        transit-rec/keys/<hex>/import  {ciphertext: k_rec.ciphertext_b64, …}
        falha → ROLLBACK; registrar a chave órfã
     d) COMMIT
```

Esquema de `kms_record.json` (fonte única: Fase 1, spec 01 §11; copiado aqui para o importador):

```json
{
  "schema": "tccelta.kms.v1",
  "dev_id": "544343454C544101",
  "security_profile": "hmac",
  "created_at": "2026-10-26T12:00:00Z",
  "k_auth": { "alg": "vault-byok-rsa-oaep-sha256-aes-kwp", "kid": "<sha256 da wrapping key do transit-auth, 8 B hex>", "ciphertext_b64": "..." },
  "k_rec":  { "alg": "vault-byok-rsa-oaep-sha256-aes-kwp", "kid": "<idem, transit-rec>", "ciphertext_b64": "..." },
  "k_sig_public": "04…"
}
```

`ciphertext_b64` é exatamente o formato BYOK do Transit (chave AES-256 efêmera cifrada com RSA-OAEP-SHA256 pela wrapping key do mount, concatenada ao material embrulhado com AES-KWP, RFC 5649), gerado pelo `provision.py` com `cryptography`. `alg: "none"` (chaves em claro) só é aceito com `AMBIENTE != producao`: o importador embrulha localmente antes de importar.

Variáveis (definidas no serviço `ferramentas`, spec 01): `VAULT_ADDR`, `VAULT_CACERT`, `VAULT_TOKEN_ARQUIVO=/run/approle/provisionador_token` (volume `vault_approle_ferramentas`), `BANCO_URL` (papel `tccelta_provisionador`), `BANCO_SENHA_ARQUIVO=/run/secrets/pg_senha_provisionador`.

Fluxo de `novo`, por dongle:

```
1. dev_id := 8 bytes crypto/rand; repetir se existir em `dongle` OU se transit-*/keys/<hex> existir
2. k_auth, k_rec := 32 bytes crypto/rand cada
3. BEGIN; InserirDongle(dev_id, perfil, kms_chave_auth=hex, kms_chave_rec=hex); InserirDongleEstado
4. importar(transit-auth, hex, k_auth); importar(transit-rec, hex, k_rec)
     falha → ROLLBACK; registrar a chave órfã (DevId nunca usado; inofensiva)
5. COMMIT
6. montar o CSV da `secnvs` SÓ NA MEMÓRIA e passá-lo por stdin ao gerador da Fase 1 (ver abaixo);
   escrever a imagem e o restante do material do firmware (0600); zerar k_auth/k_rec e o CSV
```

Saídas:

**(a) Material do firmware (só para dongles de teste; contrato cruzado com a Fase 1)** em `saida/<dev_id_hex>/`:

- **[Decisão] A partição NVS é gerada direto pela ferramenta `novo`, sem `secnvs.csv` em disco** (escolha do autor: "fazer junto com a Fase 1", reaproveitando o fluxo de imagem do `provision.py`). A ferramenta monta o CSV **na memória**, no formato fixado pela Fase 1 (spec 01 §11: partição `secnvs`, namespace `sec`), e o entrega por *stdin* ao mesmo gerador da Fase 1, `python -m esp_idf_nvs_partition_gen generate - secnvs.bin 0x3000` (`/dev/stdin`; **[verificar]** que o gerador aceita o CSV por stdin; se não, usar um *named pipe*/`tmpfs` em RAM). Por isso a imagem `ferramentas` não é a distroless do binário: o alvo `ferramentas` do `Dockerfile` (spec 01) é uma `python:3.12-slim` com `esp-idf-nvs-partition-gen` e o binário `tccelta`. Saídas por dongle em `saida/<hex>/`:
  - `secnvs.bin`: imagem pronta, para gravar com `esptool.py write_flash 0x19000 secnvs.bin`. Com `--hw dev` (padrão, **plano B1 da Fase 1**: o `dev` não tem NVS encryption) a imagem é **em claro** (`generate`); com `--hw devsec`, a ferramenta gera a chave XTS (`generate-key`, gravada em `nvs_keys.bin`, 0600) e produz a imagem **cifrada** (`encrypt … --inputkey`), como o `provision.py --profile devsec`, e a gravação de `nvs_keys.bin` em `0x17000` segue a spec 01 §10.2;
  - o `provision.py --import` da Fase 1 deixa de ser necessário para dongles de teste (continua aceitando um CSV de outra origem).

  Formato do CSV em memória (idêntico ao da Fase 1):

  ```csv
  key,type,encoding,value
  sec,namespace,,
  prov_ver,data,u8,1
  dev_id,data,hex2bin,0011223344556677
  k_auth,data,hex2bin,<64 hex>
  k_rec,data,hex2bin,<64 hex>
  prof,data,u8,1
  ```
  (`prof`: 0 = none, 1 = hmac, 2 = asym; com `asym`, linha extra `k_sig,data,hex2bin,<64 hex>`.)
- `bench_keys.json` (`dev_id`, `k_auth`, `k_rec` em hex), o mesmo arquivo que o `provision.py --profile dev` produz para o servidor de bancada do app (Fase 1, spec 09). É o único arquivo com chaves em claro que sobra em disco (dongles descartáveis).
- `dongle.json` **sem segredos**: `{"dev_id":"…","perfil":"hmac","criado_em":"…","kms":{"auth":"transit-auth/…","rec":"transit-rec/…"}}`.
- O README da Fase 3 instrui: gravar `secnvs.bin` na placa e **apagar** `bench_keys.json`/`nvs_keys.bin` (`shred -u`) depois de usá-los. `saida/` está no `.gitignore`. Dongles reais **não** passam por aqui (ver `importar-registro`).

**(b) Importação no Vault:** feita pela própria ferramenta (passo 4).

**`importar` (dev/CI):** lê `testdata/dongles-teste.csv` (`dev_id_hex,k_auth_hex,k_rec_hex,perfil`), com 3 dongles fixos usados pelo simulador e pelos testes. Faz os passos 3–5 com as chaves dadas e, em seguida, `verificar`. Recusa se `AMBIENTE=producao`. As chaves de teste são públicas por definição (estão no repositório) e nunca podem ser usadas em dongle real; o CSV tem um cabeçalho de aviso.

**Teste dourado (`verificar`, só dev/CI), a premissa crítica:**

1. `transit-auth/hmac/<hex>` de uma entrada conhecida == `HMAC-SHA256(k_auth, entrada)` calculado localmente;
2. `transit-rec/verify/<hex>` de uma tag correta ⇒ `valid=true`, e com 1 bit trocado ⇒ `valid=false`;
3. com o token **da API**: `transit-rec/hmac/<hex>` ⇒ **403**.

Se (1) falhar, o tipo de chave ou a versão do Vault não servem, e a fase para aqui (checkpoint 2).

### Cliente Go (`internal/kms`)

```go
package kms

var ErrIndisponivel = errors.New("kms_indisponivel") // timeout, 5xx, disjuntor aberto, token inválido

// Autenticador: operações com as chaves por dongle.
type Autenticador interface {
    // HMACAuth devolve o HMAC-SHA256 COMPLETO (32 B) de cada entrada com K_auth do dongle.
    // Quem chama trunca para 16 B e compara com protocolo.CompararTag.
    HMACAuth(ctx context.Context, d dongles.Dongle, entradas [][]byte) ([][]byte, error)
    // VerificarBlocos verifica tags de 32 B com K_rec; uma chamada com batch_input.
    VerificarBlocos(ctx context.Context, d dongles.Dongle, entradas, tags [][]byte) ([]bool, error)
}

// AssinadorJWT: chave Ed25519 no Transit.
type AssinadorJWT interface {
    Assinar(ctx context.Context, entrada []byte) (assinatura []byte, versao int, err error)
    ChavesPublicas(ctx context.Context) (map[int]ed25519.PublicKey, error)
}

type Vault struct { /* *api.Client, mounts, semáforo, disjuntor, métricas */ }
func NovoVault(ctx context.Context, cfg config.VaultConfig, log *slog.Logger, m *metricas.KMS) (*Vault, error)
func (v *Vault) Pronto() bool                 // token válido e disjuntor fechado
func (v *Vault) Fechar()
```

Chamadas HTTP (via `client.Logical().WriteWithContext`):

| Operação | Caminho | Corpo | Resposta usada |
|---|---|---|---|
| `HMACAuth` (1 entrada) | `POST transit-auth/hmac/<nome>/sha2-256` | `{"input":"<b64>"}` | `data.hmac` = `vault:v1:<b64>` |
| `HMACAuth` (n) | idem | `{"batch_input":[{"input":"<b64>"},…]}` | `data.batch_results[i].hmac` (ou `.error`) |
| `VerificarBlocos` | `POST transit-rec/verify/<nome>/sha2-256` | `{"batch_input":[{"input":"<b64>","hmac":"vault:v1:<b64 tag>"},…]}` | `data.batch_results[i].valid` |
| `Assinar` | `POST transit-jwt/sign/jwt` | `{"input":"<b64 da signing string>"}` | `data.signature` = `vault:v<k>:<b64 64 B>` |
| `ChavesPublicas` | `GET transit-jwt/keys/jwt` | — | `data.keys["<k>"].public_key` (b64) |

- **Versão das chaves por dongle:** sempre `v1`, porque são importadas e não rotacionáveis (`allow_rotation=false`). Uma resposta com outra versão é tratada como erro de integridade.
- **Erro por item no batch** (`batch_results[i].error`): tratado como falha do KMS (503), **não** como tag inválida. Só `valid:false` explícito vira tag inválida.
- **Timeouts:** `VAULT_TIMEOUT` (2 s local / 5 s VPN) por chamada; `VAULT_RETENTATIVAS` (1 local / 2 VPN) só para erros de rede/5xx/429, com backoff de 100–400 ms.
- **Concorrência:** semáforo `VAULT_MAX_SIMULTANEAS=32` por réplica.
- **Disjuntor** (`disjuntor.go`): 5 falhas seguidas abrem por 10 s (respostas imediatas `ErrIndisponivel` ⇒ 503 + `Retry-After: 10`); depois, meia-abertura com 1 sonda.
- **Transporte:** `http.Transport{MaxIdleConnsPerHost: 64, IdleConnTimeout: 90s, TLSClientConfig: CA de VAULT_CACERT, certificado de cliente se configurado}`.
- **Métricas:** `tccelta_kms_chamadas_total{operacao,resultado}`, `tccelta_kms_duracao_segundos{operacao}`, `tccelta_kms_disjuntor_aberto` (gauge).
- **Logs:** nunca o `input` nem o HMAC; apenas a operação, o DevId, o tamanho do lote e a duração.

`internal/kms/kmsteste/memoria.go` implementa as duas interfaces com chaves em memória (`crypto/hmac`, `crypto/ed25519`), com contadores de chamadas e injeção de falhas. Só pode ser importado por testes, garantido por um `depguard` no `.golangci.yml` que proíbe o import fora de `_test.go` e de `internal/simulador`.

### Perfis `none` e `asym` (benchmark da Fase 1)

`kms.Autenticador` é escolhido **por perfil do dongle** num registro `map[string]Verificador`, então o caminho HMAC não muda:

| Perfil | 0x02 (resposta) | 0x03 (bloco) | 0x01/0x04 (servidor → dongle) |
|---|---|---|---|
| `hmac` (padrão) | HMAC K_auth, 16 B (Vault `hmac` + prefixo) | HMAC K_rec, 32 B (Vault `verify`) | HMAC K_auth, 16 B |
| `asym` ([Decisão]: depois do HMAC estável) | HMAC K_auth, 16 B (igual) | ECDSA P-256 determinística do dongle, `r‖s` 64 B | HMAC K_auth, 16 B |
| `none` (bancada) | HMAC K_auth, 16 B (igual) | sem tag (0 B) | HMAC K_auth, 16 B |

**Reconciliado com a Fase 1 (spec 01 §1):** o perfil só muda a tag do **bloco**; o handshake usa `K_auth` com tag de 16 B nos três perfis. Logo `K_auth` é importada no Vault para todo dongle, inclusive `none` e `asym`.

- **[Decisão] `asym`: implementar depois que o caminho HMAC estiver estável** (escolha do autor; não compete com a estabilização do HMAC nem verifica no Transit):
  - Algoritmo fixado pela Fase 1: **ECDSA P-256 determinística (RFC 6979), SHA-256, assinatura `r‖s` de 64 B** (o mbedTLS 2.28 do Arduino 2.0.17 não tem Ed25519). O par `k_sig` é gerado pelo `provision.py` da Fase 1 na fabricação: a privada vai para a `secnvs`, a pública (65 B, ponto não comprimido `04‖X‖Y`) vem no `kms_record.json` (`k_sig_public`) e é gravada em `dongle.chave_publica`.
  - A verificação é **local** na API com `crypto/ecdsa` (a chave pública não é segredo), sobre `SHA-256("MALCONG" || 0x03 || DevId || dados)`. Opcionalmente, a chave pública pode ser importada no Transit (`transit-rec-asym/keys/<hex>/import`, tipo `ecdsa-p256`, só `public_key`) para manter a verificação no KMS e medir esse custo também.
  - A tag de 64 B muda só `L` do bloco (`protocolo.PerfilAsym`).
  - Ordem: primeiro `hmac` completo (specs 03–06 e carga da spec 10); só então o verificador `asym` (local, `crypto/ecdsa`), o registro de verificadores por perfil já previsto acima e o benchmark `hmac × asym` ponta a ponta. A verificação no Transit (`transit-rec-asym`) e o desenho "só `hmac`/`none` no servidor" **não** são adotados. O desenho acima não muda o caminho HMAC.
- **`none`:** aceito somente com `PERMITIR_PERFIL_NONE=true` **e** `AMBIENTE != producao` (`config.Carregar` falha caso contrário), e somente para dongles cadastrados com `perfil_seguranca='none'`. O verificador de blocos devolve `true` sem chamar o Vault; o handshake continua passando pelo Vault com `K_auth`. Os blocos ficam marcados (`bloco_bruto.assinatura` vazia), então nunca se confundem com dados autenticados nas análises.

## Plano arquivo por arquivo

| Arquivo | Conteúdo |
|---|---|
| `deploy/vault/politicas/tccelta-api.hcl`, `tccelta-provisionador.hcl`, `tccelta-teste-dourado.hcl`, `tccelta-rotacao-jwt.hcl` | políticas |
| `deploy/vault/scripts/bootstrap.sh`, `init-unseal.sh` | scripts |
| `internal/kms/kms.go` | interfaces, erros |
| `internal/kms/vault.go` | implementação Vault + login/renovação AppRole |
| `internal/kms/disjuntor.go` (+`_test`) | circuit breaker |
| `internal/kms/vault_integracao_test.go` (`integracao`) | Vault real (testcontainers `hashicorp/vault` em `-dev`) |
| `internal/kms/kmsteste/memoria.go` | dublê |
| `internal/provisionamento/embrulho.go` (+`_test` com vetores RFC 5649) | BYOK |
| `internal/provisionamento/provisionar.go` | `Novo`, `Importar`, `Verificar` |
| `internal/provisionamento/material.go` | CSV da `secnvs` só em memória → `secnvs.bin` via gerador (stdin), `bench_keys.json`, `dongle.json` (dongles de teste) |
| `internal/provisionamento/nvsgen.go` | chamada ao `esp_idf_nvs_partition_gen` por `os/exec` (stdin), `--hw dev|devsec` |
| `internal/kms/rotacao.go` (+`_test`) | rotação e expurgo da chave do JWT (`jwt rotacionar`), ver *Rotação da chave do JWT* |
| `cmd/tccelta/jwt.go` | subcomando `jwt rotacionar` / `jwt agendar` |
| `internal/provisionamento/registro.go` | leitura/validação de `kms_record.json` e `importar-registro`; `exportar-wrapping` |
| `cmd/tccelta/provisionar.go` | subcomando |
| `testdata/dongles-teste.csv` | 3 dongles de teste (chaves públicas de teste) |

## Testes

1. `aesKWP` com os vetores do RFC 5649 §6 (chaves de 20 e 7 bytes) e com 32 bytes (ida e volta via *unwrap* de teste).
2. Integração com Vault real:
   - bootstrap no contêiner;
   - `importar` das chaves de teste;
   - teste dourado (1)–(3);
   - `VerificarBlocos` com 32 entradas, uma delas inválida ⇒ `[true…false…true]`;
   - `Assinar` + verificação local com `ChavesPublicas`.
3. **Políticas (negativos), logado com o AppRole da API**, cada uma deve dar 403:
   - `transit-rec/hmac/<x>`;
   - `transit-rec/export/hmac-key/<x>`;
   - `transit-rec/keys/<x>/config exportable=true`;
   - `transit-rec/backup/<x>`;
   - `transit-rec/byok-export/<y>/<x>`;
   - `transit-rec/keys/<x>/rotate`;
   - `transit-auth/export/hmac-key/<x>`;
   - `transit-jwt/keys/jwt/rotate`;
   - `sys/policies/acl/tccelta-api`;
   - `transit-auth/keys/<x>/import`.
4. **Provisionador:**
   - não consegue `transit-rec/verify`, `transit-auth/hmac` (fora de dev) nem `import_version`;
   - importar com `exportable=true`, `allow_rotation=true` ou `type=aes256-gcm96` ⇒ 403;
   - importar duas vezes o mesmo nome falha.
5b. **Isolamento de credenciais (revisão):** dentro do contêiner `api`, `/run/approle` contém só `api_role_id` e `api_secret_id` (sem o token do provisionador).
5. Disjuntor: 5 erros ⇒ aberto ⇒ `ErrIndisponivel` imediato ⇒ fecha após a sonda bem-sucedida.
6. Renovação: `token_ttl=10s` no teste ⇒ a API continua chamando o Vault por 60 s sem erro (renovação e relogin).

## Passos ordenados com checkpoints

1. Políticas + `bootstrap.sh` no compose dev. **Checkpoint 1:** `vault policy read tccelta-api` igual ao arquivo; arquivos de AppRole criados com dono 65532.
2. `embrulho.go` + `provisionar importar` + `verificar`. **Checkpoint 2 (crítico):** teste dourado (1)–(3) passa. Se falhar, reavaliar a versão do Vault ou o tipo de chave antes de seguir.
3. `kms/vault.go` (login, renovação, `HMACAuth`, `VerificarBlocos`, `Assinar`, `ChavesPublicas`, disjuntor). **Checkpoint 3:** testes 2, 5 e 6.
4. Testes de política negativos. **Checkpoint 4:** teste 3 verde.
5. `provisionar novo` + material. **Checkpoint 5:** `make provisionar N=1` gera `saida/<hex>/secnvs.bin` (sem `secnvs.csv` em disco: `find saida -name '*.csv'` vazio), e o dongle aparece no banco e no Vault (`vault read transit-auth/keys/<hex>` com `exportable=false`).
6. `exportar-wrapping` + `importar-registro`. **Checkpoint 6:** um `kms_record.json` gerado pelo `provision.py` da Fase 1 (ou pelo teste, que embrulha com a mesma rotina) é importado e passa no teste dourado.
6. Perfil prodlike (TLS, Raft, unseal). **Checkpoint 6:** depois de `restart` + unseal, a API volta a ficar pronta sozinha.

## Critérios de aceite

- [ ] O HMAC do Vault com a chave importada é idêntico ao HMAC local (teste dourado).
- [ ] O token da API não consegue gerar HMAC, exportar, fazer backup nem reconfigurar `K_rec` (testes negativos).
- [ ] Os segredos de K_auth/K_rec nunca aparecem em logs, no banco nem no git. Só existem na NVS do dongle, no Vault e, para dongles de teste, em `saida/<hex>/bench_keys.json`/`secnvs.bin` (descartáveis; o CSV da `secnvs` nunca toca o disco) ou na memória da estação de fabricação (Fase 1).
- [ ] A chave do JWT é rotacionada e expurgada sem intervenção: o job `jwt-rotacao` gera `v2`, os tokens `v1` continuam válidos até expirar, e depois a `v1` deixa de ser aceita.
- [ ] Um lote de até 32 blocos usa 1 chamada de verificação.
- [ ] O KMS indisponível vira 503 + `Retry-After` em todos os endpoints, sem 500.

## Riscos

| Risco | Mitigação |
|---|---|
| Versão do Vault sem tipo `hmac` importável | checkpoint 2 logo no início; fixar a versão |
| `root_token` e chave de unseal no mesmo volume (laboratório) | passos manuais de revogação; recusa em `AMBIENTE=producao` |
| Comprometimento da API ⇒ abuso de `transit-auth/hmac` (forjar Desafio/ACK) | inerente ao desenho do doc (`K_auth` pode ser gerada pela API); auditoria do Vault registra cada chamada; `K_rec` continua protegida, então blocos não podem ser forjados |
| Chave órfã no Vault após falha parcial do provisionamento | DevId nunca entregue; inofensiva; registrada no log |
| `bench_keys.json`/`secnvs.bin` com chaves em claro no disco (só dongles de teste, perfil `dev`) | permissões 0600, `saida/` fora do git, instrução de `shred` no README; o CSV nunca é gravado; dongles reais usam o `provision.py` da Fase 1 na estação + `importar-registro` |
| A imagem `ferramentas` (Python + gerador NVS) é maior e tem shell | só no perfil `ferramentas`, rede interna, sem porta; o binário distroless da API/worker não muda |

## Rotação da chave do JWT (automática)

**[Decisão] Rotação automatizada** (escolha do autor, em vez de só documentar o procedimento). O desenho de `kid` = versão (spec 08) já suporta duas versões convivendo. Um serviço dedicado, **fora da API e do worker**, faz o ciclo de vida da chave `transit-jwt/keys/jwt`:

- **Serviço `jwt-rotacao`** (spec 01): o mesmo binário, `tccelta jwt agendar`, em laço a cada `JWT_ROTACAO_VERIFICACAO` (padrão `1h`); credencial própria (AppRole `tccelta-rotacao-jwt`, volume `vault_approle_rotacao`), só na rede `kms`. A API continua com `deny` em `transit-jwt/keys/jwt/*`: **o token da API não rotaciona nem expurga nada**.
- **Política `tccelta-rotacao-jwt.hcl`** (única do serviço, `token_no_default_policy=true`):
  ```hcl
  path "transit-jwt/keys/jwt"        { capabilities = ["read"] }     # versões e creation_time
  path "transit-jwt/keys/jwt/rotate" { capabilities = ["update"] }
  path "transit-jwt/keys/jwt/config" { capabilities = ["update"] }   # min_decryption_version
  # sem sign/*, export/*, backup/*: este serviço não assina nem lê material
  ```
- **Estado só no Vault** (sem tabela nova): cada verificação lê `transit-jwt/keys/jwt` (`latest_version`, `keys.<v>.creation_time`).
  1. **Rotacionar** se `agora − creation_time(latest) ≥ JWT_ROTACAO_PERIODO` (padrão `720h`, 30 dias): `POST transit-jwt/keys/jwt/rotate`. A API passa a assinar com a versão nova a partir da próxima chamada (`kid = jwt-v<nova>`) e recarrega as públicas ao ver o `kid` desconhecido (ou a cada 10 min).
  2. **Expurgar** a versão `k` quando `agora ≥ creation_time(k+1) + JWT_VALIDADE + JWT_FOLGA + 10 min` (nenhum token assinado com `k` pode mais estar válido nem em cache de verificação): `POST transit-jwt/keys/jwt/config {"min_decryption_version": k+1}`. Depois disso a API deixa de listar a chave pública `k` e tokens `kid = jwt-v<k>` passam a 401 `token_invalido`.
  3. Idempotente e seguro a reexecuções; `tccelta jwt rotacionar --agora` força o passo 1 (manutenção).
- **Concorrência:** duas instâncias do serviço rodando juntas apenas tentam as mesmas operações (o Vault rejeita/ignora a segunda `rotate` dentro do mesmo período pela checagem de `creation_time` antes de chamar); em produção roda uma só.
- **Alternativa não adotada:** `auto_rotate_period` do próprio Transit cobriria só a rotação, não o expurgo coordenado com `JWT_VALIDADE`. **[verificar]** se `min_decryption_version` omite as versões antigas de `GET transit-jwt/keys/jwt` para chaves Ed25519 na versão do Vault fixada; se não omitir, usar `min_available_version` (trim) além dele.
- **Testes** (Vault real em `-dev`, relógio injetado): chave criada na `v1`; com o relógio avançando 30 dias, `v2` existe; token assinado antes continua válido (`kid=jwt-v1`) durante `JWT_VALIDADE`; depois do prazo de expurgo `min_decryption_version = 2` e o token `v1` falha; token novo `jwt-v2` válido; o AppRole da API recebe 403 em `.../rotate` e `.../config`; o AppRole de rotação recebe 403 em `transit-jwt/sign/jwt` e `transit-rec/*`.

## Decisões do autor (antes em aberto)

- **[Decisão] Gerar a partição NVS direto na ferramenta `novo`** (ver *Ferramenta de provisionamento*): o CSV de dongles de teste nunca toca o disco. Para dongles reais o problema não existe: a Fase 1 gera a imagem na estação e só o `kms_record.json` (cifrado) chega aqui.
- **[Decisão] Perfil `asym`: depois do caminho HMAC estável** (verificação local com `crypto/ecdsa`; ver *Perfis `none` e `asym`*).
- **[Decisão] Rotação da chave do JWT automatizada** (ver acima).
- **[Decisão] Auto-unseal e HA do Vault:** unseal manual 3 de 5 em produção; auto-unseal e HA só documentados (spec 01).
