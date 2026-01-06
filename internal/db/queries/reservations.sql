-- name: CreateReservation :one
INSERT INTO reservations (
    facility_id,
    reservation_type_id,
    recurrence_rule_id,
    primary_user_id,
    pro_id,
    open_play_rule_id,
    start_time,
    end_time,
    is_open_event,
    teams_per_court,
    people_per_team
) VALUES (
    @facility_id,
    @reservation_type_id,
    @recurrence_rule_id,
    @primary_user_id,
    @pro_id,
    @open_play_rule_id,
    @start_time,
    @end_time,
    @is_open_event,
    @teams_per_court,
    @people_per_team
)
RETURNING id, facility_id, reservation_type_id, recurrence_rule_id,
    primary_user_id, pro_id, open_play_rule_id, start_time, end_time,
    is_open_event, teams_per_court, people_per_team, created_at, updated_at;

-- name: GetReservation :one
SELECT id, facility_id, reservation_type_id, recurrence_rule_id,
    primary_user_id, pro_id, open_play_rule_id, start_time, end_time,
    is_open_event, teams_per_court, people_per_team, created_at, updated_at
FROM reservations
WHERE id = @id
  AND facility_id = @facility_id;

-- name: GetReservationByID :one
SELECT id, facility_id, reservation_type_id, recurrence_rule_id,
    primary_user_id, pro_id, open_play_rule_id, start_time, end_time,
    is_open_event, teams_per_court, people_per_team, created_at, updated_at
FROM reservations
WHERE id = @id;

-- name: ListReservationsByDateRange :many
SELECT id, facility_id, reservation_type_id, recurrence_rule_id,
    primary_user_id, pro_id, open_play_rule_id, start_time, end_time,
    is_open_event, teams_per_court, people_per_team, created_at, updated_at
FROM reservations
WHERE facility_id = @facility_id
  AND start_time < @end_time
  AND end_time > @start_time
ORDER BY start_time;

-- name: ListReservationCourtsByDateRange :many
SELECT rc.reservation_id, c.court_number
FROM reservation_courts rc
JOIN reservations r ON r.id = rc.reservation_id
JOIN courts c ON c.id = rc.court_id
WHERE r.facility_id = @facility_id
  AND r.start_time < @end_time
  AND r.end_time > @start_time
ORDER BY rc.reservation_id, c.court_number;

-- name: UpdateReservation :one
UPDATE reservations
SET reservation_type_id = @reservation_type_id,
    recurrence_rule_id = @recurrence_rule_id,
    primary_user_id = @primary_user_id,
    pro_id = @pro_id,
    open_play_rule_id = @open_play_rule_id,
    start_time = @start_time,
    end_time = @end_time,
    is_open_event = @is_open_event,
    teams_per_court = @teams_per_court,
    people_per_team = @people_per_team,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND facility_id = @facility_id
RETURNING id, facility_id, reservation_type_id, recurrence_rule_id,
    primary_user_id, pro_id, open_play_rule_id, start_time, end_time,
    is_open_event, teams_per_court, people_per_team, created_at, updated_at;

-- name: DeleteReservation :execrows
DELETE FROM reservations
WHERE id = @id
  AND facility_id = @facility_id;

-- name: AddParticipant :exec
INSERT INTO reservation_participants (reservation_id, user_id)
VALUES (@reservation_id, @user_id);

-- name: RemoveParticipant :exec
DELETE FROM reservation_participants
WHERE reservation_id = @reservation_id
  AND user_id = @user_id;

-- name: ListParticipantsForReservation :many
SELECT u.id, u.email, u.phone, u.first_name, u.last_name, u.photo_url,
    u.is_member, u.is_staff, u.membership_level, u.status
FROM reservation_participants rp
JOIN users u ON u.id = rp.user_id
WHERE rp.reservation_id = @reservation_id
ORDER BY u.last_name, u.first_name;

-- name: GetReservationType :one
SELECT id, name, description, color, created_at, updated_at
FROM reservation_types
WHERE id = @id;

-- name: ListReservationTypes :many
SELECT id, name, description, color, created_at, updated_at
FROM reservation_types
ORDER BY name;

-- name: DeleteReservationParticipantsByReservationID :exec
DELETE FROM reservation_participants
WHERE reservation_id = @reservation_id;

-- name: DeleteReservationCourtsByReservationID :exec
DELETE FROM reservation_courts
WHERE reservation_id = @reservation_id;
