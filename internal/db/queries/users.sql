-- internal/db/queries/users.sql

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = @email LIMIT 1;

-- name: GetUserByPhone :one
SELECT * FROM users WHERE phone = @phone LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = @id LIMIT 1;

-- name: UpdateUserCognitoStatus :exec
UPDATE users
SET cognito_status = @cognito_status,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: CreateStaffUser :execlastid
INSERT INTO users (
    first_name, last_name, email, phone,
    home_facility_id, local_auth_enabled,
    is_staff, staff_role, status
) VALUES (
    @first_name, @last_name, @email, @phone,
    @home_facility_id, @local_auth_enabled,
    1, @staff_role, @status
);

-- name: UpdateStaffUser :exec
UPDATE users
SET first_name = @first_name,
    last_name = @last_name,
    email = @email,
    phone = @phone,
    home_facility_id = @home_facility_id,
    local_auth_enabled = @local_auth_enabled,
    staff_role = @staff_role,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id AND is_staff = 1;

-- name: UpdateUserStatus :exec
UPDATE users
SET status = @status,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: UpdateUserPasswordHash :exec
UPDATE users
SET password_hash = @password_hash,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;
