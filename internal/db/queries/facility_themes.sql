-- internal/db/queries/facility_themes.sql
-- name: GetActiveThemeID :one
SELECT active_theme_id FROM facility_theme_settings
WHERE facility_id = @facility_id;

-- name: UpsertActiveThemeID :exec
INSERT INTO facility_theme_settings (
    facility_id,
    active_theme_id
) VALUES (
    @facility_id,
    @active_theme_id
)
ON CONFLICT(facility_id) DO UPDATE SET
    active_theme_id = excluded.active_theme_id,
    updated_at = CURRENT_TIMESTAMP;

-- name: CountThemeUsage :one
SELECT COUNT(*) FROM facility_theme_settings
WHERE active_theme_id = @theme_id;
