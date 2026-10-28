-- name: FindDongleByDevID :one
SELECT dongle_id, dev_id, dev_id_hex, security_profile, kms_auth_key, kms_rec_key,
       public_key, active, requested_diag_min
FROM dongle WHERE dev_id = @dev_id;

-- name: InsertDongle :one
INSERT INTO dongle (dev_id, security_profile, kms_auth_key, kms_rec_key, public_key, description)
VALUES (@dev_id, @security_profile, @kms_auth_key, @kms_rec_key, @public_key, @description)
RETURNING dongle_id;

-- name: InsertDongleState :exec
INSERT INTO dongle_state (dongle_id) VALUES (@dongle_id);

