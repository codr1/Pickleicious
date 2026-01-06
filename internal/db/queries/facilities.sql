-- internal/db/queries/facilities.sql

-- name: ListFacilities :many
SELECT
    id,
    organization_id,
    name,
    slug,
    timezone,
    active_theme_id,
    max_advance_booking_days,
    max_member_reservations,
    created_at,
    updated_at
FROM facilities
ORDER BY name;

-- name: GetFacilityByID :one
SELECT
    id,
    organization_id,
    name,
    slug,
    timezone,
    active_theme_id,
    max_advance_booking_days,
    max_member_reservations,
    created_at,
    updated_at
FROM facilities
WHERE id = ?;
