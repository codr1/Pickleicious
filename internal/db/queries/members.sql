-- internal/db/queries/members.sql
-- Queries for users who are members (is_member = 1)
-- Uses consolidated users table

-- name: ListMembers :many
SELECT
    u.*,
    p.id as photo_id,
    ub.card_type,
    ub.card_last_four,
    ub.billing_address,
    ub.billing_city,
    ub.billing_state,
    ub.billing_postal_code
FROM users u
LEFT JOIN user_photos p ON p.user_id = u.id
LEFT JOIN user_billing ub ON ub.user_id = u.id
WHERE u.is_member = 1
    AND u.status <> 'deleted'
    AND (
        @facility_id IS NULL
        OR u.home_facility_id = @facility_id
    )
    AND (
        @search_term IS NULL
        OR u.first_name LIKE '%' || @search_term || '%'
        OR u.last_name LIKE '%' || @search_term || '%'
        OR u.email LIKE '%' || @search_term || '%'
    )
ORDER BY u.last_name, u.first_name
LIMIT @limit OFFSET @offset;

-- name: SearchMembers :many
SELECT
    u.id,
    u.first_name,
    u.last_name,
    u.email
FROM users u
WHERE u.is_member = 1
    AND u.status <> 'deleted'
    AND (
        @search_term IS NULL
        OR u.first_name LIKE '%' || @search_term || '%'
        OR u.last_name LIKE '%' || @search_term || '%'
        OR u.email LIKE '%' || @search_term || '%'
    )
ORDER BY u.last_name, u.first_name
LIMIT @limit;

-- name: GetMemberByID :one
SELECT
    u.id,
    u.first_name,
    u.last_name,
    u.email,
    u.phone,
    u.street_address,
    u.city,
    u.state,
    u.postal_code,
    u.status,
    u.date_of_birth,
    u.waiver_signed,
    u.membership_level,
    u.created_at,
    u.updated_at,
    up.id as photo_id
FROM users u
LEFT JOIN user_photos up ON up.user_id = u.id
WHERE u.id = @id AND u.is_member = 1 AND u.status != 'deleted';

-- name: GetMemberByEmail :one
SELECT * FROM users
WHERE email = @email AND is_member = 1 AND status != 'deleted'
LIMIT 1;

-- name: GetMemberByEmailIncludeDeleted :one
SELECT * FROM users
WHERE email = @email
  AND email IS NOT NULL
  AND is_member = 1
LIMIT 1;

-- name: CreateMember :execlastid
INSERT INTO users (
    first_name, last_name, email, phone,
    street_address, city, state, postal_code,
    status, date_of_birth, waiver_signed,
    is_member, membership_level
) VALUES (
    @first_name, @last_name, @email, @phone,
    @street_address, @city, @state, @postal_code,
    @status,
    strftime('%Y-%m-%d', @date_of_birth),
    @waiver_signed,
    1, -- is_member = true
    @membership_level
);

-- name: GetCreatedMember :one
SELECT
    u.*,
    p.id as photo_id,
    ub.card_type,
    ub.card_last_four,
    ub.billing_address,
    ub.billing_city,
    ub.billing_state,
    ub.billing_postal_code
FROM users u
LEFT JOIN user_photos p ON p.user_id = u.id
LEFT JOIN user_billing ub ON ub.user_id = u.id
WHERE u.id = last_insert_rowid();

-- name: DeleteMember :exec
UPDATE users
SET status = 'deleted',
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id AND is_member = 1;

-- name: UpdateMember :exec
UPDATE users
SET first_name = @first_name,
    last_name = @last_name,
    email = @email,
    phone = @phone,
    street_address = @street_address,
    city = @city,
    state = @state,
    postal_code = @postal_code,
    status = @status,
    date_of_birth = strftime('%Y-%m-%d', @date_of_birth),
    waiver_signed = @waiver_signed,
    membership_level = @membership_level,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id AND is_member = 1;

-- name: GetUpdatedMember :one
SELECT
    u.*,
    p.id as photo_id,
    ub.card_type,
    ub.card_last_four,
    ub.billing_address,
    ub.billing_city,
    ub.billing_state,
    ub.billing_postal_code
FROM users u
LEFT JOIN user_photos p ON p.user_id = u.id
LEFT JOIN user_billing ub ON ub.user_id = u.id
WHERE u.id = @id;

-- name: GetMemberPhoto :one
SELECT data, content_type
FROM user_photos
WHERE user_id = @user_id;

-- name: UpdateBillingInfo :one
INSERT INTO user_billing (
    user_id, card_last_four, card_type,
    billing_address, billing_city, billing_state, billing_postal_code
) VALUES (
    @user_id, @card_last_four, @card_type,
    @billing_address, @billing_city, @billing_state, @billing_postal_code
)
ON CONFLICT(user_id) DO UPDATE SET
    card_last_four = excluded.card_last_four,
    card_type = excluded.card_type,
    billing_address = excluded.billing_address,
    billing_city = excluded.billing_city,
    billing_state = excluded.billing_state,
    billing_postal_code = excluded.billing_postal_code,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: CreatePhoto :one
INSERT INTO user_photos (user_id, data, content_type, size)
VALUES (@user_id, @data, @content_type, @size)
RETURNING id;

-- name: GetPhoto :one
SELECT data, content_type
FROM user_photos
WHERE id = @id;

-- name: DeletePhoto :exec
DELETE FROM user_photos
WHERE id = @id;

-- name: RestoreMember :exec
UPDATE users
SET status = 'active',
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id AND is_member = 1;

-- name: GetRestoredMember :one
SELECT
    u.*,
    p.id as photo_id,
    ub.card_type,
    ub.card_last_four,
    ub.billing_address,
    ub.billing_city,
    ub.billing_state,
    ub.billing_postal_code
FROM users u
LEFT JOIN user_photos p ON p.user_id = u.id
LEFT JOIN user_billing ub ON ub.user_id = u.id
WHERE u.id = @id;

-- name: UpdateMemberEmail :one
UPDATE users
SET email = @email,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id AND is_member = 1
RETURNING *;

-- name: GetMemberBilling :one
SELECT
    card_last_four,
    card_type,
    billing_address,
    billing_city,
    billing_state,
    billing_postal_code
FROM user_billing
WHERE user_id = @user_id;

-- name: UpsertPhoto :one
INSERT INTO user_photos (user_id, data, content_type, size)
VALUES (@user_id, @data, @content_type, @size)
ON CONFLICT(user_id) DO UPDATE SET
    data = excluded.data,
    content_type = excluded.content_type,
    size = excluded.size,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;
