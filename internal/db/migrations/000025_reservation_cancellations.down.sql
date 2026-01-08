PRAGMA foreign_keys = OFF;

DROP INDEX IF EXISTS idx_reservation_cancellations_cancelled_at;
DROP INDEX IF EXISTS idx_reservation_cancellations_cancelled_by_user_id;
DROP INDEX IF EXISTS idx_reservation_cancellations_reservation_id;
DROP TABLE IF EXISTS reservation_cancellations;
