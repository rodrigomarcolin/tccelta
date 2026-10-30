# 06 — Worker assíncrono (âncora de tempo por boot)

> Fase 3 · Servidor. Depende de: 02 (`boot`, `ancora_tempo`, `alerta`), 03 (binário, `banco/escuta.go`), 04 e 05 (marcam boots pendentes e fazem `NOTIFY`). É dependência de: 10 (validação ponta a ponta do tempo).

## Objetivo

Rodar o mesmo binário em modo `worker` para:

1. recalcular a reta de tempo de cada boot (`a_ms`, `b`, `incerteza_base_ms`, `origem_tempo`), conforme `handshake-servidor-worker.puml` e a seção "Tempo absoluto" do `server-specification.md`. Isso inclui grupos de âncoras a menos de 10 min, interseção de intervalos, reta por mínimos quadrados ponderados, alerta de deriva acima de 40 ppm e boot sem âncora estimado pelos vizinhos;
2. expirar desafios não respondidos;
3. persistir os grupos de âncoras de cada boot (`grupo_ancora`), para o cálculo barato da incerteza por leitura;
4. servir de gancho para a camada gold (fica para fase posterior: só o `boot.pendente_gold`).

O processamento usa goroutines, `LISTEN/NOTIFY` e `FOR UPDATE SKIP LOCKED`.

## Escopo

Dentro: laço do worker, algoritmo puro de tempo (pacote `tempo`), queries `worker.sql`, job de expiração, métricas, testes. Fora: camada gold e decodificação assíncrona. A decodificação continua síncrona na ingestão (decisão justificada na spec 05). O desenho assíncrono pronto está no [Aberto] da spec 05.

## Design

### Processo

```
tccelta worker
 ├─ goroutine "escuta": banco.Escutar(ctx, pool, "boot_pendente") → canal de sinais (coalescidos)
 ├─ WORKER_CONCORRENCIA goroutines "tempo": esperam sinal OU tick de WORKER_INTERVALO_POLL;
 │     drenam a fila: repetir { processarUmBoot() } até não haver pendente
 ├─ goroutine "expiração": a cada WORKER_INTERVALO_EXPIRACAO, ExpirarDesafios(agora)
 ├─ HTTP :8090 /saude/viva  (+ métricas em :9091/metricas)
 └─ SIGTERM: para de pegar trabalho, termina a transação corrente, sai (≤ 25 s)
```

| Variável | Padrão | Uso |
|---|---|---|
| `WORKER_CONCORRENCIA` | `4` | goroutines de tempo (com `SKIP LOCKED` não há coordenação) |
| `WORKER_INTERVALO_POLL` | `30s` | rede de segurança se um `NOTIFY` se perder (reconexão) |
| `WORKER_INTERVALO_EXPIRACAO` | `60s` | job de desafios |
| `ANCORA_JANELA_GRUPO` | `10m` | âncoras a menos disso (em `t_ms`) podem formar um grupo |
| `DERIVA_MAX_PPM` | `40` | alerta se `|b| − 2σ_b` passar disso; também entra na incerteza dos vizinhos |
| `ANCORA_SPAN_MIN` | `1h` | intervalo mínimo entre centros de grupos para estimar `b ≠ 0` |
| `MEIA_LARGURA_MIN_MS` | `0.5` | piso da meia largura (evita peso infinito) |

`banco.Escutar` usa uma **conexão dedicada** (não do pool) com `LISTEN boot_pendente` e `WaitForNotification`. Se a conexão cair, ela reconecta com backoff de 1 s a 30 s e emite um sinal sintético ao reconectar, porque notificações podem ter sido perdidas. Os sinais são coalescidos num canal com buffer 1: vários `NOTIFY` viram uma única drenagem. O payload (`dongle_id:geracao_boot:boot_id`) só vai para o log; a fonte da verdade é a coluna `boot.pendente_tempo`.

### Uma unidade de trabalho = um boot = uma transação

```
TX1 (statement_timeout 10 s):
  b := PegarBootPendente()  … FOR UPDATE SKIP LOCKED LIMIT 1      → nenhum: fim da drenagem
  ancoras := ListarAncorasDoBoot(b)
  se len(ancoras) > 0:   est := tempo.EstimarPorAncoras(ancoras, cfg)
  senão se b.seq_primeiro != NULL:
       ant  := BootAncoradoAnterior(b.dongle_id, b.seq_primeiro)   (pode não existir)
       prox := BootAncoradoSeguinte(b.dongle_id, b.seq_primeiro)   (pode não existir)
       est  := tempo.EstimarPorVizinhos(ant, prox, faixa(b), b.deslocamento_max_ms, cfg)
  senão: est := sem estimativa (boot só conhecido pelo handshake, sem dados ainda)
  AtualizarTempoBoot(b, est)  → pendente_tempo = false
  SubstituirGruposAncora(b, est.Grupos)   ← apaga os grupos antigos e grava os atuais (nenhum, se origem ≠ 'ancoras')
  para cada alerta em est.Alertas: InserirAlertaSeNovo(...)
COMMIT
TX2 (só se est.Origem == 'ancoras' e a reta mudou; lock_timeout 3 s):
  TravarDongle(b.dongle_id)                  ← o MESMO pg_advisory_xact_lock(1, dongle_id) da API
  MarcarVizinhosSemAncora(b.dongle_id, b.seq_primeiro)
COMMIT          (lock_timeout ⇒ registra e deixa para o próximo ciclo; deadlock residual ⇒ 1 retentativa)
```

**[Decisão] Um boot por transação (TX1), e os vizinhos numa segunda transação (TX2) sob o advisory lock do dongle.**

- A TX1 trava uma única linha de `boot` com `SKIP LOCKED` (nunca espera) e não toca em outras linhas que a API possa estar segurando.
- A TX2 é um `UPDATE` sobre **várias** linhas de `boot`, travadas na ordem da varredura (não por `boot_id`). Ela formaria ciclo com um lote que faz upsert de dois boots, por exemplo um bloco BOOT que fecha o boot 5 e abre o 6.
  - **Revisão:** a primeira versão afirmava, erroneamente, que o worker "nunca segura dois locks de `boot`".
  - Por isso a TX2 toma antes o mesmo `pg_advisory_xact_lock(1, dongle_id)` usado pela ingestão e pelo handshake. Assim, ela fica serializada com todas as transações da API daquele dongle, e não há ciclo possível (a TX1 não toma o advisory lock nem espera a API).
- `EmTransacao` ainda trata um `40P01` residual com uma retentativa.

**Corrida com a API:** se a API marcar `pendente_tempo = true` enquanto o worker processa o boot, o `UPDATE` dela espera o `FOR UPDATE` do worker e só aplica depois do commit. O boot volta à fila com os dados novos, e nenhuma atualização se perde. Esse é o mesmo padrão do doc ("cada boot é travado por quem o pegou").

### Algoritmo (pacote puro `internal/tempo`)

Unidades: milissegundos em `float64`. S1/S2 são convertidos com `float64(t.UnixMicro())/1000`. Um instante de 2026 em ms (~1,8·10¹²) cabe com folga na mantissa do `float64` (resolução ~0,2 µs).

```go
package tempo

type Ancora struct{ TMs int64; S1Ms, S2Ms float64 }

type Grupo struct {
    Lo, Hi   float64 // limites do deslocamento o = hora_real − t_ms
    TCentro  float64 // média dos t_ms das âncoras do grupo
    TPrimeiro, TUltimo int64 // t_ms da primeira e da última âncora do grupo
    N        int
}
func (g Grupo) Centro() float64     { return (g.Lo + g.Hi) / 2 }
func (g Grupo) MeiaLargura() float64 { return (g.Hi - g.Lo) / 2 }

type Alerta struct{ Tipo string; Detalhe map[string]any } // tipos da tabela alerta

type Estimativa struct {
    AMs              float64  // o(t) = AMs + B·t ; hora_real(t) = AMs + (1+B)·t
    B                float64
    IncertezaBaseMs  int
    Origem           string   // "ancoras" | "vizinhos" | "" (sem estimativa)
    Grupos           []Grupo  // persistidos em `grupo_ancora` pelo worker (decisão do autor; o doc pedia para não persistir)
    Alertas          []Alerta
}

type Config struct {
    JanelaGrupoMs      float64 // 600000
    DerivaMaxPPM       float64 // 40
    MeiaLarguraMinMs   float64 // 0.5
    SpanMinMs          float64 // 3600000: só estima b ≠ 0 com grupos espalhados por ≥ 1 h
}

func AgruparAncoras(a []Ancora, c Config) ([]Grupo, []Alerta)
func AjustarReta(g []Grupo, c Config) (a, b float64)
func EstimarPorAncoras(a []Ancora, c Config) Estimativa

type ReferenciaVizinho struct {
    AMs, B            float64
    IncertezaBaseMs   int
    TMsPrimeiro, TMsUltimo int64
}
type FaixaBoot struct{ TMsPrimeiro, TMsUltimo int64 }
func EstimarPorVizinhos(ant, prox *ReferenciaVizinho, f FaixaBoot, deslocMaxMs *float64, c Config) Estimativa

// Incerteza de uma leitura: meia largura do grupo mais próximo + DerivaMax·|t − t_centro|.
// Os grupos vêm de `grupo_ancora` (ListarGruposDoBoot), sem refazer o agrupamento.
func Incerteza(t int64, grupos []Grupo, c Config) float64
```

#### 1. Uma âncora

Cada âncora `(t_ms, S1, S2)` limita o deslocamento do boot:

```
lo_i = S1 − t_ms_i        hi_i = S2 − t_ms_i        (lo_i ≤ o ≤ hi_i)
```

#### 2. Grupos (cruzar âncoras próximas)

Ordena as âncoras por `t_ms`. Percorre-as:

- O grupo corrente começa na primeira âncora.
- A próxima âncora entra no grupo se `t_ms − t_ms_da_primeira_do_grupo < 10 min` (janela medida **a partir da primeira âncora do grupo**, não encadeada) **e** a interseção continuar não vazia (`max(lo) ≤ min(hi)`).
- Se a janela estourar, fecha o grupo e abre outro com essa âncora.
- Se a interseção ficar vazia, gera o alerta `intersecao_vazia` (com as duas âncoras e a diferença em ms), fecha o grupo e abre outro com essa âncora. É o "não cruza, mantém separados" do doc.

Por grupo: `Lo = max(lo_i)`, `Hi = min(hi_i)`, `TCentro = média(t_ms_i)`.

**[Decisão] A janela é contada a partir da primeira âncora do grupo, não por encadeamento.** Com encadeamento, âncoras a cada 5 min por 3 horas formariam um único grupo com deriva acumulada de até 432 ms (40 ppm × 3 h), quebrando a premissa de que "em 10 min a deriva fica abaixo de 24 ms".

#### 3. Reta entre grupos

- Um grupo: `b = 0` e `a = Centro()`.
- Dois ou mais: mínimos quadrados ponderados sobre `(x_g = TCentro_g, y_g = Centro_g)`, com peso `w_g = 1 / max(MeiaLargura_g, 0,5)²`:

```
S  = Σw         Sx = Σw·x        Sy = Σw·y        Sxx = Σw·x²        Sxy = Σw·x·y
D  = S·Sxx − Sx²
b  = (S·Sxy − Sx·Sy) / D           (se |D| < ε: b = 0)
a  = (Sy − b·Sx) / S
```

Para evitar cancelamento numérico, centralize `x` (subtraia a média ponderada de `x`) e `y` (subtraia o `Centro` do primeiro grupo) antes de somar, e corrija `a` depois.

- **Incerteza do ajuste (revisão).** O erro padrão da inclinação, com pesos `w = 1/h²`, é `σ_b = sqrt(S / D)`.
  - Com poucos grupos próximos, `b` é dominado pelo ruído. Dois grupos a 10 min com meia largura de 200 ms admitem `|b|` de até ~667 ppm, e extrapolar essa reta erraria segundos no fim de um boot de 2 h (pior do que `b = 0`).
  - Regra: **usar o `b` ajustado só se** o intervalo entre os centros dos grupos for ≥ `ANCORA_SPAN_MIN` (padrão 1 h) **e** `|b| > σ_b`. Caso contrário, `b = 0` e `a` = média ponderada dos centros (`Sy/S`).
- `|b| − 2σ_b > 40·10⁻⁶` ⇒ alerta `deriva_excessiva` (com `b` e `σ_b` em ppm). A estimativa é gravada mesmo assim (o doc só pede o alerta). O alerta usa `2σ_b` para não disparar só por ruído.
- `IncertezaBaseMs = ceil(min_g MeiaLargura_g)`.
- `Origem = "ancoras"`.

#### 4. Boot sem âncora (vizinhos)

O `seq` ordena os boots (`boot.seq_primeiro`). Para o boot B com faixa `[t_ini, t_fim]` (= `t_ms_primeiro`, `t_ms_ultimo`):

- **Limite inferior** pelo boot **ancorado** anterior P. Os registros de B são posteriores ao último registro de P:
  `T_P_fim = a_P + (1+b_P)·t_fim_P`, `u_P = incerteza_base_P + 40 ppm·(t_fim_P − t_ini_P)` (conservador),
  `lo = (T_P_fim − u_P) − t_ini_B`.
- **Limite superior** pelo boot **ancorado** seguinte N: `T_N_ini = a_N + (1+b_N)·t_ini_N`, `hi_N = (T_N_ini + u_N) − t_fim_B`.
- **Limite superior pelo recebimento** (sempre disponível quando houve bloco): `hi_R = boot.deslocamento_max_ms`. Nenhum registro pode ter sido gerado depois de o bloco que o contém chegar ao servidor.
- `hi = min(hi_N, hi_R)` dos que existirem.
- Com `lo` e `hi`:
  - se `lo ≤ hi`: `a = (lo + hi)/2`, `b = 0`, `IncertezaBaseMs = ceil((hi − lo)/2)` (pode ser de horas), `Origem = "vizinhos"`;
  - se `lo > hi`: alerta `intersecao_vazia` e sem estimativa.
- Sem `lo` (não há boot ancorado anterior): sem estimativa (colunas nulas). Um limite só de um lado não dá centro.

**[Decisão] Os vizinhos são os boots ancorados mais próximos, não os imediatamente adjacentes.** Usar um vizinho que também foi estimado por vizinhos encadearia incertezas de horas. Pular os não ancorados mantém o limite válido, porque os registros dos boots intermediários estão entre B e o próximo boot ancorado, e só fica mais largo.

**[Decisão] Acrescentar o limite superior `deslocamento_max_ms`** (não estava no doc). É grátis, sempre correto e transforma "sem boot seguinte" (o caso mais comum: o boot mais recente, sem handshake) numa janela finita.

#### 5. Quando recalcular

| Evento | Quem marca `pendente_tempo` |
|---|---|
| Handshake aceito (com ou sem âncora) | API, `GarantirBootPendente` (spec 04) |
| Boot novo aparece num lote (inclusive pelo bloco BOOT) | API, `AtualizarBootsIngestao` (nasce pendente) |
| Novos dados num boot **sem** âncora (faixa mudou) | API, `AtualizarBootsIngestao` |
| Boot ganhou ou mudou a reta por âncoras | worker, TX2: `MarcarVizinhosSemAncora` |

### Queries (`internal/banco/queries/worker.sql`)

```sql
-- name: PegarBootPendente :one
SELECT dongle_id, geracao_boot, boot_id, seq_primeiro, t_ms_primeiro, t_ms_ultimo, deslocamento_max_ms,
       a_ms, b, origem_tempo
FROM boot
WHERE pendente_tempo
ORDER BY pendente_desde
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: ListarAncorasDoBoot :many
SELECT t_ms, s1, s2 FROM ancora_tempo
WHERE dongle_id = @dongle_id AND geracao_boot = @geracao_boot AND boot_id = @boot_id
ORDER BY t_ms;

-- name: BootAncoradoAnterior :one
SELECT geracao_boot, boot_id, a_ms, b, incerteza_base_ms, t_ms_primeiro, t_ms_ultimo
FROM boot
WHERE dongle_id = @dongle_id AND origem_tempo = 'ancoras' AND seq_primeiro < @seq_primeiro
ORDER BY seq_primeiro DESC
LIMIT 1;

-- name: BootAncoradoSeguinte :one
SELECT geracao_boot, boot_id, a_ms, b, incerteza_base_ms, t_ms_primeiro, t_ms_ultimo
FROM boot
WHERE dongle_id = @dongle_id AND origem_tempo = 'ancoras' AND seq_primeiro > @seq_primeiro
ORDER BY seq_primeiro ASC
LIMIT 1;

-- name: AtualizarTempoBoot :exec
UPDATE boot SET
  a_ms = @a_ms, b = @b, incerteza_base_ms = @incerteza_base_ms, origem_tempo = @origem_tempo,
  tempo_atualizado_em = @agora, pendente_tempo = false
WHERE dongle_id = @dongle_id AND geracao_boot = @geracao_boot AND boot_id = @boot_id;

-- name: ApagarGruposAncora :exec
DELETE FROM grupo_ancora
WHERE dongle_id = @dongle_id AND geracao_boot = @geracao_boot AND boot_id = @boot_id;

-- name: CopiarGruposAncora :copyfrom
INSERT INTO grupo_ancora (dongle_id, geracao_boot, boot_id, ordem, t_primeiro_ms, t_ultimo_ms,
                          t_centro_ms, lo_ms, hi_ms, n_ancoras, calculado_em)
VALUES (@dongle_id, @geracao_boot, @boot_id, @ordem, @t_primeiro_ms, @t_ultimo_ms,
        @t_centro_ms, @lo_ms, @hi_ms, @n_ancoras, @calculado_em);

-- name: ListarGruposDoBoot :many
SELECT ordem, t_primeiro_ms, t_ultimo_ms, t_centro_ms, lo_ms, hi_ms, n_ancoras
FROM grupo_ancora
WHERE dongle_id = @dongle_id AND geracao_boot = @geracao_boot AND boot_id = @boot_id
ORDER BY ordem;

-- name: MarcarVizinhosSemAncora :execrows
-- Todos os boots sem âncora entre o boot ancorado anterior e o seguinte (por seq).
UPDATE boot SET pendente_tempo = true, pendente_desde = @agora
WHERE dongle_id = @dongle_id
  AND NOT pendente_tempo
  AND origem_tempo IS DISTINCT FROM 'ancoras'
  AND seq_primeiro IS NOT NULL
  AND seq_primeiro > COALESCE((SELECT max(x.seq_primeiro) FROM boot x
                               WHERE x.dongle_id = @dongle_id AND x.origem_tempo = 'ancoras'
                                 AND x.seq_primeiro < @seq_primeiro), 0)
  AND seq_primeiro < COALESCE((SELECT min(x.seq_primeiro) FROM boot x
                               WHERE x.dongle_id = @dongle_id AND x.origem_tempo = 'ancoras'
                                 AND x.seq_primeiro > @seq_primeiro), 9223372036854775807);

-- name: InserirAlertaSeNovo :exec
-- Evita repetir o mesmo alerta a cada recálculo (janela de 24 h por boot e tipo).
INSERT INTO alerta (dongle_id, geracao_boot, boot_id, tipo, detalhe)
SELECT @dongle_id, @geracao_boot, @boot_id, @tipo, @detalhe
WHERE NOT EXISTS (SELECT 1 FROM alerta
                  WHERE dongle_id = @dongle_id AND geracao_boot = @geracao_boot AND boot_id = @boot_id
                    AND tipo = @tipo AND criado_em > @agora - INTERVAL '24 hours');

-- name: ContarBootsPendentes :one
SELECT count(*) FROM boot WHERE pendente_tempo;
```

(`ExpirarDesafios` está na spec 04.)

### Consulta de uso (para validar e para o TCC)

```sql
-- Hora e origem de cada leitura (join por boot, como no doc)
SELECT l.*, to_timestamp((bo.a_ms + (1 + bo.b) * l.t_ms) / 1000.0) AS ts,
       bo.origem_tempo, bo.incerteza_base_ms
FROM leitura l
JOIN boot bo USING (dongle_id, geracao_boot, boot_id)
WHERE bo.a_ms IS NOT NULL;

-- Incerteza de cada leitura (ms) a partir dos grupos persistidos: meia largura do grupo mais próximo
-- mais a deriva máxima (DERIVA_MAX_PPM) vezes a distância ao centro dele. Só boots com origem 'ancoras'.
SELECT l.t_ms, l.valor, g.u AS incerteza_ms
FROM leitura l
JOIN LATERAL (
  SELECT (ga.hi_ms - ga.lo_ms) / 2 + 40e-6 * abs(l.t_ms - ga.t_centro_ms) AS u
  FROM grupo_ancora ga
  WHERE ga.dongle_id = l.dongle_id AND ga.geracao_boot = l.geracao_boot AND ga.boot_id = l.boot_id
  ORDER BY abs(l.t_ms - ga.t_centro_ms)
  LIMIT 1
) g ON true;
```

### Métricas e logs

- `tccelta_worker_boots_processados_total{origem="ancoras|vizinhos|sem_estimativa"}`
- `tccelta_worker_duracao_segundos` (por boot)
- `tccelta_worker_fila_tempo` (gauge, `ContarBootsPendentes` a cada 15 s)
- `tccelta_worker_alertas_total{tipo}`
- `tccelta_worker_desafios_expirados_total`
- `tccelta_worker_escuta_reconexoes_total`
- `tccelta_worker_grupos_gravados_total`
- Log `worker.boot_recalculado{dongle_id, geracao_boot, boot_id, origem, a_ms, b_ppm, incerteza_ms, grupos, ancoras}`.

## Plano arquivo por arquivo

| Arquivo | Conteúdo |
|---|---|
| `internal/tempo/ancoras.go` | `AgruparAncoras`, `AjustarReta`, `EstimarPorAncoras`, `Incerteza` |
| `internal/tempo/vizinhos.go` | `EstimarPorVizinhos` |
| `internal/tempo/desembrulhar.go` | (spec 05) |
| `internal/tempo/grupos.go` | (parte de `ancoras.go`) conversão `Grupo` ⇄ linha de `grupo_ancora` e `Incerteza` sobre os grupos persistidos |
| `internal/tempo/*_test.go` | casos abaixo + propriedades |
| `internal/banco/queries/worker.sql` | queries acima |
| `internal/banco/escuta.go` | `Escutar` |
| `internal/worker/worker.go` | `Executar(ctx, deps) error` — goroutines, drenagem, desligamento |
| `internal/worker/boot.go` | `processarUmBoot(ctx) (processou bool, err error)` |
| `internal/worker/expiracao.go` | job de desafios |
| `cmd/tccelta/worker.go` | subcomando |

## Testes

**Unitários (`tempo`):**

1. **Exemplo do doc:**
   - A: S1 = 10:00:00,000, S2 = 10:00:00,800, t_ms = 600.000;
   - B: S1 = 10:05:00,400, S2 = 10:05:01,000, t_ms = 900.000.

   Esperado: 1 grupo, `Lo` = 09:50:00,400, `Hi` = 09:50:00,800, meia largura 200 ms, `b = 0`, `a` = 09:50:00,600 (em ms Unix), `IncertezaBaseMs = 200`.
2. **Interseção vazia:** duas âncoras a 1 min com intervalos disjuntos ⇒ 2 grupos + alerta `intersecao_vazia`.
3. **Deriva conhecida:** âncoras sintéticas geradas com `o(t) = a₀ + 20 ppm·t` e ruído uniforme de ±100 ms, a cada 30 min durante 6 h ⇒ `|b − 20 ppm| < 5 ppm` e nenhum alerta.
4. **Deriva excessiva:** 60 ppm sintético, 6 h ⇒ alerta `deriva_excessiva`.
4b. **Grupos próximos (revisão):** 2 grupos a 10 min com meia largura de 200 ms e centros desalinhados em 300 ms ⇒ `b = 0` (abaixo de `ANCORA_SPAN_MIN`), `a` = média ponderada, **sem** alerta.
5. **Janela não encadeia:** âncoras a cada 4 min por 1 h ⇒ grupos de no máximo 3 âncoras (0, 4, 8 min).
6. **Vizinhos:** P ancorado (`a`, `b = 0`, incerteza 100 ms, faixa `[0, 3.600.000]`), N ancorado depois, B no meio ⇒ janela correta. Sem N ⇒ usa `deslocamento_max_ms`. Sem P ⇒ sem estimativa. `lo > hi` ⇒ alerta.
7. **Propriedade (fuzz):** para âncoras geradas a partir de um `o(t)` verdadeiro e intervalos que o contêm, o `o(t)` verdadeiro fica dentro de `a + b·t ± (incerteza do grupo mais próximo + 40 ppm·distância)` em todos os `t` das âncoras.

**Integração:**

1. Handshake via `ServicoHandshake` ⇒ `NOTIFY` ⇒ o worker (rodando no teste) atualiza `boot.a_ms` em menos de 1 s.
2. Dois workers concorrentes com 200 boots pendentes ⇒ cada boot processado exatamente uma vez (contador por boot) e fila vazia ao fim.
3. Conexão de `LISTEN` derrubada (`pg_terminate_backend`) ⇒ reconecta, e o poll de 30 s (reduzido a 1 s no teste) processa o que chegou no meio.
4. API marcando pendente durante o processamento (teste que segura o `FOR UPDATE`) ⇒ o boot é reprocessado depois.
4b. Lote com bloco BOOT (upsert de 2 boots) concorrente com a TX2 do worker no mesmo dongle, 200 repetições ⇒ nenhum `40P01` chega ao cliente.
5. Expiração: desafio com `expira_em` no passado ⇒ `resultado = 'expirado'`; desafio com `tag_invalida` não é sobrescrito.
6. Grupos persistidos: boot com 3 âncoras em 2 grupos ⇒ 2 linhas em `grupo_ancora` iguais a `Estimativa.Grupos` (`Lo`, `Hi`, `TCentro`, `N`); recalcular com uma âncora nova **substitui** (não acumula) as linhas; boot que passa de `ancoras` a `vizinhos` fica sem grupos; a consulta de incerteza por leitura devolve `meia_largura + 40 ppm·distância` do grupo mais próximo; o papel `tccelta_api` não consegue `INSERT` em `grupo_ancora`.
7. Volta do `boot_id`: boots `(geracao 0, boot 5)` e `(geracao 1, boot 5)` do mesmo dongle são processados e ancorados independentemente (âncoras, grupos e vizinhos por `seq_primeiro`).

## Passos ordenados com checkpoints

1. Pacote `tempo` com testes 1–7. **Checkpoint:** verde; o exemplo do doc bate até o ms.
2. Queries + `make sqlc` + `processarUmBoot`. **Checkpoint:** integração 2.
3. `Escutar` + laço + desligamento. **Checkpoint:** integração 1 e 3; `docker compose stop worker` sai em menos de 5 s sem erro.
4. Job de expiração e métricas. **Checkpoint:** integração 5; o Prometheus mostra `tccelta_worker_fila_tempo`.

## Critérios de aceite

- [ ] O exemplo numérico do `server-specification.md` (A ∩ B = ±200 ms) é reproduzido pelo teste.
- [ ] Boots com âncora têm `origem_tempo = 'ancoras'`, e boots sem âncora entre boots ancorados têm `'vizinhos'` com janela coerente.
- [ ] Alertas `deriva_excessiva` e `intersecao_vazia` são gerados e não se repetem a cada recálculo.
- [ ] Várias instâncias de worker processam a fila sem duplicar trabalho nem causar deadlock com a API.

## Riscos

| Risco | Mitigação |
|---|---|
| Salto de NTP do servidor gera âncoras incoerentes | detectado como `intersecao_vazia`/`relogio_servidor_regrediu`; grupos separados |
| `t_ms` u32 com volta dentro do boot | `tempo.Desembrulhar` na ingestão e no handshake, com a mesma referência por boot; `ancora_tempo.t_ms` e `leitura.t_ms` ficam na mesma escala (spec 04, passo 7g) |
| Fila crescendo (worker parado) | gauge `tccelta_worker_fila_tempo` + `/saude/viva` |
| Boot com milhares de âncoras (dongle que reconecta muito) | O(n log n) por recálculo; aceitável. **Sem limite** (decisão do autor): usa todas as âncoras (melhor reta); monitorar `tccelta_worker_duracao_segundos` e `tccelta_worker_fila_tempo`, e só limitar às 500 mais recentes se a fila crescer |

## Decisões do autor (antes em aberto)

- **[Decisão] Camada gold: fase posterior.** O worker só mantém o gancho `boot.pendente_gold` (marcado pela ingestão), sem consumidor nesta fase. Quando vier, usa o mesmo padrão (`FOR UPDATE SKIP LOCKED` por boot) e lê `grupo_ancora` para a incerteza.
- **[Decisão] Sem limite de âncoras por boot** (ver Riscos).
- **[Decisão] Persistir os grupos de âncoras** em `grupo_ancora` (spec 02): desvio consciente do doc, que pedia para não persistir. Regravados por inteiro na TX1, junto com a reta do boot.
- **[Decisão] `DERIVA_MAX_PPM = 40` do doc, mantido sem medir.** Não se faz a bancada de 24 h agora; o valor fica **sem validação** na placa usada (limitação registrada). Como seria medir (documentado, não executado):
  1. **Montagem:** 1–3 dongles reais (`collector_twai_prod`/`debug` conforme o perfil) ligados em USB estável, com o simulador de ECU (ECUSim) alimentando o OBD, num ambiente de temperatura estável; app com o servidor real (Fase 3) e `T_lat` curto.
  2. **Coleta:** forçar um handshake a cada ~10 min por 24 h (`FakeDongle`/script que reconecta, ou `AT#`/reset de sessão do dongle), deixando o RTT da âncora ≤ 2 s; o worker acumula `ancora_tempo` e `grupo_ancora`.
  3. **Extração do `b` medido:** `SELECT dongle_id, geracao_boot, boot_id, b*1e6 AS ppm, incerteza_base_ms FROM boot WHERE origem_tempo = 'ancoras'` — só vale com span ≥ `ANCORA_SPAN_MIN` (1 h), então um boot contínuo de 24 h dá um `b` por dongle; repetir com variação de temperatura (dongle ao sol/ar-condicionado) para ver a faixa.
  4. **RTT real da âncora:** `SELECT percentile_cont(ARRAY[0.5,0.95,0.99]) WITHIN GROUP (ORDER BY extract(epoch FROM s2 - s1)) FROM ancora_tempo` mostra quanto da janela de 2 s o enlace celular consome.
  5. **Decisão:** `DERIVA_MAX_PPM` = máximo de `|b|` medido entre dongles/temperaturas + margem (ex. 25 %); se o máximo medido for muito menor que 40 ppm, reduzir melhora a incerteza dos vizinhos; se maior, o alerta `deriva_excessiva` já dispararia.
  6. **Registro:** relatório curto em `backend/docs/relatorios/` com `b` por dongle, RTT p50/p95/p99 e o valor escolhido. Não faz parte dos testes de carga (spec 10), que usam o simulador Go.
