-- internal/db/queries/clinics.sql

-- name: CreateClinicType :one
INSERT INTO clinic_types (
    facility_id,
    name,
    description,
    min_participants,
    max_participants,
    price_cents,
    status
) VALUES (
    @facility_id,
    @name,
    @description,
    @min_participants,
    @max_participants,
    @price_cents,
    @status
)
RETURNING id, facility_id, name, description, min_participants, max_participants,
    price_cents, status, created_at, updated_at;

-- name: GetClinicType :one
SELECT id, facility_id, name, description, min_participants, max_participants,
    price_cents, status, created_at, updated_at
FROM clinic_types
WHERE id = @id
  AND facility_id = @facility_id;

-- name: ListClinicTypesByFacility :many
SELECT id, facility_id, name, description, min_participants, max_participants,
    price_cents, status, created_at, updated_at
FROM clinic_types
WHERE facility_id = @facility_id
ORDER BY name;

-- name: UpdateClinicType :one
UPDATE clinic_types
SET name = @name,
    description = @description,
    min_participants = @min_participants,
    max_participants = @max_participants,
    price_cents = @price_cents,
    status = @status,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, facility_id, name, description, min_participants, max_participants,
    price_cents, status, created_at, updated_at;

-- name: DeleteClinicType :execrows
DELETE FROM clinic_types
WHERE id = @id
  AND facility_id = @facility_id;
