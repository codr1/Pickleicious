-- internal/db/queries/waitlist.sql
-- name: CreateWaitlistEntry :one
INSERT INTO waitlists (
    facility_id,
    user_id,
    target_court_id,
    target_date,
    target_start_time,
    target_end_time,
    position,
    status
) VALUES (
    @facility_id,
    @user_id,
    @target_court_id,
    @target_date,
    @target_start_time,
    @target_end_time,
    @position,
    @status
)
RETURNING
    id,
    facility_id,
    user_id,
    target_court_id,
    target_date,
    target_start_time,
    target_end_time,
    position,
    status,
    created_at,
    updated_at;

-- name: GetWaitlistEntry :one
SELECT
    id,
    facility_id,
    user_id,
    target_court_id,
    target_date,
    target_start_time,
    target_end_time,
    position,
    status,
    created_at,
    updated_at
FROM waitlists
WHERE id = @id
  AND facility_id = @facility_id
LIMIT 1;

-- name: ListWaitlistsForSlot :many
SELECT
    id,
    facility_id,
    user_id,
    target_court_id,
    target_date,
    target_start_time,
    target_end_time,
    position,
    status,
    created_at,
    updated_at
FROM waitlists
WHERE facility_id = @facility_id
  AND target_date = @target_date
  AND target_start_time = @target_start_time
  AND target_end_time = @target_end_time
  AND (
      (@target_court_id IS NOT NULL AND (target_court_id = @target_court_id OR target_court_id IS NULL))
      OR (@target_court_id IS NULL AND target_court_id IS NULL)
  )
ORDER BY position;

-- name: ListWaitlistsByUser :many
SELECT
    id,
    facility_id,
    user_id,
    target_court_id,
    target_date,
    target_start_time,
    target_end_time,
    position,
    status,
    created_at,
    updated_at
FROM waitlists
WHERE user_id = @user_id
ORDER BY created_at DESC;

-- name: ListWaitlistsByUserAndFacility :many
SELECT
    id,
    facility_id,
    user_id,
    target_court_id,
    target_date,
    target_start_time,
    target_end_time,
    position,
    status,
    created_at,
    updated_at
FROM waitlists
WHERE user_id = @user_id
  AND facility_id = @facility_id
ORDER BY created_at DESC;

-- name: ListWaitlistsByFacility :many
SELECT
    id,
    facility_id,
    user_id,
    target_court_id,
    target_date,
    target_start_time,
    target_end_time,
    position,
    status,
    created_at,
    updated_at
FROM waitlists
WHERE facility_id = @facility_id
ORDER BY target_date, target_start_time, position;

-- name: UpdateWaitlistStatus :one
UPDATE waitlists
SET status = @status,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING
    id,
    facility_id,
    user_id,
    target_court_id,
    target_date,
    target_start_time,
    target_end_time,
    position,
    status,
    created_at,
    updated_at;

-- name: DeleteWaitlistEntry :execrows
DELETE FROM waitlists
WHERE id = @id
  AND facility_id = @facility_id;

-- name: GetWaitlistConfig :one
SELECT
    id,
    facility_id,
    max_waitlist_size,
    notification_mode,
    offer_expiry_minutes,
    notification_window_minutes,
    created_at,
    updated_at
FROM waitlist_config
WHERE facility_id = @facility_id
LIMIT 1;

-- name: UpsertWaitlistConfig :one
INSERT INTO waitlist_config (
    facility_id,
    max_waitlist_size,
    notification_mode,
    offer_expiry_minutes,
    notification_window_minutes
) VALUES (
    @facility_id,
    @max_waitlist_size,
    @notification_mode,
    @offer_expiry_minutes,
    @notification_window_minutes
)
ON CONFLICT(facility_id) DO UPDATE SET
    max_waitlist_size = excluded.max_waitlist_size,
    notification_mode = excluded.notification_mode,
    offer_expiry_minutes = excluded.offer_expiry_minutes,
    notification_window_minutes = excluded.notification_window_minutes,
    updated_at = CURRENT_TIMESTAMP
RETURNING
    id,
    facility_id,
    max_waitlist_size,
    notification_mode,
    offer_expiry_minutes,
    notification_window_minutes,
    created_at,
    updated_at;

-- name: CreateWaitlistOffer :one
INSERT INTO waitlist_offers (
    waitlist_id,
    expires_at,
    status
) VALUES (
    @waitlist_id,
    @expires_at,
    @status
)
RETURNING
    id,
    waitlist_id,
    offered_at,
    expires_at,
    status;

-- name: GetPendingOffer :one
SELECT
    id,
    waitlist_id,
    offered_at,
    expires_at,
    status
FROM waitlist_offers
WHERE waitlist_id = @waitlist_id
  AND status = 'pending'
ORDER BY offered_at DESC
LIMIT 1;

-- name: ExpireOffer :one
UPDATE waitlist_offers
SET status = 'expired'
WHERE id = @id
  AND waitlist_id = @waitlist_id
RETURNING
    id,
    waitlist_id,
    offered_at,
    expires_at,
    status;

-- name: AcceptOffer :one
UPDATE waitlist_offers
SET status = 'accepted'
WHERE id = @id
  AND waitlist_id = @waitlist_id
RETURNING
    id,
    waitlist_id,
    offered_at,
    expires_at,
    status;
