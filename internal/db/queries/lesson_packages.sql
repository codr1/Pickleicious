-- internal/db/queries/lesson_packages.sql

-- name: CreateLessonPackageType :one
INSERT INTO lesson_package_types (
    facility_id,
    name,
    price_cents,
    lesson_count,
    valid_days,
    status
) VALUES (
    @facility_id,
    @name,
    @price_cents,
    @lesson_count,
    @valid_days,
    @status
)
RETURNING id, facility_id, name, price_cents, lesson_count, valid_days, status,
    created_at, updated_at;

-- name: ListLessonPackageTypes :many
SELECT id, facility_id, name, price_cents, lesson_count, valid_days, status,
    created_at, updated_at
FROM lesson_package_types
WHERE facility_id = @facility_id
ORDER BY name;

-- name: GetLessonPackageType :one
SELECT id, facility_id, name, price_cents, lesson_count, valid_days, status,
    created_at, updated_at
FROM lesson_package_types
WHERE id = @id
  AND facility_id = @facility_id;

-- name: GetLessonPackageRedemptionInfo :one
SELECT lp.id AS lesson_package_id,
    lp.pack_type_id,
    lpt.facility_id AS pack_facility_id,
    f.organization_id AS organization_id
FROM lesson_packages lp
JOIN lesson_package_types lpt ON lpt.id = lp.pack_type_id
JOIN facilities f ON f.id = lpt.facility_id
WHERE lp.id = @id;

-- name: UpdateLessonPackageType :one
UPDATE lesson_package_types
SET name = @name,
    price_cents = @price_cents,
    lesson_count = @lesson_count,
    valid_days = @valid_days,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, facility_id, name, price_cents, lesson_count, valid_days, status,
    created_at, updated_at;

-- name: DeactivateLessonPackageType :one
UPDATE lesson_package_types
SET status = 'inactive',
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, facility_id, name, price_cents, lesson_count, valid_days, status,
    created_at, updated_at;

-- name: CountLessonPackageTypesByFacility :one
SELECT COUNT(*)
FROM lesson_package_types
WHERE facility_id = @facility_id;

-- name: CreateLessonPackage :one
INSERT INTO lesson_packages (
    pack_type_id,
    user_id,
    purchase_date,
    expires_at,
    lessons_remaining,
    status
)
SELECT
    lpt.id,
    @user_id,
    @purchase_date,
    datetime(@purchase_date, '+' || lpt.valid_days || ' days'),
    lpt.lesson_count,
    @status
FROM lesson_package_types lpt
WHERE lpt.id = @pack_type_id
  AND lpt.status = 'active'
RETURNING id, pack_type_id, user_id, purchase_date, expires_at,
    lessons_remaining, status, created_at, updated_at;

-- name: GetLessonPackage :one
SELECT id, pack_type_id, user_id, purchase_date, expires_at,
    lessons_remaining, status, created_at, updated_at
FROM lesson_packages
WHERE id = @id
  AND user_id = @user_id;

-- name: ListActiveLessonPackagesForUser :many
SELECT id, pack_type_id, user_id, purchase_date, expires_at,
    lessons_remaining, status, created_at, updated_at
FROM lesson_packages
WHERE user_id = @user_id
  AND status = 'active'
  AND lessons_remaining > 0
  AND expires_at > @comparison_time
ORDER BY expires_at;

-- name: ListActiveLessonPackagesForUserByFacility :many
SELECT lp.id, lp.pack_type_id, lp.user_id, lp.purchase_date, lp.expires_at,
    lp.lessons_remaining, lp.status, lp.created_at, lp.updated_at
FROM lesson_packages lp
JOIN lesson_package_types lpt ON lpt.id = lp.pack_type_id
WHERE lp.user_id = @user_id
  AND lpt.facility_id = @facility_id
  AND lp.status = 'active'
  AND lp.lessons_remaining > 0
  AND lp.expires_at > @comparison_time
ORDER BY lp.expires_at;

-- name: ListActiveLessonPackagesForUserByOrganization :many
SELECT lp.id, lp.pack_type_id, lp.user_id, lp.purchase_date, lp.expires_at,
    lp.lessons_remaining, lp.status, lp.created_at, lp.updated_at
FROM lesson_packages lp
JOIN lesson_package_types lpt ON lpt.id = lp.pack_type_id
JOIN facilities f ON f.id = lpt.facility_id
WHERE lp.user_id = @user_id
  AND f.organization_id = @organization_id
  AND lp.status = 'active'
  AND lp.lessons_remaining > 0
  AND lp.expires_at > @comparison_time
ORDER BY lp.expires_at;

-- name: DecrementLessonPackageLesson :one
UPDATE lesson_packages
SET lessons_remaining = lessons_remaining - 1,
    status = CASE
        WHEN lessons_remaining - 1 <= 0 THEN 'depleted'
        ELSE status
    END,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND status = 'active'
  AND lessons_remaining > 0
  AND expires_at > CURRENT_TIMESTAMP
RETURNING id, pack_type_id, user_id, purchase_date, expires_at,
    lessons_remaining, status, created_at, updated_at;

-- name: CreateLessonPackageRedemption :one
INSERT INTO lesson_package_redemptions (
    lesson_package_id,
    facility_id,
    redeemed_at,
    reservation_id
) VALUES (
    @lesson_package_id,
    @facility_id,
    @redeemed_at,
    @reservation_id
)
RETURNING id, lesson_package_id, facility_id, redeemed_at, reservation_id,
    created_at;
