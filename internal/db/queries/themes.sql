-- internal/db/queries/themes.sql
-- name: ListSystemThemes :many
SELECT * FROM themes
WHERE facility_id IS NULL
ORDER BY name;

-- name: ListFacilityThemes :many
SELECT * FROM themes
WHERE facility_id = @facility_id
ORDER BY name;
