PRAGMA foreign_keys = OFF;

CREATE TABLE reservations_new (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    reservation_type_id INTEGER NOT NULL,
    recurrence_rule_id INTEGER,
    primary_user_id INTEGER,
    pro_id INTEGER,
    open_play_rule_id INTEGER,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    is_open_event BOOLEAN NOT NULL DEFAULT 0,
    teams_per_court INTEGER,
    people_per_team INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (start_time < end_time),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (reservation_type_id) REFERENCES reservation_types(id),
    FOREIGN KEY (recurrence_rule_id) REFERENCES recurrence_rules(id),
    FOREIGN KEY (primary_user_id) REFERENCES users(id),
    FOREIGN KEY (pro_id) REFERENCES staff(id),
    FOREIGN KEY (open_play_rule_id) REFERENCES open_play_rules(id)
);

INSERT INTO reservations_new (
    id,
    facility_id,
    reservation_type_id,
    recurrence_rule_id,
    primary_user_id,
    pro_id,
    open_play_rule_id,
    start_time,
    end_time,
    is_open_event,
    teams_per_court,
    people_per_team,
    created_at,
    updated_at
)
SELECT
    id,
    facility_id,
    reservation_type_id,
    recurrence_rule_id,
    primary_user_id,
    pro_id,
    open_play_rule_id,
    start_time,
    end_time,
    is_open_event,
    teams_per_court,
    people_per_team,
    created_at,
    updated_at
FROM reservations;

DROP TABLE reservations;
ALTER TABLE reservations_new RENAME TO reservations;

PRAGMA foreign_keys = ON;
