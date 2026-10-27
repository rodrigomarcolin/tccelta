-- name: PegarBootPendente :one
SELECT dongle_id, geracao_boot, boot_id, seq_primeiro, t_ms_primeiro, t_ms_ultimo, deslocamento_max_ms,
       a_ms, b, origem_tempo
FROM boot
WHERE pendente_tempo
ORDER BY pendente_desde
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: ListarAncorasDoBoot :many
SELECT t_ms, s1, s2 FROM ancora_tempo
WHERE dongle_id = @dongle_id AND geracao_boot = @geracao_boot AND boot_id = @boot_id
ORDER BY t_ms;

-- name: BootAncoradoAnterior :one
SELECT geracao_boot, boot_id, a_ms, b, incerteza_base_ms, t_ms_primeiro, t_ms_ultimo
FROM boot
WHERE dongle_id = @dongle_id AND origem_tempo = 'ancoras' AND seq_primeiro < @seq_primeiro
ORDER BY seq_primeiro DESC
LIMIT 1;

-- name: BootAncoradoSeguinte :one
SELECT geracao_boot, boot_id, a_ms, b, incerteza_base_ms, t_ms_primeiro, t_ms_ultimo
FROM boot
WHERE dongle_id = @dongle_id AND origem_tempo = 'ancoras' AND seq_primeiro > @seq_primeiro
ORDER BY seq_primeiro ASC
LIMIT 1;

-- name: AtualizarTempoBoot :exec
UPDATE boot SET
  a_ms = @a_ms, b = @b, incerteza_base_ms = @incerteza_base_ms, origem_tempo = @origem_tempo,
  tempo_atualizado_em = @agora, pendente_tempo = false
WHERE dongle_id = @dongle_id AND geracao_boot = @geracao_boot AND boot_id = @boot_id;

-- name: ApagarGruposAncora :exec
DELETE FROM grupo_ancora
WHERE dongle_id = @dongle_id AND geracao_boot = @geracao_boot AND boot_id = @boot_id;

-- name: CopiarGruposAncora :copyfrom
INSERT INTO grupo_ancora (dongle_id, geracao_boot, boot_id, ordem, t_primeiro_ms, t_ultimo_ms,
                          t_centro_ms, lo_ms, hi_ms, n_ancoras, calculado_em)
VALUES (@dongle_id, @geracao_boot, @boot_id, @ordem, @t_primeiro_ms, @t_ultimo_ms,
        @t_centro_ms, @lo_ms, @hi_ms, @n_ancoras, @calculado_em);

-- name: ListarGruposDoBoot :many
SELECT ordem, t_primeiro_ms, t_ultimo_ms, t_centro_ms, lo_ms, hi_ms, n_ancoras
FROM grupo_ancora
WHERE dongle_id = @dongle_id AND geracao_boot = @geracao_boot AND boot_id = @boot_id
ORDER BY ordem;

-- name: MarcarVizinhosSemAncora :execrows
-- Todos os boots sem âncora entre o boot ancorado anterior e o seguinte (por seq).
UPDATE boot SET pendente_tempo = true, pendente_desde = @agora
WHERE boot.dongle_id = @dongle_id
  AND NOT pendente_tempo
  AND origem_tempo IS DISTINCT FROM 'ancoras'
  AND seq_primeiro IS NOT NULL
  AND seq_primeiro > COALESCE((SELECT max(x.seq_primeiro) FROM boot x
                               WHERE x.dongle_id = @dongle_id AND x.origem_tempo = 'ancoras'
                                 AND x.seq_primeiro < @seq_primeiro), 0)
  AND seq_primeiro < COALESCE((SELECT min(x.seq_primeiro) FROM boot x
                               WHERE x.dongle_id = @dongle_id AND x.origem_tempo = 'ancoras'
                                 AND x.seq_primeiro > @seq_primeiro), 9223372036854775807);

-- name: InserirAlertaSeNovo :exec
-- Evita repetir o mesmo alerta a cada recalculo (janela de 24 h por boot e tipo).
WITH p AS (
  SELECT @dongle_id::int AS dongle_id, @geracao_boot::int AS geracao_boot, @boot_id::int AS boot_id,
         @tipo::text AS tipo, @detalhe::jsonb AS detalhe, @agora::timestamptz AS agora
)
INSERT INTO alerta (dongle_id, geracao_boot, boot_id, tipo, detalhe)
SELECT p.dongle_id, p.geracao_boot, p.boot_id, p.tipo, p.detalhe FROM p
WHERE NOT EXISTS (SELECT 1 FROM alerta a
                  WHERE a.dongle_id = p.dongle_id AND a.geracao_boot = p.geracao_boot AND a.boot_id = p.boot_id
                    AND a.tipo = p.tipo AND a.criado_em > p.agora - INTERVAL '24 hours');

-- name: ContarBootsPendentes :one
SELECT count(*) FROM boot WHERE pendente_tempo;
