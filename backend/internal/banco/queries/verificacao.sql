-- name: VerEstadoDongle :one
SELECT d.dongle_id, e.seq_contiguo, e.head_seq, e.c_conf, e.geracao_boot, e.ultimo_boot_id
FROM dongle d JOIN dongle_estado e USING (dongle_id) WHERE d.dev_id = @dev_id;

-- name: VerListarLacunas :many
SELECT from_seq, to_seq, causa FROM lacuna WHERE dongle_id = @dongle_id ORDER BY from_seq;

-- name: VerContarLeituras :one
SELECT count(*) FROM leitura WHERE dongle_id = @dongle_id;

-- name: VerContarPosicoes :one
SELECT count(*) FROM posicao WHERE dongle_id = @dongle_id;

-- name: VerContarRejeitados :one
SELECT count(*) FROM bloco_rejeitado WHERE dongle_id = @dongle_id;

-- name: VerContarAncoras :one
SELECT count(*) FROM ancora_tempo WHERE dongle_id = @dongle_id;

-- name: VerListarBoots :many
SELECT geracao_boot, boot_id, origem_tempo, a_ms, b, incerteza_base_ms, pendente_tempo
FROM boot WHERE dongle_id = @dongle_id ORDER BY geracao_boot, boot_id;

-- name: VerContarAlertas :one
SELECT count(*) FROM alerta WHERE dongle_id = @dongle_id AND tipo = @tipo;

-- name: VerHoraDaLeitura :one
SELECT (bo.a_ms + (1 + bo.b) * l.t_ms)::float8 AS hora_ms
FROM leitura l JOIN boot bo USING (dongle_id, geracao_boot, boot_id)
WHERE l.dongle_id = @dongle_id AND bo.a_ms IS NOT NULL ORDER BY l.recebido_em DESC, l.t_ms DESC LIMIT 1;
