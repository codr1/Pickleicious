-- internal/db/queries/facility_themes.sql
-- name: GetActiveThemeID :one
SELECT COALESCE(active_theme_id, 0)
FROM facilities
WHERE id = @facility_id;

-- name: UpsertActiveThemeID :execrows
UPDATE facilities
SET active_theme_id = @active_theme_id,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @facility_id;

-- name: CountThemeUsage :one
SELECT COUNT(*) FROM facilities
WHERE active_theme_id = @theme_id;

-- name: FacilityExists :one
SELECT COUNT(*) FROM facilities
WHERE id = @facility_id;
