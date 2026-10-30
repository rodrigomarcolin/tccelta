-- name: ListDataBlocks :many
SELECT seq, boot_generation, boot_id, t0_ms, payload, received_at FROM raw_block
WHERE dongle_id = @dongle_id AND marker IS NULL AND seq >= @from_seq AND seq <= @to_seq
ORDER BY seq
LIMIT @row_limit;

-- name: DeleteBlockReadings :execrows
DELETE FROM reading
WHERE dongle_id = @dongle_id AND block_seq >= @from_seq AND block_seq <= @to_seq
  AND received_at >= @received_min AND received_at <= @received_max;

-- name: DeleteBlockPositions :execrows
DELETE FROM position
WHERE dongle_id = @dongle_id AND block_seq >= @from_seq AND block_seq <= @to_seq
  AND received_at >= @received_min AND received_at <= @received_max;

-- name: MarkGoldBoots :exec
UPDATE boot SET gold_pending = true
WHERE dongle_id = @dongle_id AND first_seq <= @to_seq AND last_seq >= @from_seq;

-- name: CountReadings :one
SELECT count(*) FROM reading WHERE dongle_id = @dongle_id AND block_seq >= @from_seq AND block_seq <= @to_seq;
