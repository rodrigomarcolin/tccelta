# 02 — Banco de dados e migrations

> Fase 3 · Servidor. Depende de: 01 (banco no ar, papéis e extensões criados pelo init). É dependência de: 03, 04, 05, 06, 07 (cadastro `dongle`).

## Objetivo

Criar, com migrations versionadas, todas as tabelas descritas na seção 6 do `.tex` (figuras `modelo-dados-bronze`, `modelo-dados-contexto` e `modelo-dados-silver`) e no `server-specification.md`, mais o que o servidor precisa para operar: o cadastro `dongle` (DevId → `dongle_id`), a tabela de rejeitados e a de alertas. O DDL precisa ser o esquema que o `sqlc` lê para gerar código, sem duplicação.

## Escopo

Dentro: DDL completo (bronze, contexto, silver), seed dos 7 PIDs, índices, hypertables, compressão, FKs, *triggers* de invariantes, `NOTIFY`, `GRANT`s por papel, escolha e uso da ferramenta de migration e convenções de nomes.

Fora: camada gold (`viagem`, `amostra_alinhada`, `viagem_metricas`), que fica como trabalho futuro e aparece em [Aberto]. As queries de cada fluxo estão nas specs 04, 05 e 06; aqui ficam só as de cadastro (`dongle`, `pid`).

## Design

### Ferramenta de migration

**[Decisão] `goose` v3 (`github.com/pressly/goose/v3`), com migrations SQL embutidas no binário.**

| Critério | goose | golang-migrate |
|---|---|---|
| Arquivo por migration | um `.sql` com `-- +goose Up` / `-- +goose Down` | dois arquivos (`.up.sql` / `.down.sql`) |
| Embutir no binário | `goose.SetBaseFS(embed.FS)` + `goose.UpContext` | possível (`iofs`), mas mais verboso |
| Blocos `$$` (funções/triggers) | `-- +goose StatementBegin/End` | depende do driver |
| Migração sem transação (ex.: `CREATE INDEX CONCURRENTLY`) | `-- +goose NO TRANSACTION` por arquivo | não há equivalente direto |
| Integração com sqlc | sqlc lê migrations goose nativamente e ignora o `Down` | também suportado |

Com goose, a migration vira um único arquivo legível, roda no subcomando `tccelta migrar` (a imagem distroless não precisa do CLI) e o mesmo diretório alimenta o `sqlc`. A numeração é **sequencial** (`goose create -s`, ex.: `00003_bronze.sql`), não por timestamp: com um time pequeno, sequência é mais legível e conflitos aparecem cedo no PR.

`internal/banco/migracoes.go`:

```go
package banco

import "embed"

//go:embed migrations/*.sql
var Migracoes embed.FS   // backend/internal/banco/migrations/ (symlink proibido: arquivos ficam aqui)

// Migrar aplica todas as migrations pendentes com goose, sob o advisory lock do próprio goose.
func Migrar(ctx context.Context, db *sql.DB) error
// Implementação: goose.NewProvider(goose.DialectPostgres, db, fs.Sub(Migracoes, "migrations"),
//   goose.WithSessionLocker(lock.NewPostgresSessionLocker())) e p.Up(ctx).
```

**[Decisão, revisão] Usar a API `goose.NewProvider`, não a legada (`SetBaseFS` + `UpContext`).** Só o Provider aceita `WithSessionLocker`, que trava a execução com `pg_advisory_lock`. Sem ele, duas execuções de `migrar` (ou um reinício durante a migração) poderiam aplicar migrations em paralelo.

> Local: `backend/internal/banco/migrations/` (precisa estar sob o pacote por causa do `embed`). O `sqlc.yaml` aponta `schema` para esse diretório.

O `tccelta migrar` usa `pgx/v5/stdlib` para abrir `*sql.DB` (o goose exige `database/sql`) e roda o Provider acima. O lock de sessão garante que duas réplicas não migrem ao mesmo tempo.

### Convenções

- Identificadores de domínio em pt-BR e `snake_case`, como no doc (`bloco_bruto`, `seq_contiguo`, `recebido_em`). Nomes do protocolo ficam como estão (`c_s`, `t_ms`, `head_seq`, `tail_seq`, `boot_id`).
- Inteiros `u32` do protocolo viram `bigint` com `CHECK (… BETWEEN 0 AND 4294967295)`; `u16` viram `integer` com `CHECK (… BETWEEN 0 AND 65535)`.
- Instantes: `timestamptz`. S1/S2 vêm do relógio da API (spec 04), nunca de `now()`. **[Decisão]** O Postgres guarda microssegundos. A API arredonda S1 **para baixo** e S2 **para cima** no µs, para o intervalo `[S1, S2]` nunca encolher.
- Índices: `<tabela>_<propósito>`. Checks e FKs: nome automático.
- Cada migration tem `Down` funcional (testado no CI com `goose down-to 0` + `goose up`).

### Mapa das tabelas

```
dongle (cadastro) ─1:1─ dongle_estado (bronze, estado operacional)
   │
   ├─1:N─ bloco_bruto (bronze, PK dongle_id+seq, append-only)
   ├─1:N─ bloco_rejeitado (bronze, auditoria)
   ├─1:N─ desafio (contexto, PK dongle_id+c_s) ─1:0..1─ ancora_tempo (contexto)
   ├─1:N─ boot (contexto, PK dongle_id+boot_id) ─1:N─ ancora_tempo
   ├─1:N─ lacuna (contexto; OVERFLOW→desafio, BOOT/LOSS→bloco_bruto)
   ├─1:N─ alerta (operacional)
   └─1:N─ leitura / posicao (silver, hypertables; sem FK para dongle, ver Decisão)
pid (silver, catálogo) ─1:N─ leitura
```

### Migrations

#### `00001_extensoes.sql`

```sql
-- +goose Up
-- As extensões são criadas pelo init do contêiner (superusuário). Aqui só
-- garantimos que existem; em bancos de teste (testcontainers) o usuário é
-- superusuário e elas são criadas de fato.
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS postgis;

-- +goose Down
SELECT 1; -- extensões não são removidas por migration
```

#### `00002_dongle.sql`

```sql
-- +goose Up
-- Cadastro dos dongles fabricados. DevId (8 bytes) é o identificador do
-- protocolo; dongle_id é a chave interna usada por todas as outras tabelas.
CREATE TABLE dongle (
  dongle_id            integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dev_id               bytea   NOT NULL UNIQUE CHECK (octet_length(dev_id) = 8),
  dev_id_hex           text    GENERATED ALWAYS AS (encode(dev_id, 'hex')) STORED UNIQUE,
  perfil_seguranca     text    NOT NULL DEFAULT 'hmac'
                         CHECK (perfil_seguranca IN ('hmac', 'asym', 'none')),
  kms_chave_auth       text    NOT NULL,  -- nome da chave no mount transit-auth (= dev_id_hex)
  kms_chave_rec        text    NOT NULL,  -- nome da chave no mount transit-rec  (= dev_id_hex)
  chave_publica        bytea   NULL CHECK (chave_publica IS NULL OR octet_length(chave_publica) = 32),
  ativo                boolean NOT NULL DEFAULT true,
  diag_solicitado_min  integer NULL CHECK (diag_solicitado_min BETWEEN 1 AND 65535),
  descricao            text    NULL,
  criado_em            timestamptz NOT NULL DEFAULT now(),
  CHECK (perfil_seguranca <> 'asym' OR chave_publica IS NOT NULL)
);

-- +goose Down
DROP TABLE dongle;
```

`chave_publica` e o perfil `asym` existem para o benchmark do perfil assimétrico (spec 07, [Aberto]). `diag_solicitado_min` é o gancho do modo diagnóstico (`flags_c` bit 0, spec 04, [Aberto]).

#### `00003_bronze.sql`

```sql
-- +goose Up
-- Estado operacional de cada dongle visto pelo servidor.
CREATE TABLE dongle_estado (
  dongle_id         integer PRIMARY KEY REFERENCES dongle (dongle_id),
  seq_contiguo      bigint  NOT NULL DEFAULT 0 CHECK (seq_contiguo BETWEEN 0 AND 4294967295), -- ACK_seq
  maior_seq_visto   bigint  NOT NULL DEFAULT 0,  -- maior seq com tag válida já visto (aceito ou não; spec 05 verifica o lote inteiro)
  head_seq          bigint  NOT NULL DEFAULT 0,  -- HEAD_seq do último handshake, ou maior seq recebido
  tail_seq          bigint  NULL,                -- TAIL_seq do último handshake aceito
  head_relatado_em  timestamptz NULL,            -- S2 do handshake que relatou head e tail
  ultimo_c_s        bigint  NOT NULL DEFAULT 0 CHECK (ultimo_c_s BETWEEN 0 AND 4294967295),
  c_conf            bigint  NOT NULL DEFAULT 0,  -- c_S do último handshake aceito
  ultimo_ack        bytea   NULL,                -- TLV 0x04 completo e assinado (GET /confirmacao)
  ultimo_ack_em     timestamptz NULL,
  atualizado_em     timestamptz NOT NULL DEFAULT now(),
  CHECK (seq_contiguo <= head_seq),
  CHECK (seq_contiguo <= maior_seq_visto),
  CHECK (c_conf <= ultimo_c_s)
);

-- Invariantes de monotonicidade: nenhum caminho de código pode regredir a
-- fronteira, o HEAD ou os contadores do handshake.
-- +goose StatementBegin
CREATE FUNCTION dongle_estado_monotono() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.seq_contiguo < OLD.seq_contiguo OR NEW.head_seq < OLD.head_seq
     OR NEW.ultimo_c_s < OLD.ultimo_c_s OR NEW.c_conf < OLD.c_conf
     OR NEW.maior_seq_visto < OLD.maior_seq_visto THEN
    RAISE EXCEPTION 'dongle_estado: regressão proibida (dongle_id=%)', OLD.dongle_id
      USING ERRCODE = 'check_violation';
  END IF;
  NEW.atualizado_em := now();
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER dongle_estado_monotono BEFORE UPDATE ON dongle_estado
  FOR EACH ROW EXECUTE FUNCTION dongle_estado_monotono();

-- Blocos aceitos, exatamente como vieram. Append-only para a aplicação.
CREATE TABLE bloco_bruto (
  dongle_id    integer NOT NULL REFERENCES dongle (dongle_id),
  seq          bigint  NOT NULL CHECK (seq BETWEEN 1 AND 4294967295),
  boot_id      integer NOT NULL CHECK (boot_id BETWEEN 0 AND 65535),
  t0_ms        bigint  NOT NULL,
  marcador     text    NULL CHECK (marcador IN ('BOOT', 'LOSS')),
  epoca        bigint  NULL,      -- c_S do handshake cujo JWT trouxe o bloco (claim "cs")
  payload      bytea   NOT NULL CHECK (octet_length(payload) BETWEEN 17 AND 1024), -- área de dados (tamanho bytes)
  assinatura   bytea   NOT NULL CHECK (octet_length(assinatura) IN (0, 32, 64)),   -- tag K_rec (32) | ECDSA P-256 r‖s (64) | none (0)
  recebido_em  timestamptz NOT NULL,
  PRIMARY KEY (dongle_id, seq)    -- idempotência: replay vira no-op
);

-- +goose StatementBegin
CREATE FUNCTION bloco_bruto_imutavel() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'bloco_bruto é imutável' USING ERRCODE = 'insufficient_privilege';
END $$;
-- +goose StatementEnd
CREATE TRIGGER bloco_bruto_imutavel BEFORE UPDATE ON bloco_bruto
  FOR EACH ROW EXECUTE FUNCTION bloco_bruto_imutavel();

-- Blocos recusados (tag inválida, PID desconhecido, marcador fora da regra,
-- registro malformado). Campos do cabeçalho são "como declarados": não confiáveis.
CREATE TABLE bloco_rejeitado (
  id            bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dongle_id     integer NOT NULL REFERENCES dongle (dongle_id),
  seq           bigint  NULL,
  boot_id       integer NULL,
  t0_ms         bigint  NULL,
  payload       bytea   NOT NULL,
  assinatura    bytea   NULL,
  motivo        text    NOT NULL CHECK (motivo IN
                  ('tag_invalida', 'pid_desconhecido', 'marcador_invalido', 'registro_malformado')),
  detalhe       text    NULL,
  hash_conteudo bytea   NOT NULL CHECK (octet_length(hash_conteudo) = 32), -- sha256(payload || assinatura)
  token_jti     uuid    NULL,
  recebido_em   timestamptz NOT NULL,
  ocorrencias   integer NOT NULL DEFAULT 1,
  UNIQUE (dongle_id, hash_conteudo)  -- reenvio do mesmo bloco ruim só incrementa ocorrencias
);
CREATE INDEX bloco_rejeitado_recentes ON bloco_rejeitado (dongle_id, recebido_em DESC);

-- +goose Down
DROP TABLE bloco_rejeitado;
DROP TABLE bloco_bruto;
DROP FUNCTION bloco_bruto_imutavel();
DROP TABLE dongle_estado;
DROP FUNCTION dongle_estado_monotono();
```

**[Decisão] `bloco_bruto` é tabela comum, não hypertable.** A PK `(dongle_id, seq)` é a garantia de unicidade do doc. Um hypertable exige a coluna de partição em todo índice único, o que quebraria essa garantia. Volume: com 1.000 dongles, cerca de 15 GB/dia. Isso é aceitável no TCC; ver [Aberto] particionamento.

**[Decisão] `epoca` foi reaproveitada:** o doc a define como "época em que o bloco foi assinado" (resquício do desenho com chave de sessão, que não existe mais). Aqui ela guarda o `c_S` do handshake cujo JWT autorizou o lote. Isso rastreia qual sessão do app entregou cada bloco (auditoria A3.x) sem coluna nova.

#### `00004_contexto.sql`

```sql
-- +goose Up
-- Uma linha por boot. Colunas de faixa são mantidas pela ingestão/handshake;
-- colunas de tempo são derivadas e recalculadas pelo worker (spec 06).
CREATE TABLE boot (
  dongle_id             integer NOT NULL REFERENCES dongle (dongle_id),
  boot_id               integer NOT NULL CHECK (boot_id BETWEEN 0 AND 65535),
  config                jsonb   NULL,              -- protocolo, Tmax, PIDs e períodos ([Aberto] origem)
  -- faixa observada
  seq_primeiro          bigint  NULL,
  seq_ultimo            bigint  NULL,
  t_ms_primeiro         bigint  NULL,              -- menor t_ms de registro visto neste boot
  t_ms_ultimo           bigint  NULL,              -- maior t_ms de registro visto neste boot
  deslocamento_max_ms   double precision NULL,     -- min(recebido_em − t_ms_fim_do_bloco): o ≤ isto
  -- reta de tempo: hora_real_ms(t) = a_ms + (1 + b) * t_ms
  a_ms                  double precision NULL,
  b                     double precision NULL,
  incerteza_base_ms     integer NULL,
  origem_tempo          text    NULL CHECK (origem_tempo IN ('ancoras', 'vizinhos')),
  tempo_atualizado_em   timestamptz NULL,
  -- fila do worker
  pendente_tempo        boolean NOT NULL DEFAULT true,
  pendente_gold         boolean NOT NULL DEFAULT false,
  pendente_desde        timestamptz NOT NULL DEFAULT now(),
  criado_em             timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (dongle_id, boot_id)
);
CREATE INDEX boot_fila_tempo ON boot (pendente_desde) WHERE pendente_tempo;
CREATE INDEX boot_ordem_seq  ON boot (dongle_id, seq_primeiro);

-- Uma linha por desafio emitido: o rastro de cada c_S e N_S.
CREATE TABLE desafio (
  dongle_id               integer NOT NULL REFERENCES dongle (dongle_id),
  c_s                     bigint  NOT NULL CHECK (c_s BETWEEN 1 AND 4294967295),
  nonce                   bytea   NOT NULL UNIQUE CHECK (octet_length(nonce) = 16),
  s1                      timestamptz NOT NULL,      -- relógio da API, antes do INSERT
  expira_em               timestamptz NOT NULL,
  ack_seq_emitido         bigint  NOT NULL,          -- ACK_seq levado no Desafio 0x01
  c_conf_emitido          bigint  NOT NULL,
  flags_c                 smallint NOT NULL DEFAULT 0,
  diag_min                integer NULL,
  -- preenchidos com a Resposta 0x02
  respondido_em           timestamptz NULL,          -- S2; NULL enquanto pendente
  boot_id                 integer NULL,
  t_ms                    bigint  NULL,
  head_seq                bigint  NULL,
  tail_seq                bigint  NULL,
  resultado               text    NULL CHECK (resultado IN
                            ('aceito', 'aceito_sem_ancora', 'tag_invalida', 'inconsistente', 'expirado')),
  tentativas_tag_invalida integer NOT NULL DEFAULT 0,
  -- efeitos de um handshake aceito
  ack_seq                 bigint  NULL,              -- ACK_seq devolvido no ACK 0x04
  perda_ini               bigint  NULL,              -- faixa OVERFLOW, se houve
  perda_fim               bigint  NULL,
  token_jti               uuid    NULL UNIQUE,
  PRIMARY KEY (dongle_id, c_s)
);
CREATE INDEX desafio_pendentes ON desafio (dongle_id, expira_em) WHERE respondido_em IS NULL;
CREATE INDEX desafio_a_expirar ON desafio (expira_em) WHERE respondido_em IS NULL AND resultado IS NULL;

-- Uma linha por handshake com S2 − S1 ≤ 2 s: a medida crua.
-- O deslocamento do boot está em [s1 − t_ms, s2 − t_ms].
CREATE TABLE ancora_tempo (
  dongle_id  integer NOT NULL,
  boot_id    integer NOT NULL,
  t_ms       bigint  NOT NULL,
  nonce      bytea   NOT NULL UNIQUE REFERENCES desafio (nonce),
  s1         timestamptz NOT NULL,
  s2         timestamptz NOT NULL,
  PRIMARY KEY (dongle_id, boot_id, t_ms),
  FOREIGN KEY (dongle_id, boot_id) REFERENCES boot (dongle_id, boot_id),
  CHECK (s2 >= s1)
);

-- Lacunas declaradas: OVERFLOW (handshake), BOOT/LOSS (blocos marcadores) ou
-- OPERADOR (salto administrativo de uma fila travada por bloco autêntico; spec 05).
CREATE TABLE lacuna (
  id             bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dongle_id      integer NOT NULL REFERENCES dongle (dongle_id),
  from_seq       bigint  NOT NULL,
  to_seq         bigint  NOT NULL,
  causa          text    NOT NULL CHECK (causa IN ('BOOT', 'LOSS', 'OVERFLOW', 'OPERADOR')),
  registrado_em  timestamptz NOT NULL,
  c_s            bigint  NULL,   -- handshake que declarou (OVERFLOW)
  seq_bloco      bigint  NULL,   -- bloco marcador que declarou (BOOT/LOSS)
  justificativa  text    NULL,   -- obrigatória para OPERADOR
  CHECK (to_seq >= from_seq),
  CHECK ((causa = 'OVERFLOW') = (c_s IS NOT NULL)),
  CHECK ((causa IN ('BOOT', 'LOSS')) = (seq_bloco IS NOT NULL)),
  CHECK ((causa = 'OPERADOR') = (justificativa IS NOT NULL)),
  FOREIGN KEY (dongle_id, c_s)       REFERENCES desafio (dongle_id, c_s),
  FOREIGN KEY (dongle_id, seq_bloco) REFERENCES bloco_bruto (dongle_id, seq),
  UNIQUE (dongle_id, from_seq)    -- duas lacunas não começam no mesmo seq
);

-- Alertas operacionais (deriva, interseção vazia, PID desconhecido, etc.).
CREATE TABLE alerta (
  id         bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dongle_id  integer NOT NULL REFERENCES dongle (dongle_id),
  boot_id    integer NULL,
  tipo       text    NOT NULL CHECK (tipo IN
               ('deriva_excessiva', 'intersecao_vazia', 'relogio_servidor_regrediu', 'pid_desconhecido',
                'tag_invalida', 'marcador_invalido', 'boot_id_reutilizado', 'invariante_violada')),
  detalhe    jsonb   NOT NULL DEFAULT '{}',
  criado_em  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX alerta_recentes ON alerta (dongle_id, criado_em DESC);

-- +goose Down
DROP TABLE alerta;
DROP TABLE lacuna;
DROP TABLE ancora_tempo;
DROP TABLE desafio;
DROP TABLE boot;
```

Notas de projeto:

- **[Decisão] Tabela `lacuna` própria.** O `server-specification.md` dizia que as lacunas "saem de uma consulta", mas o `.tex` (versão mais recente) e o `modelo-dados-contexto.puml` criam `lacuna` com causa. Prevalece o `.tex`. `UNIQUE (dongle_id, from_seq)` torna a gravação idempotente.
- **`desafio.ack_seq_emitido`/`c_conf_emitido`** guardam o que o Desafio 0x01 levou. Isso permite auditar e reconstruir os bytes do desafio sem chamar o KMS.
- **`boot.pendente_tempo DEFAULT true`:** todo boot novo nasce pendente, porque precisa ao menos da estimativa por vizinhos.
- **`boot.deslocamento_max_ms`:** cada bloco foi gerado antes de ser recebido, então `o + t_fim(bloco) ≤ recebido_em(bloco)`. A ingestão mantém o mínimo de `recebido_em_ms − t_fim_ms`. Esse é um limite superior gratuito para boots sem âncora (spec 06).

#### `00005_pid.sql`

```sql
-- +goose Up
-- Fonte da verdade dos PIDs: o que cada um mede e como decodificar.
-- valor = (bruto * fator) + deslocamento, com bruto = A (1 byte) ou 256*A + B (2 bytes).
-- 0xFD (LOSS), 0xFE (BOOT) e 0xFF (GPS) são tipos de registro reservados.
CREATE TABLE pid (
  pid           smallint PRIMARY KEY CHECK (pid BETWEEN 0 AND 252),
  nome          text NOT NULL,
  unidade       text NOT NULL,
  n_bytes       smallint NOT NULL CHECK (n_bytes IN (1, 2)),
  fator         double precision NOT NULL,
  deslocamento  double precision NOT NULL DEFAULT 0,
  valor_min     double precision NULL,   -- faixa válida: fora dela, leitura.valor = NULL
  valor_max     double precision NULL
);

INSERT INTO pid (pid, nome, unidade, n_bytes, fator, deslocamento, valor_min, valor_max) VALUES
  (4,  'Carga do motor',         '%',    1, 100.0/255, 0,   0,   100),        -- 0x04
  (5,  'Temperatura do líquido', '°C',   1, 1,         -40, -40, 215),        -- 0x05
  (12, 'Rotação',                'RPM',  2, 0.25,      0,   0,   16383.75),   -- 0x0C
  (13, 'Velocidade',             'km/h', 1, 1,         0,   0,   255),        -- 0x0D
  (16, 'MAF',                    'g/s',  2, 0.01,      0,   0,   655.35),     -- 0x10
  (47, 'Nível de combustível',   '%',    1, 100.0/255, 0,   0,   100),        -- 0x2F
  (73, 'Pedal do acelerador',    '%',    1, 100.0/255, 0,   0,   100);        -- 0x49

-- Avisa a API para recarregar o cache de PIDs (spec 05) sem novo deploy.
-- +goose StatementBegin
CREATE FUNCTION notificar_pid_alterado() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM pg_notify('pid_alterado', '');
  RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER pid_alterado AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON pid
  FOR EACH STATEMENT EXECUTE FUNCTION notificar_pid_alterado();

-- +goose Down
DROP TRIGGER pid_alterado ON pid;
DROP FUNCTION notificar_pid_alterado();
DROP TABLE pid;
```

#### `00006_silver.sql`

```sql
-- +goose Up
CREATE TABLE leitura (
  dongle_id    integer  NOT NULL,
  boot_id      integer  NOT NULL,
  t_ms         bigint   NOT NULL,          -- t0_ms do bloco + Δt
  pid          smallint NOT NULL REFERENCES pid (pid),
  valor_bruto  integer  NOT NULL,          -- A ou 256*A + B
  valor        real     NULL,             -- fator/deslocamento; NULL se fora da faixa
  seq_bloco    bigint   NOT NULL,
  recebido_em  timestamptz NOT NULL
);
SELECT create_hypertable('leitura', 'recebido_em', chunk_time_interval => INTERVAL '1 day');
CREATE INDEX leitura_viagem ON leitura (dongle_id, boot_id, pid, t_ms);
ALTER TABLE leitura SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'dongle_id, pid',
  timescaledb.compress_orderby   = 'boot_id, t_ms'
);
SELECT add_compression_policy('leitura',
  COALESCE(NULLIF(current_setting('tccelta.compressao_apos', true), ''), '7 days')::interval);

CREATE TABLE posicao (
  dongle_id    integer NOT NULL,
  boot_id      integer NOT NULL,
  t_ms         bigint  NOT NULL,
  geom         geography(PointZ, 4326) NOT NULL,  -- lon/lat graus, alt metros
  origem       text    NOT NULL DEFAULT 'app',     -- GPS vem do app via AT#GPS, assinado pelo dongle
  seq_bloco    bigint  NOT NULL,
  recebido_em  timestamptz NOT NULL
);
SELECT create_hypertable('posicao', 'recebido_em', chunk_time_interval => INTERVAL '1 day');
CREATE INDEX posicao_viagem ON posicao (dongle_id, boot_id, t_ms);
ALTER TABLE posicao SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'dongle_id',
  timescaledb.compress_orderby   = 'boot_id, t_ms'
);
SELECT add_compression_policy('posicao',
  COALESCE(NULLIF(current_setting('tccelta.compressao_apos', true), ''), '7 days')::interval);

-- +goose Down
SELECT remove_compression_policy('posicao', if_exists => true);
DROP TABLE posicao;
SELECT remove_compression_policy('leitura', if_exists => true);
DROP TABLE leitura;
```

- **[Decisão] Particionar por `recebido_em`, com chunks de 1 dia e compressão aos 7 dias**, como no doc. A ingestão grava sempre no chunk mais recente, então nunca escreve em chunk comprimido.
- **[Decisão] Sem FK de `leitura`/`posicao` para `dongle`.** Seria uma verificação por linha em ~34 mil linhas/s (cenário de 1.000 dongles). A consistência já é garantida porque essas linhas só nascem da mesma transação que insere em `bloco_bruto`, e essa tabela tem FK. A FK para `pid` fica, como pede o doc: a tabela é minúscula e serve de rede de segurança para o cache.
- **Sintaxe de compressão:** usamos `timescaledb.compress`/`add_compression_policy`, que funcionam em todo TimescaleDB 2.x. A partir do 2.18 elas viram aliases de *columnstore* e continuam válidas.
- **sqlc e Timescale:** o sqlc ignora `SELECT` em arquivos de schema. **Checkpoint 3** confirma isso. Se a versão fixada reclamar, os `SELECT create_hypertable(...)` **e também** os `ALTER TABLE … SET (timescaledb.compress …)` (que o parser do sqlc pode rejeitar como `AT_SetRelOptions`) vão para blocos `DO $$ BEGIN PERFORM …; EXECUTE '…'; END $$;`, que o sqlc não interpreta.
- **Volume de `leitura`:** com 34 mil linhas/s são cerca de 150 GB/dia antes da compressão. Isso não importa no TCC (poucos dongles), mas enche o disco da VM no teste de carga. O intervalo da política vem de uma variável de sessão lida na migration com padrão 7 dias (`current_setting('tccelta.compressao_apos', true)`, aplicada com `ALTER DATABASE … SET` no compose de carga = `1 day`). A spec 10 roda a carga com compressão após 1 dia e chunk de 6 h.

#### `00007_papeis_extras.sql`

```sql
-- +goose Up
-- Papel do provisionamento (ferramenta administrativa, spec 07). Criado aqui
-- se o init do contêiner não o criou (bancos de teste).
-- +goose StatementBegin
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tccelta_provisionador') THEN
    CREATE ROLE tccelta_provisionador NOLOGIN;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tccelta_api')     THEN CREATE ROLE tccelta_api NOLOGIN; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tccelta_worker')  THEN CREATE ROLE tccelta_worker NOLOGIN; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tccelta_leitura') THEN CREATE ROLE tccelta_leitura NOLOGIN; END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
```

(O `00-papeis.sh` da spec 01 também cria `tccelta_provisionador LOGIN`, com a senha `pg_senha_provisionador`, que está na lista de segredos da spec 01.)

#### `00008_permissoes.sql`

```sql
-- +goose Up
GRANT USAGE ON SCHEMA public TO tccelta_api, tccelta_worker, tccelta_provisionador, tccelta_leitura;

-- API: bloco_bruto é append-only (sem UPDATE/DELETE); demais tabelas no mínimo necessário.
GRANT SELECT                 ON dongle, pid                                    TO tccelta_api;
GRANT SELECT, UPDATE         ON dongle_estado                                  TO tccelta_api;
GRANT SELECT, INSERT         ON bloco_bruto, ancora_tempo, leitura, posicao    TO tccelta_api;
GRANT SELECT, INSERT, UPDATE ON bloco_rejeitado, desafio, boot                 TO tccelta_api;
GRANT SELECT, INSERT         ON lacuna, alerta                                 TO tccelta_api;

-- Worker: lê o contexto e grava só a reta de tempo, a fila e os alertas.
GRANT SELECT                 ON dongle, dongle_estado, bloco_bruto, ancora_tempo, lacuna, pid TO tccelta_worker;
GRANT SELECT, UPDATE         ON boot                                           TO tccelta_worker;
GRANT SELECT, UPDATE (resultado) ON desafio                                    TO tccelta_worker;
GRANT SELECT, INSERT         ON alerta                                         TO tccelta_worker;

-- O salto administrativo (tccelta admin saltar, spec 05) roda como tccelta_dono, dono das tabelas.

-- Provisionamento: cria dongles e seu estado inicial.
GRANT SELECT, INSERT         ON dongle, dongle_estado                          TO tccelta_provisionador;

-- Leitura analítica (Grafana, notebooks).
GRANT SELECT ON ALL TABLES IN SCHEMA public TO tccelta_leitura;

-- Colunas identity usam sequences.
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO tccelta_api, tccelta_worker, tccelta_provisionador;

-- +goose Down
REVOKE ALL ON ALL TABLES    IN SCHEMA public FROM tccelta_api, tccelta_worker, tccelta_provisionador, tccelta_leitura;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM tccelta_api, tccelta_worker, tccelta_provisionador;
```

O TimescaleDB propaga os `GRANT` do hypertable para os chunks novos. Migrations futuras que criarem tabelas devem incluir os próprios `GRANT`s (regra de revisão de PR).

### Mecanismos transversais usados pelas outras specs

| Mecanismo | Onde | Para quê |
|---|---|---|
| `pg_advisory_xact_lock(1, dongle_id)` | ingestão (05) e resposta do handshake (04) | serializa as mudanças de `seq_contiguo` por dongle; o namespace `1` evita colisão com o lock do goose |
| `UPDATE … RETURNING` atômico | `ultimo_c_s` (04) e consumo do desafio (04) | `c_S` estritamente crescente; uso único do desafio |
| `pg_notify('boot_pendente', '<dongle_id>:<boot_id>')` dentro da transação | 04, 05 | entregue só no `COMMIT`; acorda o worker |
| `pg_notify('pid_alterado', '')` (trigger) | 00005 | recarrega o cache de PIDs da API |
| `FOR UPDATE SKIP LOCKED` | worker (06) | várias instâncias sem coordenação |
| `SET LOCAL lock_timeout = '3s'; SET LOCAL statement_timeout = '10s'` | início das transações de 04/05 | um lote preso no lock vira 503 + `Retry-After` em vez de fila infinita |

### Queries de cadastro (`internal/banco/queries/dongle.sql`, `pid.sql`)

```sql
-- name: BuscarDonglePorDevId :one
SELECT dongle_id, dev_id, dev_id_hex, perfil_seguranca, kms_chave_auth, kms_chave_rec,
       chave_publica, ativo, diag_solicitado_min
FROM dongle WHERE dev_id = @dev_id;

-- name: InserirDongle :one
INSERT INTO dongle (dev_id, perfil_seguranca, kms_chave_auth, kms_chave_rec, chave_publica, descricao)
VALUES (@dev_id, @perfil_seguranca, @kms_chave_auth, @kms_chave_rec, @chave_publica, @descricao)
RETURNING dongle_id;

-- name: InserirDongleEstado :exec
INSERT INTO dongle_estado (dongle_id) VALUES (@dongle_id);

-- name: ListarPids :many
SELECT pid, nome, unidade, n_bytes, fator, deslocamento, valor_min, valor_max FROM pid ORDER BY pid;
```

## Plano arquivo por arquivo

| Arquivo | Conteúdo |
|---|---|
| `backend/internal/banco/migrations/00001_extensoes.sql` … `00008_permissoes.sql` | DDL acima |
| `backend/internal/banco/migracoes.go` | `embed` + `Migrar(ctx, db)` + `Status` |
| `backend/internal/banco/queries/dongle.sql`, `pid.sql` | queries de cadastro |
| `backend/internal/banco/migracoes_test.go` (`//go:build integracao`) | up → down-to 0 → up num Timescale do testcontainers |
| `backend/cmd/tccelta/migrar.go` | subcomando `migrar [up|down-to N|status]` |
| `backend/deploy/postgres/init/00-papeis.sh` | (spec 01) + papel `tccelta_provisionador` |

## Testes

1. **Ida e volta das migrations** (`integracao`): sobe `timescale/timescaledb-ha:pg17` via testcontainers, roda `up`, confere `\d` de cada tabela (consultando `information_schema.columns`), roda `down-to 0`, roda `up` de novo.
2. **Invariantes:**
   - `UPDATE dongle_estado SET seq_contiguo = seq_contiguo - 1` falha com `check_violation`.
   - `UPDATE bloco_bruto SET payload = …` falha.
   - Inserir `lacuna` OVERFLOW sem `c_s` falha.
   - Inserir `leitura` com `pid = 99` falha por FK.
3. **Permissões:** conectado como `tccelta_api`, `DELETE FROM bloco_bruto` e `UPDATE bloco_bruto` falham com `insufficient_privilege`. Como `tccelta_worker`, `INSERT INTO bloco_bruto` falha.
4. **Hypertables:** `SELECT hypertable_name FROM timescaledb_information.hypertables` devolve `leitura` e `posicao`. `timescaledb_information.jobs` lista as duas políticas de compressão.
5. **NOTIFY de PID:** uma sessão faz `LISTEN pid_alterado` e outra faz `INSERT INTO pid …`. A notificação chega.
6. **Seed:** `SELECT count(*) FROM pid` = 7.

## Passos ordenados com checkpoints

1. Escrever `00001`–`00004` e `migracoes.go`. **Checkpoint 1:** `make migrar` aplica; `goose status` mostra 4 aplicadas.
2. Escrever `00005`–`00006`. **Checkpoint 2:** os hypertables existem e as políticas de compressão estão registradas.
3. Configurar `sqlc.yaml` (spec 03) apontando para `internal/banco/migrations`. **Checkpoint 3:** `make sqlc` gera sem erro, inclusive com `create_hypertable` e `geography` (override, spec 03).
4. Escrever `00007`–`00008` e o teste de permissões. **Checkpoint 4:** testes 1–6 verdes em `make teste-integracao`.

## Critérios de aceite

- [ ] Todas as tabelas do doc existem com as colunas do doc, mais as adições justificadas aqui (`dongle`, `bloco_rejeitado`, `alerta`, colunas extras de `boot`, `desafio` e `dongle_estado`).
- [ ] `pid` tem os 7 PIDs (0x04, 0x05, 0x0C, 0x0D, 0x10, 0x2F, 0x49).
- [ ] `leitura` e `posicao` são hypertables com compressão e índice `(dongle_id, boot_id, [pid,] t_ms)`.
- [ ] Ida e volta (`up`/`down`) funciona.
- [ ] A API não consegue alterar nem apagar `bloco_bruto`.

## Riscos

| Risco | Mitigação |
|---|---|
| `boot_id` é u16 e dá a volta após 65.536 boots; a PK `(dongle_id, boot_id)` colidiria | improvável no TCC; a ingestão detecta boot "reaparecendo" com `seq_primeiro` muito à frente de `seq_ultimo` do registro existente e gera o alerta `boot_id_reutilizado`; ver [Aberto] |
| `t_ms` é u32 e dá a volta após 49,7 dias de boot contínuo | guardado como `bigint`; a ingestão aplica *unwrap* por boot (spec 05) quando `t0_ms` cai mais de 2³¹ em relação ao último bloco do mesmo boot |
| `bloco_bruto` cresce rápido | [Aberto] particionamento/arquivamento |
| sqlc não entender sintaxe Timescale/PostGIS | Checkpoint 3 com plano B (`DO $$`) |

## [Aberto]

- **[Aberto] Particionar `bloco_bruto`.** Recomendação: quando passar de ~100 GB, particionar por `dongle_id` com hash (`PARTITION BY HASH (dongle_id)`, 16 partições); a PK continua válida. Outra opção é mover o `payload` para object storage (MinIO) e guardar o hash.
- **[Aberto] Encadeamento por hash dos blocos aceitos** (proteção contra o adversário AD5 no banco). Recomendação: coluna `hash_cadeia = sha256(hash_anterior || payload || assinatura)` por dongle e publicação periódica do último hash fora do banco. Fica fora do escopo mínimo.
- **[Aberto] `boot.config`.** O protocolo atual não envia a configuração do boot. Recomendação: a Fase 1 pode incluí-la num registro de configuração no primeiro bloco de cada boot. Até lá, a coluna fica nula.
- **[Aberto] Camada gold** (`viagem`, `amostra_alinhada`, `viagem_metricas`). Recomendação: fase posterior. A coluna `boot.pendente_gold` já existe como gancho.
- **[Aberto] Volta do `boot_id`.** Recomendação: se virar problema real, acrescentar uma `geracao_boot` derivada de `seq` à PK.
