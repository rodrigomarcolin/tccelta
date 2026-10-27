# Contrato HTTP entre a API e o KMS

Este documento é a **fonte da verdade** do contrato. A API (`backend/internal/kmsremoto`) e o serviço
(`kms/internal/servidor`) são módulos Go separados, sem nenhum import entre si; a única ligação é este
contrato. Os dois lados têm testes que usam os exemplos abaixo, palavra por palavra.

## Convenções

- JSON em UTF-8. Requisições com corpo exigem `Content-Type: application/json`; campos desconhecidos são
  recusados (`400 corpo_malformado`). Corpo máximo: 1 MiB.
- Todo dado binário vai em **base64 padrão com padding** (RFC 4648 §4).
- Respostas de erro são `application/problem+json` (RFC 9457), no mesmo formato da API:
  `{"type","title","status","codigo","detail"}`. O campo `codigo` é o que o cliente deve ler.
- `X-Request-Id` é opcional na requisição e sempre devolvido.
- **Segurança**: fora de `AMBIENTE=dev` o serviço só aceita TLS com **certificado de cliente** assinado pela CA
  configurada em `KMS_TLS_CLIENT_CA` (mTLS). O KMS escuta apenas na rede privada/VPN.
- `chave` é o **nome da chave** no Vault (`Dongle.ChaveAuth` / `Dongle.ChaveRec` do cadastro), com o
  alfabeto `[A-Za-z0-9_-]` e até 64 caracteres (o nome entra no caminho do Vault, por isso é restrito).
- Listas (`entradas`, `tags`): de 1 a 256 itens; cada item decodificado tem de 1 a 4096 bytes.

## `POST /v1/hmac`

HMAC-SHA256 **completo** (32 B) de cada entrada com a chave K_auth `chave`. Quem chama trunca para 16 B e
compara em tempo constante.

Requisição:

```json
{"chave":"0011223344556677","entradas":["YQ==","Yg=="]}
```

Resposta `200`:

```json
{"macs":["AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=","AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI="]}
```

`macs[i]` corresponde a `entradas[i]`.

## `POST /v1/blocos/verificar`

Verifica as tags de 32 B dos blocos com a chave K_rec `chave`, numa única chamada ao Vault (`batch_input`).
`entradas` e `tags` têm o mesmo tamanho. Tag que não confere é `false`, **não** é erro.

Requisição:

```json
{"chave":"0011223344556677","entradas":["YQ==","Yg=="],"tags":["dA==","dQ=="]}
```

Resposta `200`:

```json
{"validos":[true,false]}
```

## `POST /v1/jwt/assinar`

Assina com a chave Ed25519 do JWT. `entrada` é a *signing string* (`base64url(header).base64url(claims)`) como
bytes; a resposta traz a assinatura de 64 B e a **versão** da chave que assinou (a API a põe no `kid`,
`jwt-v<versao>`).

Requisição:

```json
{"entrada":"YWJj"}
```

Resposta `200`:

```json
{"assinatura":"<base64 de 64 bytes>","versao":3}
```

## `GET /v1/jwt/chaves`

Chaves públicas Ed25519 que ainda validam tokens (versões `>= min_decryption_version`), por versão.

Resposta `200`:

```json
{"2":"<base64 de 32 bytes>","3":"<base64 de 32 bytes>"}
```

## Saúde

| Rota | Resposta |
|---|---|
| `GET /saude/viva` | `200` se o processo está de pé |
| `GET /saude/pronta` | `200` se o token do Vault é válido e o disjuntor está fechado; senão `503 nao_pronto` |

As duas rotas também existem em HTTP simples na porta de métricas (`KMS_METRICAS_ENDERECO`, padrão `:9300`),
para o healthcheck do contêiner. Em `:9300` fica também `GET /metricas` (séries `tccelta_kmssvc_*`).

## Erros

| Status | `codigo` | Quando |
|---|---|---|
| 400 | `corpo_malformado` | JSON inválido, campo desconhecido ou conteúdo extra |
| 400 | `chave_invalida` | `chave` fora do alfabeto/tamanho |
| 400 | `entradas_invalidas` | lista vazia/grande demais, base64 inválido, tamanho de item, `tags` ≠ `entradas` |
| 401 | `cliente_nao_autenticado` | sem certificado de cliente verificado (fora do dev) |
| 413 | `corpo_grande` | corpo acima de 1 MiB |
| 415 | `tipo_conteudo` | `Content-Type` diferente de `application/json` |
| 503 | `kms_indisponivel` | Vault fora do ar, erro do Vault (inclusive chave inexistente), disjuntor aberto; vem com `Retry-After` |
| 503 | `nao_pronto` | só em `/saude/pronta` |
| 500 | `erro_interno` | bug |

Exemplo de `503`:

```json
{"type":"https://tccelta.local/erros/kms/kms_indisponivel","title":"kms_indisponivel","status":503,"codigo":"kms_indisponivel","detail":"Vault indisponível"}
```

### Como a API interpreta as respostas

- Erro de rede (conexão recusada, falha de TLS), timeout, `429`, `5xx` ou resposta que não segue o contrato →
  `porta.ErrKMSIndisponivel`, que a API devolve ao app/firmware como `503 kms_indisponivel` (e conta para o
  disjuntor da própria API: 5 falhas seguidas abrem por 10 s).
- Qualquer outro `4xx` é bug de contrato: vira erro interno (`500`) e **não** abre o disjuntor.
- `GET /saude/pronta` do KMS alimenta o `/saude/pronta` da API (consulta em cache de 2 s).

## Timeouts e concorrência

O cliente da API usa timeout por chamada (`KMS_TIMEOUT`, padrão 2 s), no máximo `KMS_MAX_CONCORRENCIA` chamadas
simultâneas (padrão 32) e uma nova tentativa para falhas de rede que não sejam timeout. O serviço fala com o
Vault com o próprio timeout (`VAULT_TIMEOUT`), semáforo (`VAULT_MAX_SIMULTANEAS`) e disjuntor.

## Mudanças no contrato

Mudar este arquivo exige mudar, no mesmo PR, `backend/internal/kmsremoto` (DTOs e testes) e
`kms/internal/servidor` (handlers e testes).
