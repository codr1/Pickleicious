ALTER TABLE organizations
ADD COLUMN email_from_address TEXT;

ALTER TABLE facilities
ADD COLUMN email_from_address TEXT;

ALTER TABLE facilities
ADD COLUMN reminder_hours_before INTEGER NOT NULL DEFAULT 24 CHECK (reminder_hours_before > 0);
