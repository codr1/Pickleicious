PRAGMA foreign_keys = OFF;

DROP INDEX IF EXISTS idx_facilities_active_theme_id;

CREATE TABLE facilities_new (
    id INTEGER PRIMARY KEY,
    organization_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    timezone TEXT NOT NULL,
    active_theme_id INTEGER,
    max_advance_booking_days INTEGER NOT NULL DEFAULT 7,
    max_member_reservations INTEGER NOT NULL DEFAULT 30,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (active_theme_id) REFERENCES themes(id)
);

INSERT INTO facilities_new (
    id,
    organization_id,
    name,
    slug,
    timezone,
    active_theme_id,
    max_advance_booking_days,
    max_member_reservations,
    created_at,
    updated_at
)
SELECT
    id,
    organization_id,
    name,
    slug,
    timezone,
    active_theme_id,
    max_advance_booking_days,
    max_member_reservations,
    created_at,
    updated_at
FROM facilities;

DROP TABLE facilities;
ALTER TABLE facilities_new RENAME TO facilities;

CREATE INDEX idx_facilities_active_theme_id ON facilities(active_theme_id);

PRAGMA foreign_keys = ON;
