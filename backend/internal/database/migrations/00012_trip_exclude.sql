-- +goose Up
-- EXC1: a API exclui a viagem (DELETE /v1/viagens/{id}) preenchendo excluded_at. O GRANT é por coluna: o tccelta_api continua sem
-- poder mudar status, nome ou entrada.
GRANT UPDATE (excluded_at) ON gold.trip TO tccelta_api;

-- +goose Down
REVOKE UPDATE (excluded_at) ON gold.trip FROM tccelta_api;
