-- name: CreateCancellationPolicyTier :one
INSERT INTO cancellation_policy_tiers (
    facility_id,
    min_hours_before,
    refund_percentage
) VALUES (
    @facility_id,
    @min_hours_before,
    @refund_percentage
)
RETURNING
    id,
    facility_id,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at;

-- name: ListCancellationPolicyTiers :many
SELECT
    id,
    facility_id,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at
FROM cancellation_policy_tiers
WHERE facility_id = @facility_id
ORDER BY min_hours_before DESC;

-- name: GetCancellationPolicyTier :one
SELECT
    id,
    facility_id,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at
FROM cancellation_policy_tiers
WHERE id = @id
  AND facility_id = @facility_id;

-- name: UpdateCancellationPolicyTier :one
UPDATE cancellation_policy_tiers
SET min_hours_before = @min_hours_before,
    refund_percentage = @refund_percentage,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING
    id,
    facility_id,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at;

-- name: DeleteCancellationPolicyTier :execrows
DELETE FROM cancellation_policy_tiers
WHERE id = @id
  AND facility_id = @facility_id;

-- name: GetApplicableCancellationTier :one
SELECT
    id,
    facility_id,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at
FROM cancellation_policy_tiers
WHERE facility_id = @facility_id
  AND min_hours_before <= @hours_until_reservation
ORDER BY min_hours_before DESC
LIMIT 1;
