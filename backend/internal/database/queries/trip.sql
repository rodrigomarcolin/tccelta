-- trip.sql: fila e gravação da análise da viagem (schema gold) e leitura da silver para o cálculo (API1).

-- name: ListWaitingTrips :many
SELECT * FROM gold.trip WHERE status IN ('pending', 'waiting_data') ORDER BY created_at;

-- name: MarkTripWaiting :exec
UPDATE gold.trip SET status = 'waiting_data' WHERE trip_id = @trip_id AND status = 'pending';

-- name: ResolveTripGeneration :one
-- A API grava boot_generation nula quando o boot ainda não existe; vale a maior geração que tem o boot_id.
UPDATE gold.trip t SET boot_generation = g.generation
FROM (SELECT max(b.boot_generation) AS generation FROM boot b WHERE b.dongle_id = @dongle_id AND b.boot_id = @boot_id) g
WHERE t.trip_id = @trip_id AND t.boot_generation IS NULL AND g.generation IS NOT NULL
RETURNING t.boot_generation;

-- name: TakeTrip :one
SELECT * FROM gold.trip
WHERE trip_id = @trip_id AND status IN ('pending', 'waiting_data')
FOR UPDATE SKIP LOCKED;

-- name: TakeRecalcTrip :one
SELECT * FROM gold.trip
WHERE status = 'ready' AND recalc_pending
ORDER BY created_at
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: DeleteTripParts :exec
DELETE FROM gold.trip_part WHERE trip_id = @trip_id;

-- name: DeleteTripSummary :exec
DELETE FROM gold.trip_summary WHERE trip_id = @trip_id;

-- name: DeleteTripSpeedBands :exec
DELETE FROM gold.trip_speed_band WHERE trip_id = @trip_id;

-- name: DeleteTripEvents :exec
DELETE FROM gold.trip_event WHERE trip_id = @trip_id;

-- name: DeleteTripStops :exec
DELETE FROM gold.trip_stop WHERE trip_id = @trip_id;

-- name: DeleteTripGears :exec
DELETE FROM gold.trip_gear WHERE trip_id = @trip_id;

-- name: DeleteTripGearShifts :exec
DELETE FROM gold.trip_gear_shift WHERE trip_id = @trip_id;

-- name: DeleteTripSeries :exec
DELETE FROM gold.trip_series WHERE trip_id = @trip_id;

-- name: InsertTripPart :exec
INSERT INTO gold.trip_part (trip_id, part_no, boot_generation, boot_id, data_start_t_ms, data_end_t_ms, start_t_ms, end_t_ms,
                            clock_a_ms, clock_b, clock_uncertainty_ms)
VALUES (@trip_id, @part_no, @boot_generation, @boot_id, @data_start_t_ms, @data_end_t_ms, @start_t_ms, @end_t_ms,
        @clock_a_ms, @clock_b, @clock_uncertainty_ms);

-- name: InsertTripSpeedBand :exec
INSERT INTO gold.trip_speed_band (trip_id, band_id, from_kmh, to_kmh, time_s, distance_km, distance_pct)
VALUES (@trip_id, @band_id, @from_kmh, @to_kmh, @time_s, @distance_km, @distance_pct);

-- name: InsertTripEvent :exec
INSERT INTO gold.trip_event (trip_id, event_no, type, start_t_ms, duration_s, peak, peak_unit, start_speed_kmh, end_speed_kmh, lat, lon)
VALUES (@trip_id, @event_no, @type, @start_t_ms, @duration_s, @peak, @peak_unit, @start_speed_kmh, @end_speed_kmh, @lat, @lon);

-- name: InsertTripStop :exec
INSERT INTO gold.trip_stop (trip_id, stop_no, start_t_ms, duration_s, engine_on, lat, lon)
VALUES (@trip_id, @stop_no, @start_t_ms, @duration_s, @engine_on, @lat, @lon);

-- name: InsertTripGear :exec
INSERT INTO gold.trip_gear (trip_id, gear, rpm_per_kmh, time_s, time_pct)
VALUES (@trip_id, @gear, @rpm_per_kmh, @time_s, @time_pct);

-- name: InsertTripGearShift :exec
INSERT INTO gold.trip_gear_shift (trip_id, shift_no, t_ms, direction, from_gear, to_gear, rpm_before, rpm_after, speed_kmh)
VALUES (@trip_id, @shift_no, @t_ms, @direction, @from_gear, @to_gear, @rpm_before, @rpm_after, @speed_kmh);

-- name: InsertTripSeries :exec
INSERT INTO gold.trip_series (trip_id, step_ms, exposure_step_ms, series, slow_series, route, rpm_distribution,
                              pedal_distribution, rpm_pedal_matrix)
VALUES (@trip_id, @step_ms, @exposure_step_ms, @series, @slow_series, @route, @rpm_distribution,
        @pedal_distribution, @rpm_pedal_matrix);

-- name: InsertTripSummary :exec
INSERT INTO gold.trip_summary (
  trip_id, start_t_ms, end_t_ms, started_at, ended_at, clock_offset_ms, clock_uncertainty_ms, warnings, parameters,
  distance_km, duration_s, no_data_s, moving_s, stopped_s, avg_speed_kmh, max_speed_kmh, engine_on_s, idle_s, idle_pct,
  fuel_l, km_per_l, intake_air_g, n_hard_accel, n_hard_brake, hard_accel_per_100km, hard_brake_per_100km,
  pedal_mean_pct, pedal_high_s, pedal_rate_p95_pct_s, engine_brake_pct, accel_rms_ms2, cruise_s,
  warmup_s, max_coolant_c, cold_load_s, idle_rpm, idle_rpm_origin, idle_margin_rpm, neutral_clutch_s,
  upshift_rpm_median, upshift_rpm_p25, upshift_rpm_p75, upshift_rpm_n, downshift_rpm_median, downshift_rpm_n,
  n_readings, readings_per_s, pid_periods_ms, fuel_coverage_pct,
  gps_period_s, gps_coverage_pct, gps_n_gaps, gps_max_gap_s, gps_distance_km, gps_distance_diff_pct)
VALUES (
  @trip_id, @start_t_ms, @end_t_ms, @started_at, @ended_at, @clock_offset_ms, @clock_uncertainty_ms, @warnings, @parameters,
  @distance_km, @duration_s, @no_data_s, @moving_s, @stopped_s, @avg_speed_kmh, @max_speed_kmh, @engine_on_s, @idle_s, @idle_pct,
  @fuel_l, @km_per_l, @intake_air_g, @n_hard_accel, @n_hard_brake, @hard_accel_per_100km, @hard_brake_per_100km,
  @pedal_mean_pct, @pedal_high_s, @pedal_rate_p95_pct_s, @engine_brake_pct, @accel_rms_ms2, @cruise_s,
  @warmup_s, @max_coolant_c, @cold_load_s, @idle_rpm, @idle_rpm_origin, @idle_margin_rpm, @neutral_clutch_s,
  @upshift_rpm_median, @upshift_rpm_p25, @upshift_rpm_p75, @upshift_rpm_n, @downshift_rpm_median, @downshift_rpm_n,
  @n_readings, @readings_per_s, @pid_periods_ms, @fuel_coverage_pct,
  @gps_period_s, @gps_coverage_pct, @gps_n_gaps, @gps_max_gap_s, @gps_distance_km, @gps_distance_diff_pct);

-- name: SaveTripResult :exec
UPDATE gold.trip SET
  status = 'ready', calculation_version = @calculation_version, calculated_at = @calculated_at,
  first_seq = @first_seq, last_seq = @last_seq, calculated_through_seq = @calculated_through_seq,
  provisional = @provisional, recalc_pending = false
WHERE trip_id = @trip_id;

-- name: FailTrip :exec
-- Uma viagem nova que falha vira failed (e libera o nome); um recálculo que falha mantém o resultado anterior.
UPDATE gold.trip SET
  status = CASE WHEN status = 'ready' THEN 'ready' ELSE 'failed' END,
  recalc_pending = false
WHERE trip_id = @trip_id;

-- name: ListProvisionalTrips :many
SELECT t.*, s.warnings AS summary_warnings
FROM gold.trip t LEFT JOIN gold.trip_summary s USING (trip_id)
WHERE t.status = 'ready' AND t.provisional AND NOT t.recalc_pending
ORDER BY t.created_at;

-- name: MarkTripsRecalcByID :exec
UPDATE gold.trip SET recalc_pending = true WHERE trip_id = ANY(@trip_ids::uuid[]) AND status = 'ready';

-- name: ClearTripProvisional :exec
UPDATE gold.trip SET provisional = false WHERE trip_id = @trip_id AND status = 'ready';

-- name: MarkTripsRecalc :exec
-- reprocess: as viagens prontas cuja faixa de seq cruza a reprocessada.
UPDATE gold.trip SET recalc_pending = true
WHERE dongle_id = @dongle_id AND status = 'ready' AND first_seq <= @to_seq AND last_seq >= @from_seq;

-- name: NotifyTripPending :exec
SELECT pg_notify('trip_pending', '');

-- name: CountTripQueue :one
SELECT count(*) FILTER (WHERE status IN ('pending', 'waiting_data')) AS waiting,
       count(*) FILTER (WHERE recalc_pending) AS recalc
FROM gold.trip WHERE status IN ('pending', 'waiting_data') OR recalc_pending;

-- Leitura da silver (papel tccelta_worker: SELECT em reading, position e boot).

-- name: FindBootForTrip :one
SELECT boot_generation, boot_id, first_seq, last_seq, first_t_ms, last_t_ms, max_offset_ms, a_ms, b, base_uncertainty_ms, time_source
FROM boot WHERE dongle_id = @dongle_id AND boot_generation = @boot_generation AND boot_id = @boot_id;

-- name: ListIntervalBoots :many
-- Boots com conversão de hora cuja faixa [first, last] (hora absoluta) toca [from_ms, to_ms).
SELECT boot_generation, boot_id, first_seq, last_seq, first_t_ms, last_t_ms, max_offset_ms, a_ms, b, base_uncertainty_ms, time_source
FROM boot
WHERE dongle_id = @dongle_id AND time_source IS NOT NULL AND first_seq IS NOT NULL
  AND a_ms + (1 + b) * last_t_ms >= @from_ms::float8
  AND a_ms + (1 + b) * first_t_ms < @to_ms::float8
ORDER BY first_seq;

-- name: ListUnconvertedBoots :many
-- Boots sem hora que podem cair no intervalo: a hora real é no máximo o recebimento (max_offset_ms) e a posição por
-- seq fica entre o último boot convertido que termina antes de t0 e o primeiro que começa em ou depois de t1.
SELECT u.boot_generation, u.boot_id, u.first_seq, u.last_seq, u.first_t_ms, u.last_t_ms, u.max_offset_ms, u.a_ms, u.b, u.base_uncertainty_ms, u.time_source
FROM boot u
WHERE u.dongle_id = @dongle_id AND u.time_source IS NULL AND u.first_seq IS NOT NULL AND u.max_offset_ms IS NOT NULL
  AND u.max_offset_ms + u.last_t_ms >= @t0_ms::float8
  AND u.first_seq > COALESCE((SELECT max(x.first_seq) FROM boot x
                             WHERE x.dongle_id = @dongle_id AND x.time_source IS NOT NULL
                               AND x.a_ms + (1 + x.b) * x.last_t_ms < @t0_ms::float8), 0)
  AND u.first_seq < COALESCE((SELECT min(x.first_seq) FROM boot x
                             WHERE x.dongle_id = @dongle_id AND x.time_source IS NOT NULL
                               AND x.a_ms + (1 + x.b) * x.first_t_ms >= @t1_ms::float8), 9223372036854775807)
ORDER BY u.first_seq;

-- name: BootFirstReceived :one
SELECT received_at FROM raw_block
WHERE dongle_id = @dongle_id AND seq >= @first_seq AND seq <= @last_seq
  AND boot_id = @boot_id AND boot_generation = @boot_generation AND marker IS NULL
ORDER BY received_at ASC
LIMIT 1;

-- name: BootLastReceived :one
SELECT received_at FROM raw_block
WHERE dongle_id = @dongle_id AND seq >= @first_seq AND seq <= @last_seq
  AND boot_id = @boot_id AND boot_generation = @boot_generation AND marker IS NULL
ORDER BY received_at DESC
LIMIT 1;

-- name: LoadTripReadings :many
SELECT t_ms, pid, value FROM reading
WHERE dongle_id = @dongle_id AND boot_generation = @boot_generation AND boot_id = @boot_id
  AND pid = ANY(@pids::smallint[])
  AND received_at >= @received_from AND received_at <= @received_to
  AND t_ms >= @t_from AND t_ms <= @t_to
  AND value IS NOT NULL
ORDER BY pid, t_ms;

-- name: LoadTripPositions :many
SELECT t_ms, position_lon(geom) AS lon, position_lat(geom) AS lat FROM position
WHERE dongle_id = @dongle_id AND boot_generation = @boot_generation AND boot_id = @boot_id
  AND received_at >= @received_from AND received_at <= @received_to
  AND t_ms >= @t_from AND t_ms <= @t_to
ORDER BY t_ms;

-- name: NextBootFirstSeq :one
SELECT first_seq FROM boot WHERE dongle_id = @dongle_id AND first_seq > @after_seq ORDER BY first_seq LIMIT 1;

-- name: MaxConvertedBlockTime :one
-- Hora absoluta (ms) do t0 do bloco mais recente já convertido, entre os blocos de seq em [from_seq, to_seq].
-- Janela curta de propósito: raw_block é percorrido por seq, e a busca só precisa dos últimos blocos.
SELECT (b.a_ms + (1 + b.b) * rb.t0_ms)::float8 AS t0_epoch_ms
FROM raw_block rb
JOIN boot b ON b.dongle_id = rb.dongle_id AND b.boot_generation = rb.boot_generation AND b.boot_id = rb.boot_id
WHERE rb.dongle_id = @dongle_id AND rb.marker IS NULL AND rb.seq >= @from_seq AND rb.seq <= @to_seq
  AND b.time_source IS NOT NULL
ORDER BY 1 DESC
LIMIT 1;

-- API2: criação e leitura da viagem (papel tccelta_api: INSERT em gold.trip e SELECT nas tabelas da gold).

-- name: InsertTrip :one
INSERT INTO gold.trip (trip_category, driver_name, driver_name_normalized, fuel, dongle_id, entry_type, boot_generation, boot_id,
                       interval_start, interval_end, status, target_seq)
VALUES (@trip_category, @driver_name, @driver_name_normalized, @fuel, @dongle_id, @entry_type, @boot_generation, @boot_id,
        @interval_start, @interval_end, 'pending', @target_seq)
RETURNING *;

-- name: NotifyTripCreated :exec
SELECT pg_notify('trip_pending', @payload::text);

-- name: FindTrip :one
SELECT * FROM gold.trip WHERE trip_id = @trip_id AND excluded_at IS NULL;

-- name: LatestBootGeneration :one
SELECT boot_generation FROM boot WHERE dongle_id = @dongle_id AND boot_id = @boot_id ORDER BY boot_generation DESC LIMIT 1;

-- name: GetTripSummary :one
SELECT * FROM gold.trip_summary WHERE trip_id = @trip_id;

-- name: ListTripParts :many
SELECT * FROM gold.trip_part WHERE trip_id = @trip_id ORDER BY part_no;

-- name: ListTripSpeedBands :many
SELECT * FROM gold.trip_speed_band WHERE trip_id = @trip_id ORDER BY from_kmh;

-- name: ListTripEvents :many
SELECT * FROM gold.trip_event WHERE trip_id = @trip_id ORDER BY event_no;

-- name: ListTripStops :many
SELECT * FROM gold.trip_stop WHERE trip_id = @trip_id ORDER BY stop_no;

-- name: ListTripGears :many
SELECT * FROM gold.trip_gear WHERE trip_id = @trip_id ORDER BY gear;

-- name: ListTripGearShifts :many
SELECT * FROM gold.trip_gear_shift WHERE trip_id = @trip_id ORDER BY shift_no;

-- name: GetTripSeries :one
SELECT * FROM gold.trip_series WHERE trip_id = @trip_id;
