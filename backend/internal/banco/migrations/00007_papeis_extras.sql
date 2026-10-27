-- +goose Up
-- Papel do provisionamento (ferramenta administrativa, spec 07). Criado aqui
-- se o init do contêiner não o criou (bancos de teste).
-- +goose StatementBegin
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tccelta_provisionador') THEN
    CREATE ROLE tccelta_provisionador NOLOGIN;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tccelta_api')     THEN CREATE ROLE tccelta_api NOLOGIN; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tccelta_worker')  THEN CREATE ROLE tccelta_worker NOLOGIN; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tccelta_leitura') THEN CREATE ROLE tccelta_leitura NOLOGIN; END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
