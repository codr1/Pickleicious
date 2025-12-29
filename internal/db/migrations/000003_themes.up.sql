PRAGMA foreign_keys = ON;

------ THEMES ------
CREATE TABLE themes (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER,
    name TEXT NOT NULL,
    is_system BOOLEAN NOT NULL DEFAULT 0,
    primary_color TEXT NOT NULL,
    secondary_color TEXT NOT NULL,
    tertiary_color TEXT NOT NULL,
    accent_color TEXT NOT NULL,
    highlight_color TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (
        (facility_id IS NULL AND is_system = 1)
        OR (facility_id IS NOT NULL AND is_system = 0)
    ),
    FOREIGN KEY (facility_id) REFERENCES facilities(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_themes_system_name ON themes(name) WHERE facility_id IS NULL;
CREATE UNIQUE INDEX idx_themes_facility_name ON themes(facility_id, name) WHERE facility_id IS NOT NULL;
