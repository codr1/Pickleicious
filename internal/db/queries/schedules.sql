-- internal/db/queries/schedules.sql
-- name: GetFacilityHours :many
SELECT * FROM operating_hours
WHERE facility_id = ?
ORDER BY day_of_week;

-- name: UpdateOperatingHours :one
UPDATE operating_hours
SET opens_at = ?,
    closes_at = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE facility_id = ? AND day_of_week = ?
RETURNING *;
