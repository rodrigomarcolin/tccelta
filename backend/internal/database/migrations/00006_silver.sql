-- +goose Up
CREATE TABLE reading (
  dongle_id       integer  NOT NULL,
  boot_generation integer  NOT NULL,
  boot_id         integer  NOT NULL,
  t_ms            bigint   NOT NULL,          -- t0_ms do bloco + Δt
  pid             smallint NOT NULL REFERENCES pid (pid),
  raw_value       integer  NOT NULL,          -- A ou 256*A + B
  value           real     NULL,             -- factor/value_offset; NULL se fora da faixa
  block_seq       bigint   NOT NULL,
  received_at     timestamptz NOT NULL
);
SELECT create_hypertable('reading', 'received_at', chunk_time_interval => INTERVAL '1 day');
CREATE INDEX reading_trip ON reading (dongle_id, boot_generation, boot_id, pid, t_ms);
ALTER TABLE reading SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'dongle_id, pid',
  timescaledb.compress_orderby   = 'boot_generation, boot_id, t_ms'
);
SELECT add_compression_policy('reading',
  COALESCE(NULLIF(current_setting('tccelta.compression_after', true), ''), '7 days')::interval);

CREATE TABLE position (
  dongle_id       integer NOT NULL,
  boot_generation integer NOT NULL,
  boot_id         integer NOT NULL,
  t_ms            bigint  NOT NULL,
  geom            geography(PointZ, 4326) NOT NULL,  -- lon/lat graus, alt metros
  source          text    NOT NULL DEFAULT 'app',     -- GPS vem do app via AT#GPS, assinado pelo dongle
  block_seq       bigint  NOT NULL,
  received_at     timestamptz NOT NULL
);
SELECT create_hypertable('position', 'received_at', chunk_time_interval => INTERVAL '1 day');
CREATE INDEX position_trip ON position (dongle_id, boot_generation, boot_id, t_ms);
ALTER TABLE position SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'dongle_id',
  timescaledb.compress_orderby   = 'boot_generation, boot_id, t_ms'
);
SELECT add_compression_policy('position',
  COALESCE(NULLIF(current_setting('tccelta.compression_after', true), ''), '7 days')::interval);

-- +goose Down
SELECT remove_compression_policy('position', if_exists => true);
DROP TABLE position;
SELECT remove_compression_policy('reading', if_exists => true);
DROP TABLE reading;

