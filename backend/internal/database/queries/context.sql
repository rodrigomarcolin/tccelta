-- context.sql
-- name: InsertGap :exec
INSERT INTO gap (dongle_id, from_seq, to_seq, cause, registered_at, c_s, block_seq)
VALUES (@dongle_id, @from_seq, @to_seq, @cause, @registered_at, @c_s, @block_seq)
ON CONFLICT (dongle_id, from_seq) DO NOTHING;

-- name: EnsurePendingBoot :exec
INSERT INTO boot (dongle_id, boot_generation, boot_id, time_pending, pending_since)
VALUES (@dongle_id, @boot_generation, @boot_id, true, @now)
ON CONFLICT (dongle_id, boot_generation, boot_id) DO UPDATE
   SET time_pending = true, pending_since = EXCLUDED.pending_since
 WHERE NOT boot.time_pending;     -- não regrava se já pendente (evita churn)

-- name: InsertAnchor :exec
INSERT INTO time_anchor (dongle_id, boot_generation, boot_id, t_ms, nonce, s1, s2)
VALUES (@dongle_id, @boot_generation, @boot_id, @t_ms, @nonce, @s1, @s2)
ON CONFLICT DO NOTHING;

-- name: InsertAlert :exec
INSERT INTO alert (dongle_id, boot_generation, boot_id, type, detail)
VALUES (@dongle_id, @boot_generation, @boot_id, @type, @detail);

-- name: NotifyPendingBoot :exec
SELECT pg_notify('boot_pending', @payload::text);
