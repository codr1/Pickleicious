-- name: CreateCancellationPolicyTier :one
INSERT INTO cancellation_policy_tiers (
    facility_id,
    reservation_type_id,
    min_hours_before,
    refund_percentage
) VALUES (
    @facility_id,
    @reservation_type_id,
    @min_hours_before,
    @refund_percentage
)
RETURNING
    id,
    facility_id,
    reservation_type_id,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at;

-- name: ListCancellationPolicyTiers :many
SELECT
    id,
    facility_id,
    reservation_type_id,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at
FROM cancellation_policy_tiers
WHERE facility_id = @facility_id
  AND (@reservation_type_id IS NULL OR reservation_type_id = @reservation_type_id)
ORDER BY
    reservation_type_id IS NULL,
    reservation_type_id,
    min_hours_before DESC;

-- name: GetCancellationPolicyTier :one
SELECT
    id,
    facility_id,
    reservation_type_id,
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
    reservation_type_id = @reservation_type_id,
    refund_percentage = @refund_percentage,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING
    id,
    facility_id,
    reservation_type_id,
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
    reservation_type_id,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at
FROM cancellation_policy_tiers cpt
WHERE cpt.facility_id = @facility_id
  AND cpt.min_hours_before <= @hours_until_reservation
  AND (
    cpt.reservation_type_id = @reservation_type_id
    OR cpt.reservation_type_id IS NULL
  )
-- Prefer type-specific tiers when available for the same hours window; fall back to defaults.
ORDER BY
    cpt.min_hours_before DESC,
    CASE WHEN cpt.reservation_type_id IS NULL THEN 1 ELSE 0 END
LIMIT 1;

-- name: LogCancellation :one
INSERT INTO reservation_cancellations (
    reservation_id,
    cancelled_by_user_id,
    cancelled_at,
    refund_percentage_applied,
    fee_waived,
    hours_before_start
) VALUES (
    @reservation_id,
    @cancelled_by_user_id,
    @cancelled_at,
    @refund_percentage_applied,
    @fee_waived,
    @hours_before_start
)
RETURNING
    id,
    reservation_id,
    cancelled_by_user_id,
    cancelled_at,
    refund_percentage_applied,
    fee_waived,
    hours_before_start,
    created_at;

-- name: GetLatestCancellationByReservationID :one
SELECT
    id,
    reservation_id,
    cancelled_by_user_id,
    cancelled_at,
    refund_percentage_applied,
    fee_waived,
    hours_before_start,
    created_at
FROM reservation_cancellations
WHERE reservation_id = @reservation_id
ORDER BY cancelled_at DESC
LIMIT 1;
