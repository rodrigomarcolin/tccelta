-- +goose Up
-- As extensões são criadas pelo init do contêiner (superusuário). Aqui só
-- garantimos que existem; em bancos de teste (testcontainers) o usuário é
-- superusuário e elas são criadas de fato.
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS postgis;

-- +goose Down
SELECT 1; -- extensões não são removidas por migration
