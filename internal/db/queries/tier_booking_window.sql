-- internal/db/queries/tier_booking_window.sql

-- name: GetTierBookingWindow :one
SELECT
    facility_id,
    membership_level,
    max_advance_days
FROM member_tier_booking_windows
WHERE facility_id = ? AND membership_level = ?;

-- name: ListTierBookingWindowsForFacility :many
SELECT
    facility_id,
    membership_level,
    max_advance_days
FROM member_tier_booking_windows
WHERE facility_id = ?
ORDER BY membership_level;

-- name: UpsertTierBookingWindow :one
INSERT INTO member_tier_booking_windows (
    facility_id,
    membership_level,
    max_advance_days
) VALUES (?, ?, ?)
ON CONFLICT(facility_id, membership_level) DO UPDATE SET
    max_advance_days = excluded.max_advance_days
RETURNING
    facility_id,
    membership_level,
    max_advance_days;

-- name: DeleteTierBookingWindow :execrows
DELETE FROM member_tier_booking_windows
WHERE facility_id = ? AND membership_level = ?;

-- name: GetFacilityTierBookingEnabled :one
SELECT tier_booking_enabled
FROM facilities
WHERE id = ?;
