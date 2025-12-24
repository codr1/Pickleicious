-- internal/db/queries/cognito.sql

-- name: GetCognitoConfig :one
SELECT * FROM cognito_config WHERE organization_id = @organization_id LIMIT 1;
