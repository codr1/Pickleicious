PRAGMA foreign_keys = ON;

ALTER TABLE organizations
ADD COLUMN reminder_hours_before INTEGER NOT NULL DEFAULT 24 CHECK (reminder_hours_before > 0);
