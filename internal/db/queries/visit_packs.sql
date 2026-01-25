-- internal/db/queries/visit_packs.sql

-- name: CreateVisitPackType :one
INSERT INTO visit_pack_types (
    facility_id,
    name,
    price_cents,
    visit_count,
    valid_days,
    status
) VALUES (
    @facility_id,
    @name,
    @price_cents,
    @visit_count,
    @valid_days,
    @status
)
RETURNING id, facility_id, name, price_cents, visit_count, valid_days, status,
    created_at, updated_at;

-- name: ListVisitPackTypes :many
SELECT id, facility_id, name, price_cents, visit_count, valid_days, status,
    created_at, updated_at
FROM visit_pack_types
WHERE facility_id = @facility_id
ORDER BY name;

-- name: GetVisitPackType :one
SELECT id, facility_id, name, price_cents, visit_count, valid_days, status,
    created_at, updated_at
FROM visit_pack_types
WHERE id = @id
  AND facility_id = @facility_id;

-- name: UpdateVisitPackType :one
UPDATE visit_pack_types
SET name = @name,
    price_cents = @price_cents,
    visit_count = @visit_count,
    valid_days = @valid_days,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, facility_id, name, price_cents, visit_count, valid_days, status,
    created_at, updated_at;

-- name: DeactivateVisitPackType :one
UPDATE visit_pack_types
SET status = 'inactive',
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, facility_id, name, price_cents, visit_count, valid_days, status,
    created_at, updated_at;

-- name: CountVisitPackTypesByFacility :one
SELECT COUNT(*)
FROM visit_pack_types
WHERE facility_id = @facility_id;

-- name: CreateVisitPack :one
INSERT INTO visit_packs (
    pack_type_id,
    user_id,
    purchase_date,
    expires_at,
    visits_remaining,
    status
)
SELECT
    vpt.id,
    @user_id,
    @purchase_date,
    datetime(@purchase_date, '+' || vpt.valid_days || ' days'),
    vpt.visit_count,
    @status
FROM visit_pack_types vpt
WHERE vpt.id = @pack_type_id
  AND vpt.status = 'active'
RETURNING id, pack_type_id, user_id, purchase_date, expires_at,
    visits_remaining, status, created_at, updated_at;

-- name: GetVisitPack :one
SELECT id, pack_type_id, user_id, purchase_date, expires_at,
    visits_remaining, status, created_at, updated_at
FROM visit_packs
WHERE id = @id
  AND user_id = @user_id;

-- name: ListActiveVisitPacksForUser :many
SELECT id, pack_type_id, user_id, purchase_date, expires_at,
    visits_remaining, status, created_at, updated_at
FROM visit_packs
WHERE user_id = @user_id
  AND status = 'active'
  AND visits_remaining > 0
  AND expires_at > @comparison_time
ORDER BY expires_at;

-- name: DecrementVisitPackVisit :one
UPDATE visit_packs
SET visits_remaining = visits_remaining - 1,
    status = CASE
        WHEN visits_remaining - 1 <= 0 THEN 'depleted'
        ELSE status
    END,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND status = 'active'
  AND visits_remaining > 0
RETURNING id, pack_type_id, user_id, purchase_date, expires_at,
    visits_remaining, status, created_at, updated_at;

-- name: CreateVisitPackRedemption :one
INSERT INTO visit_pack_redemptions (
    visit_pack_id,
    facility_id,
    redeemed_at,
    reservation_id
) VALUES (
    @visit_pack_id,
    @facility_id,
    @redeemed_at,
    @reservation_id
)
RETURNING id, visit_pack_id, facility_id, redeemed_at, reservation_id, created_at;
