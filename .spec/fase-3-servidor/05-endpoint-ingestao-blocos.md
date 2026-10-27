# 05 — Ingestão de blocos e confirmação

> Fase 3 · Servidor. Depende de: 02, 03, 04 (estado `c_conf` e `ultimo_ack`), 07 (`VerificarBlocos`, `HMACAuth`), 08 (middleware JWT). É dependência de: 06 (boots pendentes), 09 (app), 10 (carga).

## Objetivo

Implementar `POST /v1/dongles/blocos` conforme `ingestao-lote-servidor.puml` e a seção 6.2 do `.tex`:

- JWT obrigatório; o DevId vem do token, nunca da URL;
- até 32 blocos por lote;
- tags K_rec verificadas no KMS antes de qualquer escrita;
- Go-Back-N estrito (`seq = seq_contiguo + 1`), com as exceções dos marcadores BOOT e LOSS;
- uma transação por lote com `pg_advisory_xact_lock`, `ON CONFLICT DO NOTHING` e `COPY`;
- decodificação para `leitura` e `posicao`;
- atualização de `dongle_estado`, marcação dos boots pendentes e resposta com ACK 0x04 assinado.

Implementar também `GET /v1/dongles/{id}/confirmacao`.

## Escopo

Dentro: handler, serviço `ingestao`, decodificador guiado pela tabela `pid`, queries `blocos.sql`, análise de custo do KMS, concorrência, testes. Fora: worker (06), detalhes de Vault (07) e de JWT (08).

## Design

### Contrato HTTP

#### `POST /v1/dongles/blocos`

| Item | Valor |
|---|---|
| Headers de request | `Authorization: Bearer <JWT>` (obrigatório); `Content-Type: application/octet-stream` |
| Corpo | 1 a 32 TLVs `0x03` concatenados, em ordem crescente de `seq` (como o app os leu do dongle), sem separadores. Tamanho máximo `32 × 1059 + 64` B |
| 200 | `Content-Type: application/octet-stream`; corpo = TLV 0x04 (28 B: `04 19 00` + 25 B) |
| Headers de resposta | `X-Lote-Status: completo \| interrompido`; `X-Lote-Motivo: tag_invalida \| salto \| marcador_invalido \| pid_desconhecido \| registro_malformado` (ausente se completo); `X-Lote-Acao: nenhuma \| reler \| aguardar`; `X-Lote-Aceitos: n`; `X-Lote-Duplicados: n` |
| 400 | `lote_malformado` (TLV truncado, tipo ≠ 0x03, `L ≠ tamanho + 32`, `tamanho` fora de 17..1024, corpo vazio). Nada é processado |
| 401 / 403 | spec 08 (`token_*`, `dongle_inativo`) |
| 413 / 415 | corpo grande / content-type errado |
| 422 | `lote_sem_progresso`: nenhum bloco avançou o ACK **e** o motivo é `tag_invalida`. O corpo continua sendo o TLV 0x04 atual (`Content-Type: application/octet-stream`, diferente de todos os outros 422, que são `problem+json`), com os mesmos headers `X-Lote-*`. O app decide "o corpo é ACK" pelo `Content-Type`, não pelo status |
| 429 | `lote_grande` (mais de 32 TLVs; `Retry-After: 1`) ou `limite_taxa` |
| 503 | `sobrecarga` (semáforo), `kms_indisponivel`, `banco_ocupado`; sempre com `Retry-After` |

**[Decisão] Lote interrompido responde 200 com ACK (Tabela 5), e 422 só quando não houve progresso por tag inválida.** O `.tex` lista "422 para assinatura inválida" de forma genérica, e a Tabela 5 (o que o app faz) define "200 com ACK e `ACK_seq` < último seq do lote" para tag inválida, salto ou fora de ordem. Conciliação: se algo avançou, 200 (o app precisa gravar o ACK no dongle); se nada avançou por tag inválida, 422 com o ACK no corpo. Nos dois casos o app trata o corpo como ACK e segue `X-Lote-Acao`.

**[Decisão] `X-Lote-Acao`** diz ao app o que fazer com o buffer, sem ele precisar entender a causa:

| Motivo | Ação | Por quê |
|---|---|---|
| — (completo) | `nenhuma` | remove do buffer até `ACK_seq` |
| `tag_invalida` | `reler` | os bytes do buffer podem estar corrompidos. O app descarta o buffer acima de `ACK_seq` e envia o Erro 0x07 ao dongle (cursor volta para `ACK_seq + 1`), como a seta "Erro 0x07" do `ingestao-lote-servidor.puml` |
| `salto` | `reler` | falta no buffer o bloco `ACK_seq + 1`; reenviar o mesmo buffer não resolve |
| `marcador_invalido` | `reler` | BOOT/LOSS fora da regra indica buffer dessincronizado. Se repetir 3× com o mesmo ACK, o app faz handshake (spec 09) |
| `pid_desconhecido`, `registro_malformado` | `aguardar` | o bloco é autêntico; reler dá os mesmos bytes. O app mantém o buffer e reenvia com backoff. O operador cadastra o PID (`INSERT INTO pid`, recarregado sem deploy); para bloco malformado não há saída manual (ver *Fila travada*, abaixo): corrige-se o firmware do dongle |

#### `GET /v1/dongles/{id}/confirmacao`

| Item | Valor |
|---|---|
| Headers | `Authorization: Bearer <JWT>` com `sub = {id}` (senão 403 `token_outro_dongle`) |
| 200 | `application/octet-stream`, TLV 0x04 = `dongle_estado.ultimo_ack`, sem chamar o KMS |
| 404 | `sem_confirmacao` (nunca houve handshake aceito) |
| 429 | 60/min por DevId |

**[Decisão] A confirmação exige JWT.** O caso de uso é "app que perdeu o último ACK antes de escrevê-lo no dongle", e esse app ainda tem um token válido. Sem token, o app precisa de handshake de qualquer forma, e o handshake devolve um ACK. Exigir o token mantém o 403 da Tabela 5 com significado e evita um oráculo anônimo de estado do dongle.

### Serviço (`internal/ingestao`)

```go
package ingestao

type Servico struct {
    pool   *pgxpool.Pool
    kms    kms.Autenticador
    pids   *pids.Catalogo
    agora  func() time.Time
    cfg    Config      // LoteMax, VerifyLote, VerifyParalelo
    log    *slog.Logger
    met    *metricas.Ingestao
}

type Status string   // "completo" | "interrompido"
type Motivo string   // "", "tag_invalida", "salto", "marcador_invalido", "pid_desconhecido", "registro_malformado"
type Acao string     // "nenhuma" | "reler" | "aguardar"

type ResultadoLote struct {
    AckTLV      []byte
    AckSeq      uint32
    Status      Status
    Motivo      Motivo
    Acao        Acao
    SemProgresso bool  // → 422 quando Motivo == tag_invalida
    Aceitos, Duplicados int
}

// sessao vem do JWT validado (sub → dongle; cs → epoca; jti → auditoria de rejeitados).
func (s *Servico) ProcessarLote(ctx context.Context, d dongles.Dongle, sessao token.Sessao, corpo []byte) (ResultadoLote, error)
func (s *Servico) UltimaConfirmacao(ctx context.Context, d dongles.Dongle) ([]byte, error) // ErrSemConfirmacao
```

### Algoritmo do lote

```
1. FORMA (sem I/O)
   tlvs := protocolo.LerLote(corpo, LOTE_MAX_BLOCOS)      → ErrLoteGrande: 429; demais: 400
   para cada TLV: T == 0x03 e DecodificarBloco(V, perfil) ok   → senão 400 (lote inteiro)

2. SNAPSHOT (fora da transação, sem lock)
   snap := LerSnapshotEstado(dongle_id)   → seq_contiguo, c_conf, ultimo_ack   (seq_contiguo só cresce)
   aVerificar := todos os blocos com b.seq > snap.seq_contiguo (sem repetir o mesmo seq)

3. KMS, EM PARALELO (errgroup), fora da transação
   a) validos := kms.VerificarBlocos(DevId, [EntradaTag(0x03, DevId, b.Dados)], [b.Tag]) para TODO aVerificar
        em sub-lotes de VAULT_VERIFY_LOTE (padrão 32 ⇒ 1 chamada), até VAULT_VERIFY_PARALELO simultâneos
   b) ackEsperado := simular(passo 4) com estado = snap e todas as tags supostas válidas
      (puro, usa espia(b) = LerRegistros em bytes não autenticados e o catálogo de PIDs)
      se Ack{ackEsperado, snap.c_conf, 0} ≠ snap.ultimo_ack decodificado:
         tagPre := HMACAuth(DevId, EntradaTag(0x04, …))[:16]       ← pré-assinatura do ACK provável
   erro do KMS em (a) ou (b)                                       → 503 kms_indisponivel

4. TRANSAÇÃO (lock_timeout 3 s, statement_timeout 10 s; EmTransacao repete 1× em 40P01/40001)
   TravarDongle(dongle_id)                                ← pg_advisory_xact_lock(1, dongle_id)
   e := LerEstado(dongle_id)                              ← autoritativo (inclui geracao_boot e ultimo_boot_id)
   G, U := e.geracao_boot, e.ultimo_boot_id               ← estado da geração do boot_id (avança no laço)
   faixas := LerTMsUltimoBoots(dongle_id, boot_ids do lote, G)  ← para desembrulhar t_ms (u32); gerações G e G+1
   exp := e.seq_contiguo + 1; aceitos := []; motivo := ""
   maiorVisto := max(seq dos blocos com validos[b] == true)      ← inclusive os que não serão aceitos
   para cada bloco b com validos[b] == false: RegistrarRejeitado(tag_invalida)   (≤ 32, dedup por hash)
   para cada bloco b na ordem do corpo:
     b.seq < exp                     → duplicado; continue
     validos[b] == false             → motivo = tag_invalida; PARA
     c, err := LerRegistros(b.Dados) → err: RegistrarRejeitado(registro_malformado); PARA
     se c.Marcador == BOOT:          ← regra do doc, estendida para BOOT após OVERFLOW (ver Decisão)
        aceita se to == b.seq − 1  &&  from ≤ exp  &&  b.seq ≥ exp
        senão RegistrarRejeitado(marcador_invalido); PARA
        lacuna BOOT [exp, to] se exp ≤ to (a parte de [from, to] ainda não coberta); exp = b.seq + 1
        b.ger, G, U, avancou = atribuirGeracao(G, U, b.BootID)       ← o BOOT abre a geração nova, se for o caso
     se c.Marcador == LOSS:          ← regra do doc, estendida para BOOT corrompido (ver Decisão)
        aceita se from == exp  &&  from ≤ b.seq ≤ to
        senão RegistrarRejeitado(marcador_invalido); PARA
        lacuna LOSS [from, to]; exp = to + 1
     se dados:
        b.seq ≠ exp                  → motivo = salto; PARA
        algum PID ∉ catálogo         → RegistrarRejeitado(pid_desconhecido); InserirAlerta; PARA
        b.ger, G, U, avancou = atribuirGeracao(G, U, b.BootID)
        linhas := decodificar(b, c)  ← leitura/posicao (com b.ger)
        exp = b.seq + 1
     aceitos += b
   novoAck := exp − 1

   se len(aceitos) > 0:
     InserirBlocosBrutos(aceitos)   RETURNING seq      → len ≠ len(aceitos): ROLLBACK + alerta invariante_violada (500)
     CopiarLeituras(linhas)         (COPY binário)
     CopiarPosicoes(linhas)         (COPY binário, geography EWKB)
     InserirLacuna(...) para cada marcador
     InserirAlerta(geracao_boot_avancou, {de: G0, para: G, boot_id, ultimo_boot_id}) se G > e.geracao_boot (uma por lote)
     AtualizarBootsIngestao(agregado por (geracao_boot, boot_id), ordenado por essa chave)
     NotificarBootPendente('<dongle_id>')
   ackMsg := Ack{novoAck, e.c_conf, 0}
   ackTLV := e.ultimo_ack, se decodifica exatamente para ackMsg
          senão Montar(tagPre), se ackMsg == ACK pré-assinado no passo 3b
          senão Montar(HMACAuth(...)[:16])            ← só em divergência (tag inválida, corrida)
   se mudou algo (novoAck, maiorVisto, ackTLV ou geração):
     AtualizarEstadoIngestao(seq_contiguo=novoAck, maior_visto=maiorVisto, ultimo_ack=ackTLV,
                             geracao_boot=G, ultimo_boot_id=U)
   COMMIT

5. RESPOSTA
   Status = completo se novoAck ≥ seq do último bloco do lote; senão interrompido
   SemProgresso = (novoAck == e.seq_contiguo) && motivo == tag_invalida   → 422
   Acao = tabela de motivos
```

**[Decisão, revisão] Verificar todas as tags não duplicadas do lote**, e não só a "cadeia candidata" da primeira versão. O custo é o mesmo (1 `batch_input`, alguns KB a mais). Os ganhos:

1. `maior_seq_visto`/`head_seq` passam a registrar blocos autênticos acima de um furo. É o sinal "faltando no meio" (A3.6/A3.7) do `server-specification.md`, que antes ficava morto (com `maior_seq_visto == seq_contiguo` sempre).
2. Todo bloco com tag inválida vai para rejeitados, como diz o doc ("blocos inválidos vão para a tabela de rejeitados").
3. Some o motivo `concorrencia`: com tudo acima do snapshot verificado, qualquer bloco que a transação precise já tem resultado, porque `e.seq_contiguo ≥ snap`.

**[Decisão, revisão] ACK pré-assinado em paralelo com a verificação.** Na primeira versão, o ACK era assinado dentro da transação, com o advisory lock e uma conexão do pool presos durante uma ida e volta ao Vault (20–50 ms via VPN) e em série com o `verify`. Agora, o ACK provável é calculado e assinado no passo 3, em paralelo. Em regime (lote todo válido, sem corrida), a transação não chama o Vault, e cada lote custa uma ida e volta ao KMS. A assinatura dentro da transação só acontece em divergência, mantendo a atomicidade (o ACK gravado em `ultimo_ack` é sempre o do estado commitado).

**[Decisão, revisão] BOOT aceito com `from ≤ exp` (não só `from == exp`).** O firmware descarta flash por segmento inteiro (`LittleFsBlockStore::discardOldest`). Considere: ACK=10, flash `[11..30]`, queda de energia, BOOT `seq=64 [31,63]`. A flash enche e o segmento `[11..30]` é descartado. No handshake, OVERFLOW `[11,63]` faz ACK=63. Com a regra literal (`from == ACK+1`), o BOOT 64 (`from=31 ≠ 64`) seria `marcador_invalido` para sempre, travando a fila. A regra estendida aceita o BOOT e registra só a parte da faixa ainda não coberta (`[exp, to]`, vazia nesse caso). Ela continua recusando um BOOT com `from > exp`, que indicaria blocos pulados sem declaração. Para `b.seq > exp` com `from == exp`, é exatamente a regra do doc.

**[Decisão, revisão; contrato cruzado com a Fase 1] LOSS aceito com `from == exp` e `from ≤ seq ≤ to`.** O doc pede `seq = from`. Isso cobre o caso comum (bloco de dados corrompido gera LOSS no próprio `seq`), que continua aceito. Mas não cobre um **BOOT corrompido na flash**: o cabeçalho com o `seq` dele (ex.: 64) é ilegível, e o furo `[31, 63]` nunca seria declarável. A extensão aceita o que a Fase 1 de fato gera nesse caso (spec 05 §6, teste 9b): um BOOT só vale como marcador se o CRC conferir; com CRC inválido ele é tratado como bloco corrompido comum, e o cursor em `ACK + 1` gera LOSS em pedaços de até 64 `seq` cobrindo os `seq` inexistentes (`[31, 63]`, com `seq = 31`) e depois um LOSS `[64, 64]` para o próprio bloco. Cada um desses LOSS satisfaz `from == exp` e `from ≤ seq ≤ to`. O bloco LOSS é gravado em `bloco_bruto` no seu próprio `seq`.

**Regras de reconciliação com a Fase 1 (confirmadas nos dois lados):**

1. **BOOT com faixa vazia** (`from_seq = seq`, `to_seq = seq − 1`): a Fase 1 grava um BOOT em **todo** boot, inclusive sem furo. O algoritmo acima já o aceita (`to == seq − 1`, `from == exp`) e não registra lacuna (`exp ≤ to` é falso). Caso de teste 20 abaixo.
2. **Blocos com `seq ≤ seq_contiguo` dentro do lote** são duplicados (`b.seq < exp → continue`): nunca interrompem o lote nem chamam o KMS. A Fase 1 conta com isso ao drenar o buffer do app após um OVERFLOW (`drainThenHandshake`).
3. **`seq = seq_contiguo + 1` é sempre aceito** quando a tag confere e a forma é válida, seja bloco de dados, BOOT ou LOSS.
4. **TAIL efetivo (Fase 1, spec 05 §4) × OVERFLOW:** o dongle relata `TAIL_seq = from_seq` do BOOT mais antigo ainda na flash (com CRC válido), não o `seq` do BOOT. Assim, um furo de boot `[from, seq − 1]` já apagado abaixo do BOOT nunca é classificado como OVERFLOW pelo passo 7f do handshake (spec 04): a regra `TAIL_seq > seq_contiguo + 1` só dispara quando houve descarte real. Quando o descarte apaga o próprio segmento do BOOT, a Fase 1 relata o `seq` do bloco mais antigo restante, e o OVERFLOW cobre `[seq_contiguo + 1, TAIL − 1]`; o BOOT seguinte (se houver) cai na regra estendida `from ≤ exp`.

### Fila travada por bloco autêntico (sem saída manual)

Alguns casos fazem o dongle reenviar para sempre um bloco **autêntico** que nenhuma regra aceita:

- `registro_malformado` por bug de firmware;
- `L ≠ tamanho + 32` (400) com bytes relidos idênticos;
- `tag_invalida` com CRC válido (chave ou DevId errados na NVS).

**[Decisão] Não há saída manual.** O comando `tccelta admin saltar` (e a causa `OPERADOR` em `lacuna`) foram **descartados** por decisão do autor: avançar `seq_contiguo` sem prova do dongle seria declarar perda sem que o dongle a tenha declarado (LOSS/BOOT/OVERFLOW são as únicas formas, e todas partem do dongle). Consequência aceita: **a fila daquele dongle fica travada** até corrigir a causa no dongle — firmware com o bug de registro, ou chave/DevId certos na NVS (`provision.py` + `importar-registro`; contadores zerados ⇒ novo DevId). Nenhum outro dongle é afetado e nenhum dado é perdido: o bloco continua na flash do dongle.

Como o problema aparece (para o operador agir no dongle, não no servidor): alertas `tag_invalida`/`marcador_invalido`/`pid_desconhecido`, `bloco_rejeitado.ocorrencias` crescendo para o mesmo `hash_conteudo`, `seq_contiguo` parado enquanto `maior_seq_visto`/`head_seq` sobem, e a métrica `tccelta_blocos_total{resultado}`.

Regras e invariantes:

- **Go-Back-N estrito.** Nada acima de `seq_contiguo` é guardado fora de ordem: um bloco só entra em `bloco_bruto` se for o próximo da cadeia. Depois do commit, todos os `seq` em `bloco_bruto` são ≤ `seq_contiguo`, por isso o `ON CONFLICT DO NOTHING` nunca dispara em operação normal. Ele fica como defesa em profundidade (o `RETURNING` confere).
- **Marcadores BOOT/LOSS:** as regras são as do `.tex` §4.3 e §5. O LOSS é gravado em `bloco_bruto` no próprio `seq` (= `from_seq`), com `boot_id = 0` e `t0_ms = 0`, como vierem. O BOOT é gravado no seu `seq` com o `boot_id` novo. A lacuna referencia `seq_bloco`.
- **Geração do `boot_id` (`atribuirGeracao`, função pura em `aplicacao.go`).** Os blocos aceitos chegam em ordem de `seq`, então dentro de uma geração os `boot_id` são crescentes; um `boot_id` **menor** que `ultimo_boot_id` é a volta do u16 (ou uma NVS apagada) e abre a geração seguinte:
  ```go
  // boot_id 0 (LOSS) não participa: devolve a geração corrente e deixa o estado intacto.
  func atribuirGeracao(g, u, bootID uint16) (ger, novoU uint16, avancou bool) {
      if bootID == 0      { return g, u, false }
      if bootID < u       { return g + 1, bootID, true }   // volta do u16
      return g, bootID, false
  }
  ```
  Só blocos **aceitos** (dados e BOOT; nunca duplicados, rejeitados ou LOSS) passam por ela, na ordem do corpo; por isso o estado `(G, U)` só avança com blocos que entram em `bloco_bruto`. A geração vai para `bloco_bruto`, `leitura`, `posicao` e `boot`, e `(G, U)` final é gravado em `dongle_estado` na mesma transação. É a mesma regra que o handshake infere, sem gravar (spec 04, passo 7g). Limite: menos de 65.536 boots entre dois blocos que chegam ao servidor.
- **Bloco de dados com PID desconhecido é rejeitado inteiro** (doc), e o lote para nele. Como o bloco é autêntico, o dongle o reenviará até o PID ser cadastrado. Isso trava a fila daquele dongle de propósito, porque o doc prefere isso a perder dados. Alerta `pid_desconhecido` para o operador.
- **`maior_seq_visto`** = maior `seq` com tag **válida** observado neste lote, inclusive acima de um furo e sem gravar o bloco, comparado com o valor salvo. **`head_seq`** sobe junto (`GREATEST`), sem alterar `head_relatado_em`, como no doc. Assim, `(seq_contiguo, maior_seq_visto]` é a faixa "faltando no meio" (A3.6/A3.7).
- **`epoca`** de cada bloco = claim `cs` do JWT.
- **`recebido_em`** = `agora()` da API no início da transação, o mesmo valor em `bloco_bruto`, `leitura` e `posicao` do lote.
- **Rejeitados:** gravados na mesma transação, com `ON CONFLICT (dongle_id, hash_conteudo) DO UPDATE SET ocorrencias = ocorrencias + 1`. Cada requisição grava no máximo 32 linhas (todas as tags inválidas + o primeiro rejeitado por conteúdo), e o reenvio do mesmo lixo só incrementa `ocorrencias` (crescimento limitado).
- **Desembrulhar `t_ms` (u32):** `t0 = tempo.Desembrulhar(cab.T0Ms, referencia = boot.t_ms_ultimo ou t0 do bloco anterior do mesmo boot no lote)`, escolhendo `k` em `t0 + k·2³²` mais próximo da referência. A mesma função é usada no handshake (spec 04) para o `t_ms` da resposta.

### Decodificação (`internal/ingestao/decodificador.go`)

```go
type LinhaLeitura struct {
    Geracao, BootID int32; TMs int64; PID int16; ValorBruto int32; Valor *float32; SeqBloco int64
}
type LinhaPosicao struct {
    Geracao, BootID int32; TMs int64; Geom geo.PontoZ; SeqBloco int64
}
// Decodificar aplica o catálogo: bruto = A (n_bytes=1) ou 256·A + B (n_bytes=2);
// valor = bruto·fator + deslocamento; fora de [valor_min, valor_max] ⇒ Valor = nil.
func Decodificar(b protocolo.Bloco, c protocolo.Conteudo, t0 int64, cat *pids.Catalogo) ([]LinhaLeitura, []LinhaPosicao, error) // ErrPIDDesconhecido
// GPS: Geom = PontoZ{LonGraus: lon/1e7, LatGraus: lat/1e7, AltM: alt/1000}; t_ms = t0 + Δt.
// Faixa do bloco para o boot: t_ini = t0 + DtMin, t_fim = t0 + DtMax.
```

**[Decisão] A decodificação fica na transação síncrona da ingestão, como no doc.** Motivos:

1. **Rejeitar PID desconhecido exige decodificar antes do ACK**, porque depois do ACK o dongle apaga o bloco. A parte cara de CPU (percorrer os registros) já acontece no caminho síncrono de qualquer forma.
2. **Atomicidade:** ACK dado implica silver populada. Não existe estado intermediário "bronze sem silver" para monitorar, reprocessar ou explicar.
3. **Custo:** um lote de 32 blocos tem cerca de 6.400 linhas em dois `COPY` binários, na faixa de poucos ms. A meta de 1.000 dongles (~170 blocos/s ≈ 34 mil linhas/s) cabe com folga num Postgres dedicado, como o doc estima.
4. O lock por dongle serializa só os lotes do mesmo dongle, então transações maiores não reduzem o paralelismo entre dongles.

A alternativa assíncrona fica desenhada em [Aberto]. Ela só deve ser ativada se o teste de carga (spec 10) mostrar p99 do `POST /blocos` acima da meta por causa do `COPY`.

### Queries (`internal/banco/queries/blocos.sql`)

```sql
-- name: LerSnapshotEstado :one
SELECT seq_contiguo, c_conf, ultimo_ack FROM dongle_estado WHERE dongle_id = @dongle_id;

-- name: LerTMsUltimoBoots :many
-- Devolve as linhas candidatas nas gerações @geracao e @geracao + 1; o chamador escolhe a certa (atribuirGeracao).
SELECT geracao_boot, boot_id, t_ms_ultimo FROM boot
WHERE dongle_id = @dongle_id AND boot_id = ANY(@boot_ids::int[])
  AND geracao_boot IN (@geracao::int, @geracao::int + 1);

-- name: InserirBlocosBrutos :many
INSERT INTO bloco_bruto (dongle_id, seq, boot_id, geracao_boot, t0_ms, marcador, epoca, payload, assinatura, recebido_em)
SELECT @dongle_id, u.seq, u.boot_id, u.geracao_boot, u.t0_ms, NULLIF(u.marcador, ''), @epoca, u.payload, u.assinatura, @recebido_em
FROM unnest(@seqs::bigint[], @boot_ids::int[], @geracoes::int[], @t0s::bigint[], @marcadores::text[],
            @payloads::bytea[], @assinaturas::bytea[])
     AS u(seq, boot_id, geracao_boot, t0_ms, marcador, payload, assinatura)
ON CONFLICT (dongle_id, seq) DO NOTHING
RETURNING seq;

-- name: CopiarLeituras :copyfrom
INSERT INTO leitura (dongle_id, geracao_boot, boot_id, t_ms, pid, valor_bruto, valor, seq_bloco, recebido_em)
VALUES (@dongle_id, @geracao_boot, @boot_id, @t_ms, @pid, @valor_bruto, @valor, @seq_bloco, @recebido_em);

-- name: CopiarPosicoes :copyfrom
INSERT INTO posicao (dongle_id, geracao_boot, boot_id, t_ms, geom, origem, seq_bloco, recebido_em)
VALUES (@dongle_id, @geracao_boot, @boot_id, @t_ms, @geom, @origem, @seq_bloco, @recebido_em);

-- name: RegistrarRejeitado :exec
INSERT INTO bloco_rejeitado (dongle_id, seq, boot_id, t0_ms, payload, assinatura, motivo, detalhe,
                             hash_conteudo, token_jti, recebido_em)
VALUES (@dongle_id, @seq, @boot_id, @t0_ms, @payload, @assinatura, @motivo, @detalhe,
        @hash_conteudo, @token_jti, @recebido_em)
ON CONFLICT (dongle_id, hash_conteudo) DO UPDATE
   SET ocorrencias = bloco_rejeitado.ocorrencias + 1, recebido_em = EXCLUDED.recebido_em;

-- name: AtualizarBootsIngestao :exec
-- Uma linha por (geracao_boot, boot_id) (agregado em Go, ordenado por essa chave para ordem de locks estável).
INSERT INTO boot (dongle_id, geracao_boot, boot_id, seq_primeiro, seq_ultimo, t_ms_primeiro, t_ms_ultimo,
                  deslocamento_max_ms, pendente_tempo, pendente_gold, pendente_desde)
SELECT @dongle_id, u.geracao_boot, u.boot_id, u.seq_ini, u.seq_fim, u.t_ini, u.t_fim, u.desl_max, true, true, @agora
FROM unnest(@geracoes::int[], @boot_ids::int[], @seq_inis::bigint[], @seq_fims::bigint[], @t_inis::bigint[],
            @t_fims::bigint[], @desl_maxs::float8[])
     AS u(geracao_boot, boot_id, seq_ini, seq_fim, t_ini, t_fim, desl_max)
ORDER BY u.geracao_boot, u.boot_id
ON CONFLICT (dongle_id, geracao_boot, boot_id) DO UPDATE SET
  seq_primeiro        = LEAST(boot.seq_primeiro, EXCLUDED.seq_primeiro),
  seq_ultimo          = GREATEST(boot.seq_ultimo, EXCLUDED.seq_ultimo),
  t_ms_primeiro       = LEAST(boot.t_ms_primeiro, EXCLUDED.t_ms_primeiro),
  t_ms_ultimo         = GREATEST(boot.t_ms_ultimo, EXCLUDED.t_ms_ultimo),
  deslocamento_max_ms = LEAST(boot.deslocamento_max_ms, EXCLUDED.deslocamento_max_ms),
  pendente_gold       = true,
  -- boot sem âncora: a janela por vizinhos muda com a faixa ⇒ recalcular
  pendente_tempo      = boot.pendente_tempo OR boot.origem_tempo IS DISTINCT FROM 'ancoras',
  pendente_desde      = CASE WHEN boot.pendente_tempo THEN boot.pendente_desde ELSE EXCLUDED.pendente_desde END;

-- name: AtualizarEstadoIngestao :exec
UPDATE dongle_estado SET
  seq_contiguo    = @seq_contiguo,
  maior_seq_visto = GREATEST(maior_seq_visto, @maior_visto, @seq_contiguo),
  head_seq        = GREATEST(head_seq, @maior_visto, @seq_contiguo),
  ultimo_ack      = @ultimo_ack,
  ultimo_ack_em   = @agora,
  geracao_boot    = @geracao_boot,
  ultimo_boot_id  = @ultimo_boot_id
WHERE dongle_id = @dongle_id;
```

`deslocamento_max_ms` por bloco = `recebido_em_ms − (t0 + DtMax)`. O agregado por boot no lote é o mínimo. Para marcadores LOSS (`boot_id = 0`, sem tempo confiável), nenhuma linha de `boot` é tocada.

### Custo do KMS no caminho quente

| Estratégia | Chamadas ao Vault por POST de 32 blocos | Observação |
|---|---|---|
| Uma `verify` por bloco + `hmac` do ACK | 33 | inviável sob carga; latência × 33 via VPN |
| `verify` com `batch_input` (1 chamada) + `hmac` do ACK dentro da transação | 2 (em série, 1 delas segurando o lock) | primeira versão, descartada na revisão |
| **`verify` com `batch_input` ‖ `hmac` do ACK provável, em paralelo, fora da transação** | **2 chamadas, 1 ida e volta, 0 sob o lock** | escolhida |
| + reaproveitar `ultimo_ack` quando o ACK não muda | **0–2** | lotes só de duplicados não chamam o KMS |
| Cache local de K_rec | 0 | **proibido**: anula o isolamento do KMS (premissa do doc) |
| Cache de resultado de verificação | — | sem ganho: cada bloco válido é verificado uma vez (duplicados são pulados pelo `seq` antes do KMS) |

**[Decisão] `batch_input` + pular duplicados + pré-assinar o ACK em paralelo + reaproveitar o ACK.**

- Custo estimado: com 1.000 dongles e `T_lat = 60 s`, ~17 POST/s ⇒ ≤ 34 chamadas/s. Com `T_lat = 10 s`, ~100 POST/s ⇒ ≤ 200 chamadas/s. Ambos estão bem dentro do que um Vault com Transit atende numa VM modesta. O teste de carga mede (spec 10).
- Latência: ~0,5–2 ms local (HTTP keep-alive + JSON + base64 de ~45 KB), +RTT via VPN. É pago uma vez por lote, não por bloco.
- **Paralelismo com goroutines:** o trabalho de CPU de cada lote (parse, montagem das entradas, decodificação) roda em paralelo entre requisições, e cada requisição é uma goroutine. Dentro do lote, `VAULT_VERIFY_LOTE`/`VAULT_VERIFY_PARALELO` permitem dividir a verificação em sub-lotes paralelos via `errgroup`. O padrão é 32/1 (uma chamada); o teste de carga compara com 8/4.
- **Proteção:** semáforo `VAULT_MAX_SIMULTANEAS` (padrão 32 por réplica), timeout e *circuit breaker* (spec 07). Falha ⇒ 503 + `Retry-After`. Nada foi escrito, e o app reenvia o lote.

### Concorrência

| Situação | Garantia |
|---|---|
| Dois lotes do mesmo dongle ao mesmo tempo (app reenviou após timeout) | `pg_advisory_xact_lock(1, dongle_id)` serializa. O segundo vê o `seq_contiguo` novo: o que o primeiro gravou vira duplicado; o resto segue a cadeia. Todo bloco acima do snapshot já foi verificado, então nenhum bloco não verificado pode ser gravado (`e.seq_contiguo ≥ snap`). O ACK pré-assinado diverge e é reassinado dentro da transação |
| Lote × resposta do handshake (overflow avançando a fronteira) | mesmo lock; o lote que vier depois vê a fronteira nova |
| Dongles diferentes | locks diferentes ⇒ paralelo, limitado pelo semáforo e pelo pool |
| Lote × worker no `boot` | a TX1 do worker trava só 1 `boot` e não espera a API; a TX2 (vários `boot`) toma antes o mesmo advisory lock do dongle (spec 06) ⇒ sem ciclo. `40P01` residual: 1 retentativa em `EmTransacao`, depois 503 |
| Lote × `POST /desafio` | o desafio espera o lock de linha de `dongle_estado`, pego só no fim do lote |
| Lote preso no lock por mais de 3 s | `lock_timeout` ⇒ `ErrBancoOcupado` ⇒ 503 `banco_ocupado` + `Retry-After` |

### Handler (`internal/httpapi/blocos.go`, `confirmacao.go`)

```go
func (h *Handlers) ReceberLote(w http.ResponseWriter, r *http.Request)      // POST /v1/dongles/blocos
func (h *Handlers) ObterConfirmacao(w http.ResponseWriter, r *http.Request) // GET /v1/dongles/{id}/confirmacao
```

O handler lê o corpo inteiro (`io.ReadAll` sob `MaxBytesReader`), obtém `token.Sessao` e `dongles.Dongle` do contexto (postos pelo middleware JWT), chama o serviço e escreve os headers e o corpo. Não há lógica de protocolo no handler.

## Plano arquivo por arquivo

| Arquivo | Conteúdo |
|---|---|
| `internal/banco/queries/blocos.sql` | queries acima |
| `internal/ingestao/servico.go` | `ProcessarLote`, `UltimaConfirmacao` |
| `internal/ingestao/aplicacao.go` | `aplicar(blocos, validos, estado, catalogo) (aceitos, linhas, lacunas, motivo, novoAck, maiorVisto, geracao)` (puro, testável sem banco), com `atribuirGeracao`. O passo 3b chama a mesma função com `validos = todos true` e `estado = snapshot` para obter o ACK provável: uma regra, uma implementação |
| `internal/ingestao/reprocessar.go` + `cmd/tccelta/worker.go` (subcomando `reprocessar`) | `tccelta worker reprocessar` (seção *Reprocessamento*) |
| `internal/ingestao/decodificador.go` | `Decodificar` |
| `internal/tempo/desembrulhar.go` | `Desembrulhar(t32 uint32, ref int64) int64` |
| `internal/pids/catalogo.go` | (spec 03) |
| `internal/httpapi/blocos.go`, `confirmacao.go` (+testes) | handlers |
| `internal/ingestao/*_test.go`, `servico_integracao_test.go` | testes |

## Testes

**Puros (`aplicacao_test.go`).** `S` é o `seq_contiguo` inicial; a lista é a ordem do corpo.

| # | S | Lote (seq) | Tags | Esperado (ACK, status, motivo, ação) |
|---|---|---|---|---|
| 1 | 10 | 11..20 | ok | 20, completo |
| 2 | 10 | 5..15 | ok | 15, completo, 5 duplicados |
| 3 | 10 | 11, 12, 14 | ok | 12, interrompido, salto, reler |
| 4 | 10 | 11, 12(tag ruim), 13 | — | 11, interrompido, tag_invalida, reler (13 não gravado) |
| 5 | 10 | 11(tag ruim) | — | 10, 422 sem progresso, tag_invalida |
| 6 | 10 | 11, BOOT(seq=64, from=12, to=63), 65 | ok | 65; lacuna BOOT [12,63] |
| 7 | 10 | BOOT(seq=11, from=11, to=10) | ok | 11; sem lacuna (BOOT sem furo) |
| 8 | 10 | BOOT(seq=64, from=13, to=63) | ok | 10, marcador_invalido, reler |
| 9 | 10 | LOSS(seq=11, from=11, to=15), 16 | ok | 16; lacuna LOSS [11,15] |
| 10 | 10 | LOSS(seq=11, from=11, to=15), 13 | ok | 15, completo (13 ≤ 15 vira duplicado; último seq do lote ≤ ACK) |
| 11 | 10 | 11 com PID 0x99 | ok | 10, pid_desconhecido, aguardar; alerta |
| 12 | 10 | 11 com marcador + PID no mesmo bloco | ok | 10, registro_malformado, aguardar |
| 13 | 10 | 11, 11, 12 | ok | 12, completo (o segundo 11 é duplicado dentro do lote) |
| 14 | 10→15 (corrida) | 11..20, verificados com snap=10 | ok | 20, completo (11..15 duplicados); ACK pré-assinado (20) confere |
| 15 | 10→24 (overflow concorrente) | 11..20, 25 (verificado: > snap) | ok | 25, completo (11..20 duplicados); ACK pré-assinado (20) diverge ⇒ reassinado na tx |
| 16 | 63 (OVERFLOW [11,63] já aplicado) | BOOT(seq=64, from=31, to=63), 65 | ok | 65, completo; **sem** lacuna BOOT (a faixa já está coberta pelo OVERFLOW) |
| 17 | 30 | BOOT(seq=64, from=20, to=63) | ok | 64; lacuna BOOT [31,63] (só a parte não coberta) |
| 18 | 30 | LOSS(seq=64, from=31, to=64) (BOOT corrompido) | ok | 64; lacuna LOSS [31,64] |
| 19 | 30 | LOSS(seq=31, from=32, to=40) | ok | 30, marcador_invalido (`from ≠ exp`) |
| 20 | 30 | BOOT(seq=31, from=31, to=30) (faixa vazia, todo boot grava BOOT) | ok | 31, completo, nenhuma lacuna |
| 21 | 30 | LOSS(seq=31, from=31, to=63), LOSS(seq=64, from=64, to=64), 65 (BOOT 64 corrompido, Fase 1 teste 9b) | ok | 65, completo; lacunas LOSS [31,63] e [64,64] |
| 22 | 10 | 11, 13, 14(tag ruim) | — | 11, salto; `maior_seq_visto = 13`; 14 em `bloco_rejeitado` |
| 23 | 10; `(G=0, U=65535)` | BOOT(seq=11, boot_id=1, from=11, to=10), 12(boot 1) | ok | 12, completo; geração do BOOT e do 12 = 1; estado final `(G=1, U=1)`; alerta `geracao_boot_avancou` |
| 24 | 10; `(G=0, U=7)` | 11(boot 7), LOSS(seq=12, from=12, to=14), 15(boot 7), duplicado 5(boot 3) | ok | 15; todos na geração 0 (LOSS e o duplicado não mexem em `(G,U)`) |

`atribuirGeracao` (puro): `(0,7,9)` → `(0,9,falso)`; `(0,7,7)` → `(0,7,falso)`; `(0,7,3)` → `(1,3,verdadeiro)`; `(0,65535,1)` → `(1,1,verdadeiro)`; `(2,5,0)` → `(2,5,falso)`.

Forma (400): TLV com `L` maior que o restante; tipo 0x02 no lote; `L ≠ tamanho + 32`; `tamanho = 16`; `tamanho = 1025`; corpo vazio. 33 TLVs ⇒ 429 `lote_grande`.

Decodificador:
- PID 0x0C com A=0x17, B=0x70 ⇒ bruto 6000, valor 1500 RPM;
- PID 0x05 com A=0x28 ⇒ 0 °C;
- PID 0x0D com A=0xFF ⇒ 255 dentro da faixa;
- valor fora da faixa ⇒ `Valor=nil`;
- GPS lat −235505200 ⇒ −23,55052°;
- Δt = 65 535 (máximo do `u16`; o campo não tem sinal, e t0 pode recuar entre blocos consecutivos porque o dongle abre outro bloco para um GPS anterior ao t0 do aberto);
- desembrulhar `t0` perto de 2³².

**Integração (Postgres + `kmsteste`):**

1. Lote de 32 blocos válidos ⇒ 32 linhas em `bloco_bruto`; linhas em `leitura`/`posicao` iguais à soma dos registros; `dongle_estado.seq_contiguo` atualizado; boot `pendente_gold`; `NOTIFY` entregue; ACK decodificável com tag correta.
2. O mesmo lote reenviado ⇒ 200 completo, 32 duplicados, nenhuma linha nova, **nenhuma chamada ao KMS** (dublê conta chamadas: 0 verify, 0 hmac).
3. 10 goroutines enviando lotes sobrepostos do mesmo dongle ⇒ estado final igual ao sequencial, sem linhas duplicadas em `leitura` (`count(*) = soma esperada`).
4. 100 dongles em paralelo ⇒ sem deadlock (`-race` + `pg_stat_activity` sem `Lock` > 1 s).
5. Bloco com tag inválida ⇒ `bloco_rejeitado` com `motivo='tag_invalida'`; reenvio ⇒ `ocorrencias=2`.
6. PID desconhecido ⇒ `aguardar`; `INSERT INTO pid` ⇒ (após NOTIFY) o mesmo lote ⇒ completo.
7. KMS fora ⇒ 503 + `Retry-After`; nada gravado.
8. `lock_timeout`: transação externa segurando `pg_advisory_lock(1, id)` ⇒ 503 `banco_ocupado` em ~3 s.
9. `GET /confirmacao` devolve exatamente o último ACK; com token de outro DevId ⇒ 403; dongle sem handshake ⇒ 404.
10. Permissão: a transação roda como `tccelta_api` e nenhuma query tenta `UPDATE bloco_bruto`.
11. Em regime (lote todo válido, sem corrida), o dublê do KMS registra exatamente 1 `verify` + 1 `hmac` **antes** da abertura da transação e 0 dentro dela.
12. Fila travada sem saída manual: bloco autêntico com `registro_malformado` ⇒ 200 interrompido com `aguardar`; `seq_contiguo` parado; reenviar o mesmo lote só incrementa `bloco_rejeitado.ocorrencias`; nenhuma rota, subcomando ou papel do banco avança a fila (o teste confere que `admin saltar` não existe e que `lacuna` não aceita causa `OPERADOR`).
13. Cenário "OVERFLOW até o BOOT" (caso 16) de ponta a ponta: handshake com TAIL=64, seguido do lote com o BOOT 64 ⇒ completo.
14. Volta do `boot_id` (casos 23–24): `leitura` das duas gerações coexistem sem colisão de PK em `boot`; `boot` ganha a geração 1; `bloco_bruto.geracao_boot` e `leitura.geracao_boot` conferem; alerta `geracao_boot_avancou` registrado uma vez.
15. Particionamento: lotes de 100 dongles diferentes caem em partições diferentes de `bloco_bruto` (`SELECT tableoid::regclass, count(*) …`), sem alterar o comportamento.
16. Reprocessamento (seção *Reprocessamento*): corrigir `fator` do PID 0x05 ⇒ `worker reprocessar --aplicar` ⇒ `valor` novo, contagem de linhas igual, `valor_bruto` intacto; repetir (idempotente); com o chunk comprimido (`compress_chunk`) também funciona; dry-run não escreve; concorrente com a ingestão do mesmo dongle ⇒ serializado pelo advisory lock; `tccelta_api` e `tccelta_worker` não conseguem `DELETE` em `leitura`.

## Passos ordenados com checkpoints

1. `aplicacao.go` (com `atribuirGeracao`) + `decodificador.go` com testes puros. **Checkpoint:** tabela de 24 casos verde.
2. Queries + `make sqlc`; teste do `COPY` de `posicao` (EWKB). **Checkpoint:** `SELECT ST_AsText(geom) FROM posicao` mostra o ponto esperado.
3. `ProcessarLote` com `kmsteste`. **Checkpoint:** integração 1–3.
4. Handler + middlewares (semáforo, JWT, limites). **Checkpoint:** o simulador (spec 10) envia 1.000 blocos de 1 dongle via HTTP, e o ACK final = 1.000.
5. Integração 4–10. **Checkpoint:** verde com `-race`.

## Critérios de aceite

- [ ] O DevId do lote vem só do JWT. Blocos de outro dongle falham na tag (o DevId entra no HMAC).
- [ ] Nenhum bloco acima de `seq_contiguo` é persistido fora de ordem; BOOT/LOSS seguem as regras do doc mais as extensões justificadas (BOOT após OVERFLOW, LOSS de BOOT corrompido).
- [ ] Um lote = uma transação = no máximo 2 chamadas ao KMS, feitas em paralelo antes da transação em regime.
- [ ] O reenvio do mesmo lote é idempotente (sem linhas novas, sem chamadas ao KMS).
- [ ] Status, headers `X-Lote-*` e corpo conforme o contrato usado pela spec 09.

## Riscos

| Risco | Mitigação |
|---|---|
| PID desconhecido trava a fila do dongle | comportamento desejado pelo doc; alerta + métrica `tccelta_blocos_total{resultado="pid_desconhecido"}` |
| `COPY` de muitas linhas segurando o lock | medido no teste de carga; [Aberto] modo assíncrono |
| Marcadores "espiados" antes da verificação | só alimentam o ACK *provável* (pré-assinatura); nenhuma decisão de escrita usa conteúdo não autenticado, e um ACK provável errado só custa uma reassinatura |
| Fila travada por bloco autêntico que nenhuma regra aceita | **sem saída manual (decisão do autor):** a fila do dongle fica travada até corrigir firmware/chave no dongle; alertas e `bloco_rejeitado.ocorrencias` sinalizam; nenhuma perda é declarada sem prova do dongle |
| App mal-comportado reenvia lixo em loop | rate limit por DevId + `bloco_rejeitado` deduplicado por hash |

## [Aberto]

- **[Aberto] Decodificação assíncrona.** Desenho pronto, para ativar só se o teste de carga exigir (`INGESTAO_MODO=assincrono`):
  - a transação grava `bloco_bruto` + `dongle_estado` e valida PIDs em memória (a rejeição continua síncrona);
  - nova coluna `dongle_estado.seq_decodificado`;
  - o worker pega dongles com `seq_decodificado < seq_contiguo` (`FOR UPDATE SKIP LOCKED` em `dongle_estado`… ou numa tabela `fila_decodificacao(dongle_id PK)` para não disputar a linha com a API), decodifica `(seq_decodificado, seq_contiguo]` a partir de `bloco_bruto` com o mesmo `ingestao.Decodificar`, faz o `COPY` e avança a marca na mesma transação.

  Recomendação: manter o modo síncrono no TCC.

## Reprocessamento (`tccelta worker reprocessar`)

**[Decisão] Implementado nesta fase** (decisão do autor, em vez de deixar para uma fase futura com `UPDATE` sobre `valor_bruto`). Se o `fator`/`deslocamento`/faixa de um PID for corrigido no catálogo, ou o decodificador tiver um bug, `leitura` e `posicao` ficam com o valor antigo. O comando refaz as duas tabelas **a partir de `bloco_bruto`** (a fonte da verdade, imutável), inclusive para dados antigos já em chunks comprimidos.

```
tccelta worker reprocessar --dongle <hex> [--desde-seq N] [--ate-seq M] [--lote 200] [--aplicar]
```

- **Onde roda:** no contêiner `ferramentas` (`make reprocessar`), com o papel `tccelta_dono` (`ADMIN_BANCO_URL`). **Nunca** no worker contínuo nem pela API: eles não têm `DELETE` em `leitura`/`posicao`. Não precisa do Vault: as tags já foram verificadas quando o bloco entrou, e o comando não reverifica nem reassina nada.
- **Sem `--aplicar` é um ensaio** (padrão): decodifica com o catálogo atual, compara com o que está em `leitura`/`posicao` e imprime o resumo (blocos, linhas, quantas mudariam de `valor`); não escreve.
- **Com `--aplicar`**, em janelas de `--lote` blocos de dados (`marcador IS NULL`) em ordem de `seq`, **uma transação por janela**:
  1. `TravarDongle(dongle_id)` (o mesmo `pg_advisory_xact_lock(1, dongle_id)` da ingestão e do handshake; `lock_timeout` 3 s, repete a janela até 5 vezes);
  2. lê os blocos (`seq, geracao_boot, boot_id, t0_ms, payload, recebido_em`) — o `t0_ms` gravado já está desembrulhado;
  3. decodifica com o **mesmo** `ingestao.Decodificar` e o catálogo `pid` recarregado no início;
  4. `DELETE FROM leitura/posicao WHERE dongle_id = … AND seq_bloco BETWEEN a AND b AND recebido_em IN (…)` — o `recebido_em` de cada bloco entra na cláusula para o TimescaleDB excluir chunks. DML em chunk **comprimido** é suportado a partir do TimescaleDB 2.11 (descomprime só os segmentos afetados); **[verificar]** na imagem fixada (`pg17`);
  5. `COPY` das linhas novas com o `recebido_em` **original** do bloco (as linhas continuam no mesmo chunk);
  6. `COMMIT`.
- **PID desconhecido** (removido do catálogo): a janela faz rollback, o comando para e imprime o `seq`; nada fica pela metade.
- **Idempotente:** rodar duas vezes dá o mesmo estado. `boot` (faixa de tempo e reta) não muda; o comando só marca `boot.pendente_gold = true` dos boots tocados (gancho da camada gold futura).
- **Auditoria:** ao fim grava `alerta(tipo = 'reprocessamento', detalhe = {desde_seq, ate_seq, blocos, linhas_removidas, linhas_inseridas, aplicado, operador})` e log `ingestao.reprocessado`; métrica `tccelta_reprocessamento_linhas_total`.
- **Custo:** proporcional ao número de blocos; em chunks comprimidos cada janela descomprime e recomprime os segmentos tocados (lento, mas correto). Recomenda-se limitar por `--desde-seq`.

## Decisões do autor (antes em aberto)

- **[Decisão] Sem saída manual para fila travada** (`admin saltar` descartado; ver *Fila travada por bloco autêntico*).
- **[Decisão] Reprocessamento implementado nesta fase** (ver acima).
- **[Decisão] Decodificação síncrona mantida** (a assíncrona fica só desenhada, abaixo, como o autor não pediu mudança).
