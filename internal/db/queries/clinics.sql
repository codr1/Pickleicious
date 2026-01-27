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

-- name: CreateClinicSession :one
INSERT INTO clinic_sessions (
    clinic_type_id,
    facility_id,
    pro_id,
    start_time,
    end_time,
    enrollment_status,
    created_by_user_id
) VALUES (
    @clinic_type_id,
    @facility_id,
    @pro_id,
    @start_time,
    @end_time,
    @enrollment_status,
    @created_by_user_id
)
RETURNING id, clinic_type_id, facility_id, pro_id, start_time, end_time,
    enrollment_status, created_by_user_id, created_at, updated_at;

-- name: GetClinicSessionByID :one
SELECT id, clinic_type_id, facility_id, pro_id, start_time, end_time,
    enrollment_status, created_by_user_id, created_at, updated_at
FROM clinic_sessions
WHERE id = @id;

-- name: GetClinicSession :one
SELECT id, clinic_type_id, facility_id, pro_id, start_time, end_time,
    enrollment_status, created_by_user_id, created_at, updated_at
FROM clinic_sessions
WHERE id = @id
  AND facility_id = @facility_id;

-- name: ListClinicSessionsByFacility :many
SELECT id, clinic_type_id, facility_id, pro_id, start_time, end_time,
    enrollment_status, created_by_user_id, created_at, updated_at
FROM clinic_sessions
WHERE facility_id = @facility_id
ORDER BY start_time;

-- name: UpdateClinicSession :one
UPDATE clinic_sessions
SET clinic_type_id = @clinic_type_id,
    pro_id = @pro_id,
    start_time = @start_time,
    end_time = @end_time,
    enrollment_status = @enrollment_status,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, clinic_type_id, facility_id, pro_id, start_time, end_time,
    enrollment_status, created_by_user_id, created_at, updated_at;

-- name: DeleteClinicSession :execrows
DELETE FROM clinic_sessions
WHERE id = @id
  AND facility_id = @facility_id;

-- name: CreateClinicEnrollment :one
INSERT INTO clinic_enrollments (
    clinic_session_id,
    user_id,
    status,
    position
) SELECT
    @clinic_session_id,
    @user_id,
    @status,
    COALESCE(MAX(e.position), 0) + 1
FROM clinic_sessions s
LEFT JOIN clinic_enrollments e ON e.clinic_session_id = s.id
WHERE s.id = @clinic_session_id
  AND s.facility_id = @facility_id
GROUP BY s.id
RETURNING id, clinic_session_id, user_id, status, position, created_at, updated_at;

-- name: ListEnrollmentsForClinic :many
SELECT e.id, e.clinic_session_id, e.user_id, e.status, e.position, e.created_at, e.updated_at
FROM clinic_enrollments e
JOIN clinic_sessions s ON s.id = e.clinic_session_id
WHERE s.id = @clinic_session_id
  AND s.facility_id = @facility_id
ORDER BY e.position, e.created_at;

-- name: GetEnrollmentCount :one
SELECT COUNT(*) AS count
FROM clinic_enrollments e
JOIN clinic_sessions s ON s.id = e.clinic_session_id
WHERE s.id = @clinic_session_id
  AND s.facility_id = @facility_id
  AND e.status != 'cancelled';

-- name: UpdateEnrollmentStatus :one
UPDATE clinic_enrollments
SET status = @status,
    updated_at = CURRENT_TIMESTAMP
WHERE clinic_enrollments.id = @id
  AND clinic_session_id IN (
      SELECT id
      FROM clinic_sessions
      WHERE facility_id = @facility_id
  )
RETURNING id, clinic_session_id, user_id, status, position, created_at, updated_at;

-- name: DeleteClinicEnrollment :execrows
DELETE FROM clinic_enrollments
WHERE clinic_enrollments.id = @id
  AND clinic_session_id IN (
      SELECT id
      FROM clinic_sessions
      WHERE facility_id = @facility_id
  );
