-- name: ListPids :many
SELECT pid, name, unit, n_bytes, factor, value_offset, min_value, max_value FROM pid ORDER BY pid;
