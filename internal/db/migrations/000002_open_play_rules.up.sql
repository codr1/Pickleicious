PRAGMA foreign_keys = ON;

------ OPEN PLAY RULES ------
CREATE TABLE open_play_rules (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    min_participants INTEGER NOT NULL DEFAULT 4,
    max_participants_per_court INTEGER NOT NULL DEFAULT 8,
    cancellation_cutoff_minutes INTEGER NOT NULL DEFAULT 60,
    auto_scale_enabled BOOLEAN NOT NULL DEFAULT 1,
    min_courts INTEGER NOT NULL DEFAULT 1,
    max_courts INTEGER NOT NULL DEFAULT 4,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (facility_id) REFERENCES facilities(id)
);

ALTER TABLE reservations ADD COLUMN open_play_rule_id INTEGER;

INSERT INTO reservation_types (name) VALUES ('OPEN_PLAY');
