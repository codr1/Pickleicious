PRAGMA foreign_keys = OFF;

DROP INDEX IF EXISTS idx_themes_facility_name;
DROP INDEX IF EXISTS idx_themes_system_name;
DROP TABLE IF EXISTS themes;

PRAGMA foreign_keys = ON;
