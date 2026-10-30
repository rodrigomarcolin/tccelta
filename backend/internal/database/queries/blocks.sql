-- name: ReadStateSnapshot :one
SELECT contiguous_seq, c_conf, last_ack FROM dongle_state WHERE dongle_id = @dongle_id;

-- name: ReadLastBootTMs :many
-- Devolve as linhas candidatas nas geracoes G e G+1; o chamador escolhe a certa (atribuirGeracao).
SELECT boot_generation, boot_id, last_t_ms FROM boot
WHERE dongle_id = @dongle_id AND boot_id = ANY(@boot_ids::int[])
  AND boot_generation IN (@generation_a, @generation_b);

-- name: CopyReadings :copyfrom
INSERT INTO reading (dongle_id, boot_generation, boot_id, t_ms, pid, raw_value, value, block_seq, received_at)
VALUES (@dongle_id, @boot_generation, @boot_id, @t_ms, @pid, @raw_value, @value, @block_seq, @received_at);

-- name: CopyPositions :copyfrom
INSERT INTO position (dongle_id, boot_generation, boot_id, t_ms, geom, source, block_seq, received_at)
VALUES (@dongle_id, @boot_generation, @boot_id, @t_ms, @geom, @source, @block_seq, @received_at);

-- name: RegisterRejected :exec
INSERT INTO rejected_block (dongle_id, seq, boot_id, t0_ms, payload, signature, reason, detail,
                             content_hash, token_jti, received_at)
VALUES (@dongle_id, @seq, @boot_id, @t0_ms, @payload, @signature, @reason, @detail,
        @content_hash, @token_jti, @received_at)
ON CONFLICT (dongle_id, content_hash) DO UPDATE
   SET occurrences = rejected_block.occurrences + 1, received_at = EXCLUDED.received_at;

-- name: UpdateIngestionState :exec
UPDATE dongle_state SET
  contiguous_seq    = @contiguous_seq,
  max_seen_seq = GREATEST(max_seen_seq, @max_seen, @contiguous_seq),
  head_seq        = GREATEST(head_seq, @max_seen, @contiguous_seq),
  last_ack      = @last_ack,
  last_ack_at   = @now,
  boot_generation    = @boot_generation,
  last_boot_id  = @last_boot_id
WHERE dongle_id = @dongle_id;

-- name: InsertRawBlocks :many
INSERT INTO raw_block (dongle_id, seq, boot_id, boot_generation, t0_ms, marker, epoch, payload, signature, received_at)
SELECT @dongle_id::int, u.seq::bigint, u.boot_id::int, u.boot_generation::int, u.t0_ms::bigint, NULLIF(u.marker::text, ''),
       @epoch::bigint, u.payload::bytea, u.signature::bytea, @received_at::timestamptz
FROM ROWS FROM (unnest(@seqs::bigint[]), unnest(@boot_ids::int[]), unnest(@generations::int[]), unnest(@t0s::bigint[]),
                unnest(@markers::text[]), unnest(@payloads::bytea[]), unnest(@signatures::bytea[]))
     AS u(seq, boot_id, boot_generation, t0_ms, marker, payload, signature)
ON CONFLICT (dongle_id, seq) DO NOTHING
RETURNING seq;

-- name: UpdateIngestionBoots :exec
INSERT INTO boot (dongle_id, boot_generation, boot_id, first_seq, last_seq, first_t_ms, last_t_ms,
                  max_offset_ms, time_pending, gold_pending, pending_since)
SELECT @dongle_id::int, u.boot_generation::int, u.boot_id::int, u.from_seq::bigint, u.to_seq::bigint, u.t_start::bigint,
       u.t_end::bigint, u.max_offset::float8, true, true, @now::timestamptz
FROM ROWS FROM (unnest(@generations::int[]), unnest(@boot_ids::int[]), unnest(@from_seqs::bigint[]), unnest(@to_seqs::bigint[]),
                unnest(@t_starts::bigint[]), unnest(@t_ends::bigint[]), unnest(@max_offsets::float8[]))
     AS u(boot_generation, boot_id, from_seq, to_seq, t_start, t_end, max_offset)
ORDER BY u.boot_generation, u.boot_id
ON CONFLICT (dongle_id, boot_generation, boot_id) DO UPDATE SET
  first_seq        = LEAST(boot.first_seq, EXCLUDED.first_seq),
  last_seq          = GREATEST(boot.last_seq, EXCLUDED.last_seq),
  first_t_ms       = LEAST(boot.first_t_ms, EXCLUDED.first_t_ms),
  last_t_ms         = GREATEST(boot.last_t_ms, EXCLUDED.last_t_ms),
  max_offset_ms = LEAST(boot.max_offset_ms, EXCLUDED.max_offset_ms),
  gold_pending       = true,
  time_pending      = boot.time_pending OR boot.time_source IS DISTINCT FROM 'anchors',
  pending_since      = CASE WHEN boot.time_pending THEN boot.pending_since ELSE EXCLUDED.pending_since END;
