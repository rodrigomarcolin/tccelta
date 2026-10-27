-- name: ListarPids :many
SELECT pid, nome, unidade, n_bytes, fator, deslocamento, valor_min, valor_max FROM pid ORDER BY pid;
