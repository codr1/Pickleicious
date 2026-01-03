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
SELECT id, facility_id, open_play_rule_id, start_time, end_time, status,
    current_court_count, auto_scale_override, cancelled_at, cancellation_reason,
    created_at, updated_at
FROM open_play_sessions
WHERE id = @id
  AND facility_id = @facility_id;

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
RETURNING id, facility_id, notification_type, message, related_session_id, read,
    created_at, updated_at;

-- name: ListStaffNotifications :many
SELECT id, facility_id, notification_type, message, related_session_id, read,
    created_at, updated_at
FROM staff_notifications
WHERE facility_id = @facility_id
ORDER BY created_at DESC
LIMIT @limit OFFSET @offset;

-- name: MarkStaffNotificationAsRead :one
UPDATE staff_notifications
SET read = 1,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, facility_id, notification_type, message, related_session_id, read,
    created_at, updated_at;
