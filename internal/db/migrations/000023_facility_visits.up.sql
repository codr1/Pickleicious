PRAGMA foreign_keys = ON;

------ FACILITY VISITS ------
CREATE TABLE facility_visits (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    facility_id INTEGER NOT NULL,
    check_in_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    check_out_time DATETIME,
    checked_in_by_staff_id INTEGER,
    activity_type TEXT CHECK (activity_type IS NULL OR activity_type IN ('court_reservation', 'open_play', 'league')),
    related_reservation_id INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (checked_in_by_staff_id) REFERENCES users(id),
    FOREIGN KEY (related_reservation_id) REFERENCES reservations(id)
);

CREATE INDEX idx_facility_visits_facility_id ON facility_visits(facility_id);
CREATE INDEX idx_facility_visits_user_id ON facility_visits(user_id);
CREATE INDEX idx_facility_visits_check_in_time ON facility_visits(check_in_time);
