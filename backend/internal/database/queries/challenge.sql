-- name: IssueChallenge :one
-- Incrementa c_S (lock de linha curto em dongle_state) e insere o desafio,
-- só se o dongle tiver menos de @max_pending desafios pendentes no prazo.
WITH cs AS (
  UPDATE dongle_state e
     SET last_c_s = GREATEST(e.last_c_s + 1, @now_s::bigint)   -- @now_s = segundos Unix do relógio da API
   WHERE e.dongle_id = @dongle_id
     AND GREATEST(e.last_c_s + 1, @now_s::bigint) <= 4294967295
     AND (SELECT count(*) FROM challenge d
           WHERE d.dongle_id = @dongle_id
             AND d.answered_at IS NULL
             AND d.expires_at > @s1) < @max_pending::int
  RETURNING e.last_c_s AS c_s, e.contiguous_seq, e.c_conf
), ins AS (
  INSERT INTO challenge (dongle_id, c_s, nonce, s1, expires_at, issued_ack_seq, issued_c_conf, flags_c, diag_min)
  SELECT @dongle_id, cs.c_s, @nonce, @s1, @expires_at, cs.contiguous_seq, cs.c_conf, @flags_c, @diag_min
  FROM cs
  RETURNING c_s
)
SELECT cs.c_s, cs.contiguous_seq, cs.c_conf FROM cs JOIN ins USING (c_s);

-- name: FindChallenge :one
SELECT nonce, s1, expires_at, answered_at FROM challenge
WHERE dongle_id = @dongle_id AND c_s = @c_s;

-- name: MarkInvalidTag :exec
UPDATE challenge
   SET invalid_tag_attempts = invalid_tag_attempts + 1,
       result = COALESCE(result, 'invalid_tag')
 WHERE dongle_id = @dongle_id AND c_s = @c_s AND nonce = @nonce AND answered_at IS NULL;

-- name: ConsumeChallenge :one
-- Uso único: pertence ao dongle, nonce confere, pendente e no prazo. Duas
-- respostas simultâneas: só uma recebe a linha.
UPDATE challenge
   SET answered_at = @s2, boot_id = @boot_id, t_ms = @t_ms,
       head_seq = @head_seq, tail_seq = @tail_seq
 WHERE dongle_id = @dongle_id AND c_s = @c_s AND nonce = @nonce
   AND answered_at IS NULL AND expires_at > @s2
RETURNING s1;

-- name: FinalizeChallenge :exec
UPDATE challenge
   SET result = @result, ack_seq = @ack_seq, loss_start = @loss_start,
       loss_end = @loss_end, token_jti = @token_jti, t_ms = COALESCE(@unwrapped_t_ms, t_ms),
       boot_generation = COALESCE(@boot_generation, boot_generation)
 WHERE dongle_id = @dongle_id AND c_s = @c_s;

-- name: ExpireChallenges :execrows
-- Job periódico do worker (spec 06).
UPDATE challenge SET result = 'expired'
 WHERE answered_at IS NULL AND result IS NULL AND expires_at < @now;
