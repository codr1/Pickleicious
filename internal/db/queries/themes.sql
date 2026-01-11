-- internal/db/queries/themes.sql

-- name: GetTheme :one
SELECT * FROM themes
WHERE id = @id;

-- name: CreateTheme :one
INSERT INTO themes (
    facility_id,
    name,
    is_system,
    primary_color,
    secondary_color,
    tertiary_color,
    accent_color,
    highlight_color
) VALUES (
    @facility_id,
    @name,
    @is_system,
    @primary_color,
    @secondary_color,
    @tertiary_color,
    @accent_color,
    @highlight_color
)
RETURNING id, facility_id, name, is_system, primary_color, secondary_color,
    tertiary_color, accent_color, highlight_color, created_at, updated_at;

-- name: UpdateTheme :one
UPDATE themes
SET name = @name,
    primary_color = @primary_color,
    secondary_color = @secondary_color,
    tertiary_color = @tertiary_color,
    accent_color = @accent_color,
    highlight_color = @highlight_color,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING id, facility_id, name, is_system, primary_color, secondary_color,
    tertiary_color, accent_color, highlight_color, created_at, updated_at;

-- name: DeleteTheme :execrows
DELETE FROM themes
WHERE id = @id;

-- name: CountFacilityThemes :one
SELECT COUNT(*) FROM themes
WHERE facility_id = @facility_id;

-- name: CountFacilityThemeName :one
SELECT COUNT(*) FROM themes
WHERE facility_id = @facility_id
  AND name = @name;

-- name: CountFacilityThemeNameExcludingID :one
SELECT COUNT(*) FROM themes
WHERE facility_id = @facility_id
  AND name = @name
  AND id != @id;

-- name: ListSystemThemes :many
SELECT * FROM themes
WHERE facility_id IS NULL
ORDER BY name;

-- name: ListFacilityThemes :many
SELECT * FROM themes
WHERE facility_id = @facility_id
ORDER BY name;
