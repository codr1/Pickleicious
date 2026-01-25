-- name: ListDistinctFacilitiesWithScheduledSessions :many
SELECT DISTINCT facility_id
FROM open_play_sessions
WHERE status = 'scheduled'
  AND start_time > @comparison_time
ORDER BY facility_id;

-- name: ListMemberUpcomingOpenPlaySessions :many
SELECT ops.id,
    ops.start_time,
    ops.end_time,
    ops.status,
    opr.name AS rule_name,
    (
        SELECT COUNT(*)
        FROM reservation_participants rp
        JOIN reservations r ON r.id = rp.reservation_id
        JOIN reservation_types rt ON rt.id = r.reservation_type_id
        WHERE r.facility_id = ops.facility_id
          AND r.open_play_rule_id = ops.open_play_rule_id
          AND r.start_time = ops.start_time
          AND r.end_time = ops.end_time
          AND rt.name = 'OPEN_PLAY'
    ) AS participant_count,
    opr.min_participants
FROM open_play_sessions ops
JOIN open_play_rules opr
  ON ops.open_play_rule_id = opr.id
-- Empty facility_ids intentionally yields zero rows (caller should prefilter).
WHERE ops.facility_id IN (sqlc.slice('facility_ids'))
  AND ops.status = 'scheduled'
  AND ops.start_time > @comparison_time
ORDER BY ops.start_time;

-- name: ListOpenPlaySessionsApproachingCutoff :many
SELECT open_play_sessions.id,
    open_play_sessions.facility_id,
    open_play_sessions.open_play_rule_id,
    open_play_sessions.start_time,
    open_play_sessions.end_time,
    open_play_sessions.status,
    open_play_sessions.current_court_count,
    open_play_sessions.auto_scale_override,
    open_play_sessions.cancelled_at,
    open_play_sessions.cancellation_reason,
    open_play_sessions.created_at,
    open_play_sessions.updated_at
FROM open_play_sessions
JOIN open_play_rules
  ON open_play_sessions.open_play_rule_id = open_play_rules.id
WHERE open_play_sessions.facility_id = @facility_id
  AND open_play_sessions.status = 'scheduled'
  AND open_play_sessions.start_time > @comparison_time
  AND open_play_sessions.start_time <= datetime(
        @comparison_time,
        '+' || open_play_rules.cancellation_cutoff_minutes || ' minutes'
      )
ORDER BY open_play_sessions.start_time;

-- name: ListOpenPlaySessions :many
SELECT id, facility_id, open_play_rule_id, start_time, end_time, status,
    current_court_count, auto_scale_override, cancelled_at, cancellation_reason,
    created_at, updated_at
FROM open_play_sessions
WHERE facility_id = @facility_id
ORDER BY start_time;

-- name: CreateOpenPlaySession :one
INSERT INTO open_play_sessions (
    facility_id,
    open_play_rule_id,
    start_time,
    end_time,
    status,
    current_court_count,
    auto_scale_override,
    cancelled_at,
    cancellation_reason
) VALUES (
    @facility_id,
    @open_play_rule_id,
    @start_time,
    @end_time,
    @status,
    @current_court_count,
    @auto_scale_override,
    @cancelled_at,
    @cancellation_reason
)
RETURNING id, facility_id, open_play_rule_id, start_time, end_time, status,
    current_court_count, auto_scale_override, cancelled_at, cancellation_reason,
    created_at, updated_at;

-- name: GetOpenPlaySession :one
SELECT ops.id,
    ops.facility_id,
    ops.open_play_rule_id,
    ops.start_time,
    ops.end_time,
    ops.status,
    ops.current_court_count,
    ops.auto_scale_override,
    ops.cancelled_at,
    ops.cancellation_reason,
    ops.created_at,
    ops.updated_at,
    (
        SELECT COUNT(*)
        FROM reservation_participants rp
        JOIN reservations r ON r.id = rp.reservation_id
        JOIN reservation_types rt ON rt.id = r.reservation_type_id
        WHERE r.facility_id = ops.facility_id
          AND r.open_play_rule_id = ops.open_play_rule_id
          AND r.start_time = ops.start_time
          AND r.end_time = ops.end_time
          AND rt.name = 'OPEN_PLAY'
    ) AS participant_count
FROM open_play_sessions ops
WHERE ops.id = @id
  AND ops.facility_id = @facility_id;

-- name: UpdateOpenPlaySessionStatus :one
UPDATE open_play_sessions
SET status = @status,
    cancelled_at = @cancelled_at,
    cancellation_reason = @cancellation_reason,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, facility_id, open_play_rule_id, start_time, end_time, status,
    current_court_count, auto_scale_override, cancelled_at, cancellation_reason,
    created_at, updated_at;

-- name: UpdateOpenPlaySessionCourtCount :one
UPDATE open_play_sessions
SET current_court_count = @current_court_count,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, facility_id, open_play_rule_id, start_time, end_time, status,
    current_court_count, auto_scale_override, cancelled_at, cancellation_reason,
    created_at, updated_at;

-- name: UpdateSessionAutoScaleOverride :one
UPDATE open_play_sessions
SET auto_scale_override = @auto_scale_override,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, facility_id, open_play_rule_id, start_time, end_time, status,
    current_court_count, auto_scale_override, cancelled_at, cancellation_reason,
    created_at, updated_at;

-- name: CreateOpenPlayAuditLog :one
INSERT INTO open_play_audit_log (
    session_id,
    action,
    before_state,
    after_state,
    reason
) VALUES (
    @session_id,
    @action,
    @before_state,
    @after_state,
    @reason
)
RETURNING id, session_id, action, before_state, after_state, reason, created_at;

-- name: ListOpenPlayAuditLog :many
SELECT open_play_audit_log.id,
    open_play_audit_log.session_id,
    open_play_audit_log.action,
    open_play_audit_log.before_state,
    open_play_audit_log.after_state,
    open_play_audit_log.reason,
    open_play_audit_log.created_at
FROM open_play_audit_log
JOIN open_play_sessions
  ON open_play_audit_log.session_id = open_play_sessions.id
WHERE open_play_audit_log.session_id = @session_id
  AND open_play_sessions.facility_id = @facility_id
ORDER BY open_play_audit_log.created_at;

-- name: AddOpenPlayParticipant :one
INSERT INTO reservation_participants (reservation_id, user_id)
SELECT r.id, @user_id
FROM reservations r
JOIN reservation_types rt ON rt.id = r.reservation_type_id
WHERE r.facility_id = @facility_id
  AND r.open_play_rule_id = @open_play_rule_id
  AND r.start_time = @start_time
  AND r.end_time = @end_time
  AND rt.name = 'OPEN_PLAY'
LIMIT 1
RETURNING id, reservation_id, user_id, created_at, updated_at;

-- name: IsMemberOpenPlayParticipant :one
SELECT EXISTS (
    SELECT 1
    FROM open_play_sessions ops
    JOIN reservations r
      ON r.facility_id = ops.facility_id
     AND r.open_play_rule_id = ops.open_play_rule_id
     AND r.start_time = ops.start_time
     AND r.end_time = ops.end_time
    JOIN reservation_types rt ON rt.id = r.reservation_type_id
    JOIN reservation_participants rp ON rp.reservation_id = r.id
    WHERE ops.id = @session_id
      AND ops.facility_id = @facility_id
      AND rt.name = 'OPEN_PLAY'
      AND rp.user_id = @user_id
) AS is_participant;

-- name: CountOpenPlayReservationsForSession :one
SELECT COUNT(*)
FROM reservations r
JOIN reservation_types rt ON rt.id = r.reservation_type_id
WHERE r.facility_id = @facility_id
  AND r.open_play_rule_id = @open_play_rule_id
  AND r.start_time = @start_time
  AND r.end_time = @end_time
  AND rt.name = 'OPEN_PLAY';

-- name: RemoveOpenPlayParticipant :execrows
DELETE FROM reservation_participants
WHERE reservation_id = (
    SELECT r.id
    FROM reservations r
    JOIN reservation_types rt ON rt.id = r.reservation_type_id
    WHERE r.facility_id = @facility_id
      AND r.open_play_rule_id = @open_play_rule_id
      AND r.start_time = @start_time
      AND r.end_time = @end_time
      AND rt.name = 'OPEN_PLAY'
    LIMIT 1
)
  AND user_id = @user_id;

-- name: ListOpenPlayParticipants :many
SELECT u.id,
    u.first_name,
    u.last_name,
    u.photo_url
FROM reservation_participants rp
JOIN reservations r ON r.id = rp.reservation_id
JOIN reservation_types rt ON rt.id = r.reservation_type_id
JOIN users u ON u.id = rp.user_id
WHERE r.facility_id = @facility_id
  AND r.open_play_rule_id = @open_play_rule_id
  AND r.start_time = @start_time
  AND r.end_time = @end_time
  AND rt.name = 'OPEN_PLAY'
ORDER BY u.last_name, u.first_name;

-- name: CreateStaffNotification :one
INSERT INTO staff_notifications (
    facility_id,
    notification_type,
    message,
    related_session_id
) VALUES (
    @facility_id,
    @notification_type,
    @message,
    @related_session_id
)
RETURNING id, facility_id, notification_type, message, related_session_id,
    related_reservation_id, target_staff_id, read, created_at, updated_at;

-- name: CreateLessonCancelledNotification :one
INSERT INTO staff_notifications (
    facility_id,
    notification_type,
    message,
    related_reservation_id,
    target_staff_id
)
SELECT
    r.facility_id,
    'lesson_cancelled',
    printf(
        'Lesson cancelled: %s with %s (%s - %s)',
        COALESCE(NULLIF(TRIM(COALESCE(mu.first_name, '') || ' ' || COALESCE(mu.last_name, '')), ''), 'Member'),
        COALESCE(NULLIF(TRIM(COALESCE(ps.first_name, '') || ' ' || COALESCE(ps.last_name, '')), ''), 'Pro'),
        strftime('%Y-%m-%d %H:%M', r.start_time),
        strftime('%Y-%m-%d %H:%M', r.end_time)
    ),
    r.id,
    r.pro_id
FROM reservations r
LEFT JOIN users mu ON mu.id = r.primary_user_id
LEFT JOIN staff ps ON ps.id = r.pro_id
WHERE r.id = @reservation_id
RETURNING id, facility_id, notification_type, message, related_session_id,
    related_reservation_id, target_staff_id, read, created_at, updated_at;

-- name: CountUnreadStaffNotifications :one
SELECT COUNT(*)
FROM staff_notifications
WHERE (@facility_id IS NULL OR facility_id = @facility_id)
  AND read = 0;

-- name: ListStaffNotifications :many
SELECT id, facility_id, notification_type, message, related_session_id,
    related_reservation_id, target_staff_id, read, created_at, updated_at
FROM staff_notifications
WHERE facility_id = @facility_id
ORDER BY created_at DESC
LIMIT @limit OFFSET @offset;

-- name: ListStaffNotificationsForStaff :many
SELECT id, facility_id, notification_type, message, related_session_id,
    related_reservation_id, target_staff_id, read, created_at, updated_at
FROM staff_notifications
WHERE target_staff_id = @target_staff_id
ORDER BY created_at DESC
LIMIT @limit OFFSET @offset;

-- name: ListStaffNotificationsForFacilityOrCorporate :many
SELECT id, facility_id, notification_type, message, related_session_id,
    related_reservation_id, target_staff_id, read, created_at, updated_at
FROM staff_notifications
WHERE @facility_id IS NULL
   OR facility_id = @facility_id
ORDER BY created_at DESC
LIMIT @limit OFFSET @offset;

-- name: GetStaffNotificationByID :one
SELECT id, facility_id, notification_type, message, related_session_id,
    related_reservation_id, target_staff_id, read, created_at, updated_at
FROM staff_notifications
WHERE id = @id;

-- name: MarkStaffNotificationAsRead :one
UPDATE staff_notifications
SET read = 1,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, facility_id, notification_type, message, related_session_id,
    related_reservation_id, target_staff_id, read, created_at, updated_at;
