CREATE TABLE IF NOT EXISTS clinic_sessions (
    id INTEGER PRIMARY KEY,
    clinic_type_id INTEGER NOT NULL,
    facility_id INTEGER NOT NULL,
    pro_id INTEGER NOT NULL,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    enrollment_status TEXT NOT NULL DEFAULT 'open',
    created_by_user_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (start_time < end_time),
    CHECK (enrollment_status IN ('open', 'closed')),
    FOREIGN KEY (clinic_type_id) REFERENCES clinic_types(id) ON DELETE RESTRICT,
    FOREIGN KEY (facility_id) REFERENCES facilities(id) ON DELETE RESTRICT,
    FOREIGN KEY (pro_id) REFERENCES staff(id) ON DELETE RESTRICT,
    FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_clinic_sessions_facility_id ON clinic_sessions(facility_id);
CREATE INDEX IF NOT EXISTS idx_clinic_sessions_clinic_type_id ON clinic_sessions(clinic_type_id);
CREATE INDEX IF NOT EXISTS idx_clinic_sessions_pro_id ON clinic_sessions(pro_id);
CREATE INDEX IF NOT EXISTS idx_clinic_sessions_start_time ON clinic_sessions(start_time);

CREATE TABLE IF NOT EXISTS clinic_enrollments (
    id INTEGER PRIMARY KEY,
    clinic_session_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'enrolled',
    position INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('enrolled', 'waitlisted', 'cancelled')),
    UNIQUE (clinic_session_id, user_id),
    FOREIGN KEY (clinic_session_id) REFERENCES clinic_sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_clinic_enrollments_session_id ON clinic_enrollments(clinic_session_id);
CREATE INDEX IF NOT EXISTS idx_clinic_enrollments_user_id ON clinic_enrollments(user_id);
CREATE INDEX IF NOT EXISTS idx_clinic_enrollments_status ON clinic_enrollments(status);
