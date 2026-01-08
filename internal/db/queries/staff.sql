-- internal/db/queries/staff.sql
-- Queries for staff members (join staff table with users for auth/contact info)

-- name: ListStaff :many
SELECT
    s.*,
    u.email,
    u.phone,
    u.local_auth_enabled,
    u.status as user_status
FROM staff s
JOIN users u ON u.id = s.user_id
WHERE u.status <> 'deleted'
ORDER BY s.last_name, s.first_name;

-- name: GetStaffByID :one
SELECT
    s.*,
    u.email,
    u.phone,
    u.local_auth_enabled,
    u.password_hash,
    u.status as user_status
FROM staff s
JOIN users u ON u.id = s.user_id
WHERE s.id = @id;

-- name: GetStaffByUserID :one
SELECT
    s.*,
    u.email,
    u.phone,
    u.local_auth_enabled,
    u.password_hash,
    u.status as user_status
FROM staff s
JOIN users u ON u.id = s.user_id
WHERE s.user_id = @user_id;

-- name: GetStaffByEmail :one
SELECT
    s.*,
    u.email,
    u.phone,
    u.local_auth_enabled,
    u.password_hash,
    u.status as user_status
FROM staff s
JOIN users u ON u.id = s.user_id
WHERE u.email = @email AND u.status <> 'deleted';

-- name: GetStaffByPhone :one
SELECT
    s.*,
    u.email,
    u.phone,
    u.local_auth_enabled,
    u.password_hash,
    u.status as user_status
FROM staff s
JOIN users u ON u.id = s.user_id
WHERE u.phone = @phone AND u.status <> 'deleted';

-- name: CreateStaff :execlastid
INSERT INTO staff (
    user_id, first_name, last_name, home_facility_id, role
) VALUES (
    @user_id, @first_name, @last_name, @home_facility_id, @role
);

-- name: UpdateStaff :exec
UPDATE staff
SET first_name = @first_name,
    last_name = @last_name,
    home_facility_id = @home_facility_id,
    role = @role,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: DeleteStaff :exec
DELETE FROM staff WHERE id = @id;

-- name: ListStaffByFacility :many
SELECT
    s.*,
    u.email,
    u.phone
FROM staff s
JOIN users u ON u.id = s.user_id
WHERE s.home_facility_id = @facility_id
    AND u.status <> 'deleted'
ORDER BY s.last_name, s.first_name;

-- name: ListProsByFacility :many
SELECT
    s.*,
    u.email,
    u.phone
FROM staff s
JOIN users u ON u.id = s.user_id
WHERE s.home_facility_id = @facility_id
    AND s.role = 'pro'
    AND u.status <> 'deleted'
ORDER BY s.last_name, s.first_name;

-- name: ListStaffByRole :many
SELECT
    s.*,
    u.email,
    u.phone
FROM staff s
JOIN users u ON u.id = s.user_id
WHERE s.role = @role
    AND u.status <> 'deleted'
ORDER BY s.last_name, s.first_name;

-- name: GetFutureProSessionsByStaffID :many
SELECT id, facility_id, reservation_type_id, recurrence_rule_id,
    primary_user_id, pro_id, open_play_rule_id, start_time, end_time,
    is_open_event, teams_per_court, people_per_team
FROM reservations
WHERE pro_id = @pro_id
  AND start_time > @start_time
ORDER BY start_time;

-- name: CreateProUnavailability :execlastid
INSERT INTO pro_unavailability (
    pro_id, start_time, end_time, reason
) VALUES (
    @pro_id, @start_time, @end_time, @reason
);

-- name: GetProUnavailabilityByID :one
SELECT id, pro_id, start_time, end_time, reason, created_at, updated_at
FROM pro_unavailability
WHERE id = @id;

-- name: DeleteProUnavailability :exec
DELETE FROM pro_unavailability
WHERE id = @id;

-- name: ListProUnavailabilityByProID :many
SELECT id, pro_id, start_time, end_time, reason, created_at, updated_at
FROM pro_unavailability
WHERE pro_id = @pro_id
ORDER BY start_time;

-- name: ListProUnavailabilityByFacilityAndDateRange :many
SELECT pu.id, pu.pro_id, pu.start_time, pu.end_time, pu.reason, pu.created_at, pu.updated_at
FROM pro_unavailability pu
JOIN staff s ON s.id = pu.pro_id
WHERE s.home_facility_id = @facility_id
  AND pu.start_time < @end_time
  AND pu.end_time > @start_time
ORDER BY pu.start_time;

-- name: GetProLessonSlots :many
WITH RECURSIVE
    hours AS (
        SELECT
            datetime(date(@target_date) || ' ' || oh.opens_at) AS day_open,
            datetime(date(@target_date) || ' ' || oh.closes_at) AS day_close
        FROM operating_hours oh
        WHERE oh.facility_id = @facility_id
          AND oh.day_of_week = CAST(strftime('%w', date(@target_date)) AS INTEGER)
    ),
    slots AS (
        SELECT
            hours.day_open AS start_time,
            datetime(hours.day_open, '+' || @slot_minutes || ' minutes') AS end_time
        FROM hours
        WHERE datetime(hours.day_open, '+' || @slot_minutes || ' minutes') <= hours.day_close
        UNION ALL
        SELECT
            datetime(slots.start_time, '+' || @slot_minutes || ' minutes'),
            datetime(slots.end_time, '+' || @slot_minutes || ' minutes')
        FROM slots, hours
        WHERE slots.end_time < hours.day_close
    ),
    busy AS (
        SELECT r.start_time, r.end_time
        FROM reservations r
        JOIN hours ON r.start_time < hours.day_close AND r.end_time > hours.day_open
        WHERE r.pro_id = @pro_id
        UNION ALL
        SELECT pu.start_time, pu.end_time
        FROM pro_unavailability pu
        JOIN hours ON pu.start_time < hours.day_close AND pu.end_time > hours.day_open
        WHERE pu.pro_id = @pro_id
    )
SELECT slots.start_time, slots.end_time
FROM slots
WHERE NOT EXISTS (
    SELECT 1
    FROM busy
    WHERE busy.start_time < slots.end_time
      AND busy.end_time > slots.start_time
)
ORDER BY slots.start_time;
