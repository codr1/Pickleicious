PRAGMA foreign_keys = ON;

ALTER TABLE facilities
ADD COLUMN active_theme_id INTEGER REFERENCES themes(id);

UPDATE facilities
SET active_theme_id = (
    SELECT active_theme_id
    FROM facility_theme_settings
    WHERE facility_theme_settings.facility_id = facilities.id
);

DROP INDEX IF EXISTS idx_facility_theme_settings_active_theme_id;
DROP TABLE IF EXISTS facility_theme_settings;

CREATE INDEX IF NOT EXISTS idx_facilities_active_theme_id ON facilities(active_theme_id);
