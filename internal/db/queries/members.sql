-- internal/db/queries/members.sql
-- We are using SQLite, so keep the SQL as simple as possible


-- name: ListMembers :many
SELECT 
    m.id,
    m.first_name,
    m.last_name,
    m.email,
    m.phone,
    m.street_address,
    m.city,
    m.state,
    m.postal_code,
    m.status,
    date_of_birth,
    m.waiver_signed,
    m.created_at,
    m.updated_at,
    p.id as photo_id
FROM members m
LEFT JOIN member_photos p ON p.member_id = m.id
WHERE m.status != 'deleted'
  AND (@search_term IS NULL 
    OR m.first_name LIKE '%' || @search_term || '%'
    OR m.last_name LIKE '%' || @search_term || '%'
    OR m.email LIKE '%' || @search_term || '%')
ORDER BY m.last_name, m.first_name
LIMIT @limit OFFSET @offset;

-- name: GetMemberByID :one
SELECT 
    m.id,
    m.first_name,
    m.last_name,
    m.email,
    m.phone,
    m.street_address,
    m.city,
    m.state,
    m.postal_code,
    m.status,
    date_of_birth,
    m.waiver_signed,
    m.created_at,
    m.updated_at,
    p.id as photo_id,
    b.card_last_four,
    b.card_type,
    b.billing_address,
    b.billing_city,
    b.billing_state,
    b.billing_postal_code
FROM members m
LEFT JOIN member_photos p ON p.member_id = m.id
LEFT JOIN member_billing b ON b.member_id = m.id
WHERE m.id = @id AND m.status != 'deleted';

-- name: CreateMember :one
INSERT INTO members (
    first_name, last_name, email, phone,
    street_address, city, state, postal_code, 
    status, date_of_birth, waiver_signed
) VALUES (
    @first_name, @last_name, @email, @phone,
    @street_address, @city, @state, @postal_code,
    @status, 
    strftime('%Y-%m-%d', @date_of_birth),
    @waiver_signed
)
RETURNING *;


-- name: DeleteMember :exec
UPDATE members 
SET status = 'deleted',
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id;


-- name: UpdateMember :one
UPDATE members
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
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING *;

-- name: SaveMemberPhoto :one
INSERT INTO member_photos (member_id, data, content_type, size)
VALUES (@member_id, @data, @content_type, @size)
ON CONFLICT(member_id) DO UPDATE SET
    data = excluded.data,
    content_type = excluded.content_type,
    size = excluded.size,
    updated_at = CURRENT_TIMESTAMP
RETURNING id;

-- name: GetMemberPhoto :one
SELECT data, content_type 
FROM member_photos
WHERE member_id = @member_id;

-- name: UpdateBillingInfo :one
INSERT INTO member_billing (
    member_id, card_last_four, card_type,
    billing_address, billing_city, billing_state, billing_postal_code
) VALUES (
    @member_id, @card_last_four, @card_type,
    @billing_address, @billing_city, @billing_state, @billing_postal_code
)
ON CONFLICT(member_id) DO UPDATE SET
    card_last_four = excluded.card_last_four,
    card_type = excluded.card_type,
    billing_address = excluded.billing_address,
    billing_city = excluded.billing_city,
    billing_state = excluded.billing_state,
    billing_postal_code = excluded.billing_postal_code,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: CreatePhoto :one
INSERT INTO member_photos (member_id, data, content_type, size)
VALUES (@member_id, @data, @content_type, @size)
RETURNING id;

-- name: GetPhoto :one
SELECT data, content_type 
FROM member_photos
WHERE id = @id;

-- name: DeletePhoto :exec
DELETE FROM member_photos
WHERE id = @id;
