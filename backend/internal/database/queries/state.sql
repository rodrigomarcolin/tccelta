-- estado.sql
-- name: TravarDongle :exec
SELECT pg_advisory_xact_lock(1, @dongle_id::int);

-- name: LerEstado :one
SELECT seq_contiguo, maior_seq_visto, head_seq, tail_seq, ultimo_c_s, c_conf, geracao_boot, ultimo_boot_id
FROM dongle_estado WHERE dongle_id = @dongle_id;

-- name: AtualizarEstadoHandshake :exec
UPDATE dongle_estado SET
  seq_contiguo     = @seq_contiguo,
  maior_seq_visto  = GREATEST(maior_seq_visto, @seq_contiguo),
  head_seq         = @head_seq,
  tail_seq         = @tail_seq,
  head_relatado_em = @s2,
  c_conf           = @c_conf,
  ultimo_ack       = @ultimo_ack,
  ultimo_ack_em    = @agora
WHERE dongle_id = @dongle_id;

-- name: LerUltimoAck :one
SELECT ultimo_ack FROM dongle_estado WHERE dongle_id = @dongle_id;
