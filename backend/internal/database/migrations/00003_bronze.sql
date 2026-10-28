-- +goose Up
-- Estado operacional de cada dongle visto pelo servidor.
CREATE TABLE dongle_state (
  dongle_id        integer PRIMARY KEY REFERENCES dongle (dongle_id),
  contiguous_seq   bigint  NOT NULL DEFAULT 0 CHECK (contiguous_seq BETWEEN 0 AND 4294967295), -- ACK_seq
  max_seen_seq     bigint  NOT NULL DEFAULT 0,  -- maior seq com tag válida já visto (aceito ou não; spec 05 verifica o lote inteiro)
  head_seq         bigint  NOT NULL DEFAULT 0,  -- HEAD_seq do último handshake, ou maior seq recebido
  tail_seq         bigint  NULL,                -- TAIL_seq do último handshake aceito
  head_reported_at timestamptz NULL,            -- S2 do handshake que relatou head e tail
  last_c_s         bigint  NOT NULL DEFAULT 0 CHECK (last_c_s BETWEEN 0 AND 4294967295),
  c_conf           bigint  NOT NULL DEFAULT 0,  -- c_S do último handshake aceito
  boot_generation  integer NOT NULL DEFAULT 0 CHECK (boot_generation BETWEEN 0 AND 65535),   -- geração do boot_id (u16 dá a volta), ver abaixo
  last_boot_id     integer NOT NULL DEFAULT 0 CHECK (last_boot_id BETWEEN 0 AND 65535), -- último boot_id ≠ 0 aceito na geração atual
  last_ack         bytea   NULL,                -- TLV 0x04 completo e assinado (GET /confirmacao)
  last_ack_at      timestamptz NULL,
  updated_at       timestamptz NOT NULL DEFAULT now(),
  CHECK (contiguous_seq <= head_seq),
  CHECK (contiguous_seq <= max_seen_seq),
  CHECK (c_conf <= last_c_s)
);

-- Invariantes de monotonicidade: nenhum caminho de código pode regredir a
-- fronteira, o HEAD ou os contadores do handshake.
-- +goose StatementBegin
CREATE FUNCTION dongle_state_monotonic() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.contiguous_seq < OLD.contiguous_seq OR NEW.head_seq < OLD.head_seq
     OR NEW.last_c_s < OLD.last_c_s OR NEW.c_conf < OLD.c_conf
     OR NEW.max_seen_seq < OLD.max_seen_seq THEN
    RAISE EXCEPTION 'dongle_state: regressão proibida (dongle_id=%)', OLD.dongle_id
      USING ERRCODE = 'check_violation';
  END IF;
  -- a geração só sobe; dentro da mesma geração o boot_id só sobe (a volta do u16 é uma geração nova)
  IF NEW.boot_generation < OLD.boot_generation
     OR (NEW.boot_generation = OLD.boot_generation AND NEW.last_boot_id < OLD.last_boot_id) THEN
    RAISE EXCEPTION 'dongle_state: geração/boot_id regrediu (dongle_id=%)', OLD.dongle_id
      USING ERRCODE = 'check_violation';
  END IF;
  NEW.updated_at := now();
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER dongle_state_monotonic BEFORE UPDATE ON dongle_state
  FOR EACH ROW EXECUTE FUNCTION dongle_state_monotonic();

-- Blocos aceitos, exatamente como vieram. Append-only para a aplicação.
-- Particionada por hash de dongle_id (16 partições) desde já; a PK inclui a chave de partição.
CREATE TABLE raw_block (
  dongle_id       integer NOT NULL REFERENCES dongle (dongle_id),
  seq             bigint  NOT NULL CHECK (seq BETWEEN 1 AND 4294967295),
  boot_id         integer NOT NULL CHECK (boot_id BETWEEN 0 AND 65535),
  boot_generation integer NOT NULL DEFAULT 0 CHECK (boot_generation BETWEEN 0 AND 65535),   -- derivada na ingestão (spec 05); LOSS (boot_id 0) leva a geração corrente
  t0_ms           bigint  NOT NULL,
  marker          text    NULL CHECK (marker IN ('BOOT', 'LOSS')),
  epoch           bigint  NULL,      -- c_S do handshake cujo JWT trouxe o bloco (claim "cs")
  payload         bytea   NOT NULL CHECK (octet_length(payload) BETWEEN 17 AND 1024), -- área de dados (tamanho bytes)
  signature       bytea   NOT NULL CHECK (octet_length(signature) IN (0, 32, 64)),   -- tag K_rec (32) | ECDSA P-256 r‖s (64) | none (0)
  received_at     timestamptz NOT NULL,
  PRIMARY KEY (dongle_id, seq)    -- idempotência: replay vira no-op
) PARTITION BY HASH (dongle_id);

-- +goose StatementBegin
DO $$ BEGIN
  FOR i IN 0..15 LOOP
    EXECUTE format('CREATE TABLE raw_block_p%s PARTITION OF raw_block FOR VALUES WITH (MODULUS 16, REMAINDER %s)', i, i);
  END LOOP;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION raw_block_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'raw_block é imutável' USING ERRCODE = 'insufficient_privilege';
END $$;
-- +goose StatementEnd
CREATE TRIGGER raw_block_immutable BEFORE UPDATE ON raw_block
  FOR EACH ROW EXECUTE FUNCTION raw_block_immutable();

-- Blocos recusados (tag inválida, PID desconhecido, marcador fora da regra,
-- registro malformado). Campos do cabeçalho são "como declarados": não confiáveis.
CREATE TABLE rejected_block (
  id           bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dongle_id    integer NOT NULL REFERENCES dongle (dongle_id),
  seq          bigint  NULL,
  boot_id      integer NULL,
  t0_ms        bigint  NULL,
  payload      bytea   NOT NULL,
  signature    bytea   NULL,
  reason       text    NOT NULL CHECK (reason IN
                  ('invalid_tag', 'unknown_pid', 'invalid_marker', 'malformed_record')),
  detail       text    NULL,
  content_hash bytea   NOT NULL CHECK (octet_length(content_hash) = 32), -- sha256(payload || signature)
  token_jti    uuid    NULL,
  received_at  timestamptz NOT NULL,
  occurrences  integer NOT NULL DEFAULT 1,
  UNIQUE (dongle_id, content_hash)  -- reenvio do mesmo bloco ruim só incrementa occurrences
);
CREATE INDEX rejected_block_recent ON rejected_block (dongle_id, received_at DESC);

-- +goose Down
DROP TABLE rejected_block;
DROP TABLE raw_block;
DROP FUNCTION raw_block_immutable();
DROP TABLE dongle_state;
DROP FUNCTION dongle_state_monotonic();

