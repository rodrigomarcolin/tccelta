# 04 — Endpoints do handshake (desafio e resposta)

> Fase 3 · Servidor. Depende de: 02 (tabelas), 03 (protocolo, HTTP, sqlc), 07 (KMS: `HMACAuth`), 08 (emissão do JWT). É dependência de: 05 (estado `c_conf`/`ultimo_ack`), 06 (âncoras, boots pendentes), 09 (app).

## Objetivo

Implementar `POST /v1/dongles/{id}/desafio` e `POST /v1/dongles/{id}/desafio/resposta` exatamente como os diagramas `handshake-servidor-desafio.puml` e `handshake-servidor-resposta.puml` e a seção 6.3 do `.tex`. Isso inclui:

- S1/S2 pelo relógio da API;
- `c_S` estritamente crescente com `UPDATE … RETURNING`;
- consumo atômico do desafio;
- verificação da tag K_auth truncada;
- regra do `TAIL_seq` (OVERFLOW);
- âncora só com S2 − S1 ≤ 2 s;
- `NOTIFY` ao worker;
- emissão do JWT;
- ACK 0x04 assinado.

## Escopo

Dentro: handlers, serviço `handshake`, queries `desafio.sql`, `estado.sql` e `contexto.sql`, análise de concorrência, testes. Fora: algoritmo de tempo (06), detalhes do Vault (07) e do JWT (08).

## Design

### Contrato HTTP

#### `POST /v1/dongles/{id}/desafio`

| Item | Valor |
|---|---|
| `{id}` | DevId, 16 hex |
| Headers de request | nenhum obrigatório; corpo vazio (`Content-Length: 0`); qualquer corpo é ignorado até 0 B (MaxBytes 0 → 413 se vier corpo) |
| 200 | `Content-Type: application/json`, `Cache-Control: no-store` |
| Corpo 200 | `{"desafio":"<base64 do TLV 0x01: 48 ou 50 B>","expira_em":"2026-10-26T14:30:00.000Z"}` |
| 400 | `devid_invalido` |
| 403 | `dongle_inativo`, `perfil_nao_permitido` |
| 404 | `dongle_desconhecido` |
| 429 | `limite_taxa` (DevId/IP) ou `desafios_pendentes` (`Retry-After` = segundos até o pendente mais antigo vencer, no mínimo 1) |
| 503 | `kms_indisponivel`, `banco_ocupado` (`Retry-After`) |

#### `POST /v1/dongles/{id}/desafio/resposta`

| Item | Valor |
|---|---|
| Request | `Content-Type: application/json`; corpo `{"resposta":"<base64 do TLV 0x02: 53 B>"}` (máx. 2 KiB) |
| 200 | `Content-Type: application/json`, `Cache-Control: no-store` |
| Corpo 200 | `{"ack":"<base64 do TLV 0x04: 28 B>","token":"<JWT compacto>","token_expira_em":"<RFC3339>","token_validade_s":3600,"t_lat_s":60}` (`t_lat_s` omitido se `T_LAT_SUGERIDO=0`). `token_validade_s` permite ao app calcular a expiração pelo próprio relógio monotônico (recebimento + validade), sem depender de o relógio do celular estar certo |
| 400 | `corpo_malformado`, `base64_invalido`, `tlv_invalido` (tipo ≠ 0x02 ou `L` ≠ 50) |
| 403/404 | como no desafio |
| 409 | `desafio_invalido` (c_S desconhecido para o dongle, nonce diferente, vencido ou já usado); `c_s_antigo` (`c_S ≤ c_conf`) |
| 422 | `tag_invalida`; `resposta_inconsistente` (`TAIL_seq > HEAD_seq + 1` ou `HEAD_seq <` salvo) |
| 429 | `limite_taxa` |
| 503 | `kms_indisponivel`, `banco_ocupado` |

O handshake "aceito sem âncora" (S2 − S1 > 2 s) responde **200** normalmente, com `flags_a` bit 0 = 1 no ACK. O app lê esse bit e repete o handshake (spec 09).

### Serviço (`internal/handshake`)

```go
package handshake

type Servico struct {
    pool    *pgxpool.Pool
    kms     kms.Autenticador        // HMACAuth (spec 07)
    tokens  *token.Emissor          // Emitir (spec 08)
    agora   func() time.Time        // relógio injetável (testes)
    rand    io.Reader               // crypto/rand.Reader (testes: determinístico)
    cfg     config.ProtocoloConfig
    log     *slog.Logger
    met     *metricas.Handshake
}

type DesafioEmitido struct {
    TLV      []byte    // T|L|V, 48 ou 50 B
    ExpiraEm time.Time
}
func (s *Servico) EmitirDesafio(ctx context.Context, d dongles.Dongle) (DesafioEmitido, error)

type ResultadoHandshake struct {
    AckTLV         []byte // 28 B
    Token          string
    TokenExpiraEm  time.Time
    TLatSugerido   time.Duration
    ComAncora      bool
    AckSeq         uint32
}
// s2 vem do middleware anotarS2 (instante de chegada, antes de ler o corpo).
func (s *Servico) ProcessarResposta(ctx context.Context, d dongles.Dongle, s2 time.Time, tlv []byte) (ResultadoHandshake, error)

// Erros tipados → HTTP (httpapi/handshake.go)
var (
    ErrDesafiosPendentes   = errors.New("desafios_pendentes")      // 429
    ErrDesafioInvalido     = errors.New("desafio_invalido")        // 409
    ErrCSAntigo            = errors.New("c_s_antigo")              // 409
    ErrTagInvalida         = errors.New("tag_invalida")            // 422
    ErrRespostaInconsist   = errors.New("resposta_inconsistente")  // 422
)
// kms.ErrIndisponivel e banco.ErrBancoOcupado → 503
```

Relógio: `agora()` é `time.Now()`. Ele usa o relógio de parede do host (NTP), e o Go mantém a leitura monotônica para diferenças. **S1 arredonda para baixo e S2 para cima no microssegundo** (`truncarMicro`/`tetoMicro`), para o intervalo `[S1, S2]` gravado em `timestamptz` não encolher. O RTT `S2 − S1` é calculado com os valores já arredondados.

### Fluxo: `EmitirDesafio`

Segue o `handshake-servidor-desafio.puml`:

1. Middlewares: `limitarIP` → `limitarDevId(LIMITE_DESAFIO_MIN)` → 429.
2. `dongles.Buscar(DevId)`: desconhecido → 404; `ativo=false` → 403; perfil `none` sem `PERMITIR_PERFIL_NONE` → 403.
3. `N_S`: 16 bytes de `crypto/rand`.
4. `flags_c = 0`, sem `diag_min` (**[Decisão]** emissão do modo diagnóstico adiada; ver *Decisões do autor* no fim).
5. **S1** = `truncarMicro(agora())`, **antes** do `INSERT`.
6. Executar `EmitirDesafio` (query abaixo), fora de transação explícita: é um único comando atômico. 0 linhas → `ErrDesafiosPendentes`.
7. Montar o payload do 0x01 (`c_S`, `N_S`, `ACK_seq = seq_contiguo`, `c_conf`, `flags_c`) e calcular `tag = HMACAuth(DevId, EntradaTag(0x01, DevId, payload))[:16]`. Falha do KMS → 503. O desafio fica no banco e vence sozinho.
8. Responder `{"desafio": base64(TLV), "expira_em": S1 + DESAFIO_VALIDADE}`.

**[Decisão] `DESAFIO_VALIDADE = 60 s`.** Cobre o caminho app → BLE → dongle → BLE → app com folga, inclusive com reconexão BLE lenta. A âncora tem seu próprio limite de 2 s, então um prazo maior não piora o tempo: só permite um handshake sem âncora em vez de um 409.

**[Decisão] `DESAFIO_MAX_PENDENTES = 3`** desafios não respondidos e não vencidos por dongle, além do rate limit de 6/min por DevId. Isso impede que um app malicioso encha a tabela `desafio` (doc, passo 2), sem bloquear a repetição legítima de um handshake.

### Fluxo: `ProcessarResposta`

Segue o `handshake-servidor-resposta.puml`, com dois ajustes justificados abaixo (pré-checagem barata antes do KMS e checagem de consistência sob o lock).

```
0. S2 = tetoMicro(agora())                         ← middleware anotarS2, antes de ler o corpo
1. limitarIP, limitarDevId(LIMITE_RESPOSTA_MIN)     → 429
2. dongle (cache)                                   → 404 / 403
3. JSON → base64 → LerTLV(T=0x02) → DecodificarResposta(V, perfil)   → 400
4. pré-checagem SEM lock:
     d := BuscarDesafio(dongle_id, r.CS)            → ausente: 409 desafio_invalido
     d.nonce ≠ r.NS | d.respondido_em ≠ NULL | d.expira_em ≤ S2      → 409 desafio_invalido
     e := LerEstado(dongle_id); r.CS ≤ e.c_conf     → 409 c_s_antigo
5. tagCalc := HMACAuth(DevId, EntradaTag(0x02, DevId, payload))      → falha KMS: 503
   !CompararTag(tagCalc[:16], tag)                  → MarcarTagInvalida; 422 tag_invalida
6. jwt, jti, exp := tokens.Emitir(ctx, DevId, r.CS) → falha KMS: 503
7. TRANSAÇÃO (lock_timeout 3 s, statement_timeout 10 s):
     a. TravarDongle(dongle_id)                     ← pg_advisory_xact_lock(1, dongle_id)
     b. s1, ok := ConsumirDesafio(dongle_id, c_s, nonce, S2, boot_id, t_ms, head, tail)
        !ok                                         → ROLLBACK; 409 desafio_invalido (corrida)
     c. e := LerEstado(dongle_id)                   ← autoritativo, sob o lock
        r.CS ≤ e.c_conf                             → ROLLBACK; 409 c_s_antigo
     d. r.TailSeq > r.HeadSeq + 1  OU  r.HeadSeq < e.head_seq
                                                    → FinalizarDesafio(resultado='inconsistente'); COMMIT; 422
     e. ack := e.seq_contiguo; perda := nil
        se r.TailSeq > ack + 1:                      ← regra do TAIL_seq (OVERFLOW)
             InserirLacuna(OVERFLOW, from=ack+1, to=r.TailSeq−1, c_s=r.CS, registrado_em=S2)
             perda := [ack+1, r.TailSeq−1]; ack := r.TailSeq − 1
     f. rtt := S2 − s1
        comAncora := 0 ≤ rtt ≤ ANCORA_RTT_MAX
        rtt < 0 → InserirAlerta(relogio_servidor_regrediu)
     g. ger := geracaoDoHandshake(e, r.BootID)                   ← função pura (regras.go), sem tocar o estado
        ref := LerTMsUltimoBoots(dongle_id, [r.BootID])          ← sob o lock (query da spec 05); usa a geração `ger`
        tMs := tempo.Desembrulhar(r.TMs, ref)                   ← mesma referência por boot da ingestão
        GarantirBootPendente(dongle_id, ger, r.BootID, agora)
        se comAncora: InserirAncora(dongle_id, ger, boot_id, tMs, nonce, s1, S2)
        (desafio.t_ms e ancora_tempo.t_ms guardam o valor DESEMBRULHADO, igual a leitura.t_ms;
         desafio.geracao_boot guarda `ger`)
     h. ackMsg := Ack{AckSeq: ack, CConf: r.CS, Flags: comAncora ? 0 : FlagAckAncoraFalhou}
        tagAck := HMACAuth(DevId, EntradaTag(0x04, DevId, ackMsg.Payload()))[:16]   → falha: ROLLBACK; 503
        ackTLV := ackMsg.Montar(tagAck)
     i. AtualizarEstadoHandshake(seq_contiguo=ack, head_seq=r.HeadSeq, tail_seq=r.TailSeq,
                                 head_relatado_em=S2, c_conf=r.CS, ultimo_ack=ackTLV)
     j. FinalizarDesafio(resultado = comAncora ? 'aceito' : 'aceito_sem_ancora',
                         ack_seq=ack, perda_ini, perda_fim, token_jti=jti)
     k. NotificarBootPendente('<dongle_id>:<geracao_boot>:<boot_id>')   ← entregue no COMMIT
   COMMIT
8. 200 {"ack": b64(ackTLV), "token": jwt, "token_expira_em": exp, "t_lat_s": ...}
```

Notas:

- **[Decisão] Pré-checagem (passo 4) antes do KMS.** O endpoint não é autenticado. Sem a pré-checagem, qualquer um faria a API chamar o Vault com lixo. Com ela, só uma resposta para um desafio que existe, está pendente e tem o nonce certo custa uma chamada ao Vault. A checagem autoritativa continua sendo o `UPDATE` atômico do passo 7b.
- **[Decisão] Consistência (7d) sob o lock, não antes.** O `.tex` lista a consistência antes da transação. Mas `head_seq` pode subir por ingestão concorrente (spec 05), então a comparação só é correta sob `pg_advisory_xact_lock`. No caso inconsistente, a transação **faz commit** do consumo com `resultado='inconsistente'`: o desafio fica usado e auditável, como no doc ("marca como inconsistente e responde 422").
- **Tag inválida não consome o desafio.** `MarcarTagInvalida` só incrementa `tentativas_tag_invalida` e marca `resultado='tag_invalida'` enquanto pendente. Uma resposta legítima que chegue depois ainda consome o desafio (e sobrescreve `resultado`). Assim, um intermediário que injete lixo não consegue "queimar" o desafio do dongle verdadeiro.
- **[Decisão, revisão] `t_ms` desembrulhado também no handshake.** As âncoras e as leituras precisam estar na mesma escala de `t_ms` (u32 com volta a cada 49,7 dias). Senão, a reta calculada a partir das âncoras não bate com as leituras depois de uma volta. Por isso o passo 7g lê a referência do boot sob o lock e grava o valor desembrulhado. Consequência: `ConsumirDesafio` (7b) grava o `t_ms` cru, e o 7j o corrige com o desembrulhado.
- **[Decisão, revisão] Verificação da tag (passo 5) e assinatura do JWT (passo 6) em série, não em paralelo.** O endpoint é anônimo. Em paralelo, cada resposta com tag falsa para um desafio pendente (o nonce é visível para o app) custaria uma assinatura no Vault, o que amplificaria um DoS. O handshake é raro, então os poucos ms a mais não importam.
- **[Decisão] JWT assinado antes da transação (passo 6).** O JWT não depende do resultado da transação: o `jti` é aleatório, e `cs` e `exp` já são conhecidos. Assinar fora encurta o tempo com o lock do dongle e com uma conexão do pool presa. Se a transação falhar ou der 409/422, o JWT é descartado sem ter sido entregue. O ACK, por outro lado, precisa ser assinado **dentro** da transação, porque `ACK_seq` só é conhecido ali e o ACK assinado é gravado em `ultimo_ack` atomicamente com o estado.
- **Âncora quando `rtt > 2 s`:** o handshake segue sem âncora (`aceito_sem_ancora`), o boot é marcado pendente (para estimativa por vizinhos) e o ACK leva `flags_a` bit 0. Exatamente como no doc.
- **`HEAD_seq` nunca diminui:** verificado em 7d e reforçado pela trigger `dongle_estado_monotono` (spec 02).
- **[Decisão] Geração do `boot_id` no handshake (passo 7g).** O `boot` é identificado por `(dongle_id, geracao_boot, boot_id)` (spec 02). A resposta traz só o `boot_id` corrente, e o bloco BOOT dele pode ainda não ter chegado, então a geração é *inferida sem gravar o estado*: `geracaoDoHandshake(e, bootID)` devolve `e.geracao_boot` se `bootID ≥ e.ultimo_boot_id` (ou `bootID = 0`), e `e.geracao_boot + 1` se `bootID < e.ultimo_boot_id` (o u16 deu a volta). Um segundo handshake do mesmo boot dá o mesmo resultado, e quando o primeiro bloco do boot é aceito a ingestão (spec 05, `atribuirGeracao`) chega à mesma geração e só então grava `dongle_estado.geracao_boot`. Alerta `geracao_boot_avancou` é gerado pela ingestão, não aqui.
- **[Decisão, reconciliação com a Fase 1] `c_S = max(ultimo_c_s + 1, segundos Unix agora)`.** O dongle persiste o maior `c_S` aceito (`last_cs`) e recusa desafios com `c_S` menor ou igual (Erro 0x02). O servidor de bancada embutido no app (Fase 1, spec 09 §7) emite `c_S` a partir da época Unix (~1,7·10⁹), então uma placa que passou pela bancada recusaria para sempre um servidor real que começasse em 1. Ancorar `c_S` na época (cabe em `uint32` até 2106) mantém o contador estritamente crescente, cobre placas reaproveitadas e não muda nada no dongle, que só compara. O `max` preserva a monotonicidade se o relógio da API recuar (NTP). A trigger `dongle_estado_monotono` continua valendo.

### Queries (`internal/banco/queries/desafio.sql`, `estado.sql`, `contexto.sql`)

```sql
-- name: EmitirDesafio :one
-- Incrementa c_S (lock de linha curto em dongle_estado) e insere o desafio,
-- só se o dongle tiver menos de @max_pendentes desafios pendentes no prazo.
WITH cs AS (
  UPDATE dongle_estado e
     SET ultimo_c_s = GREATEST(e.ultimo_c_s + 1, @agora_s::bigint)   -- @agora_s = segundos Unix do relógio da API
   WHERE e.dongle_id = @dongle_id
     AND GREATEST(e.ultimo_c_s + 1, @agora_s::bigint) <= 4294967295
     AND (SELECT count(*) FROM desafio d
           WHERE d.dongle_id = @dongle_id
             AND d.respondido_em IS NULL
             AND d.expira_em > @s1) < @max_pendentes::int
  RETURNING e.ultimo_c_s AS c_s, e.seq_contiguo, e.c_conf
), ins AS (
  INSERT INTO desafio (dongle_id, c_s, nonce, s1, expira_em, ack_seq_emitido, c_conf_emitido, flags_c, diag_min)
  SELECT @dongle_id, cs.c_s, @nonce, @s1, @expira_em, cs.seq_contiguo, cs.c_conf, @flags_c, @diag_min
  FROM cs
  RETURNING c_s
)
SELECT cs.c_s, cs.seq_contiguo, cs.c_conf FROM cs JOIN ins USING (c_s);

-- name: BuscarDesafio :one
SELECT nonce, s1, expira_em, respondido_em FROM desafio
WHERE dongle_id = @dongle_id AND c_s = @c_s;

-- name: MarcarTagInvalida :exec
UPDATE desafio
   SET tentativas_tag_invalida = tentativas_tag_invalida + 1,
       resultado = COALESCE(resultado, 'tag_invalida')
 WHERE dongle_id = @dongle_id AND c_s = @c_s AND nonce = @nonce AND respondido_em IS NULL;

-- name: ConsumirDesafio :one
-- Uso único: pertence ao dongle, nonce confere, pendente e no prazo. Duas
-- respostas simultâneas: só uma recebe a linha.
UPDATE desafio
   SET respondido_em = @s2, boot_id = @boot_id, t_ms = @t_ms,
       head_seq = @head_seq, tail_seq = @tail_seq
 WHERE dongle_id = @dongle_id AND c_s = @c_s AND nonce = @nonce
   AND respondido_em IS NULL AND expira_em > @s2
RETURNING s1;

-- name: FinalizarDesafio :exec
UPDATE desafio
   SET resultado = @resultado, ack_seq = @ack_seq, perda_ini = @perda_ini,
       perda_fim = @perda_fim, token_jti = @token_jti, t_ms = COALESCE(@t_ms_desembrulhado, t_ms),
       geracao_boot = COALESCE(@geracao_boot, geracao_boot)
 WHERE dongle_id = @dongle_id AND c_s = @c_s;

-- name: ExpirarDesafios :execrows
-- Job periódico do worker (spec 06).
UPDATE desafio SET resultado = 'expirado'
 WHERE respondido_em IS NULL AND resultado IS NULL AND expira_em < @agora;
```

```sql
-- estado.sql
-- name: TravarDongle :exec
SELECT pg_advisory_xact_lock(1, @dongle_id::int);

-- name: LerEstado :one
SELECT seq_contiguo, maior_seq_visto, head_seq, tail_seq, ultimo_c_s, c_conf, geracao_boot, ultimo_boot_id
FROM dongle_estado WHERE dongle_id = @dongle_id;

-- name: AtualizarEstadoHandshake :exec
UPDATE dongle_estado SET
  seq_contiguo     = @seq_contiguo,
  maior_seq_visto  = GREATEST(maior_seq_visto, @seq_contiguo),
  head_seq         = @head_seq,
  tail_seq         = @tail_seq,
  head_relatado_em = @s2,
  c_conf           = @c_conf,
  ultimo_ack       = @ultimo_ack,
  ultimo_ack_em    = @agora
WHERE dongle_id = @dongle_id;

-- name: LerUltimoAck :one
SELECT ultimo_ack FROM dongle_estado WHERE dongle_id = @dongle_id;
```

```sql
-- contexto.sql
-- name: InserirLacuna :exec
INSERT INTO lacuna (dongle_id, from_seq, to_seq, causa, registrado_em, c_s, seq_bloco)
VALUES (@dongle_id, @from_seq, @to_seq, @causa, @registrado_em, @c_s, @seq_bloco)
ON CONFLICT (dongle_id, from_seq) DO NOTHING;

-- name: GarantirBootPendente :exec
INSERT INTO boot (dongle_id, geracao_boot, boot_id, pendente_tempo, pendente_desde)
VALUES (@dongle_id, @geracao_boot, @boot_id, true, @agora)
ON CONFLICT (dongle_id, geracao_boot, boot_id) DO UPDATE
   SET pendente_tempo = true, pendente_desde = EXCLUDED.pendente_desde
 WHERE NOT boot.pendente_tempo;     -- não regrava se já pendente (evita churn)

-- name: InserirAncora :exec
INSERT INTO ancora_tempo (dongle_id, geracao_boot, boot_id, t_ms, nonce, s1, s2)
VALUES (@dongle_id, @geracao_boot, @boot_id, @t_ms, @nonce, @s1, @s2)
ON CONFLICT DO NOTHING;

-- name: InserirAlerta :exec
INSERT INTO alerta (dongle_id, geracao_boot, boot_id, tipo, detalhe)
VALUES (@dongle_id, @geracao_boot, @boot_id, @tipo, @detalhe);

-- name: NotificarBootPendente :exec
SELECT pg_notify('boot_pendente', @carga::text);
```

### Concorrência

| Situação | O que garante a corretude |
|---|---|
| Dois `POST /desafio` simultâneos do mesmo dongle | O `UPDATE dongle_estado` pega lock de linha: os dois são serializados e recebem `c_S` distintos e crescentes. O limite de pendentes é "macio" (a contagem pode deixar passar 1 a mais na corrida, sem efeito de segurança). |
| Duas respostas ao mesmo desafio (app reenviou, ou replay) | As duas podem passar da pré-checagem e do KMS. Na transação, o advisory lock serializa; a primeira consome no `UPDATE … RETURNING`, e a segunda recebe 0 linhas → 409. Só um `jti` é gravado; o JWT da segunda é descartado. |
| Resposta enquanto um lote do mesmo dongle é gravado | O mesmo `pg_advisory_xact_lock(1, dongle_id)` serializa. A regra do TAIL_seq e o ACK veem o `seq_contiguo` já commitado. |
| Resposta de um `c_S` mais velho depois de um mais novo aceito | `c_S ≤ c_conf` → 409 `c_s_antigo` (checado de novo sob o lock). |
| `POST /desafio` durante um lote | O `UPDATE` do desafio espera o lock de linha do `dongle_estado`, que o lote só pega no fim da transação. A espera é curta e limitada pelo `statement_timeout`. O `ACK_seq` do desafio sai sempre de um estado commitado. |
| Resposta × worker no mesmo `boot` | `GarantirBootPendente` pode esperar o `FOR UPDATE` do worker (ms). O worker nunca pega outro lock de `boot` com o primeiro preso (spec 06), então não há ciclo. |
| Ordem dos locks | resposta: advisory → `desafio` (linha X) → `dongle_estado` → `lacuna`/`boot`/`ancora_tempo`. Emissão: `dongle_estado` → `INSERT desafio` (linha nova Y). Não existe ordem inversa entre as mesmas linhas, então não há deadlock. A transação do worker que marca vizinhos (spec 06, TX2) toma o mesmo advisory lock antes de tocar vários `boot`. Um `40P01` residual é repetido uma vez por `EmTransacao` e, se repetir, vira 503. |

### Handler (`internal/httpapi/handshake.go`)

```go
func (h *Handlers) PedirDesafio(w http.ResponseWriter, r *http.Request)    // POST /v1/dongles/{id}/desafio
func (h *Handlers) ResponderDesafio(w http.ResponseWriter, r *http.Request) // POST /v1/dongles/{id}/desafio/resposta

type corpoResposta struct{ Resposta string `json:"resposta"` }
type saidaDesafio struct {
    Desafio  string    `json:"desafio"`
    ExpiraEm time.Time `json:"expira_em"`
}
type saidaHandshake struct {
    Ack           string    `json:"ack"`
    Token         string    `json:"token"`
    TokenExpiraEm time.Time `json:"token_expira_em"`
    TokenValidadeS int      `json:"token_validade_s"`
    TLatS         *int      `json:"t_lat_s,omitempty"`
}
```

O decodificador JSON usa `DisallowUnknownFields` e no máximo 2 KiB. O base64 usa `base64.StdEncoding.Strict()`.

### Logs e métricas

- `handshake.desafio_emitido{dongle, c_s, ack_seq}`
- `handshake.tag_invalida{dongle, c_s}` (nível `warn`)
- `handshake.aceito{dongle, c_s, geracao_boot, boot_id, rtt_ms, com_ancora, ack_seq, overflow_ini, overflow_fim}`
- `handshake.inconsistente{dongle, c_s, head, tail, head_salvo}`
- Métricas: `tccelta_handshakes_total{resultado}` e `tccelta_ancora_rtt_segundos`.

## Plano arquivo por arquivo

| Arquivo | Conteúdo |
|---|---|
| `internal/banco/queries/desafio.sql`, `estado.sql`, `contexto.sql` | queries acima |
| `internal/handshake/servico.go` | `Servico`, `EmitirDesafio`, `ProcessarResposta` |
| `internal/handshake/regras.go` | funções puras: `aplicarTail(e Estado, r Resposta) (ack uint32, perda *Faixa, err error)`, `avaliarAncora(s1, s2, max) (bool, alerta)`, `geracaoDoHandshake(e Estado, bootID uint16) uint16` |
| `internal/handshake/relogio.go` | `truncarMicro`, `tetoMicro` |
| `internal/handshake/regras_test.go` | tabela de casos da regra do TAIL e da âncora |
| `internal/handshake/servico_integracao_test.go` (`integracao`) | banco real + `kmsteste` |
| `internal/httpapi/handshake.go` (+`_test`) | handlers |

## Testes

**Unitários (puros), `regras_test.go`.** Na notação abaixo, ack = `seq_contiguo`; a saída é `(ACK_seq, OVERFLOW?, erro)`.

| Caso | e.seq_contiguo | e.head | TAIL | HEAD | Esperado |
|---|---|---|---|---|---|
| sem descarte | 100 | 120 | 90 | 130 | (100, —, ok) |
| tail = ack+1 | 100 | 120 | 101 | 130 | (100, —, ok) |
| overflow | 100 | 120 | 111 | 130 | (110, [101,110], ok) |
| flash vazia | 130 | 130 | 131 | 130 | (130, —, ok) |
| overflow total | 100 | 120 | 131 | 130 | (130, [101,130], ok) |
| tail > head+1 | 100 | 120 | 132 | 130 | inconsistente |
| head regrediu | 100 | 120 | 101 | 119 | inconsistente |
| dongle novo | 0 | 0 | 1 | 0 | (0, —, ok) |
| dongle novo com overflow | 0 | 0 | 50 | 80 | (49, [1,49], ok) |

Âncora: rtt = 0 → âncora; 2,000000 s → âncora; 2,000001 s → sem âncora + flag; −5 ms → sem âncora + alerta.

Geração (`geracaoDoHandshake`): `(G=0, U=7, boot=7)` → 0; `(0, 7, 9)` → 0; `(0, 7, 3)` → 1 (volta do u16); `(2, 65535, 1)` → 3; `(0, 0, 1)` (dongle novo) → 0; `bootID = 0` → G.

**Integração (Postgres real via testcontainers + `kmsteste` com chaves conhecidas):**

1. Fluxo feliz: desafio → montar a resposta com K_auth de teste → 200. Verificar:
   - ACK decodificável, com a tag conferida pela chave de teste;
   - `desafio.resultado='aceito'` e `token_jti` igual ao `jti` do JWT;
   - uma linha em `ancora_tempo`;
   - `boot.pendente_tempo = true`;
   - `NOTIFY` recebido por um `LISTEN boot_pendente`;
   - `dongle_estado.c_conf = c_S` e `ultimo_ack` = TLV devolvido.
2. `c_S` crescente: 50 `POST /desafio` concorrentes (com limite de pendentes alto no teste) dão 50 `c_S` distintos e consecutivos.
3. Uso único: a mesma resposta enviada 2× em paralelo → exatamente um 200 e um 409.
4. Vencido: relógio injetado avança 61 s → 409.
5. Tag inválida (1 bit trocado) → 422; `tentativas_tag_invalida = 1`; a resposta correta depois → 200.
6. `c_s_antigo`: aceitar `c_S=6` e depois responder ao `c_S=5` pendente → 409.
7. Inconsistente → 422, `resultado='inconsistente'`, desafio consumido, estado inalterado.
8. Overflow → linha em `lacuna` (OVERFLOW, `c_s`), `seq_contiguo` avançado, `desafio.perda_ini/fim`.
9. RTT > 2 s (relógio injetado) → 200, `flags_a=1`, `aceito_sem_ancora`, sem `ancora_tempo`, boot pendente.
10. Pendentes: 4º desafio sem responder → 429 `desafios_pendentes` com `Retry-After`.
11. KMS fora (dublê devolvendo erro) no ACK → 503 e nada gravado (desafio continua pendente).
12. Resposta concorrente com um lote (spec 05) do mesmo dongle: as transações ficam serializadas (o teste verifica a ordem pelos timestamps de log e o estado final consistente).
13. Volta do `boot_id`: `dongle_estado` com `ultimo_boot_id = 65535`, `geracao_boot = 0`; handshake com `boot_id = 1` ⇒ `boot (geracao 1, boot 1)` criado, `desafio.geracao_boot = 1`, âncora na geração 1 e `dongle_estado.geracao_boot` ainda 0 (só a ingestão avança); repetir o handshake ⇒ a mesma linha.

## Passos ordenados com checkpoints

1. Queries + `make sqlc`. **Checkpoint:** compila.
2. `regras.go` + testes unitários. **Checkpoint:** tabela verde.
3. `EmitirDesafio` + handler. **Checkpoint:** com o stack dev e `seed-dev`, `curl -X POST .../desafio` devolve JSON cujo TLV decodifica com tag correta (verificado pelo simulador, `simulador verificar-desafio`).
4. `ProcessarResposta` + handler. **Checkpoint:** o simulador (spec 10) completa um handshake pelo HTTP.
5. Testes de integração 1–12. **Checkpoint:** verdes com `-race`.

## Critérios de aceite

- [ ] Os passos do `.tex` §6.3 e dos dois diagramas são implementados na ordem descrita, com os desvios justificados acima.
- [ ] S1 e S2 vêm do relógio da API, e S2 é anotado antes da leitura do corpo.
- [ ] `c_S` é estritamente crescente sob concorrência, e cada desafio é usado no máximo uma vez.
- [ ] O ACK do handshake e o `ultimo_ack` gravado são idênticos byte a byte.
- [ ] 409, 422 e 429 conforme o contrato; 503 com `Retry-After` quando o KMS ou o banco falham.

## Riscos

| Risco | Mitigação |
|---|---|
| Latência do Vault entre S1 e a saída do desafio alarga a âncora | ~1 ms local, ~20 ms via VPN, muito abaixo de 2 s; aceitável e conservador (nunca errado) |
| Dongle com contadores zerados (NVS/flash apagadas) passa a relatar `HEAD_seq` menor ⇒ 422 `resposta_inconsistente` para sempre | correto pelo doc (`HEAD_seq` nunca diminui). Recuperação documentada no README: dongle com contadores zerados = **novo DevId** (`provisionar novo`) |
| Salto do relógio do servidor (NTP) entre S1 e S2 | âncora descartada se rtt < 0; interseção vazia detectada pelo worker (spec 06) |
| DoS de desafios para um DevId (pendentes cheios) bloqueia o app legítimo por até 60 s | rate limit por IP; o DevId só circula no BLE (LESC) e no app; risco aceito e registrado |
| JWT assinado e não entregue (corrida 409) | inofensivo: `jti` não gravado, token nunca sai da API |

## Decisões do autor (antes em aberto)

- **[Decisão] Emissão do modo diagnóstico (`flags_c` bit 0 + `diag_min`): adiada, formato pronto e sem efeito.** O formato é suportado ponta a ponta (`protocolo.Desafio.DiagMin`, colunas `desafio.flags_c`/`diag_min`, `dongle.diag_solicitado_min`), mas o servidor emite **sempre** `flags_c = 0`, e o firmware (Fase 1, spec 05 §11) mantém o gancho `IDiagGate` vazio, com `AT#DIAG 1` sempre aceito. Fica descrito para depois: quando `dongle.diag_solicitado_min` não for nulo, o próximo desafio levaria bit 0 + `diag_min`, e o valor seria zerado quando um handshake com esse `c_S` fosse aceito; o período viraria "intervalo legítimo" nas análises (o `token-diagnostico` do `server-specification.md` também fica para depois).
- **[Decisão] `T_lat` por dongle: coluna `dongle.t_lat_s` criada já (spec 02), mas o servidor não a usa.** O handshake continua devolvendo o `T_LAT_SUGERIDO` global (`t_lat_s` omitido se `0`) e não consulta a coluna; ela fica reservada, preenchida só à mão. O app já trata o valor sugerido como teto, `min(local, servidor)` (Fase 1, spec 07 §7).
