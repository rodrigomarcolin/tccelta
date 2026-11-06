# 08 — JWT e autenticação do app

Estado implementado: emissão e validação em `internal/adapter/jwt/{issuer,validator}.go`
(portas `port.TokenIssuer`/`port.TokenValidator` em `internal/port/token.go`),
caso de uso `internal/usecase/session/authenticate.go` e middleware
`internal/http/authentication.go`. A chave de assinatura (Ed25519) fica no Vault e é
usada só pelo KMS (`07`).

## Formato

JWT compacto (`header.claims.assinatura`, base64url sem padding), assinado com EdDSA.

| Parte | Conteúdo |
|---|---|
| header | `{"alg":"EdDSA","typ":"JWT","kid":"jwt-v<versão>"}`; `<versão>` é a versão da chave devolvida pelo Vault |
| claims | `iss` (`JWT_EMISSOR`, `tccelta-api`), `aud` (`JWT_AUDIENCIA`, `tccelta-ingestao`), `sub` (DevId em 16 hex minúsculos), `jti` (UUID), `iat`, `nbf`, `exp` (`iat + JWT_VALIDADE`, 1 h), `cs` (o `c_S` do handshake) |
| assinatura | 64 B Ed25519 sobre `base64url(header).base64url(claims)` |

O `jti` é gravado em `challenge.token_jti`; o `cs` vira `raw_block.epoch` em cada bloco ingerido. Não há revogação nem identidade de usuário: o token vale até expirar.

## Fluxo

```mermaid
sequenceDiagram
    participant A as App
    participant API as API
    participant K as KMS
    participant V as Vault
    Note over API,K: emissão (fim do handshake, fora da transação)
    API->>K: POST /v1/jwt/sign (signing string)
    K->>V: transit-jwt/sign/jwt
    V-->>K: assinatura + versão
    K-->>API: signature, version
    API-->>A: token (no corpo do handshake)
    Note over API: validação local, sem chamar o KMS por requisição
    A->>API: POST /blocos com Bearer
    API->>API: chave pública por kid; EdDSA; iss, aud, exp (+folga), cs, sub, jti
    API->>API: Authenticate: dongle existe, ativo, perfil permitido
    API-->>A: 200 | 401 token_ausente / token_invalido / token_expirado | 403
```

## Emissão (`jwt.Issuer`)

`Issue(ctx, DevID, cs)` monta o payload e assina por `port.JWTSigner.Sign`. Como o `kid` entra no header assinado e depende da versão da chave, o emissor usa a última versão conhecida (a primeira vez, a maior de `PublicKeys`); se o Vault assinar com outra versão (rotação), refaz uma vez com a versão certa, senão `ErrKMSUnavailable`. Devolve `IssuedToken{Token, JTI, ExpiresAt}`; `Validity()` alimenta `token_validade_s`.

## Validação (`jwt.Validator`)

- Chaves públicas por `kid` (`jwt-v<versão>`) em memória (`atomic.Pointer`), carregadas de `GET /v1/jwt/keys` na partida (a API tenta por até ~1 min, 30 × 2 s, enquanto o KMS sobe), a cada 10 min (`UpdatePeriodically`) e quando chega um `kid` desconhecido (no máximo uma recarga a cada 30 s).
- `golang-jwt`: só `EdDSA`, `iss`, `aud`, `exp` obrigatório, folga `JWT_FOLGA` (30 s), números JSON preservados (`cs`).
- Depois do parse: `sub` com 16 caracteres e DevId válido; `cs` inteiro ≥ 1 em `uint32`; `jti` UUID. Falha ⇒ `ErrInvalidToken`; expiração ⇒ `ErrTokenExpired`.
- `Ready()` (chaves carregadas) compõe o `/saude/pronta` da API.

## Autenticação no HTTP

`authenticateJWT(requireSubEqualID)` (`http/authentication.go`): exige `Authorization: Bearer <token>`; o caso de uso `session.Authenticate` valida o token, resolve o dongle (desconhecido ⇒ `token_invalido`) e aplica `VerifyAccess` (`dongle_inativo`, `perfil_nao_permitido`, ambos 403). O `Session` (`Dev`, `CS`, `JTI`, `ExpiresAt`) vai no contexto.

| Situação | Resposta |
|---|---|
| sem cabeçalho / sem `Bearer ` | 401 `token_ausente`, `WWW-Authenticate: Bearer realm="tccelta"` |
| expirado | 401 `token_expirado` |
| inválido (assinatura, `iss`, `aud`, `kid`, claims) | 401 `token_invalido` |
| `sub` ≠ `{id}` na rota de confirmação | 403 `token_outro_dongle` |
| dongle inativo / perfil `none` não permitido | 403 `dongle_inativo` / `perfil_nao_permitido` |

`POST /blocos` usa `authenticateJWT(false)` (o DevId vem do token); `GET /confirmacao` usa `authenticateJWT(true)`. Os middlewares de limite por DevId usam o `sub` do token quando há sessão.

## Rotação

Feita pelo serviço `jwt-rotacao` (ver `07`): tokens emitidos com a versão anterior continuam válidos até `exp` porque a versão só é expurgada depois de `JWT_VALIDADE` + folga; a API recarrega as chaves por `kid` desconhecido ou a cada 10 min.

## Token de cliente (client credentials, análise de viagem)

Separado do token do app: `ClientIssuer`/`ClientValidator` (`adapter/jwt/client_{issuer,validator}.go`, portas `port.ClientTokenIssuer`/`ClientTokenValidator`), caso de uso `usecase/clienttoken` e middleware `authenticateAnalise` (`http/authentication_analysis.go`) em `/v1/viagens*`. O `Validator` do app **não muda**.

| | Token do app | Token de cliente |
|---|---|---|
| `kid` / chaves | `jwt-v<n>`, `GET /v1/jwt/keys` | `cli-v<n>`, `GET /v1/client-jwt/keys` |
| `aud` | `JWT_AUDIENCIA` (`tccelta-ingestao`) | `JWT_AUDIENCIA_ANALISE` (`tccelta-analise`) |
| `sub` | DevId (16 hex) | `client_id` |
| `cs` | obrigatório | ausente |
| validade | `JWT_VALIDADE` (1 h) | `CLI_JWT_VALIDADE` (30 min) |

Os dois mapas de chaves vêm de rotas diferentes do KMS e nunca se misturam: um token do app é recusado nas rotas de viagem (`kid`, `aud`) e um token de cliente é recusado em `/v1/dongles/*` (`kid`, `aud`, `sub` sem 16 hex, falta `cs`). Respostas do middleware: `401 token_ausente|token_expirado|token_invalido`, com `WWW-Authenticate: Bearer`. `ANALISE_SEM_AUTH=true` abre as rotas de viagem sem conferir o Bearer (inválido em `producao`).

`POST /v1/auth/token`: HTTP Basic, `grant_type=client_credentials` em `application/x-www-form-urlencoded` (até 1 KiB); erros no formato da RFC 6749 §5.2 (`{error, error_description, codigo}`); `401 invalid_client` idêntico para sem Basic, cliente desconhecido, revogado e segredo errado; `429 temporarily_unavailable` (`bloqueado`) com `Retry-After`; `Cache-Control: no-store` em todas as respostas. Bloqueio progressivo por par `(client_id, IP)` e por IP (`adapter/authblock`, Redis com dois scripts Lua e estado local de reserva), `GET /.well-known/jwks.json` com as chaves `cli`.

## Testes

`adapter/jwt` (emissão, `kid`, rotação durante a emissão, validação, expiração, claims inválidas), `usecase/session`, `http` (mapeamento de erros e cabeçalhos), cenário `negative-protocol` do simulador (token lixo ⇒ 401; token de outro dongle ⇒ 403).
