PRAGMA foreign_keys = OFF;

CREATE TABLE reservations_new (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    reservation_type_id INTEGER NOT NULL,
    recurrence_rule_id INTEGER,
    primary_user_id INTEGER,
    created_by_user_id INTEGER NOT NULL,
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
    FOREIGN KEY (created_by_user_id) REFERENCES users(id),
    FOREIGN KEY (pro_id) REFERENCES staff(id),
    FOREIGN KEY (open_play_rule_id) REFERENCES open_play_rules(id)
);

-- Ensure legacy reservations without a primary_user_id can be attributed safely.
INSERT INTO users (
    email,
    first_name,
    last_name,
    is_staff
)
SELECT
    'system+reservations@pickleball.local',
    'System',
    'Reservations',
    1
WHERE NOT EXISTS (
        SELECT 1
        FROM users u
        WHERE u.email = 'system+reservations@pickleball.local'
    )
  AND EXISTS (
        SELECT 1
        FROM reservations r
        WHERE r.primary_user_id IS NULL
    );

INSERT INTO reservations_new (
    id,
    facility_id,
    reservation_type_id,
    recurrence_rule_id,
    primary_user_id,
    created_by_user_id,
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
    r.id,
    r.facility_id,
    r.reservation_type_id,
    r.recurrence_rule_id,
    r.primary_user_id,
    CASE
        WHEN r.primary_user_id IS NOT NULL THEN r.primary_user_id
        ELSE (
            SELECT u.id
            FROM users u
            WHERE u.email = 'system+reservations@pickleball.local'
            LIMIT 1
        )
    END AS created_by_user_id,
    r.pro_id,
    r.open_play_rule_id,
    r.start_time,
    r.end_time,
    r.is_open_event,
    r.teams_per_court,
    r.people_per_team,
    r.created_at,
    r.updated_at
FROM reservations r;

DROP TABLE reservations;
ALTER TABLE reservations_new RENAME TO reservations;

CREATE INDEX idx_reservations_created_by_user_id
    ON reservations(created_by_user_id);

PRAGMA foreign_keys = ON;
