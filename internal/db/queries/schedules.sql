-- internal/db/queries/schedules.sql
-- name: GetFacilityHours :many
SELECT * FROM operating_hours
WHERE facility_id = ?
ORDER BY day_of_week;

-- name: UpsertOperatingHours :one
INSERT INTO operating_hours (
    facility_id,
    day_of_week,
    opens_at,
    closes_at
) VALUES (?, ?, ?, ?)
ON CONFLICT(facility_id, day_of_week) DO UPDATE SET
    opens_at = excluded.opens_at,
    closes_at = excluded.closes_at,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: DeleteOperatingHours :execrows
DELETE FROM operating_hours
WHERE facility_id = ? AND day_of_week = ?;
