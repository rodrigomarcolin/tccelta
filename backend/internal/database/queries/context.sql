-- contexto.sql
-- name: InserirLacuna :exec
INSERT INTO lacuna (dongle_id, from_seq, to_seq, causa, registrado_em, c_s, seq_bloco)
VALUES (@dongle_id, @from_seq, @to_seq, @causa, @registrado_em, @c_s, @seq_bloco)
ON CONFLICT (dongle_id, from_seq) DO NOTHING;

-- name: GarantirBootPendente :exec
INSERT INTO boot (dongle_id, geracao_boot, boot_id, pendente_tempo, pendente_desde)
VALUES (@dongle_id, @geracao_boot, @boot_id, true, @agora)
ON CONFLICT (dongle_id, geracao_boot, boot_id) DO UPDATE
   SET pendente_tempo = true, pendente_desde = EXCLUDED.pendente_desde
 WHERE NOT boot.pendente_tempo;     -- não regrava se já pendente (evita churn)

-- name: InserirAncora :exec
INSERT INTO ancora_tempo (dongle_id, geracao_boot, boot_id, t_ms, nonce, s1, s2)
VALUES (@dongle_id, @geracao_boot, @boot_id, @t_ms, @nonce, @s1, @s2)
ON CONFLICT DO NOTHING;

-- name: InserirAlerta :exec
INSERT INTO alerta (dongle_id, geracao_boot, boot_id, tipo, detalhe)
VALUES (@dongle_id, @geracao_boot, @boot_id, @tipo, @detalhe);

-- name: NotificarBootPendente :exec
SELECT pg_notify('boot_pendente', @carga::text);
