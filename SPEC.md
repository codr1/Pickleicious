I don't have permission to write the file. Here is the complete updated SPEC.md content:

---

# Pickleicious Specification

Court reservation and facility management system for pickleball facilities.

## Technology Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.24 |
| Database | SQLite (with golang-migrate) |
| Templates | templ |
| Styling | Tailwind CSS |
| Build | Taskfile |
| Scheduler | gocron v2 |
| Logging | zerolog |
| SQL Generation | sqlc |

## Project Structure

```
cmd/
  server/           # Main application entry point
  tools/dbmigrate/  # Database migration tool
internal/
  api/              # HTTP handlers
    authz/          # Authorization helpers
    courts/         # Court management
    members/        # Member management
    nav/            # Navigation components
    openplay/       # Open play rules API
    themes/         # Theme management
  config/           # Configuration loading
  db/               # Database layer
    migrations/     # SQL migrations (embedded)
    generated/      # sqlc-generated code
  models/           # Domain models
  openplay/         # Open play enforcement engine
  request/          # Request utilities
  scheduler/        # Background job scheduler
  templates/        # templ components
  testutil/         # Test utilities
tests/
  smoke/            # Smoke tests
web/
  static/           # Static assets
  styles/           # Tailwind source
```

## Configuration

YAML configuration file with environment variable overrides for secrets.

| Setting | Description |
|---------|-------------|
| `app.name` | Application name |
| `app.port` | HTTP server port |
| `app.environment` | Environment (development/production) |
| `app.base_url` | Base URL for the application |
| `database.driver` | Database driver (sqlite, turso) |
| `database.filename` | SQLite database path |
| `open_play.enforcement_interval` | Cron expression for enforcement job |
| `features.enable_metrics` | Enable metrics collection |
| `features.enable_tracing` | Enable tracing |
| `features.enable_debug` | Enable debug mode |

Secrets loaded from environment: `APP_SECRET_KEY`, `DATABASE_AUTH_TOKEN`.

## Database

SQLite with embedded migrations via golang-migrate. Foreign keys enabled by default (`_fk=1` DSN parameter).

### Core Tables

- `organizations` - Multi-tenant organization container
- `facilities` - Physical locations within organizations
- `operating_hours` - Facility operating schedules
- `users` - User accounts
- `user_billing`, `user_photos` - User profile data
- `staff` - Staff assignments to facilities
- `courts` - Physical courts at facilities
- `reservation_types` - Types of reservations
- `recurrence_rules` - Recurring reservation patterns
- `reservations` - Court bookings
- `reservation_courts`, `reservation_participants` - Reservation details
- `cognito_config` - AWS Cognito configuration per facility

### Open Play Tables

- `open_play_rules` - Rules defining open play sessions
- `open_play_sessions` - Individual session instances
- `staff_notifications` - Staff notification queue
- `open_play_audit_log` - Audit trail for enforcement decisions

## HTTP API

### Middleware Chain

1. `WithLogging` - Request/response logging
2. `WithRecovery` - Panic recovery
3. `WithRequestID` - Request ID injection
4. `WithContentType` - Content-Type header setting

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check (returns 200 OK) |
| GET | `/` | Main page |
| GET | `/members` | Members page |
| GET/POST | `/api/v1/members` | List/create members |
| GET/PUT/DELETE | `/api/v1/members/{id}` | Member CRUD |
| GET | `/api/v1/members/{id}/edit` | Edit member form |
| POST | `/api/v1/members/restore` | Restore deleted member |
| GET | `/courts` | Courts page |
| GET | `/api/v1/courts/calendar` | Calendar view |
| GET | `/open-play-rules` | Open play rules page |
| GET/POST | `/api/v1/open-play-rules` | List/create rules |
| GET/PUT/DELETE | `/api/v1/open-play-rules/{id}` | Rule CRUD |
| PUT | `/api/v1/open-play-sessions/{id}/auto-scale` | Toggle auto-scale |
| GET | `/admin/themes` | Theme admin page |
| GET/POST | `/api/v1/themes` | List/create themes |
| GET/PUT/DELETE | `/api/v1/themes/{id}` | Theme CRUD |
| POST | `/api/v1/themes/{id}/clone` | Clone theme |
| PUT | `/api/v1/facilities/{id}/theme` | Set facility theme |

## Authorization

Package: `internal/api/authz`

### AuthUser Struct

```go
type AuthUser struct {
    ID             int64
    IsStaff        bool
    HomeFacilityID *int64
}
```

### Authorization Rules

| User Type | Facility Access |
|-----------|----------------|
| Unauthenticated | Denied (401) |
| Staff with HomeFacilityID | Only home facility (403 otherwise) |
| Non-staff (admin) | All facilities |

### Functions

- `ContextWithUser(ctx, user)` - Inject user into context
- `UserFromContext(ctx)` - Extract user from context
- `RequireFacilityAccess(ctx, facilityID)` - Check facility access, returns `ErrUnauthenticated` or `ErrForbidden`

## Open Play Enforcement Engine

Package: `internal/openplay`

Background scheduler evaluates open play sessions approaching their cutoff time.

### Engine Operations

1. **Session Cancellation**: Sessions with signups below `min_participants` are cancelled
   - Releases all reserved courts
   - Updates session status to `cancelled`
   - Creates staff notification
   - Creates audit log entry

2. **Court Auto-Scaling**: Sessions with auto-scale enabled adjust court count based on signups
   - Calculates desired courts: `ceil(signups / max_participants_per_court)`
   - Clamps to `min_courts` and `max_courts` limits
   - Respects court availability
   - Creates staff notification for scale up/down/capped
   - Creates audit log entry

### Session Override

Per-session `auto_scale_override` can disable auto-scaling for individual sessions.

### Notification Types

- `cancelled` - Session cancelled due to low signups
- `scale_up` - Courts added
- `scale_down` - Courts removed

### Audit Actions

- `cancelled` - Session cancellation
- `scale_up` - Court count increased
- `scale_down` - Court count decreased

## Test Infrastructure

### Test Utilities

Package: `internal/testutil`

`NewTestDB(t)` creates a temporary SQLite database with migrations applied. Automatically cleans up on test completion.

### Smoke Tests

Location: `tests/smoke/server_test.go`

Build tag: `//go:build smoke`

| Test | Description |
|------|-------------|
| `TestServerStartup` | Builds server binary, starts as subprocess, verifies `/health` returns 200 |
| `TestMigrationsApplied` | Verifies core tables exist after migrations |
| `TestForeignKeyIntegrity` | Verifies foreign key constraints are enforced |

### Integration Tests

Build tag: `//go:build integration`

Integration tests use `testutil.NewTestDB(t)` for a real database with migrations.

### Taskfile Targets

| Target | Description |
|--------|-------------|
| `task test` | Run all tests (unit, smoke, integration) |
| `task test:unit` | Run unit tests (no build tags) |
| `task test:smoke` | Run smoke tests (`-tags=smoke`) |
| `task test:integration` | Run integration tests (`-tags=integration`) |
| `task generate` | Generate templ and sqlc code |
| `task generate-sqlc` | Generate sqlc code only |

All test targets depend on `generate-sqlc` to ensure generated code exists.

### CI/CD

GitHub Actions workflow: `.github/workflows/pr-checks.yml`

Triggers on pull requests to `main` branch.

Steps:
1. Checkout
2. Setup Go (version from go.mod)
3. Install task
4. Install templ and sqlc
5. Run `task generate`
6. Run `task test:smoke`

Full CI/CD expansion tracked in GitHub Issue #29.

## Build System

Taskfile.yml provides all build and development commands.

| Target | Description |
|--------|-------------|
| `task build` | Build server binary (development) |
| `task build:prod` | Build server binary (production, stripped) |
| `task dev` | Run dev server |
| `task dev:watch` | Run dev server with Air hot reload |
| `task css` | Build Tailwind CSS |
| `task static_assets` | Copy static assets to build directory |
| `task db:migrate` | Run database migrations |
| `task db:reset` | Reset database and re-run migrations |
| `task clean` | Remove build artifacts |

## Server Lifecycle

1. Load configuration from `config.yaml`
2. Initialize database with migrations
3. Initialize API handlers
4. Initialize scheduler
5. Initialize open play enforcement engine
6. Register enforcement cron job
7. Register HTTP routes
8. Start HTTP server with timeouts (read: 15s, write: 15s, idle: 60s)