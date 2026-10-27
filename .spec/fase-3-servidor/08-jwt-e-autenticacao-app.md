# 08 — JWT e autenticação do app

> Fase 3 · Servidor. Depende de: 03 (HTTP), 07 (`AssinadorJWT` no Transit). É dependência de: 04 (emissão), 05 (validação no lote e na confirmação), 09 (app).

## Objetivo

Emitir, no handshake aceito, um JWT que autoriza o app a enviar blocos daquele dongle, e validá-lo nos endpoints protegidos com os status da Tabela 5 (401/403). Pelo doc (§6), o token "serve para identificar o usuário e limitar abusos, nunca para dar confiança aos dados": a confiança nos dados vem só das tags `K_rec`. Por isso, o JWT pode ser simples, curto e sem revogação.

## Escopo

Dentro: formato do token, emissão via Vault (Ed25519), validação local, middleware, mapeamento de erros, rotação de chave (automática, serviço `jwt-rotacao`, spec 07 e spec 01), testes. Fora: contas de usuário no app (**[Decisão]** fora do escopo do TCC; ver *Decisões do autor*).

## Design

### Formato

Header:

```json
{ "alg": "EdDSA", "typ": "JWT", "kid": "jwt-v1" }
```

Claims:

| Claim | Valor | Observação |
|---|---|---|
| `iss` | `JWT_EMISSOR` (`tccelta-api`) | |
| `aud` | `JWT_AUDIENCIA` (`tccelta-ingestao`) | string única |
| `sub` | DevId em 16 hex minúsculos | **o DevId do lote vem daqui** (spec 05) |
| `jti` | UUID v4 | gravado em `desafio.token_jti` (spec 04) |
| `iat`, `nbf` | instante de emissão (s) | |
| `exp` | `iat + JWT_VALIDADE` (padrão 3600 s) | |
| `cs` | `c_S` do handshake que emitiu o token (número) | vira `bloco_bruto.epoca` |

Tamanho típico: cerca de 330 caracteres.

**[Decisão] Validade de 1 h.**

- O app renova quando faltar menos de 5 min (doc §5.2), então um dongle ativo faz cerca de 1 handshake extra por hora. O custo é baixo: 2 requisições, 1 ida e volta BLE e 3 chamadas ao Vault.
- Como o token não protege os dados, um vazamento só permite consumir a cota do dongle e enviar lixo, que a verificação de tag rejeita. Uma validade curta limita essa janela sem custo perceptível.
- Uma validade maior (24 h) reduziria handshakes, mas também as âncoras de tempo "de brinde" que cada handshake renovado produz. Esse efeito colateral é útil: mais âncoras melhoram a reta do boot (spec 06).

**[Decisão] Sem revogação.** O doc não exige revogação, e o dano de um token vazado é limitado, como descrito acima. Desativar um dongle (`dongle.ativo = false`) bloqueia todos os tokens dele em até 60 s (TTL do cache de dongles), e é esse o "kill switch" operacional. O `jti` fica gravado só para auditoria.

### Emissão (`internal/token/emissor.go`)

```go
package token

type Emissor struct {
    assinador kms.AssinadorJWT
    emissor, audiencia string
    validade  time.Duration
    agora     func() time.Time
}

type Emitido struct {
    Token    string
    JTI      uuid.UUID
    ExpiraEm time.Time
}

// Emitir monta header.claims em base64url (sem padding), assina a "signing string"
// no Transit (Ed25519, 64 B) e devolve o JWT compacto. kid = "jwt-v<versão devolvida pelo Vault>".
func (e *Emissor) Emitir(ctx context.Context, dev protocolo.DevID, cs uint32) (Emitido, error)
```

A serialização compacta é feita à mão, com `encoding/json` + `base64.RawURLEncoding`, porque a assinatura vem do Vault com contexto e a interface `SigningMethod` do `golang-jwt` não recebe `context.Context`. A validação usa `golang-jwt/v5`, que é a parte com mais armadilhas.

A *signing string* enviada ao Transit é exatamente o ASCII de `base64url(header) + "." + base64url(claims)`, codificada em base64 padrão no campo `input`. Para `ed25519`, o Transit assina a mensagem crua (sem prehash; `hash_algorithm` é ignorado) e devolve os 64 bytes da assinatura EdDSA, que viram o terceiro segmento em base64url sem padding. O teste 6 confere isso com a chave pública lida do Vault.

### Validação (`internal/token/validador.go`)

```go
type Sessao struct {
    Dev      protocolo.DevID
    CS       uint32
    JTI      uuid.UUID
    ExpiraEm time.Time
}

type Validador struct {
    chaves   atomic.Pointer[map[string]ed25519.PublicKey] // kid → pública
    fonte    kms.AssinadorJWT                            // ChavesPublicas()
    emissor, audiencia string
    folga    time.Duration                                // JWT_FOLGA = 30 s
}

func NovoValidador(ctx context.Context, ...) (*Validador, error) // carrega as chaves; falha se o Vault não responder no boot
func (v *Validador) Validar(ctx context.Context, bruto string) (Sessao, error) // ErrTokenInvalido | ErrTokenExpirado
func (v *Validador) AtualizarPeriodicamente(ctx context.Context, cada time.Duration) // 10 min
```

Regras:

- `jwt.NewParser(jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithIssuer(...), jwt.WithAudience(...), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(30*time.Second))`. Isso fixa o algoritmo: `none`, `HS256` com a chave pública como segredo e demais confusões de algoritmo são recusados.
- `keyfunc`: lê `kid` com *type assertion* segura (`kid, ok := t.Header["kid"].(string)`; um `kid` numérico, nulo ou ausente vira token inválido, nunca panic/500) e busca no mapa. Um `kid` desconhecido dispara **uma** recarga das chaves públicas (no máximo 1 a cada 30 s) e, se continuar desconhecido, o token é inválido.
- Claims numa struct própria (`type claims struct{ jwt.RegisteredClaims; CS json.Number \`json:"cs"\` }`), decodificadas com `jwt.WithJSONNumber()`. O `cs` é validado como inteiro exato em `[1, 2³²−1]` (`strconv.ParseUint(cs, 10, 32)`), sem passar por `float64`. `sub` precisa ser 16 hex válidos. Se algo falhar, o token é inválido.
- A validação é **local** (sem Vault por requisição): a chave pública fica em memória e é atualizada a cada 10 min.

### Middleware (`internal/httpapi/autenticacao.go`)

```go
// autenticarJWT valida o Bearer, resolve o dongle e põe Sessao + Dongle no contexto.
// exigirSubIgualID: nas rotas com {id} (confirmação), sub precisa ser igual a {id}.
func autenticarJWT(v *token.Validador, d *dongles.Cache, exigirSubIgualID bool) func(http.Handler) http.Handler
func SessaoDe(ctx context.Context) (token.Sessao, dongles.Dongle)
```

| Situação | Status | `codigo` | Header |
|---|---|---|---|
| Sem `Authorization` ou esquema ≠ `Bearer` | 401 | `token_ausente` | `WWW-Authenticate: Bearer realm="tccelta"` |
| Assinatura, `alg`, `kid`, `iss`, `aud`, `sub` ou `cs` inválidos | 401 | `token_invalido` | `WWW-Authenticate: Bearer error="invalid_token"` |
| `exp` vencido (além da folga) | 401 | `token_expirado` | `WWW-Authenticate: Bearer error="invalid_token", error_description="expirado"` |
| `sub` sem dongle cadastrado | 401 | `token_invalido` | idem |
| Dongle com `ativo = false` | 403 | `dongle_inativo` | — |
| Rota com `{id}` e `sub ≠ {id}` | 403 | `token_outro_dongle` | — |

Na rota de blocos não há `{id}`: o DevId é **sempre** o `sub`, como pede o doc ("DevId identificado pelo token JWT, não pela URL"). Assim, o 403 `token_outro_dongle` só ocorre na confirmação. Se um app enviar blocos de outro dongle com o token certo, eles falham na tag (422), porque o DevId entra no HMAC.

### Transporte e higiene

- Só por HTTPS (Caddy). O app nunca envia o token em HTTP puro; o flavor `dev` sem TLS é a única exceção (spec 09).
- A resposta do handshake leva `Cache-Control: no-store`.
- Tokens nunca vão para logs (o middleware loga só `jti` e `sub`).

## Plano arquivo por arquivo

| Arquivo | Conteúdo |
|---|---|
| `internal/token/emissor.go` (+`_test`) | emissão |
| `internal/token/validador.go` (+`_test`) | validação e cache de chaves |
| `internal/token/erros.go` | `ErrTokenInvalido`, `ErrTokenExpirado` |
| `internal/httpapi/autenticacao.go` (+`_test`) | middleware |

## Testes

1. Ida e volta com `kmsteste` (Ed25519 em memória): emitir → validar ⇒ `Sessao` com `sub`, `cs` e `jti` corretos.
2. Negativos ⇒ 401 `token_invalido`:
   - `alg: none`;
   - `alg: HS256` assinado com a chave pública como segredo HMAC;
   - `kid` desconhecido (após recarga);
   - `iss`/`aud` errados;
   - `sub` com 15 hex;
   - `kid` numérico (`"kid": 1`) e `cs` fracionário (`1.5`) ou maior que 2³²−1;
   - assinatura com 1 bit trocado;
   - token sem `exp`.
3. Expiração: `exp = agora − 31 s` ⇒ `token_expirado`; `exp = agora − 29 s` ⇒ válido (folga).
4. Rotação: as chaves passam a ter `v1` e `v2`; um token `kid=jwt-v2` é válido sem reiniciar (recarga por `kid` desconhecido). Com a rotação automática (spec 07, *Rotação da chave do JWT*): depois que o serviço `jwt-rotacao` eleva `min_decryption_version` para 2, a recarga das chaves públicas deixa de listar a `v1` e um token `kid=jwt-v1` passa a 401 `token_invalido`; a API **nunca** chama `rotate`/`config` (política `deny`).
5. Middleware:
   - sem header ⇒ 401 + `WWW-Authenticate`;
   - dongle inativo ⇒ 403;
   - confirmação com `sub ≠ {id}` ⇒ 403 `token_outro_dongle`.
6. Integração com Vault real: um token assinado pelo Transit valida com a chave pública lida do Transit.

## Passos ordenados com checkpoints

1. `Emissor` e `Validador` com `kmsteste`. **Checkpoint:** testes 1–4.
2. Middleware + rotas protegidas. **Checkpoint:** teste 5.
3. Integração com o Vault. **Checkpoint:** teste 6; o token emitido no handshake real (spec 04) é aceito no `POST /blocos`.

## Critérios de aceite

- [ ] JWT EdDSA com `sub = DevId`, `jti` gravado em `desafio.token_jti` e expiração de 1 h.
- [ ] Validação local, com algoritmo fixo, sem chamada ao Vault por requisição.
- [ ] 401 e 403 exatamente como na tabela acima (= Tabela 5 do doc).

## Riscos

| Risco | Mitigação |
|---|---|
| Relógio do app ou do servidor desalinhado | folga de 30 s no servidor; o app decide renovar pelo `token_validade_s` contado no próprio relógio monotônico a partir do recebimento (spec 09), sem comparar relógios de parede |
| Vault fora no boot ⇒ API sem chave pública | `/saude/pronta` = 503 até carregar; o Compose reinicia |
| Token vazado | dano limitado (só cota); validade de 1 h; desativar o dongle |

## Decisões do autor (antes em aberto)

- **[Decisão] Identidade do usuário do app (conta, frota): fora do escopo do TCC.** O doc menciona "identificar o usuário", mas o TCC não tem contas: o token identifica só o dongle (`sub = DevId`) e o app não ganha tela de conta. Se vier no futuro, usar OIDC no app e um segundo token, nunca misturado ao `sub = DevId`.
- **[Decisão] Rotação da chave do JWT automatizada** (spec 07): job separado da API, com credencial própria; a validação desta spec só precisa recarregar as públicas (já prevista).

## Em aberto

- **[Aberto] Lista de negação por `jti`** (tabela `token_revogado(jti, exp)`). Recomendação: só se surgir um requisito de revogação imediata.
