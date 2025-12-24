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
