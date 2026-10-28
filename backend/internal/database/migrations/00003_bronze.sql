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
  geracao_boot      integer NOT NULL DEFAULT 0 CHECK (geracao_boot BETWEEN 0 AND 65535),   -- geração do boot_id (u16 dá a volta), ver abaixo
  ultimo_boot_id    integer NOT NULL DEFAULT 0 CHECK (ultimo_boot_id BETWEEN 0 AND 65535), -- último boot_id ≠ 0 aceito na geração atual
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
  -- a geração só sobe; dentro da mesma geração o boot_id só sobe (a volta do u16 é uma geração nova)
  IF NEW.geracao_boot < OLD.geracao_boot
     OR (NEW.geracao_boot = OLD.geracao_boot AND NEW.ultimo_boot_id < OLD.ultimo_boot_id) THEN
    RAISE EXCEPTION 'dongle_estado: geração/boot_id regrediu (dongle_id=%)', OLD.dongle_id
      USING ERRCODE = 'check_violation';
  END IF;
  NEW.atualizado_em := now();
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER dongle_estado_monotono BEFORE UPDATE ON dongle_estado
  FOR EACH ROW EXECUTE FUNCTION dongle_estado_monotono();

-- Blocos aceitos, exatamente como vieram. Append-only para a aplicação.
-- Particionada por hash de dongle_id (16 partições) desde já; a PK inclui a chave de partição.
CREATE TABLE bloco_bruto (
  dongle_id    integer NOT NULL REFERENCES dongle (dongle_id),
  seq          bigint  NOT NULL CHECK (seq BETWEEN 1 AND 4294967295),
  boot_id      integer NOT NULL CHECK (boot_id BETWEEN 0 AND 65535),
  geracao_boot integer NOT NULL DEFAULT 0 CHECK (geracao_boot BETWEEN 0 AND 65535),   -- derivada na ingestão (spec 05); LOSS (boot_id 0) leva a geração corrente
  t0_ms        bigint  NOT NULL,
  marcador     text    NULL CHECK (marcador IN ('BOOT', 'LOSS')),
  epoca        bigint  NULL,      -- c_S do handshake cujo JWT trouxe o bloco (claim "cs")
  payload      bytea   NOT NULL CHECK (octet_length(payload) BETWEEN 17 AND 1024), -- área de dados (tamanho bytes)
  assinatura   bytea   NOT NULL CHECK (octet_length(assinatura) IN (0, 32, 64)),   -- tag K_rec (32) | ECDSA P-256 r‖s (64) | none (0)
  recebido_em  timestamptz NOT NULL,
  PRIMARY KEY (dongle_id, seq)    -- idempotência: replay vira no-op
) PARTITION BY HASH (dongle_id);

-- +goose StatementBegin
DO $$ BEGIN
  FOR i IN 0..15 LOOP
    EXECUTE format('CREATE TABLE bloco_bruto_p%s PARTITION OF bloco_bruto FOR VALUES WITH (MODULUS 16, REMAINDER %s)', i, i);
  END LOOP;
END $$;
-- +goose StatementEnd

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
