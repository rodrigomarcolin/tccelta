-- +goose Up
-- Uma mesma viagem (boot ou intervalo exato de um dongle) só pode ser processada uma vez, para qualquer motorista, e a
-- listagem de viagens (GET /v1/viagens) ganha o índice da sua ordem. Viagens failed e excluídas não contam, como no nome do motorista.

-- 1. A API grava boot_generation nula quando o boot ainda não existe e só o worker a preenche: resolve já as que podem ser
-- resolvidas, para a chave abaixo valer também para elas.
UPDATE gold.trip t SET boot_generation = g.generation
FROM (SELECT dongle_id, boot_id, max(boot_generation) AS generation FROM boot GROUP BY dongle_id, boot_id) g
WHERE t.entry_type = 'boot' AND t.boot_generation IS NULL AND g.dongle_id = t.dongle_id AND g.boot_id = t.boot_id;

-- 2. Duplicatas já existentes: fica uma viagem por chave (a pronta, depois a mais antiga) e as outras recebem excluded_at.
-- As linhas não são apagadas, mas passam a dar 404 e somem da listagem; o Down não as restaura.
WITH ranked AS (
  SELECT trip_id, row_number() OVER (
           PARTITION BY dongle_id, boot_generation, boot_id
           ORDER BY (status = 'ready') DESC, created_at, trip_id) AS rn
  FROM gold.trip
  WHERE entry_type = 'boot' AND excluded_at IS NULL AND status <> 'failed' AND boot_generation IS NOT NULL)
UPDATE gold.trip t SET excluded_at = now() FROM ranked WHERE t.trip_id = ranked.trip_id AND ranked.rn > 1;

WITH ranked AS (
  SELECT trip_id, row_number() OVER (
           PARTITION BY dongle_id, interval_start, interval_end
           ORDER BY (status = 'ready') DESC, created_at, trip_id) AS rn
  FROM gold.trip
  WHERE entry_type = 'interval' AND excluded_at IS NULL AND status <> 'failed')
UPDATE gold.trip t SET excluded_at = now() FROM ranked WHERE t.trip_id = ranked.trip_id AND ranked.rn > 1;

-- 3. Os índices. Boot com boot_generation nula não colide (NULL é distinto); a API resolve a geração antes de gravar.
CREATE UNIQUE INDEX trip_boot_unique ON gold.trip (dongle_id, boot_generation, boot_id)
  WHERE entry_type = 'boot' AND excluded_at IS NULL AND status <> 'failed';
CREATE UNIQUE INDEX trip_interval_unique ON gold.trip (dongle_id, interval_start, interval_end)
  WHERE entry_type = 'interval' AND excluded_at IS NULL AND status <> 'failed';
CREATE INDEX trip_list ON gold.trip (created_at DESC, trip_id DESC) WHERE excluded_at IS NULL;

-- +goose Down
-- As viagens excluídas pela migração continuam excluídas.
DROP INDEX gold.trip_list;
DROP INDEX gold.trip_interval_unique;
DROP INDEX gold.trip_boot_unique;
