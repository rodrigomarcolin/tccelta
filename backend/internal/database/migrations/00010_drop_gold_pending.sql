-- +goose Up
-- gold_pending marcava o boot inteiro a cada lote e nunca teve consumidor; o gatilho do recálculo agora é gold.trip.recalc_pending.
-- Só aplicar depois que nenhuma réplica antiga da API estiver viva: ela ainda grava a coluna em UpdateIngestionBoots.
ALTER TABLE boot DROP COLUMN gold_pending;

-- +goose Down
ALTER TABLE boot ADD COLUMN gold_pending boolean NOT NULL DEFAULT false;
