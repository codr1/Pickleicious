PRAGMA foreign_keys = ON;

------ FACILITY BOOKING CONFIG ------
ALTER TABLE facilities
    ADD COLUMN max_advance_booking_days INTEGER NOT NULL DEFAULT 7;

ALTER TABLE facilities
    ADD COLUMN max_member_reservations INTEGER NOT NULL DEFAULT 30;
