CREATE TABLE IF NOT EXISTS waitlist_config (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    max_waitlist_size INTEGER NOT NULL DEFAULT 0,
    notification_mode TEXT NOT NULL CHECK (notification_mode IN ('broadcast', 'sequential')),
    offer_expiry_minutes INTEGER NOT NULL DEFAULT 0,
    notification_window_minutes INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    UNIQUE (facility_id)
);

CREATE TABLE IF NOT EXISTS waitlists (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    target_court_id INTEGER,
    target_date DATE NOT NULL,
    target_start_time TIME NOT NULL,
    target_end_time TIME NOT NULL,
    position INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'notified', 'expired', 'fulfilled')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (target_start_time < target_end_time),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (target_court_id) REFERENCES courts(id)
);

CREATE TABLE IF NOT EXISTS waitlist_offers (
    id INTEGER PRIMARY KEY,
    waitlist_id INTEGER NOT NULL,
    offered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'accepted', 'expired')),
    FOREIGN KEY (waitlist_id) REFERENCES waitlists(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_waitlists_facility_id ON waitlists(facility_id);
CREATE INDEX IF NOT EXISTS idx_waitlists_slot ON waitlists(facility_id, target_date, target_start_time, target_end_time);
CREATE UNIQUE INDEX IF NOT EXISTS idx_waitlists_slot_position_unique
    ON waitlists(facility_id, target_date, target_start_time, target_end_time, COALESCE(target_court_id, -1), position);
CREATE INDEX IF NOT EXISTS idx_waitlists_user_id ON waitlists(user_id);
CREATE INDEX IF NOT EXISTS idx_waitlists_target_date ON waitlists(target_date);
CREATE INDEX IF NOT EXISTS idx_waitlists_status ON waitlists(status);
CREATE INDEX IF NOT EXISTS idx_waitlist_offers_waitlist_id ON waitlist_offers(waitlist_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_waitlists_unique_active_user_slot
    ON waitlists(facility_id, target_date, target_start_time, target_end_time, COALESCE(target_court_id, -1), user_id)
    WHERE status IN ('pending', 'notified');
CREATE INDEX IF NOT EXISTS idx_waitlist_offers_status ON waitlist_offers(status);
CREATE INDEX IF NOT EXISTS idx_waitlist_offers_expires_at ON waitlist_offers(expires_at);
