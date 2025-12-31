PRAGMA foreign_keys = ON;

------ FACILITY THEME SETTINGS ------
CREATE TABLE facility_theme_settings (
    facility_id INTEGER PRIMARY KEY,
    active_theme_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (facility_id) REFERENCES facilities(id) ON DELETE CASCADE,
    FOREIGN KEY (active_theme_id) REFERENCES themes(id)
);

CREATE INDEX idx_facility_theme_settings_active_theme_id ON facility_theme_settings(active_theme_id);
