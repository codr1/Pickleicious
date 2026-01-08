PRAGMA foreign_keys = ON;

CREATE TABLE cancellation_policy_tiers (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    min_hours_before INTEGER NOT NULL,
    refund_percentage INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (min_hours_before >= 0),
    CHECK (refund_percentage >= 0 AND refund_percentage <= 100),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    UNIQUE (facility_id, min_hours_before)
);

CREATE INDEX idx_cancellation_policy_tiers_facility_id ON cancellation_policy_tiers(facility_id);
