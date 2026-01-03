-- name: GetOpenPlayReservationID :one
SELECT r.id
FROM reservations r
JOIN reservation_types rt ON rt.id = r.reservation_type_id
WHERE r.facility_id = @facility_id
  AND r.open_play_rule_id = @open_play_rule_id
  AND r.start_time = @start_time
  AND r.end_time = @end_time
  AND rt.name = 'OPEN_PLAY'
LIMIT 1;

-- name: CountReservationParticipants :one
SELECT COUNT(*) FROM reservation_participants
WHERE reservation_id = @reservation_id;

-- name: ListReservationCourts :many
SELECT rc.court_id, c.court_number
FROM reservation_courts rc
JOIN courts c ON c.id = rc.court_id
WHERE rc.reservation_id = @reservation_id
ORDER BY c.court_number;

-- name: ListAvailableCourtsForOpenPlay :many
SELECT c.id, c.court_number
FROM courts c
WHERE c.facility_id = @facility_id
  AND c.status = 'active'
  AND c.id NOT IN (
    SELECT rc.court_id
    FROM reservation_courts rc
    JOIN reservations r ON r.id = rc.reservation_id
    WHERE r.facility_id = @facility_id
      AND r.id != @reservation_id
      AND r.start_time < @end_time
      AND r.end_time > @start_time
  )
  AND c.id NOT IN (
    SELECT rc.court_id
    FROM reservation_courts rc
    WHERE rc.reservation_id = @reservation_id
  )
ORDER BY c.court_number;

-- name: AddReservationCourt :exec
INSERT INTO reservation_courts (reservation_id, court_id)
VALUES (@reservation_id, @court_id);

-- name: RemoveReservationCourt :exec
DELETE FROM reservation_courts
WHERE reservation_id = @reservation_id
  AND court_id = @court_id;
