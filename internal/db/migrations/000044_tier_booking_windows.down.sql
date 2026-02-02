PRAGMA foreign_keys = OFF;

DROP TABLE IF EXISTS member_tier_booking_windows;

ALTER TABLE facilities
DROP COLUMN tier_booking_enabled;

PRAGMA foreign_keys = ON;
