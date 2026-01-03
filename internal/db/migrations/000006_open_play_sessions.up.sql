PRAGMA foreign_keys = ON;

------ OPEN PLAY SESSIONS ------
CREATE TABLE open_play_sessions (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    open_play_rule_id INTEGER NOT NULL,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    status TEXT NOT NULL DEFAULT 'scheduled',
    current_court_count INTEGER NOT NULL DEFAULT 0,
    auto_scale_override BOOLEAN,
    cancelled_at DATETIME,
    cancellation_reason TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('scheduled', 'cancelled', 'completed')),
    CHECK (current_court_count >= 0),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (open_play_rule_id) REFERENCES open_play_rules(id)
);

CREATE INDEX idx_open_play_sessions_facility_id ON open_play_sessions(facility_id);
CREATE INDEX idx_open_play_sessions_rule_id ON open_play_sessions(open_play_rule_id);
CREATE INDEX idx_open_play_sessions_start_time ON open_play_sessions(start_time);
CREATE INDEX idx_open_play_sessions_status ON open_play_sessions(status);

------ STAFF NOTIFICATIONS ------
CREATE TABLE staff_notifications (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    notification_type TEXT NOT NULL,
    message TEXT NOT NULL,
    related_session_id INTEGER,
    read BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (notification_type IN ('scale_up', 'scale_down', 'cancelled')),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (related_session_id) REFERENCES open_play_sessions(id)
);

CREATE INDEX idx_staff_notifications_facility_id ON staff_notifications(facility_id);
CREATE INDEX idx_staff_notifications_related_session_id ON staff_notifications(related_session_id);
CREATE INDEX idx_staff_notifications_read ON staff_notifications(read);

------ OPEN PLAY AUDIT LOG ------
CREATE TABLE open_play_audit_log (
    id INTEGER PRIMARY KEY,
    session_id INTEGER NOT NULL,
    action TEXT NOT NULL,
    before_state TEXT,
    after_state TEXT,
    reason TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (action IN ('scale_up', 'scale_down', 'cancelled')),
    FOREIGN KEY (session_id) REFERENCES open_play_sessions(id)
);

CREATE INDEX idx_open_play_audit_log_session_id ON open_play_audit_log(session_id);
CREATE INDEX idx_open_play_audit_log_created_at ON open_play_audit_log(created_at);
