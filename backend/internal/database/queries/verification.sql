-- name: VerifyDongleState :one
SELECT d.dongle_id, e.contiguous_seq, e.head_seq, e.c_conf, e.boot_generation, e.last_boot_id
FROM dongle d JOIN dongle_state e USING (dongle_id) WHERE d.dev_id = @dev_id;

-- name: VerifyListGaps :many
SELECT from_seq, to_seq, cause FROM gap WHERE dongle_id = @dongle_id ORDER BY from_seq;

-- name: VerifyCountReadings :one
SELECT count(*) FROM reading WHERE dongle_id = @dongle_id;

-- name: VerifyCountPositions :one
SELECT count(*) FROM position WHERE dongle_id = @dongle_id;

-- name: VerifyCountRejected :one
SELECT count(*) FROM rejected_block WHERE dongle_id = @dongle_id;

-- name: VerifyCountAnchors :one
SELECT count(*) FROM time_anchor WHERE dongle_id = @dongle_id;

-- name: VerifyListBoots :many
SELECT boot_generation, boot_id, time_source, a_ms, b, base_uncertainty_ms, time_pending
FROM boot WHERE dongle_id = @dongle_id ORDER BY boot_generation, boot_id;

-- name: VerifyCountAlerts :one
SELECT count(*) FROM alert WHERE dongle_id = @dongle_id AND type = @type;

-- name: VerifyReadingTime :one
SELECT (bo.a_ms + (1 + bo.b) * l.t_ms)::float8 AS time_ms
FROM reading l JOIN boot bo USING (dongle_id, boot_generation, boot_id)
WHERE l.dongle_id = @dongle_id AND bo.a_ms IS NOT NULL ORDER BY l.received_at DESC, l.t_ms DESC LIMIT 1;

-- name: VerifyTripStatus :one
SELECT status, provisional, calculated_through_seq FROM gold.trip WHERE trip_id = @trip_id;

-- name: VerifyTripCounts :one
SELECT (SELECT count(*) FROM gold.trip_part p WHERE p.trip_id = @trip_id) AS parts,
       COALESCE((SELECT s.distance_km FROM gold.trip_summary s WHERE s.trip_id = @trip_id), 0)::float8 AS distance_km,
       COALESCE((SELECT s2.engine_on_s FROM gold.trip_summary s2 WHERE s2.trip_id = @trip_id), 0)::float8 AS engine_on_s;

-- name: SeedTripBoot :one
INSERT INTO gold.trip (trip_category, driver_name, driver_name_normalized, fuel, dongle_id, entry_type, boot_generation, boot_id,
                       status, target_seq)
VALUES (@trip_category, @driver_name, @driver_name_normalized, @fuel, @dongle_id, 'boot', @boot_generation, @boot_id,
        'pending', @target_seq)
RETURNING trip_id;

-- name: SeedTripInterval :one
INSERT INTO gold.trip (trip_category, driver_name, driver_name_normalized, fuel, dongle_id, entry_type, interval_start, interval_end,
                       status, target_seq)
VALUES (@trip_category, @driver_name, @driver_name_normalized, @fuel, @dongle_id, 'interval', @interval_start, @interval_end,
        'pending', @target_seq)
RETURNING trip_id;
