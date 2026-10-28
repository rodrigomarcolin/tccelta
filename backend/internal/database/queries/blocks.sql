-- name: LerSnapshotEstado :one
SELECT seq_contiguo, c_conf, ultimo_ack FROM dongle_estado WHERE dongle_id = @dongle_id;

-- name: LerTMsUltimoBoots :many
-- Devolve as linhas candidatas nas geracoes G e G+1; o chamador escolhe a certa (atribuirGeracao).
SELECT geracao_boot, boot_id, t_ms_ultimo FROM boot
WHERE dongle_id = @dongle_id AND boot_id = ANY(@boot_ids::int[])
  AND geracao_boot IN (@geracao_a, @geracao_b);

-- name: CopiarLeituras :copyfrom
INSERT INTO leitura (dongle_id, geracao_boot, boot_id, t_ms, pid, valor_bruto, valor, seq_bloco, recebido_em)
VALUES (@dongle_id, @geracao_boot, @boot_id, @t_ms, @pid, @valor_bruto, @valor, @seq_bloco, @recebido_em);

-- name: CopiarPosicoes :copyfrom
INSERT INTO posicao (dongle_id, geracao_boot, boot_id, t_ms, geom, origem, seq_bloco, recebido_em)
VALUES (@dongle_id, @geracao_boot, @boot_id, @t_ms, @geom, @origem, @seq_bloco, @recebido_em);

-- name: RegistrarRejeitado :exec
INSERT INTO bloco_rejeitado (dongle_id, seq, boot_id, t0_ms, payload, assinatura, motivo, detalhe,
                             hash_conteudo, token_jti, recebido_em)
VALUES (@dongle_id, @seq, @boot_id, @t0_ms, @payload, @assinatura, @motivo, @detalhe,
        @hash_conteudo, @token_jti, @recebido_em)
ON CONFLICT (dongle_id, hash_conteudo) DO UPDATE
   SET ocorrencias = bloco_rejeitado.ocorrencias + 1, recebido_em = EXCLUDED.recebido_em;

-- name: AtualizarEstadoIngestao :exec
UPDATE dongle_estado SET
  seq_contiguo    = @seq_contiguo,
  maior_seq_visto = GREATEST(maior_seq_visto, @maior_visto, @seq_contiguo),
  head_seq        = GREATEST(head_seq, @maior_visto, @seq_contiguo),
  ultimo_ack      = @ultimo_ack,
  ultimo_ack_em   = @agora,
  geracao_boot    = @geracao_boot,
  ultimo_boot_id  = @ultimo_boot_id
WHERE dongle_id = @dongle_id;

-- name: InserirBlocosBrutos :many
INSERT INTO bloco_bruto (dongle_id, seq, boot_id, geracao_boot, t0_ms, marcador, epoca, payload, assinatura, recebido_em)
SELECT @dongle_id::int, u.seq::bigint, u.boot_id::int, u.geracao_boot::int, u.t0_ms::bigint, NULLIF(u.marcador::text, ''),
       @epoca::bigint, u.payload::bytea, u.assinatura::bytea, @recebido_em::timestamptz
FROM ROWS FROM (unnest(@seqs::bigint[]), unnest(@boot_ids::int[]), unnest(@geracoes::int[]), unnest(@t0s::bigint[]),
                unnest(@marcadores::text[]), unnest(@payloads::bytea[]), unnest(@assinaturas::bytea[]))
     AS u(seq, boot_id, geracao_boot, t0_ms, marcador, payload, assinatura)
ON CONFLICT (dongle_id, seq) DO NOTHING
RETURNING seq;

-- name: AtualizarBootsIngestao :exec
INSERT INTO boot (dongle_id, geracao_boot, boot_id, seq_primeiro, seq_ultimo, t_ms_primeiro, t_ms_ultimo,
                  deslocamento_max_ms, pendente_tempo, pendente_gold, pendente_desde)
SELECT @dongle_id::int, u.geracao_boot::int, u.boot_id::int, u.seq_ini::bigint, u.seq_fim::bigint, u.t_ini::bigint,
       u.t_fim::bigint, u.desl_max::float8, true, true, @agora::timestamptz
FROM ROWS FROM (unnest(@geracoes::int[]), unnest(@boot_ids::int[]), unnest(@seq_inis::bigint[]), unnest(@seq_fims::bigint[]),
                unnest(@t_inis::bigint[]), unnest(@t_fims::bigint[]), unnest(@desl_maxs::float8[]))
     AS u(geracao_boot, boot_id, seq_ini, seq_fim, t_ini, t_fim, desl_max)
ORDER BY u.geracao_boot, u.boot_id
ON CONFLICT (dongle_id, geracao_boot, boot_id) DO UPDATE SET
  seq_primeiro        = LEAST(boot.seq_primeiro, EXCLUDED.seq_primeiro),
  seq_ultimo          = GREATEST(boot.seq_ultimo, EXCLUDED.seq_ultimo),
  t_ms_primeiro       = LEAST(boot.t_ms_primeiro, EXCLUDED.t_ms_primeiro),
  t_ms_ultimo         = GREATEST(boot.t_ms_ultimo, EXCLUDED.t_ms_ultimo),
  deslocamento_max_ms = LEAST(boot.deslocamento_max_ms, EXCLUDED.deslocamento_max_ms),
  pendente_gold       = true,
  pendente_tempo      = boot.pendente_tempo OR boot.origem_tempo IS DISTINCT FROM 'ancoras',
  pendente_desde      = CASE WHEN boot.pendente_tempo THEN boot.pendente_desde ELSE EXCLUDED.pendente_desde END;
