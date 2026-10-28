-- name: TakePendingBoot :one
SELECT dongle_id, boot_generation, boot_id, first_seq, first_t_ms, last_t_ms, max_offset_ms,
       a_ms, b, time_source
FROM boot
WHERE time_pending
ORDER BY pending_since
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: ListBootAnchors :many
SELECT t_ms, s1, s2 FROM time_anchor
WHERE dongle_id = @dongle_id AND boot_generation = @boot_generation AND boot_id = @boot_id
ORDER BY t_ms;

-- name: PreviousAnchoredBoot :one
SELECT boot_generation, boot_id, a_ms, b, base_uncertainty_ms, first_t_ms, last_t_ms
FROM boot
WHERE dongle_id = @dongle_id AND time_source = 'anchors' AND first_seq < @first_seq
ORDER BY first_seq DESC
LIMIT 1;

-- name: NextAnchoredBoot :one
SELECT boot_generation, boot_id, a_ms, b, base_uncertainty_ms, first_t_ms, last_t_ms
FROM boot
WHERE dongle_id = @dongle_id AND time_source = 'anchors' AND first_seq > @first_seq
ORDER BY first_seq ASC
LIMIT 1;

-- name: UpdateBootTime :exec
UPDATE boot SET
  a_ms = @a_ms, b = @b, base_uncertainty_ms = @base_uncertainty_ms, time_source = @time_source,
  time_updated_at = @now, time_pending = false
WHERE dongle_id = @dongle_id AND boot_generation = @boot_generation AND boot_id = @boot_id;

-- name: DeleteAnchorGroups :exec
DELETE FROM anchor_group
WHERE dongle_id = @dongle_id AND boot_generation = @boot_generation AND boot_id = @boot_id;

-- name: CopyAnchorGroups :copyfrom
INSERT INTO anchor_group (dongle_id, boot_generation, boot_id, ordinal, first_t_ms, last_t_ms,
                          t_center_ms, lo_ms, hi_ms, n_anchors, computed_at)
VALUES (@dongle_id, @boot_generation, @boot_id, @ordinal, @first_t_ms, @last_t_ms,
        @t_center_ms, @lo_ms, @hi_ms, @n_anchors, @computed_at);

-- name: ListBootAnchorGroups :many
SELECT ordinal, first_t_ms, last_t_ms, t_center_ms, lo_ms, hi_ms, n_anchors
FROM anchor_group
WHERE dongle_id = @dongle_id AND boot_generation = @boot_generation AND boot_id = @boot_id
ORDER BY ordinal;

-- name: MarkAnchorlessNeighbors :execrows
-- Todos os boots sem âncora entre o boot ancorado anterior e o seguinte (por seq).
UPDATE boot SET time_pending = true, pending_since = @now
WHERE boot.dongle_id = @dongle_id
  AND NOT time_pending
  AND time_source IS DISTINCT FROM 'anchors'
  AND first_seq IS NOT NULL
  AND first_seq > COALESCE((SELECT max(x.first_seq) FROM boot x
                               WHERE x.dongle_id = @dongle_id AND x.time_source = 'anchors'
                                 AND x.first_seq < @first_seq), 0)
  AND first_seq < COALESCE((SELECT min(x.first_seq) FROM boot x
                               WHERE x.dongle_id = @dongle_id AND x.time_source = 'anchors'
                                 AND x.first_seq > @first_seq), 9223372036854775807);

-- name: InsertAlertIfNew :exec
-- Evita repetir o mesmo alerta a cada recalculo (janela de 24 h por boot e tipo).
WITH p AS (
  SELECT @dongle_id::int AS dongle_id, @boot_generation::int AS boot_generation, @boot_id::int AS boot_id,
         @type::text AS type, @detail::jsonb AS detail, @now::timestamptz AS now
)
INSERT INTO alert (dongle_id, boot_generation, boot_id, type, detail)
SELECT p.dongle_id, p.boot_generation, p.boot_id, p.type, p.detail FROM p
WHERE NOT EXISTS (SELECT 1 FROM alert a
                  WHERE a.dongle_id = p.dongle_id AND a.boot_generation = p.boot_generation AND a.boot_id = p.boot_id
                    AND a.type = p.type AND a.created_at > p.now - INTERVAL '24 hours');

-- name: CountPendingBoots :one
SELECT count(*) FROM boot WHERE time_pending;
