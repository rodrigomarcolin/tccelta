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
