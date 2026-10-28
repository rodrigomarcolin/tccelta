-- state.sql
-- name: LockDongle :exec
SELECT pg_advisory_xact_lock(1, @dongle_id::int);

-- name: ReadState :one
SELECT contiguous_seq, max_seen_seq, head_seq, tail_seq, last_c_s, c_conf, boot_generation, last_boot_id
FROM dongle_state WHERE dongle_id = @dongle_id;

-- name: UpdateHandshakeState :exec
UPDATE dongle_state SET
  contiguous_seq     = @contiguous_seq,
  max_seen_seq  = GREATEST(max_seen_seq, @contiguous_seq),
  head_seq         = @head_seq,
  tail_seq         = @tail_seq,
  head_reported_at = @s2,
  c_conf           = @c_conf,
  last_ack       = @last_ack,
  last_ack_at    = @now
WHERE dongle_id = @dongle_id;

-- name: ReadLastAck :one
SELECT last_ack FROM dongle_state WHERE dongle_id = @dongle_id;
