-- internal/db/queries/courts.sql
-- name: GetCourt :one
SELECT * FROM courts
WHERE id = ? LIMIT 1;

-- name: ListCourts :many
SELECT * FROM courts
WHERE facility_id = ?
ORDER BY court_number;

-- name: CreateCourt :one
INSERT INTO courts (
    facility_id, name, court_number, status
) VALUES (?, ?, ?, ?)
RETURNING *;

-- name: UpdateCourtStatus :one
UPDATE courts
SET status = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;
