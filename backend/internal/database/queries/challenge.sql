-- name: EmitirDesafio :one
-- Incrementa c_S (lock de linha curto em dongle_estado) e insere o desafio,
-- só se o dongle tiver menos de @max_pendentes desafios pendentes no prazo.
WITH cs AS (
  UPDATE dongle_estado e
     SET ultimo_c_s = GREATEST(e.ultimo_c_s + 1, @agora_s::bigint)   -- @agora_s = segundos Unix do relógio da API
   WHERE e.dongle_id = @dongle_id
     AND GREATEST(e.ultimo_c_s + 1, @agora_s::bigint) <= 4294967295
     AND (SELECT count(*) FROM desafio d
           WHERE d.dongle_id = @dongle_id
             AND d.respondido_em IS NULL
             AND d.expira_em > @s1) < @max_pendentes::int
  RETURNING e.ultimo_c_s AS c_s, e.seq_contiguo, e.c_conf
), ins AS (
  INSERT INTO desafio (dongle_id, c_s, nonce, s1, expira_em, ack_seq_emitido, c_conf_emitido, flags_c, diag_min)
  SELECT @dongle_id, cs.c_s, @nonce, @s1, @expira_em, cs.seq_contiguo, cs.c_conf, @flags_c, @diag_min
  FROM cs
  RETURNING c_s
)
SELECT cs.c_s, cs.seq_contiguo, cs.c_conf FROM cs JOIN ins USING (c_s);

-- name: BuscarDesafio :one
SELECT nonce, s1, expira_em, respondido_em FROM desafio
WHERE dongle_id = @dongle_id AND c_s = @c_s;

-- name: MarcarTagInvalida :exec
UPDATE desafio
   SET tentativas_tag_invalida = tentativas_tag_invalida + 1,
       resultado = COALESCE(resultado, 'tag_invalida')
 WHERE dongle_id = @dongle_id AND c_s = @c_s AND nonce = @nonce AND respondido_em IS NULL;

-- name: ConsumirDesafio :one
-- Uso único: pertence ao dongle, nonce confere, pendente e no prazo. Duas
-- respostas simultâneas: só uma recebe a linha.
UPDATE desafio
   SET respondido_em = @s2, boot_id = @boot_id, t_ms = @t_ms,
       head_seq = @head_seq, tail_seq = @tail_seq
 WHERE dongle_id = @dongle_id AND c_s = @c_s AND nonce = @nonce
   AND respondido_em IS NULL AND expira_em > @s2
RETURNING s1;

-- name: FinalizarDesafio :exec
UPDATE desafio
   SET resultado = @resultado, ack_seq = @ack_seq, perda_ini = @perda_ini,
       perda_fim = @perda_fim, token_jti = @token_jti, t_ms = COALESCE(@t_ms_desembrulhado, t_ms),
       geracao_boot = COALESCE(@geracao_boot, geracao_boot)
 WHERE dongle_id = @dongle_id AND c_s = @c_s;

-- name: ExpirarDesafios :execrows
-- Job periódico do worker (spec 06).
UPDATE desafio SET resultado = 'expirado'
 WHERE respondido_em IS NULL AND resultado IS NULL AND expira_em < @agora;
