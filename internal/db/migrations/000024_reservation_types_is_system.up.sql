PRAGMA foreign_keys = ON;

ALTER TABLE reservation_types
    ADD COLUMN is_system BOOLEAN NOT NULL DEFAULT 0;
