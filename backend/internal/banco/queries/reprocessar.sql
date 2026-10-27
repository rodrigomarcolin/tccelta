-- name: ListarBlocosDados :many
SELECT seq, geracao_boot, boot_id, t0_ms, payload, recebido_em FROM bloco_bruto
WHERE dongle_id = @dongle_id AND marcador IS NULL AND seq >= @seq_ini AND seq <= @seq_fim
ORDER BY seq
LIMIT @limite;

-- name: ApagarLeiturasDoBloco :execrows
DELETE FROM leitura
WHERE dongle_id = @dongle_id AND seq_bloco >= @seq_ini AND seq_bloco <= @seq_fim
  AND recebido_em >= @recebido_min AND recebido_em <= @recebido_max;

-- name: ApagarPosicoesDoBloco :execrows
DELETE FROM posicao
WHERE dongle_id = @dongle_id AND seq_bloco >= @seq_ini AND seq_bloco <= @seq_fim
  AND recebido_em >= @recebido_min AND recebido_em <= @recebido_max;

-- name: MarcarBootsGold :exec
UPDATE boot SET pendente_gold = true
WHERE dongle_id = @dongle_id AND seq_primeiro <= @seq_fim AND seq_ultimo >= @seq_ini;

-- name: ContarLeituras :one
SELECT count(*) FROM leitura WHERE dongle_id = @dongle_id AND seq_bloco >= @seq_ini AND seq_bloco <= @seq_fim;
