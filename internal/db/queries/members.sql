-- internal/db/queries/members.sql
-- We are using SQLite, so keep the SQL as simple as possible


-- name: ListMembers :many
SELECT m.*, p.id as photo_id
FROM members m
LEFT JOIN member_photos p ON p.member_id = m.id
WHERE ? IS NULL 
   OR m.first_name LIKE '%' || ? || '%'
   OR m.last_name LIKE '%' || ? || '%'
   OR m.email LIKE '%' || ? || '%'
ORDER BY m.last_name, m.first_name
LIMIT ? OFFSET ?;

-- name: GetMemberByID :one
SELECT m.*, p.id as photo_id, b.card_last_four, b.card_type
FROM members m
LEFT JOIN member_photos p ON p.member_id = m.id
LEFT JOIN member_billing b ON b.member_id = m.id
WHERE m.id = ?;

-- name: CreateMember :one
INSERT INTO members (
    first_name, last_name, email, phone,
    street_address, city, state, postal_code, status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateMember :one
UPDATE members
SET first_name = ?,
    last_name = ?,
    email = ?,
    phone = ?,
    street_address = ?,
    city = ?,
    state = ?,
    postal_code = ?,
    status = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: SaveMemberPhoto :one
INSERT INTO member_photos (member_id, data, content_type, size)
VALUES (?, ?, ?, ?)
ON CONFLICT(member_id) DO UPDATE SET
    data = excluded.data,
    content_type = excluded.content_type,
    size = excluded.size,
    updated_at = CURRENT_TIMESTAMP
RETURNING id;

-- name: GetMemberPhoto :one
SELECT data, content_type 
FROM member_photos
WHERE member_id = ?;

-- name: UpdateBillingInfo :one
INSERT INTO member_billing (
    member_id, card_last_four, card_type,
    billing_address, billing_city, billing_state, billing_postal_code
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(member_id) DO UPDATE SET
    card_last_four = excluded.card_last_four,
    card_type = excluded.card_type,
    billing_address = excluded.billing_address,
    billing_city = excluded.billing_city,
    billing_state = excluded.billing_state,
    billing_postal_code = excluded.billing_postal_code,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;
