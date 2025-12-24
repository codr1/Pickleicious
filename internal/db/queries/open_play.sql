-- name: CreateOpenPlayRule :one
INSERT INTO open_play_rules (
    facility_id,
    name,
    min_participants,
    max_participants_per_court,
    cancellation_cutoff_minutes,
    auto_scale_enabled,
    min_courts,
    max_courts
) VALUES (
    @facility_id,
    @name,
    @min_participants,
    @max_participants_per_court,
    @cancellation_cutoff_minutes,
    @auto_scale_enabled,
    @min_courts,
    @max_courts
)
RETURNING id, facility_id, name, min_participants, max_participants_per_court,
    cancellation_cutoff_minutes, auto_scale_enabled, min_courts, max_courts,
    created_at, updated_at;

-- name: GetOpenPlayRule :one
SELECT id, facility_id, name, min_participants, max_participants_per_court,
    cancellation_cutoff_minutes, auto_scale_enabled, min_courts, max_courts,
    created_at, updated_at
FROM open_play_rules
WHERE id = @id
  AND facility_id = @facility_id;

-- name: ListOpenPlayRules :many
SELECT id, facility_id, name, min_participants, max_participants_per_court,
    cancellation_cutoff_minutes, auto_scale_enabled, min_courts, max_courts,
    created_at, updated_at
FROM open_play_rules
WHERE facility_id = @facility_id
ORDER BY name;

-- name: UpdateOpenPlayRule :execrows
UPDATE open_play_rules
SET name = @name,
    min_participants = @min_participants,
    max_participants_per_court = @max_participants_per_court,
    cancellation_cutoff_minutes = @cancellation_cutoff_minutes,
    auto_scale_enabled = @auto_scale_enabled,
    min_courts = @min_courts,
    max_courts = @max_courts,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id;

-- name: DeleteOpenPlayRule :execrows
DELETE FROM open_play_rules
WHERE id = @id
  AND facility_id = @facility_id;
