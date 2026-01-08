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
    lesson_min_notice_hours,
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
    lesson_min_notice_hours,
    created_at,
    updated_at
FROM facilities
WHERE id = ?;

-- name: UpdateFacilityBookingConfig :one
UPDATE facilities
SET max_advance_booking_days = @max_advance_booking_days,
    max_member_reservations = @max_member_reservations,
    lesson_min_notice_hours = @lesson_min_notice_hours,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING
    id,
    organization_id,
    name,
    slug,
    timezone,
    active_theme_id,
    max_advance_booking_days,
    max_member_reservations,
    lesson_min_notice_hours,
    created_at,
    updated_at;
