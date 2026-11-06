-- +goose Up
-- Schema gold: a análise de cada viagem (spec .spec/analise-de-viagem/04 e API1). A viagem é pedida em gold.trip e o
-- worker (fila trip_pending) grava o resultado nas tabelas filhas; as séries e a rota ficam em jsonb porque têm null por elemento.
CREATE SCHEMA gold;

CREATE TABLE gold.trip (
  trip_id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  trip_category          text NOT NULL,
  driver_name            text NOT NULL,
  driver_name_normalized text NOT NULL,
  fuel                   text NOT NULL CHECK (fuel IN
                           ('gasolina_e30', 'gasolina_e32', 'gasolina_e27', 'gasolina_pura', 'etanol_hidratado', 'diesel')),
  dongle_id              integer NOT NULL REFERENCES dongle (dongle_id),
  entry_type             text NOT NULL CHECK (entry_type IN ('boot', 'interval')),
  boot_generation        integer NULL,   -- nula até o boot existir no servidor (o worker preenche)
  boot_id                integer NULL,
  interval_start         timestamptz NULL,
  interval_end           timestamptz NULL,
  status                 text NOT NULL CHECK (status IN ('pending', 'waiting_data', 'ready', 'failed')),
  target_seq             bigint NOT NULL,
  first_seq              bigint NULL,
  last_seq               bigint NULL,
  calculated_through_seq bigint NULL,
  provisional            boolean NOT NULL DEFAULT false,
  recalc_pending         boolean NOT NULL DEFAULT false,
  calculation_version    text NULL,
  created_at             timestamptz NOT NULL DEFAULT now(),
  calculated_at          timestamptz NULL,
  excluded_at            timestamptz NULL,
  CONSTRAINT trip_entry_coherent CHECK (
    (entry_type = 'boot' AND boot_id IS NOT NULL AND interval_start IS NULL AND interval_end IS NULL)
    OR
    (entry_type = 'interval' AND interval_start IS NOT NULL AND interval_end IS NOT NULL AND interval_start < interval_end
       AND boot_generation IS NULL AND boot_id IS NULL))
);
CREATE UNIQUE INDEX trip_driver_name_unique
  ON gold.trip (trip_category, driver_name_normalized)
  WHERE excluded_at IS NULL AND status <> 'failed';
CREATE INDEX trip_queue ON gold.trip (created_at) WHERE status IN ('pending', 'waiting_data') OR recalc_pending;
CREATE INDEX trip_provisional ON gold.trip (dongle_id) WHERE status = 'ready' AND provisional;

CREATE TABLE gold.trip_part (
  trip_id              uuid     NOT NULL REFERENCES gold.trip ON DELETE CASCADE,
  part_no              smallint NOT NULL,
  boot_generation      integer  NOT NULL,
  boot_id              integer  NOT NULL,
  data_start_t_ms      bigint   NOT NULL,
  data_end_t_ms        bigint   NOT NULL,
  start_t_ms           bigint   NULL,
  end_t_ms             bigint   NULL,
  clock_a_ms           double precision NULL,
  clock_b              double precision NULL,
  clock_uncertainty_ms integer  NULL,
  PRIMARY KEY (trip_id, part_no)
);

CREATE TABLE gold.trip_summary (
  trip_id                  uuid PRIMARY KEY REFERENCES gold.trip ON DELETE CASCADE,
  start_t_ms               bigint NULL,
  end_t_ms                 bigint NULL,
  started_at               timestamptz NULL,
  ended_at                 timestamptz NULL,
  clock_offset_ms          bigint NULL,
  clock_uncertainty_ms     integer NULL,
  warnings                 jsonb NOT NULL DEFAULT '[]',
  parameters               jsonb NOT NULL DEFAULT '{}',
  distance_km              double precision NULL,
  duration_s               double precision NULL,
  no_data_s                double precision NULL,
  moving_s                 double precision NULL,
  stopped_s                double precision NULL,
  avg_speed_kmh            double precision NULL,
  max_speed_kmh            double precision NULL,
  engine_on_s              double precision NULL,
  idle_s                   double precision NULL,
  idle_pct                 double precision NULL,
  fuel_l                   double precision NULL,
  km_per_l                 double precision NULL,
  intake_air_g             double precision NULL,
  n_hard_accel             integer NULL,
  n_hard_brake             integer NULL,
  hard_accel_per_100km     double precision NULL,
  hard_brake_per_100km     double precision NULL,
  pedal_mean_pct           double precision NULL,
  pedal_high_s             double precision NULL,
  pedal_rate_p95_pct_s     double precision NULL,
  engine_brake_pct         double precision NULL,
  accel_rms_ms2            double precision NULL,
  cruise_s                 double precision NULL,
  warmup_s                 double precision NULL,
  max_coolant_c            double precision NULL,
  cold_load_s              double precision NULL,
  idle_rpm                 double precision NULL,
  idle_rpm_origin          text NULL CHECK (idle_rpm_origin IN ('informada', 'estimada', 'reserva')),
  idle_margin_rpm          double precision NULL,
  neutral_clutch_s         double precision NULL,
  upshift_rpm_median       double precision NULL,
  upshift_rpm_p25          double precision NULL,
  upshift_rpm_p75          double precision NULL,
  upshift_rpm_n            integer NULL,
  downshift_rpm_median     double precision NULL,
  downshift_rpm_n          integer NULL,
  n_readings               bigint NULL,
  readings_per_s           double precision NULL,
  pid_periods_ms           jsonb NOT NULL DEFAULT '{}',
  fuel_coverage_pct        double precision NULL,
  gps_period_s             double precision NULL,
  gps_coverage_pct         double precision NULL,
  gps_n_gaps               integer NULL,
  gps_max_gap_s            double precision NULL,
  gps_distance_km          double precision NULL,
  gps_distance_diff_pct    double precision NULL
);

CREATE TABLE gold.trip_speed_band (
  trip_id      uuid NOT NULL REFERENCES gold.trip ON DELETE CASCADE,
  band_id      text NOT NULL,
  from_kmh     double precision NOT NULL,
  to_kmh       double precision NULL,
  time_s       double precision NOT NULL,
  distance_km  double precision NOT NULL,
  distance_pct double precision NULL,
  PRIMARY KEY (trip_id, band_id)
);

CREATE TABLE gold.trip_event (
  trip_id         uuid    NOT NULL REFERENCES gold.trip ON DELETE CASCADE,
  event_no        integer NOT NULL,
  type            text    NOT NULL,
  start_t_ms      bigint  NOT NULL,
  duration_s      double precision NOT NULL,
  peak            double precision NULL,
  peak_unit       text    NOT NULL,
  start_speed_kmh double precision NULL,
  end_speed_kmh   double precision NULL,
  lat             double precision NULL,
  lon             double precision NULL,
  PRIMARY KEY (trip_id, event_no)
);

CREATE TABLE gold.trip_stop (
  trip_id     uuid    NOT NULL REFERENCES gold.trip ON DELETE CASCADE,
  stop_no     integer NOT NULL,
  start_t_ms  bigint  NOT NULL,
  duration_s  double precision NOT NULL,
  engine_on   boolean NOT NULL,
  lat         double precision NULL,
  lon         double precision NULL,
  PRIMARY KEY (trip_id, stop_no)
);

CREATE TABLE gold.trip_gear (
  trip_id      uuid     NOT NULL REFERENCES gold.trip ON DELETE CASCADE,
  gear         smallint NOT NULL,
  rpm_per_kmh  double precision NOT NULL,
  time_s       double precision NOT NULL,
  time_pct     double precision NOT NULL,
  PRIMARY KEY (trip_id, gear)
);

CREATE TABLE gold.trip_gear_shift (
  trip_id    uuid     NOT NULL REFERENCES gold.trip ON DELETE CASCADE,
  shift_no   integer  NOT NULL,
  t_ms       bigint   NOT NULL,
  direction  text     NOT NULL CHECK (direction IN ('subida', 'reducao')),
  from_gear  smallint NOT NULL,
  to_gear    smallint NOT NULL,
  rpm_before double precision NOT NULL,
  rpm_after  double precision NOT NULL,
  speed_kmh  double precision NOT NULL,
  PRIMARY KEY (trip_id, shift_no)
);

CREATE TABLE gold.trip_series (
  trip_id            uuid PRIMARY KEY REFERENCES gold.trip ON DELETE CASCADE,
  step_ms            integer NOT NULL,
  exposure_step_ms   integer NOT NULL,
  series             jsonb NOT NULL,
  slow_series        jsonb NOT NULL,
  route              jsonb NOT NULL,
  rpm_distribution   jsonb NOT NULL,
  pedal_distribution jsonb NOT NULL,
  rpm_pedal_matrix   jsonb NOT NULL
);

-- O sqlc não conhece o PostGIS: as consultas leem lon/lat por estas funções.
CREATE FUNCTION position_lon(g geography) RETURNS double precision LANGUAGE sql IMMUTABLE AS 'SELECT ST_X(g::geometry)';
CREATE FUNCTION position_lat(g geography) RETURNS double precision LANGUAGE sql IMMUTABLE AS 'SELECT ST_Y(g::geometry)';

GRANT USAGE ON SCHEMA gold TO tccelta_api, tccelta_worker, tccelta_reader;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA gold TO tccelta_worker;
GRANT SELECT, INSERT ON gold.trip TO tccelta_api;
GRANT SELECT ON gold.trip_part, gold.trip_summary, gold.trip_speed_band, gold.trip_event, gold.trip_stop,
                gold.trip_gear, gold.trip_gear_shift, gold.trip_series TO tccelta_api;
GRANT SELECT ON ALL TABLES IN SCHEMA gold TO tccelta_reader;
-- tabelas criadas depois na gold (ranking etc.) também ficam legíveis; sem FOR ROLE, vale para quem migra
ALTER DEFAULT PRIVILEGES IN SCHEMA gold GRANT SELECT ON TABLES TO tccelta_reader;
-- A silver é só leitura para o worker (viagem); a 00008 não concedia isso.
GRANT SELECT ON reading, position TO tccelta_worker;

-- +goose Down
REVOKE SELECT ON reading, position FROM tccelta_worker;
DROP FUNCTION position_lat(geography);
DROP FUNCTION position_lon(geography);
ALTER DEFAULT PRIVILEGES IN SCHEMA gold REVOKE SELECT ON TABLES FROM tccelta_reader;
DROP SCHEMA gold CASCADE;
