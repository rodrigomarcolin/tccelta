-- +goose Up
-- Uma linha por boot. Colunas de faixa são mantidas pela ingestão/handshake;
-- colunas de tempo são derivadas e recalculadas pelo worker (spec 06).
CREATE TABLE boot (
  dongle_id             integer NOT NULL REFERENCES dongle (dongle_id),
  geracao_boot          integer NOT NULL CHECK (geracao_boot BETWEEN 0 AND 65535),
  boot_id               integer NOT NULL CHECK (boot_id BETWEEN 0 AND 65535),
  config                jsonb   NULL,              -- protocolo, Tmax, PIDs e períodos: SEMPRE nula (o protocolo não envia a configuração; decisão do autor)
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
  PRIMARY KEY (dongle_id, geracao_boot, boot_id)
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
  geracao_boot            integer NULL,              -- geração atribuída ao boot_id da resposta (spec 04, passo 7g)
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
  dongle_id    integer NOT NULL,
  geracao_boot integer NOT NULL,
  boot_id      integer NOT NULL,
  t_ms         bigint  NOT NULL,
  nonce        bytea   NOT NULL UNIQUE REFERENCES desafio (nonce),
  s1           timestamptz NOT NULL,
  s2           timestamptz NOT NULL,
  PRIMARY KEY (dongle_id, geracao_boot, boot_id, t_ms),
  FOREIGN KEY (dongle_id, geracao_boot, boot_id) REFERENCES boot (dongle_id, geracao_boot, boot_id),
  CHECK (s2 >= s1)
);

-- Grupos de âncoras calculados pelo worker (spec 06): uma linha por grupo, regravada
-- junto com a reta do boot. Permite a incerteza por leitura sem refazer os grupos.
CREATE TABLE grupo_ancora (
  dongle_id     integer NOT NULL,
  geracao_boot  integer NOT NULL,
  boot_id       integer NOT NULL,
  ordem         integer NOT NULL CHECK (ordem >= 1),   -- posição do grupo por t_ms crescente
  t_primeiro_ms bigint  NOT NULL,                       -- t_ms da primeira âncora do grupo
  t_ultimo_ms   bigint  NOT NULL,
  t_centro_ms   double precision NOT NULL,              -- média dos t_ms (TCentro)
  lo_ms         double precision NOT NULL,              -- limites do deslocamento o = hora_real − t_ms
  hi_ms         double precision NOT NULL,
  n_ancoras     integer NOT NULL CHECK (n_ancoras >= 1),
  calculado_em  timestamptz NOT NULL,
  PRIMARY KEY (dongle_id, geracao_boot, boot_id, ordem),
  FOREIGN KEY (dongle_id, geracao_boot, boot_id) REFERENCES boot (dongle_id, geracao_boot, boot_id) ON DELETE CASCADE,
  CHECK (hi_ms >= lo_ms)
);

-- Lacunas declaradas: OVERFLOW (handshake) ou BOOT/LOSS (blocos marcadores).
-- Não há causa administrativa: não existe salto manual de fila travada (decisão do autor, spec 05).
CREATE TABLE lacuna (
  id             bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dongle_id      integer NOT NULL REFERENCES dongle (dongle_id),
  from_seq       bigint  NOT NULL,
  to_seq         bigint  NOT NULL,
  causa          text    NOT NULL CHECK (causa IN ('BOOT', 'LOSS', 'OVERFLOW')),
  registrado_em  timestamptz NOT NULL,
  c_s            bigint  NULL,   -- handshake que declarou (OVERFLOW)
  seq_bloco      bigint  NULL,   -- bloco marcador que declarou (BOOT/LOSS)
  CHECK (to_seq >= from_seq),
  CHECK ((causa = 'OVERFLOW') = (c_s IS NOT NULL)),
  CHECK ((causa IN ('BOOT', 'LOSS')) = (seq_bloco IS NOT NULL)),
  FOREIGN KEY (dongle_id, c_s)       REFERENCES desafio (dongle_id, c_s),
  FOREIGN KEY (dongle_id, seq_bloco) REFERENCES bloco_bruto (dongle_id, seq),
  UNIQUE (dongle_id, from_seq)    -- duas lacunas não começam no mesmo seq
);

-- Alertas operacionais (deriva, interseção vazia, PID desconhecido, etc.).
CREATE TABLE alerta (
  id         bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dongle_id    integer NOT NULL REFERENCES dongle (dongle_id),
  geracao_boot integer NULL,
  boot_id      integer NULL,
  tipo         text    NOT NULL CHECK (tipo IN
               ('deriva_excessiva', 'intersecao_vazia', 'relogio_servidor_regrediu', 'pid_desconhecido',
                'tag_invalida', 'marcador_invalido', 'geracao_boot_avancou', 'invariante_violada',
                'reprocessamento')),
  detalhe    jsonb   NOT NULL DEFAULT '{}',
  criado_em  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX alerta_recentes ON alerta (dongle_id, criado_em DESC);

-- +goose Down
DROP TABLE alerta;
DROP TABLE lacuna;
DROP TABLE grupo_ancora;
DROP TABLE ancora_tempo;
DROP TABLE desafio;
DROP TABLE boot;
