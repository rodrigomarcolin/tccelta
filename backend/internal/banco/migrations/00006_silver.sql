-- +goose Up
CREATE TABLE leitura (
  dongle_id    integer  NOT NULL,
  geracao_boot integer  NOT NULL,
  boot_id      integer  NOT NULL,
  t_ms         bigint   NOT NULL,          -- t0_ms do bloco + Δt
  pid          smallint NOT NULL REFERENCES pid (pid),
  valor_bruto  integer  NOT NULL,          -- A ou 256*A + B
  valor        real     NULL,             -- fator/deslocamento; NULL se fora da faixa
  seq_bloco    bigint   NOT NULL,
  recebido_em  timestamptz NOT NULL
);
SELECT create_hypertable('leitura', 'recebido_em', chunk_time_interval => INTERVAL '1 day');
CREATE INDEX leitura_viagem ON leitura (dongle_id, geracao_boot, boot_id, pid, t_ms);
ALTER TABLE leitura SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'dongle_id, pid',
  timescaledb.compress_orderby   = 'geracao_boot, boot_id, t_ms'
);
SELECT add_compression_policy('leitura',
  COALESCE(NULLIF(current_setting('tccelta.compressao_apos', true), ''), '7 days')::interval);

CREATE TABLE posicao (
  dongle_id    integer NOT NULL,
  geracao_boot integer NOT NULL,
  boot_id      integer NOT NULL,
  t_ms         bigint  NOT NULL,
  geom         geography(PointZ, 4326) NOT NULL,  -- lon/lat graus, alt metros
  origem       text    NOT NULL DEFAULT 'app',     -- GPS vem do app via AT#GPS, assinado pelo dongle
  seq_bloco    bigint  NOT NULL,
  recebido_em  timestamptz NOT NULL
);
SELECT create_hypertable('posicao', 'recebido_em', chunk_time_interval => INTERVAL '1 day');
CREATE INDEX posicao_viagem ON posicao (dongle_id, geracao_boot, boot_id, t_ms);
ALTER TABLE posicao SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'dongle_id',
  timescaledb.compress_orderby   = 'geracao_boot, boot_id, t_ms'
);
SELECT add_compression_policy('posicao',
  COALESCE(NULLIF(current_setting('tccelta.compressao_apos', true), ''), '7 days')::interval);

-- +goose Down
SELECT remove_compression_policy('posicao', if_exists => true);
DROP TABLE posicao;
SELECT remove_compression_policy('leitura', if_exists => true);
DROP TABLE leitura;
