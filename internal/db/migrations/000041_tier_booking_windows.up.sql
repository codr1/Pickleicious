PRAGMA foreign_keys = ON;

ALTER TABLE facilities
ADD COLUMN tier_booking_enabled BOOLEAN NOT NULL DEFAULT 0;

CREATE TABLE member_tier_booking_windows (
    facility_id INTEGER NOT NULL,
    membership_level INTEGER NOT NULL,
    max_advance_days INTEGER NOT NULL CHECK (max_advance_days >= 1 AND max_advance_days <= 364),
    PRIMARY KEY (facility_id, membership_level),
    FOREIGN KEY (facility_id) REFERENCES facilities(id) ON DELETE CASCADE
);
