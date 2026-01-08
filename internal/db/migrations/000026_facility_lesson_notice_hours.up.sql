PRAGMA foreign_keys = ON;

------ FACILITY LESSON NOTICE CONFIG ------
ALTER TABLE facilities
    ADD COLUMN lesson_min_notice_hours INTEGER NOT NULL DEFAULT 24;
