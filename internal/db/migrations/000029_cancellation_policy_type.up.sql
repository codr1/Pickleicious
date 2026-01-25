PRAGMA foreign_keys = OFF;

CREATE TABLE cancellation_policy_tiers_new (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    reservation_type_id INTEGER,
    min_hours_before INTEGER NOT NULL,
    refund_percentage INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (min_hours_before >= 0),
    CHECK (refund_percentage >= 0 AND refund_percentage <= 100),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (reservation_type_id) REFERENCES reservation_types(id)
);

INSERT INTO cancellation_policy_tiers_new (
    id,
    facility_id,
    reservation_type_id,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at
)
SELECT
    id,
    facility_id,
    NULL,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at
FROM cancellation_policy_tiers;

DROP INDEX IF EXISTS idx_cancellation_policy_tiers_facility_id;
DROP TABLE cancellation_policy_tiers;

ALTER TABLE cancellation_policy_tiers_new RENAME TO cancellation_policy_tiers;

CREATE INDEX idx_cancellation_policy_tiers_facility_id ON cancellation_policy_tiers(facility_id);
CREATE UNIQUE INDEX uniq_cancellation_policy_tiers_default
    ON cancellation_policy_tiers(facility_id, min_hours_before)
    WHERE reservation_type_id IS NULL;
CREATE UNIQUE INDEX uniq_cancellation_policy_tiers_type
    ON cancellation_policy_tiers(facility_id, reservation_type_id, min_hours_before)
    WHERE reservation_type_id IS NOT NULL;

PRAGMA foreign_keys = ON;
