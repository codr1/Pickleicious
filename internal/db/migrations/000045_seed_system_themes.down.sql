PRAGMA foreign_keys = ON;

UPDATE facilities
SET active_theme_id = NULL
WHERE active_theme_id IN (
  SELECT id
  FROM themes
  WHERE is_system = 1
);

DELETE FROM themes
WHERE is_system = 1;
