# Pickleicious
...Its so delicious.

## Development

This project uses Taskfile (go-task). Install it if you do not already have it:

- https://taskfile.dev/installation/

Common commands:

- `task generate` - Generate templ and sqlc code
- `task generate-sqlc` - Generate sqlc code only
- `task build` - Build the server binary
- `task dev` - Run the dev server (no file watching)
- `task dev:watch` - Run the dev server with Air hot reload
- `task test` - Run Go tests
- `task css` - Build Tailwind CSS
- `task db:migrate` - Run database migrations
- `task db:reset` - Delete the database and re-run migrations
- `task clean` - Remove build artifacts

Environment variants:

- `task build:prod`

## Authentication Notes

Local staff login uses a short-lived in-memory session token stored in the `pickleicious_session` cookie. The signed auth cookie (`pickleicious_auth`) remains the dev-mode/Cognito flow. This dual system is intentional while Cognito integration is still in flight; only the staff login path uses the session token.
