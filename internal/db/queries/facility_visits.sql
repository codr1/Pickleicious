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
