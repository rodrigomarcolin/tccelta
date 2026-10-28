-- +goose Up
-- Fonte da verdade dos PIDs: o que cada um mede e como decodificar.
-- value = (raw * factor) + value_offset, com raw = A (1 byte) ou 256*A + B (2 bytes).
-- 0xFD (LOSS), 0xFE (BOOT) e 0xFF (GPS) são tipos de registro reservados.
CREATE TABLE pid (
  pid          smallint PRIMARY KEY CHECK (pid BETWEEN 0 AND 252),
  name         text NOT NULL,
  unit         text NOT NULL,
  n_bytes      smallint NOT NULL CHECK (n_bytes IN (1, 2)),
  factor       double precision NOT NULL,
  value_offset double precision NOT NULL DEFAULT 0,
  min_value    double precision NULL,   -- faixa válida: fora dela, reading.value = NULL
  max_value    double precision NULL
);

INSERT INTO pid (pid, name, unit, n_bytes, factor, value_offset, min_value, max_value) VALUES
  (4,  'Carga do motor',         '%',    1, 100.0/255, 0,   0,   100),        -- 0x04
  (5,  'Temperatura do líquido', '°C',   1, 1,         -40, -40, 215),        -- 0x05
  (12, 'Rotação',                'RPM',  2, 0.25,      0,   0,   16383.75),   -- 0x0C
  (13, 'Velocidade',             'km/h', 1, 1,         0,   0,   255),        -- 0x0D
  (16, 'MAF',                    'g/s',  2, 0.01,      0,   0,   655.35),     -- 0x10
  (47, 'Nível de combustível',   '%',    1, 100.0/255, 0,   0,   100),        -- 0x2F
  (73, 'Pedal do acelerador',    '%',    1, 100.0/255, 0,   0,   100);        -- 0x49

-- Avisa a API para recarregar o cache de PIDs (spec 05) sem novo deploy.
-- +goose StatementBegin
CREATE FUNCTION notify_pid_changed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM pg_notify('pid_changed', '');
  RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER pid_changed AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON pid
  FOR EACH STATEMENT EXECUTE FUNCTION notify_pid_changed();

-- +goose Down
DROP TRIGGER pid_changed ON pid;
DROP FUNCTION notify_pid_changed();
DROP TABLE pid;

