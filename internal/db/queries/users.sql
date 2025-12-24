-- internal/db/queries/users.sql

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = @email LIMIT 1;

-- name: GetUserByPhone :one
SELECT * FROM users WHERE phone = @phone LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = @id LIMIT 1;
