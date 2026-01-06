-- internal/db/queries/facility_visits.sql

-- name: CreateFacilityVisit :one
INSERT INTO facility_visits (
    user_id,
    facility_id,
    check_out_time,
    checked_in_by_staff_id,
    activity_type,
    related_reservation_id
) VALUES (
    @user_id,
    @facility_id,
    @check_out_time,
    @checked_in_by_staff_id,
    @activity_type,
    @related_reservation_id
)
RETURNING id, user_id, facility_id, check_in_time, check_out_time,
    checked_in_by_staff_id, activity_type, related_reservation_id,
    created_at, updated_at;

-- name: ListTodayVisitsByFacility :many
SELECT id, user_id, facility_id, check_in_time, check_out_time,
    checked_in_by_staff_id, activity_type, related_reservation_id,
    created_at, updated_at
FROM facility_visits
WHERE facility_id = @facility_id
  AND check_in_time >= @today_start
  AND check_in_time < @today_end
ORDER BY check_in_time DESC;

-- name: ListRecentVisitsByUser :many
SELECT id, user_id, facility_id, check_in_time, check_out_time,
    checked_in_by_staff_id, activity_type, related_reservation_id,
    created_at, updated_at
FROM facility_visits
WHERE user_id = @user_id
ORDER BY check_in_time DESC
LIMIT 10;

-- name: GetMemberTodayActivities :many
SELECT r.id AS reservation_id,
    r.start_time,
    r.end_time,
    CASE
        WHEN rt.name = 'OPEN_PLAY' THEN 'open_play'
        WHEN rt.name = 'LEAGUE' THEN 'league'
        ELSE 'court_reservation'
    END AS activity_type,
    rt.name AS reservation_type_name,
    COALESCE(
        group_concat(DISTINCT COALESCE(NULLIF(c.name, ''), 'Court ' || c.court_number)),
        ''
    ) AS court_label
FROM reservations r
JOIN reservation_types rt ON rt.id = r.reservation_type_id
LEFT JOIN reservation_courts rc ON rc.reservation_id = r.id
LEFT JOIN courts c ON c.id = rc.court_id
LEFT JOIN reservation_participants rp ON rp.reservation_id = r.id
LEFT JOIN open_play_sessions ops
  ON rt.name = 'OPEN_PLAY'
  AND ops.facility_id = r.facility_id
  AND ops.open_play_rule_id = r.open_play_rule_id
  AND ops.start_time = r.start_time
  AND ops.end_time = r.end_time
  AND ops.status = 'scheduled'
WHERE r.facility_id = @facility_id
  AND r.start_time >= @today_start
  AND r.start_time < @today_end
  AND (
    (rt.name = 'OPEN_PLAY' AND rp.user_id = @user_id AND ops.id IS NOT NULL)
    OR (rt.name != 'OPEN_PLAY' AND (r.primary_user_id = @user_id OR rp.user_id = @user_id))
  )
GROUP BY r.id, r.start_time, r.end_time, rt.name
ORDER BY r.start_time;

-- name: UpdateFacilityVisitActivity :one
UPDATE facility_visits
SET activity_type = @activity_type,
    related_reservation_id = @related_reservation_id,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND user_id = @user_id
  AND facility_id = @facility_id
RETURNING id, user_id, facility_id, check_in_time, check_out_time,
    checked_in_by_staff_id, activity_type, related_reservation_id,
    created_at, updated_at;
