-- name: CountReservationsByTypeInRange :many
SELECT reservation_type_id,
    COUNT(*) AS reservation_count
FROM reservations
WHERE (@facility_id = 0 OR facility_id = @facility_id)
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
WHERE (@facility_id = 0 OR facility_id = @facility_id)
  AND check_in_time >= @start_time
  AND check_in_time < @end_time;

-- name: GetCancellationMetricsInRange :one
WITH cancellation_counts AS (
    SELECT COUNT(*) AS cancellations_count,
        CAST(IFNULL(SUM(rc.refund_percentage_applied), 0) AS REAL)
            AS total_refund_percentage
    FROM reservation_cancellations rc
    JOIN reservations r ON r.id = rc.reservation_id
    WHERE (@facility_id = 0 OR r.facility_id = @facility_id)
      AND rc.cancelled_at >= @start_time
      AND rc.cancelled_at < @end_time
),
reservation_counts AS (
    SELECT COUNT(*) AS total_reservations
    FROM reservations r
    WHERE (@facility_id = 0 OR r.facility_id = @facility_id)
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
    WHERE day < datetime(@end_time, 'start of day')
),
facilities_scope AS (
    SELECT id AS facility_id
    FROM facilities
    WHERE @facility_id = 0 OR id = @facility_id
),
court_count AS (
    SELECT courts.facility_id,
        COUNT(*) AS total_courts
    FROM courts
    JOIN facilities_scope fs ON fs.facility_id = courts.facility_id
    WHERE courts.facility_id = fs.facility_id
      AND status = 'active'
    GROUP BY courts.facility_id
),
hours_by_day AS (
    SELECT oh.facility_id,
        (julianday('2000-01-01 ' || oh.closes_at)
        - julianday('2000-01-01 ' || oh.opens_at)) * 24.0 AS open_hours
    FROM date_range dr
    JOIN operating_hours oh
      ON oh.facility_id IN (SELECT facility_id FROM facilities_scope)
     AND oh.day_of_week = CAST(strftime('%w', dr.day) AS INTEGER)
)
SELECT CAST(
        COALESCE(
            (
                SELECT SUM(hours_by_day.open_hours * court_count.total_courts)
                FROM hours_by_day
                JOIN court_count ON court_count.facility_id = hours_by_day.facility_id
            ),
            CAST(0 AS REAL)
        ) AS REAL
    ) AS available_court_hours
FROM facilities_scope
LIMIT 1;

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
    WHERE (@facility_id = 0 OR r.facility_id = @facility_id)
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
        WHEN r.end_time < @comparison_time THEN 'completed'
        ELSE 'scheduled'
    END AS reservation_status,
    COUNT(*) AS reservation_count
FROM reservations r
WHERE (@facility_id = 0 OR r.facility_id = @facility_id)
  AND r.start_time < @end_time
  AND r.end_time > @start_time
  AND NOT EXISTS (
      SELECT 1
      FROM reservation_cancellations rcc
      WHERE rcc.reservation_id = r.id
  )
GROUP BY reservation_status
ORDER BY reservation_status;
