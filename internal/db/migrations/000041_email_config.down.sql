PRAGMA foreign_keys = OFF;

ALTER TABLE facilities DROP COLUMN reminder_hours_before;
ALTER TABLE facilities DROP COLUMN email_from_address;
ALTER TABLE organizations DROP COLUMN email_from_address;

PRAGMA foreign_keys = ON;
