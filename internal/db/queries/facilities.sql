-- internal/db/queries/facilities.sql

-- name: ListFacilities :many
SELECT *
FROM facilities
ORDER BY name;
