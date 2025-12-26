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
    CHECK (min_participants > 0),
    CHECK (max_participants_per_court > 0),
    CHECK (min_courts > 0),
    CHECK (max_courts > 0),
    CHECK (min_courts <= max_courts),
    CHECK (min_participants <= max_participants_per_court * min_courts),
    FOREIGN KEY (facility_id) REFERENCES facilities(id)
);

CREATE INDEX idx_open_play_rules_facility_id ON open_play_rules(facility_id);

-- No ON DELETE action: deletion is intentionally blocked when reservations exist.
ALTER TABLE reservations ADD COLUMN open_play_rule_id INTEGER REFERENCES open_play_rules(id);

INSERT INTO reservation_types (name) VALUES ('OPEN_PLAY');
