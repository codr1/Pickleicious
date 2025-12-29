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

Environment variants:

- `task build:dev`
- `task build:staging`
- `task build:prod`
