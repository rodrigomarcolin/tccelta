-- name: BuscarDonglePorDevId :one
SELECT dongle_id, dev_id, dev_id_hex, perfil_seguranca, kms_chave_auth, kms_chave_rec,
       chave_publica, ativo, diag_solicitado_min
FROM dongle WHERE dev_id = @dev_id;

-- name: InserirDongle :one
INSERT INTO dongle (dev_id, perfil_seguranca, kms_chave_auth, kms_chave_rec, chave_publica, descricao)
VALUES (@dev_id, @perfil_seguranca, @kms_chave_auth, @kms_chave_rec, @chave_publica, @descricao)
RETURNING dongle_id;

-- name: InserirDongleEstado :exec
INSERT INTO dongle_estado (dongle_id) VALUES (@dongle_id);

