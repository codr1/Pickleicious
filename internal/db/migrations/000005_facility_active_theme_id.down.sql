PRAGMA foreign_keys = OFF;

DROP INDEX IF EXISTS idx_facilities_active_theme_id;

CREATE TABLE facility_theme_settings (
    facility_id INTEGER PRIMARY KEY,
    active_theme_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (facility_id) REFERENCES facilities(id) ON DELETE CASCADE,
    FOREIGN KEY (active_theme_id) REFERENCES themes(id)
);

CREATE INDEX idx_facility_theme_settings_active_theme_id ON facility_theme_settings(active_theme_id);

INSERT INTO facility_theme_settings (facility_id, active_theme_id, created_at, updated_at)
SELECT
    id,
    active_theme_id,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM facilities
WHERE active_theme_id IS NOT NULL;

CREATE TABLE facilities_new (
    id INTEGER PRIMARY KEY,
    organization_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    timezone TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (organization_id) REFERENCES organizations(id)
);

INSERT INTO facilities_new (
    id,
    organization_id,
    name,
    slug,
    timezone,
    created_at,
    updated_at
)
SELECT
    id,
    organization_id,
    name,
    slug,
    timezone,
    created_at,
    updated_at
FROM facilities;

DROP TABLE facilities;
ALTER TABLE facilities_new RENAME TO facilities;

PRAGMA foreign_keys = ON;
