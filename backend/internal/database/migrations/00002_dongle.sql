-- +goose Up
-- Cadastro dos dongles fabricados. DevId (8 bytes) é o identificador do
-- protocolo; dongle_id é a chave interna usada por todas as outras tabelas.
CREATE TABLE dongle (
  dongle_id          integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dev_id             bytea   NOT NULL UNIQUE CHECK (octet_length(dev_id) = 8),
  dev_id_hex         text    GENERATED ALWAYS AS (encode(dev_id, 'hex')) STORED UNIQUE,
  security_profile   text    NOT NULL DEFAULT 'hmac'
                         CHECK (security_profile IN ('hmac', 'asym', 'none')),
  kms_auth_key       text    NOT NULL,  -- nome da chave no mount transit-auth (= dev_id_hex)
  kms_rec_key        text    NOT NULL,  -- nome da chave no mount transit-rec  (= dev_id_hex)
  public_key         bytea   NULL CHECK (public_key IS NULL OR (octet_length(public_key) = 65 AND get_byte(public_key, 0) = 4)),  -- P-256 não comprimida 04‖X‖Y
  active             boolean NOT NULL DEFAULT true,
  requested_diag_min integer NULL CHECK (requested_diag_min BETWEEN 1 AND 65535),
  t_lat_s            integer NULL CHECK (t_lat_s BETWEEN 10 AND 300),   -- T_lat sugerido por dongle (reservado, ver abaixo)
  description        text    NULL,
  created_at         timestamptz NOT NULL DEFAULT now(),
  CHECK (security_profile <> 'asym' OR public_key IS NOT NULL)
);

-- +goose Down
DROP TABLE dongle;

