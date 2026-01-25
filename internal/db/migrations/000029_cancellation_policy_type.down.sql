-- WARNING: Down migration copies only reservation_type_id IS NULL rows from
-- cancellation_policy_tiers into cancellation_policy_tiers_old; type-specific
-- tiers are dropped.
PRAGMA foreign_keys = OFF;

CREATE TABLE cancellation_policy_tiers_old (
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

INSERT INTO cancellation_policy_tiers_old (
    id,
    facility_id,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at
)
SELECT
    id,
    facility_id,
    min_hours_before,
    refund_percentage,
    created_at,
    updated_at
FROM cancellation_policy_tiers
WHERE reservation_type_id IS NULL;

DROP INDEX IF EXISTS uniq_cancellation_policy_tiers_type;
DROP INDEX IF EXISTS uniq_cancellation_policy_tiers_default;
DROP INDEX IF EXISTS idx_cancellation_policy_tiers_facility_id;
DROP TABLE cancellation_policy_tiers;

ALTER TABLE cancellation_policy_tiers_old RENAME TO cancellation_policy_tiers;

CREATE INDEX idx_cancellation_policy_tiers_facility_id ON cancellation_policy_tiers(facility_id);

PRAGMA foreign_keys = ON;
