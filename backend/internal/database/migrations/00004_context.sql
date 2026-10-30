-- +goose Up
-- Uma linha por boot. Colunas de faixa são mantidas pela ingestão/handshake;
-- colunas de tempo são derivadas e recalculadas pelo worker (spec 06).
CREATE TABLE boot (
  dongle_id           integer NOT NULL REFERENCES dongle (dongle_id),
  boot_generation     integer NOT NULL CHECK (boot_generation BETWEEN 0 AND 65535),
  boot_id             integer NOT NULL CHECK (boot_id BETWEEN 0 AND 65535),
  config              jsonb   NULL,              -- protocolo, Tmax, PIDs e períodos: SEMPRE nula (o protocolo não envia a configuração; decisão do autor)
  -- faixa observada
  first_seq           bigint  NULL,
  last_seq            bigint  NULL,
  first_t_ms          bigint  NULL,              -- menor t_ms de registro visto neste boot
  last_t_ms           bigint  NULL,              -- maior t_ms de registro visto neste boot
  max_offset_ms       double precision NULL,     -- min(received_at − t_ms_fim_do_bloco): o ≤ isto
  -- reta de tempo: hora_real_ms(t) = a_ms + (1 + b) * t_ms
  a_ms                double precision NULL,
  b                   double precision NULL,
  base_uncertainty_ms integer NULL,
  time_source         text    NULL CHECK (time_source IN ('anchors', 'neighbors')),
  time_updated_at     timestamptz NULL,
  -- fila do worker
  time_pending        boolean NOT NULL DEFAULT true,
  gold_pending        boolean NOT NULL DEFAULT false,
  pending_since       timestamptz NOT NULL DEFAULT now(),
  created_at          timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (dongle_id, boot_generation, boot_id)
);
CREATE INDEX boot_time_queue ON boot (pending_since) WHERE time_pending;
CREATE INDEX boot_seq_order  ON boot (dongle_id, first_seq);

-- Uma linha por desafio emitido: o rastro de cada c_S e N_S.
CREATE TABLE challenge (
  dongle_id            integer NOT NULL REFERENCES dongle (dongle_id),
  c_s                  bigint  NOT NULL CHECK (c_s BETWEEN 1 AND 4294967295),
  nonce                bytea   NOT NULL UNIQUE CHECK (octet_length(nonce) = 16),
  s1                   timestamptz NOT NULL,      -- relógio da API, antes do INSERT
  expires_at           timestamptz NOT NULL,
  issued_ack_seq       bigint  NOT NULL,          -- ACK_seq levado no Desafio 0x01
  issued_c_conf        bigint  NOT NULL,
  flags_c              smallint NOT NULL DEFAULT 0,
  diag_min             integer NULL,
  -- preenchidos com a Resposta 0x02
  answered_at          timestamptz NULL,          -- S2; NULL enquanto pendente
  boot_generation      integer NULL,              -- geração atribuída ao boot_id da resposta (spec 04, passo 7g)
  boot_id              integer NULL,
  t_ms                 bigint  NULL,
  head_seq             bigint  NULL,
  tail_seq             bigint  NULL,
  result               text    NULL CHECK (result IN
                            ('accepted', 'accepted_no_anchor', 'invalid_tag', 'inconsistent', 'expired')),
  invalid_tag_attempts integer NOT NULL DEFAULT 0,
  -- efeitos de um handshake aceito
  ack_seq              bigint  NULL,              -- ACK_seq devolvido no ACK 0x04
  loss_start           bigint  NULL,              -- faixa OVERFLOW, se houve
  loss_end             bigint  NULL,
  token_jti            uuid    NULL UNIQUE,
  PRIMARY KEY (dongle_id, c_s)
);
CREATE INDEX challenge_pending ON challenge (dongle_id, expires_at) WHERE answered_at IS NULL;
CREATE INDEX challenge_to_expire ON challenge (expires_at) WHERE answered_at IS NULL AND result IS NULL;

-- Uma linha por handshake com S2 − S1 ≤ 2 s: a medida crua.
-- O deslocamento do boot está em [s1 − t_ms, s2 − t_ms].
CREATE TABLE time_anchor (
  dongle_id       integer NOT NULL,
  boot_generation integer NOT NULL,
  boot_id         integer NOT NULL,
  t_ms            bigint  NOT NULL,
  nonce           bytea   NOT NULL UNIQUE REFERENCES challenge (nonce),
  s1              timestamptz NOT NULL,
  s2              timestamptz NOT NULL,
  PRIMARY KEY (dongle_id, boot_generation, boot_id, t_ms),
  FOREIGN KEY (dongle_id, boot_generation, boot_id) REFERENCES boot (dongle_id, boot_generation, boot_id),
  CHECK (s2 >= s1)
);

-- Grupos de âncoras calculados pelo worker (spec 06): uma linha por grupo, regravada
-- junto com a reta do boot. Permite a incerteza por leitura sem refazer os grupos.
CREATE TABLE anchor_group (
  dongle_id       integer NOT NULL,
  boot_generation integer NOT NULL,
  boot_id         integer NOT NULL,
  ordinal         integer NOT NULL CHECK (ordinal >= 1),   -- posição do grupo por t_ms crescente
  first_t_ms      bigint  NOT NULL,                       -- t_ms da primeira âncora do grupo
  last_t_ms       bigint  NOT NULL,
  t_center_ms     double precision NOT NULL,              -- média dos t_ms (TCentro)
  lo_ms           double precision NOT NULL,              -- limites do deslocamento o = hora_real − t_ms
  hi_ms           double precision NOT NULL,
  n_anchors       integer NOT NULL CHECK (n_anchors >= 1),
  computed_at     timestamptz NOT NULL,
  PRIMARY KEY (dongle_id, boot_generation, boot_id, ordinal),
  FOREIGN KEY (dongle_id, boot_generation, boot_id) REFERENCES boot (dongle_id, boot_generation, boot_id) ON DELETE CASCADE,
  CHECK (hi_ms >= lo_ms)
);

-- Lacunas declaradas: OVERFLOW (handshake) ou BOOT/LOSS (blocos marcadores).
-- Não há causa administrativa: não existe salto manual de fila travada (decisão do autor, spec 05).
CREATE TABLE gap (
  id            bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dongle_id     integer NOT NULL REFERENCES dongle (dongle_id),
  from_seq      bigint  NOT NULL,
  to_seq        bigint  NOT NULL,
  cause         text    NOT NULL CHECK (cause IN ('BOOT', 'LOSS', 'OVERFLOW')),
  registered_at timestamptz NOT NULL,
  c_s           bigint  NULL,   -- handshake que declarou (OVERFLOW)
  block_seq     bigint  NULL,   -- bloco marcador que declarou (BOOT/LOSS)
  CHECK (to_seq >= from_seq),
  CHECK ((cause = 'OVERFLOW') = (c_s IS NOT NULL)),
  CHECK ((cause IN ('BOOT', 'LOSS')) = (block_seq IS NOT NULL)),
  FOREIGN KEY (dongle_id, c_s)       REFERENCES challenge (dongle_id, c_s),
  FOREIGN KEY (dongle_id, block_seq) REFERENCES raw_block (dongle_id, seq),
  UNIQUE (dongle_id, from_seq)    -- duas lacunas não começam no mesmo seq
);

-- Alertas operacionais (deriva, interseção vazia, PID desconhecido, etc.).
CREATE TABLE alert (
  id              bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dongle_id       integer NOT NULL REFERENCES dongle (dongle_id),
  boot_generation integer NULL,
  boot_id         integer NULL,
  type            text    NOT NULL CHECK (type IN
               ('excessive_drift', 'empty_intersection', 'server_clock_regressed', 'unknown_pid',
                'invalid_tag', 'invalid_marker', 'boot_generation_advanced', 'invariant_violated',
                'reprocessing')),
  detail          jsonb   NOT NULL DEFAULT '{}',
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX alert_recent ON alert (dongle_id, created_at DESC);

-- +goose Down
DROP TABLE alert;
DROP TABLE gap;
DROP TABLE anchor_group;
DROP TABLE time_anchor;
DROP TABLE challenge;
DROP TABLE boot;

