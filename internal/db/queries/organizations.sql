-- internal/db/queries/organizations.sql

-- name: GetOrganizationBySlug :one
SELECT id, name, slug, email_from_address, status, created_at, updated_at
FROM organizations
WHERE slug = @slug AND status = 'active';

-- name: GetOrganizationByID :one
SELECT id, name, slug, email_from_address, status, created_at, updated_at
FROM organizations
WHERE id = @id;

-- name: ListOrganizations :many
SELECT id, name, slug, email_from_address, status, created_at, updated_at
FROM organizations
WHERE status = 'active'
ORDER BY name;

-- name: GetOrganizationCrossFacilitySetting :one
SELECT cross_facility_visit_packs
FROM organizations
WHERE id = @id;

-- name: GetOrganizationEmailConfig :one
SELECT id, email_from_address
FROM organizations
WHERE id = @id;

-- name: UpdateOrganizationEmailConfig :one
UPDATE organizations
SET email_from_address = @email_from_address,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING id, email_from_address;
