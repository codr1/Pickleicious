PRAGMA foreign_keys = OFF;

CREATE TABLE staff_notifications_new (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    notification_type TEXT NOT NULL,
    message TEXT NOT NULL,
    related_session_id INTEGER,
    related_reservation_id INTEGER,
    related_clinic_session_id INTEGER,
    target_staff_id INTEGER,
    read BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (notification_type IN ('scale_up', 'scale_down', 'cancelled', 'lesson_cancelled', 'clinic_enrollment_below_minimum')),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (related_session_id) REFERENCES open_play_sessions(id),
    FOREIGN KEY (related_reservation_id) REFERENCES reservations(id),
    FOREIGN KEY (related_clinic_session_id) REFERENCES clinic_sessions(id),
    FOREIGN KEY (target_staff_id) REFERENCES staff(id)
);

INSERT INTO staff_notifications_new (
    id,
    facility_id,
    notification_type,
    message,
    related_session_id,
    related_reservation_id,
    target_staff_id,
    read,
    created_at,
    updated_at
)
SELECT
    id,
    facility_id,
    notification_type,
    message,
    related_session_id,
    related_reservation_id,
    target_staff_id,
    read,
    created_at,
    updated_at
FROM staff_notifications;

DROP TABLE staff_notifications;

ALTER TABLE staff_notifications_new RENAME TO staff_notifications;

CREATE INDEX idx_staff_notifications_facility_id ON staff_notifications(facility_id);
CREATE INDEX idx_staff_notifications_related_session_id ON staff_notifications(related_session_id);
CREATE INDEX idx_staff_notifications_related_reservation_id ON staff_notifications(related_reservation_id);
CREATE INDEX idx_staff_notifications_related_clinic_session_id ON staff_notifications(related_clinic_session_id);
CREATE INDEX idx_staff_notifications_read ON staff_notifications(read);
CREATE INDEX idx_staff_notifications_target_staff_id ON staff_notifications(target_staff_id);

PRAGMA foreign_keys = ON;
