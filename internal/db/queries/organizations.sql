-- internal/db/queries/organizations.sql

-- name: GetOrganizationBySlug :one
SELECT id, name, slug, status, created_at, updated_at
FROM organizations
WHERE slug = @slug AND status = 'active';

-- name: GetOrganizationByID :one
SELECT id, name, slug, status, created_at, updated_at
FROM organizations
WHERE id = @id;

-- name: ListOrganizations :many
SELECT id, name, slug, status, created_at, updated_at
FROM organizations
WHERE status = 'active'
ORDER BY name;
