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
) SELECT
    @facility_id,
    @user_id,
    @target_court_id,
    @target_date,
    @target_start_time,
    @target_end_time,
    COALESCE(MAX(position), 0) + 1,
    @status
FROM waitlists
WHERE facility_id = @facility_id
  AND target_date = @target_date
  AND target_start_time = @target_start_time
  AND target_end_time = @target_end_time
  AND (
      target_court_id = @target_court_id
      OR (@target_court_id IS NULL AND target_court_id IS NULL)
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
      target_court_id = @target_court_id
      OR (@target_court_id IS NULL AND target_court_id IS NULL)
  )
ORDER BY position;

-- name: ListMatchingPendingWaitlistsForCancelledSlot :many
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
  AND status = 'pending'
  AND (
      target_court_id = @target_court_id
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

-- name: DeletePastWaitlistEntries :execrows
DELETE FROM waitlists
WHERE facility_id = @facility_id
  AND (
    target_date < CAST(@comparison_date AS TEXT)
    OR (target_date = CAST(@comparison_date AS TEXT) AND target_end_time < CAST(@comparison_time AS TEXT))
  );

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

-- name: ListExpiredOffers :many
SELECT
    wo.id AS offer_id,
    wo.waitlist_id,
    w.facility_id,
    wc.offer_expiry_minutes
FROM waitlist_offers wo
JOIN waitlists w ON w.id = wo.waitlist_id
JOIN waitlist_config wc ON wc.facility_id = w.facility_id
WHERE wo.status = 'pending'
  AND wo.expires_at < @comparison_time
  AND w.status = 'notified'
  AND wc.notification_mode = 'sequential'
ORDER BY wo.expires_at;

-- name: AdvanceWaitlistOffer :one
INSERT INTO waitlist_offers (waitlist_id, expires_at, status)
SELECT w.id, @expires_at, 'pending'
FROM waitlists w
JOIN waitlists c ON c.id = @waitlist_id
WHERE w.facility_id = c.facility_id
  AND w.target_date = c.target_date
  AND w.target_start_time = c.target_start_time
  AND w.target_end_time = c.target_end_time
  AND (
      w.target_court_id = c.target_court_id
      OR (w.target_court_id IS NULL AND c.target_court_id IS NULL)
  )
  AND w.position = (
      SELECT MIN(w2.position)
      FROM waitlists w2
      WHERE w2.facility_id = c.facility_id
        AND w2.target_date = c.target_date
        AND w2.target_start_time = c.target_start_time
        AND w2.target_end_time = c.target_end_time
        AND (
            w2.target_court_id = c.target_court_id
            OR (w2.target_court_id IS NULL AND c.target_court_id IS NULL)
        )
        AND w2.position > c.position
        AND w2.status = 'pending'
  )
RETURNING
    id,
    waitlist_id,
    offered_at,
    expires_at,
    status;
