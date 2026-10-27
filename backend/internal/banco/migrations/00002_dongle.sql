-- +goose Up
-- Cadastro dos dongles fabricados. DevId (8 bytes) é o identificador do
-- protocolo; dongle_id é a chave interna usada por todas as outras tabelas.
CREATE TABLE dongle (
  dongle_id            integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  dev_id               bytea   NOT NULL UNIQUE CHECK (octet_length(dev_id) = 8),
  dev_id_hex           text    GENERATED ALWAYS AS (encode(dev_id, 'hex')) STORED UNIQUE,
  perfil_seguranca     text    NOT NULL DEFAULT 'hmac'
                         CHECK (perfil_seguranca IN ('hmac', 'asym', 'none')),
  kms_chave_auth       text    NOT NULL,  -- nome da chave no mount transit-auth (= dev_id_hex)
  kms_chave_rec        text    NOT NULL,  -- nome da chave no mount transit-rec  (= dev_id_hex)
  chave_publica        bytea   NULL CHECK (chave_publica IS NULL OR (octet_length(chave_publica) = 65 AND get_byte(chave_publica, 0) = 4)),  -- P-256 não comprimida 04‖X‖Y
  ativo                boolean NOT NULL DEFAULT true,
  diag_solicitado_min  integer NULL CHECK (diag_solicitado_min BETWEEN 1 AND 65535),
  t_lat_s              integer NULL CHECK (t_lat_s BETWEEN 10 AND 300),   -- T_lat sugerido por dongle (reservado, ver abaixo)
  descricao            text    NULL,
  criado_em            timestamptz NOT NULL DEFAULT now(),
  CHECK (perfil_seguranca <> 'asym' OR chave_publica IS NOT NULL)
);

-- +goose Down
DROP TABLE dongle;
