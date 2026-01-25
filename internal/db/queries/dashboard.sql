-- name: CountReservationsByTypeInRange :many
SELECT reservation_type_id,
    COUNT(*) AS reservation_count
FROM reservations
WHERE facility_id = @facility_id
  AND start_time < @end_time
  AND end_time > @start_time
  AND NOT EXISTS (
      SELECT 1
      FROM reservation_cancellations rcc
      WHERE rcc.reservation_id = reservations.id
  )
GROUP BY reservation_type_id
ORDER BY reservation_type_id;

-- name: CountCheckinsByFacilityInRange :one
SELECT COUNT(*) AS checkins_count
FROM facility_visits
WHERE facility_id = @facility_id
  AND check_in_time >= @start_time
  AND check_in_time < @end_time;

-- name: GetCancellationMetricsInRange :one
WITH cancellation_counts AS (
    SELECT COUNT(*) AS cancellations_count,
        CAST(IFNULL(SUM(rc.refund_percentage_applied), 0) AS REAL)
            AS total_refund_percentage
    FROM reservation_cancellations rc
    JOIN reservations r ON r.id = rc.reservation_id
    WHERE r.facility_id = @facility_id
      AND rc.cancelled_at >= @start_time
      AND rc.cancelled_at < @end_time
),
reservation_counts AS (
    SELECT COUNT(*) AS total_reservations
    FROM reservations r
    WHERE r.facility_id = @facility_id
      AND r.start_time < @end_time
      AND r.end_time > @start_time
)
SELECT cancellation_counts.cancellations_count,
    reservation_counts.total_reservations,
    CAST(
        COALESCE(
            CAST(cancellation_counts.cancellations_count AS REAL)
            / NULLIF(CAST(reservation_counts.total_reservations AS REAL), 0),
            0
        ) AS REAL
    ) AS cancellation_rate,
    CAST(cancellation_counts.total_refund_percentage AS REAL)
        AS total_refund_percentage
FROM cancellation_counts, reservation_counts;

-- name: GetAvailableCourtHours :one
WITH RECURSIVE date_range(day) AS (
    SELECT datetime(@start_time, 'start of day')
    UNION ALL
    SELECT datetime(day, '+1 day')
    FROM date_range
    WHERE day < datetime(@end_time, '-1 day', 'start of day')
),
court_count AS (
    SELECT COUNT(*) AS total_courts
    FROM courts
    WHERE courts.facility_id = @facility_id
      AND status = 'active'
),
hours_by_day AS (
    SELECT (julianday('2000-01-01 ' || oh.closes_at)
        - julianday('2000-01-01 ' || oh.opens_at)) * 24.0 AS open_hours
    FROM date_range dr
    JOIN operating_hours oh
      ON oh.facility_id = @facility_id
     AND oh.day_of_week = CAST(strftime('%w', dr.day) AS INTEGER)
)
SELECT CAST(
        COALESCE((SELECT SUM(open_hours) FROM hours_by_day), CAST(0 AS REAL))
        * court_count.total_courts AS REAL
    ) AS available_court_hours
FROM court_count;

-- name: GetBookedCourtHours :one
WITH booked AS (
    SELECT (julianday(
            CASE
                WHEN r.end_time < @end_time THEN r.end_time
                ELSE @end_time
            END
        ) - julianday(
            CASE
                WHEN r.start_time > @start_time THEN r.start_time
                ELSE @start_time
            END
        )) * 24.0 AS booked_hours
    FROM reservations r
    JOIN reservation_courts rc ON rc.reservation_id = r.id
    WHERE r.facility_id = @facility_id
      AND r.start_time < @end_time
      AND r.end_time > @start_time
      AND NOT EXISTS (
          SELECT 1
          FROM reservation_cancellations rcc
          WHERE rcc.reservation_id = r.id
      )
)
SELECT CAST(COALESCE(SUM(booked_hours), 0) AS REAL) AS booked_court_hours
FROM booked;

-- name: CountScheduledVsCompletedReservations :many
SELECT CASE
        WHEN r.end_time < CURRENT_TIMESTAMP THEN 'completed'
        ELSE 'scheduled'
    END AS reservation_status,
    COUNT(*) AS reservation_count
FROM reservations r
WHERE r.facility_id = @facility_id
  AND r.start_time < @end_time
  AND r.end_time > @start_time
  AND NOT EXISTS (
      SELECT 1
      FROM reservation_cancellations rcc
      WHERE rcc.reservation_id = r.id
  )
GROUP BY reservation_status
ORDER BY reservation_status;
