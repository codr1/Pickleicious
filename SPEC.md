The files don't exist in this directory - this appears to be an ops repo that manages workstreams. The diff in the context shows what was built in the actual pickleicious repo. I'll analyze the diff to produce the updated SPEC.

Based on the diff analysis, here's the complete updated SPEC.md:

```markdown
# PICKLEICIOUS - System Specification

## Overview

Pickleicious is a multi-tenant SaaS platform for pickleball facility management. It enables indoor pickleball venues to manage court reservations, member profiles, staff operations, and facility scheduling through a modern web interface optimized for front-desk operations.

## Technology Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.22+ |
| HTTP Server | Standard library `net/http` |
| Templating | Templ (type-safe HTML) |
| Database | SQLite (mattn/go-sqlite3) |
| Query Generation | SQLC |
| Migrations | golang-migrate |
| Logging | Zerolog |
| Auth | AWS Cognito SDK v2, bcrypt (local) |
| Scheduler | gocron v2 |
| Interactivity | HTMX 1.9.10 |
| Styling | Tailwind CSS 3.4 |
| Build System | Taskfile (go-task) |
| Hot Reload | Air |

## Build System

The project uses Taskfile for build orchestration.

### Development Tasks

| Task | Description | Dependencies |
|------|-------------|--------------|
| `dev` | Run development server without file watching | generate, css, db:migrate |
| `dev:watch` | Run development server with Air hot reload | generate, css, db:migrate |
| `generate` | Compile templ templates and sqlc queries | - |
| `generate-sqlc` | Generate sqlc queries only | - |
| `css` | Build Tailwind CSS | - |
| `db:migrate` | Run database migrations (creates db dir if needed) | - |
| `db:reset` | Delete database and re-run migrations | - |

### Test Tasks

| Task | Description | Dependencies |
|------|-------------|--------------|
| `test` | Run all Go tests (unit, smoke, integration) | - |
| `test:unit` | Run unit tests (exclude smoke/integration tags) | generate-sqlc |
| `test:smoke` | Run smoke tests | generate-sqlc |
| `test:integration` | Run integration tests | generate-sqlc |

### Build Tasks

| Task | Description |
|------|-------------|
| `build` | Build server binary with generate and static_assets |
| `build:prod` | Build server binary for production (stripped symbols) |
| `static_assets` | Copy static assets to build output |
| `clean` | Remove build artifacts |

### One-Command Development Startup

Running `task dev` or `task dev:watch` automatically:
1. Generates templ templates and sqlc queries
2. Builds Tailwind CSS
3. Runs database migrations (idempotent - succeeds if already applied)
4. Starts the development server

No manual pre-steps are required to start development.

## Multi-Tenancy Model

```
Organization (corporate entity)
    └── Facility (physical location)
            ├── Courts
            ├── Members
            ├── Staff
            └── Operating Hours
```

- Organizations have custom Cognito configuration and domain
- Facilities have their own theme, timezone, and operating hours
- Members and staff are scoped to facilities

## Database Schema

### Core Entities

| Table | Purpose |
|-------|---------|
| organizations | Top-level tenant entities |
| facilities | Physical locations |
| operating_hours | Per-facility schedules |
| users | Authentication records |
| members | Customer profiles |
| member_billing | Payment information |
| member_photos | Photo BLOB storage |
| staff | Employee records |
| courts | Court definitions |
| cognito_config | Per-org auth settings |

### Reservation System

| Table | Purpose |
|-------|---------|
| reservation_types | Booking type lookup (seeded: OPEN_PLAY, GAME, PRO_SESSION, EVENT, MAINTENANCE, LEAGUE) |
| recurrence_rules | Recurring patterns (WEEKLY, BIWEEKLY, MONTHLY) |
| reservations | Booking records |
| reservation_courts | Multi-court junction |
| reservation_participants | Multi-member junction |

### Open Play System

| Table | Purpose |
|-------|---------|
| open_play_rules | Configuration for open play sessions |
| open_play_sessions | Individual open play session instances |
| staff_notifications | Staff notification storage |
| audit_log | Audit trail for automated decisions |

### Key Constraints

- `organizations.slug` - UNIQUE
- `facilities.slug` - UNIQUE
- `users.email` - UNIQUE
- `courts(facility_id, court_number)` - UNIQUE
- `operating_hours(facility_id, day_of_week)` - UNIQUE
- `member_photos.member_id` - UNIQUE INDEX

## API Routes

### Navigation

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Base layout |
| GET | `/health` | Health check |
| GET | `/api/v1/nav/menu` | Load menu HTML |
| GET | `/api/v1/nav/menu/close` | Clear menu |
| GET | `/api/v1/nav/search` | Global search |

### Authentication

| Method | Path | Description |
|--------|------|-------------|
| GET | `/login` | Login page |
| POST | `/api/v1/auth/check-staff` | Check if identifier belongs to staff |
| POST | `/api/v1/auth/send-code` | Send Cognito OTP code |
| POST | `/api/v1/auth/verify-code` | Verify Cognito OTP code |
| POST | `/api/v1/auth/resend-code` | Resend OTP code |
| POST | `/api/v1/auth/staff-login` | Staff local password login |
| POST | `/api/v1/auth/reset-password` | Password reset flow |
| POST | `/api/v1/auth/standard-login` | Standard member login |

### Members

| Method | Path | Description |
|--------|------|-------------|
| GET | `/members` | Members page |
| GET | `/api/v1/members` | List members |
| POST | `/api/v1/members` | Create member |
| GET | `/api/v1/members/search` | Search members |
| GET | `/api/v1/members/new` | New member form |
| GET | `/api/v1/members/{id}` | Member detail |
| GET | `/api/v1/members/{id}/edit` | Edit form |
| PUT | `/api/v1/members/{id}` | Update member |
| DELETE | `/api/v1/members/{id}` | Soft delete member |
| GET | `/api/v1/members/{id}/billing` | Billing info |
| GET | `/api/v1/members/photo/{id}` | Member photo |
| POST | `/api/v1/members/restore` | Restore/create decision |

### Courts and Calendar

| Method | Path | Description |
|--------|------|-------------|
| GET | `/courts` | Courts page with calendar |
| GET | `/api/v1/courts/calendar` | Calendar view (HTMX partial) |
| GET | `/api/v1/courts/booking/new` | Quick booking form modal |

### Reservations

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/reservations` | List reservations by facility and date range |
| POST | `/api/v1/reservations` | Create reservation |
| GET | `/api/v1/reservations/{id}/edit` | Edit reservation form |
| PUT | `/api/v1/reservations/{id}` | Update reservation |
| DELETE | `/api/v1/reservations/{id}` | Delete reservation |
| GET | `/api/v1/events/booking/new` | Event booking form (multi-court) |

### Open Play

| Method | Path | Description |
|--------|------|-------------|
| GET | `/open-play-rules` | Open play rules page |
| GET | `/api/v1/open-play-rules` | List rules |
| POST | `/api/v1/open-play-rules` | Create rule |
| GET | `/api/v1/open-play-rules/new` | New rule form |
| GET | `/api/v1/open-play-rules/{id}` | Rule detail |
| GET | `/api/v1/open-play-rules/{id}/edit` | Edit form |
| PUT | `/api/v1/open-play-rules/{id}` | Update rule |
| DELETE | `/api/v1/open-play-rules/{id}` | Delete rule |
| PUT | `/api/v1/open-play-sessions/{id}/auto-scale` | Toggle auto-scale override |

### Themes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/themes` | List themes |
| POST | `/api/v1/themes` | Create theme |
| GET | `/api/v1/themes/{id}` | Theme detail |
| PUT | `/api/v1/themes/{id}` | Update theme |
| DELETE | `/api/v1/themes/{id}` | Delete theme |
| PUT | `/api/v1/themes/{id}/activate` | Activate theme for facility |

## Reservation System

### Reservation Types (Seeded)

| Name | Color | Purpose |
|------|-------|---------|
| OPEN_PLAY | #10B981 | Drop-in open play sessions |
| GAME | #3B82F6 | Standard court booking |
| PRO_SESSION | #8B5CF6 | Lessons with pro staff |
| EVENT | #F59E0B | Special events |
| MAINTENANCE | #6B7280 | Court maintenance blocks |
| LEAGUE | #EF4444 | League play |

Types are seeded via migration `000008_seed_reservation_types`. Default colors are provided and can be customized via future admin UI.

### SQLC Queries

| Query | Purpose |
|-------|---------|
| CreateReservation | Insert new reservation |
| GetReservation | Get reservation by ID with type info |
| ListReservationsByDateRange | List by facility_id and date range |
| UpdateReservation | Update reservation fields |
| DeleteReservation | Delete reservation by ID |
| AddCourtsToReservation | Add court to reservation_courts junction |
| RemoveCourtsFromReservation | Remove court from junction |
| ListCourtsForReservation | Get courts for a reservation |
| AddParticipant | Add member to reservation_participants |
| RemoveParticipant | Remove member from participants |
| ListParticipantsForReservation | Get participants with user details |

### Booking Workflows

**Quick Booking (`/api/v1/courts/booking/new`)**
- Single court selection (pre-filled from calendar click)
- Start/end time with 1-hour default duration
- Date pre-filled from calendar's displayed date
- Reservation type dropdown (populated from database)
- Optional primary user (member search)
- Validates: start < end, minimum 1-hour duration, no double-booking
- On success: triggers `calendar-refresh` HTMX event and closes modal

**Event Booking (`/api/v1/events/booking/new`)**
- Multi-court selection (checkboxes for all facility courts)
- Participant management (add/remove members via search)
- Extended options for events
- Same validation as quick booking
- On success: triggers `calendar-refresh` HTMX event

### Calendar Display

- Courts shown as columns, hours as rows (6 AM - 10 PM range)
- Reservations rendered as colored blocks positioned by time
- Block height calculated from duration: `(end - start) / 60 * 4rem`
- Block top position calculated from start time offset
- Block color determined by reservation_type.color
- Clicking empty cell opens quick booking form with court, hour, and date pre-filled
- Clicking reservation block opens edit form
- Date navigation with query parameter `?date=YYYY-MM-DD`
- Date picker allows jumping to any date
- Today button returns to current date

### Validation Rules

- Start time must be before end time
- Minimum duration: 1 hour
- Court must be available (no overlapping reservations)
- Facility must exist and user must have access
- Conflict errors shown inline with red border styling
- Form state preserved on validation failure

### Conflict Detection

The `CheckCourtAvailability` query checks for overlapping reservations:
- Same court
- Overlapping time range (start < existing_end AND end > existing_start)
- Excludes the current reservation when editing

## Member Management

### Member Entity

| Field | Description |
|-------|-------------|
| first_name, last_name | Name (required) |
| email | Unique email address |
| phone | 10-20 characters |
| date_of_birth | YYYY-MM-DD format |
| street_address, city, state, postal_code | Address fields |
| waiver_signed | Legal waiver acceptance |
| status | active, suspended, archived, deleted |
| home_facility_id | Primary location |
| membership_level | 0=Unverified Guest, 1=Verified Guest, 2=Member, 3+=Member+ |

### Member Photos

Photos are stored as BLOBs in the database with content_type and size metadata. The photo capture workflow uses browser MediaDevices API, captures to canvas, converts to Base64, and stores server-side.

### Soft Delete and Restoration

- Deleting a member sets status to 'deleted' (not physical delete)
- Deleted members are excluded from normal queries
- Creating a member with a deleted account's email offers restoration or new account creation

## Authentication

### Session Management

Two cookie-based session mechanisms operate in parallel:

| Cookie | Purpose | Used By |
|--------|---------|---------|
| `pickleicious_session` | In-memory token-based session | Staff local login |
| `pickleicious_auth` | HMAC-signed JSON payload | Dev mode bypass, future Cognito |

Session characteristics:
- 8-hour TTL for both session types
- HttpOnly, SameSite=Lax cookies
- Secure flag enabled in non-development environments
- In-memory session store with 15-minute cleanup interval
- Single active session per user (previous sessions cleared on new login)

### Staff Local Password Auth

Staff members with `local_auth_enabled=true` and a valid `password_hash` can authenticate via `/api/v1/auth/staff-login`:

1. Rate limiting applied (100/sec burst 10)
2. Lookup user by email or phone
3. Verify `is_staff=true` and `local_auth_enabled=true`
4. Verify bcrypt password hash
5. Create session token, set `pickleicious_session` cookie
6. Return `HX-Redirect: /` on success

Security measures:
- Generic "Invalid credentials" error regardless of failure reason
- Constant-time comparison via bcrypt
- Dummy hash verification when user not found (timing attack mitigation)

### Dev Mode Bypass

When `config.App.Environment == "development"`:
- Credentials `dev@test.local` / `devpass` bypass database lookup
- Creates authenticated session with `IsStaff=true`
- Optional `facility_id` parameter sets `HomeFacilityID`
- Warning logged: "Dev mode staff login bypass used"
- Completely disabled in non-development environments

### Passwordless Member Auth (Cognito)

1. Enter email/phone
2. Cognito sends OTP
3. Verify code
4. Create session

Note: Cognito integration handlers exist but are not yet fully implemented (marked TODO).

### Auth Middleware

The `WithAuth` middleware:
1. Attempts to load user from session token or auth cookie
2. If valid, attaches `AuthUser` to request context via `authz.ContextWithUser`
3. Proceeds to next handler regardless of auth status (endpoints enforce their own requirements)

## Authorization

### AuthUser Model

```go
type AuthUser struct {
    ID             int64
    IsStaff        bool
    HomeFacilityID *int64
}
```

### Facility Access Rules

| User Type | Access |
|-----------|--------|
| Staff with matching HomeFacilityID | Allowed |
| Staff with nil HomeFacilityID (admin) | Allowed to all facilities |
| Non-staff | Denied |
| Unauthenticated | Denied |

### Error Codes

| Code | Description |
|------|-------------|
| 401 | Unauthenticated - no valid session |
| 403 | Forbidden - authenticated but lacks permission |

### Protected Endpoints

All facility-scoped endpoints enforce authorization:
- Theme management (6 endpoints)
- Open play rules (6 endpoints)
- Court calendar and booking
- Reservations CRUD

Authorization failures are logged with facility_id and user_id.

## Open Play Enforcement Engine

### Session Lifecycle

Open play sessions are created from rules and managed by a gocron scheduler that:
- Evaluates sessions at configured intervals
- Auto-scales courts based on participant count
- Notifies staff of important events
- Logs all automated decisions to audit_log

### Auto-Scale Override

Staff can toggle auto-scaling for individual sessions via PUT `/api/v1/open-play-sessions/{id}/auto-scale`. Options:
- Override for current session only
- Disable for entire rule (affects future sessions)

## UI Framework

### Layout

- Fixed top navigation with menu toggle, search, theme toggle, notifications
- Slide-out menu with Dashboard, Courts, Members, Settings
- User section with avatar, name, email

### HTMX Patterns

| Pattern | Usage |
|---------|-------|
| hx-get | Load content fragments |
| hx-post | Form submissions |
| hx-put | Update operations |
| hx-delete | Delete operations |
| hx-trigger | Custom events, delays |
| hx-target | Where to swap content |
| hx-swap | How to swap (innerHTML, outerHTML) |
| HX-Trigger | Server-sent events (e.g., calendar-refresh) |
| HX-Redirect | Server-initiated redirects (used after login) |

### Calendar-Specific HTMX

The calendar uses custom HTMX events for coordination:
- `calendar-refresh`: Triggered after successful booking operations to reload calendar data
- Modal forms target `#modal-container` for overlays
- Forms preserve state on validation errors via template re-rendering

## Shared Utilities

### apiutil Package

Common handler utilities in `internal/api/apiutil`:
- `DecodeJSON` - Strict JSON decoding with unknown field rejection
- `WriteJSON` - JSON response writing
- `RequireFacilityAccess` - Authorization check with logging
- `FieldError` - Field-level validation error
- `HandlerError` - HTTP error with status code

### request Package

Request parsing utilities in `internal/request`:
- `ParseFacilityID` - Parse facility_id from string
- `FacilityIDFromBookingRequest` - Extract facility_id from query or form

### testutil Package

Test helpers in `internal/testutil`:
- `NewTestDB` - Create test database with migrations applied

## Configuration

### config.yaml

```yaml
app:
  name: "Pickleicious"
  environment: "development"
  port: 8080
  base_url: "http://localhost:8080"
  secret_key: "your-secret-key-here"  # Required

database:
  driver: "sqlite"
  filename: "build/db/pickleicious.db"

features:
  enable_metrics: false
  enable_tracing: false
  enable_debug: true

open_play:
  enforcement_interval: "5m"
```

### Environment Variables

| Variable | Purpose |
|----------|---------|
| APP_SECRET_KEY | Application secret (required for auth cookie signing) |
| DATABASE_AUTH_TOKEN | Turso auth token |
| STATIC_DIR | Override static file path |

### Validation

Config validation enforces:
- `app.port` required
- `app.secret_key` required
- `database.driver` required

## Middleware Stack

1. WithLogging - Request/response logging
2. WithRecovery - Panic recovery
3. WithRequestID - UUID per request
4. WithAuth - Load auth session into context
5. WithContentType - Default Accept header

## Testing Infrastructure

### Test Database Helper

`internal/testutil.NewTestDB(t)` creates temporary SQLite databases with migrations applied for testing.

### Test Categories

| Category | Build Tag | Description |
|----------|-----------|-------------|
| Unit | (none) | Fast isolated tests |
| Smoke | `smoke` | Server startup and basic endpoint tests |
| Integration | `integration` | Full database and handler tests |

### GitHub Actions

PR checks workflow (`.github/workflows/pr-checks.yml`):
1. Checkout code
2. Setup Go from `go.mod`
3. Install task, templ, sqlc
4. Run `task generate`
5. Run `task test:smoke`

## Project Structure

```
pickleicious/
├── cmd/
│   ├── server/              # Main application
│   └── tools/dbmigrate/     # Migration tool
├── internal/
│   ├── api/                 # HTTP handlers
│   │   ├── apiutil/         # Shared handler utilities
│   │   ├── auth/            # Authentication (handlers, password, session)
│   │   ├── authz/           # Authorization helpers
│   │   ├── courts/          # Court/calendar
│   │   ├── members/         # Member CRUD
│   │   ├── nav/             # Navigation
│   │   ├── openplay/        # Open play rules
│   │   ├── reservations/    # Reservation CRUD
│   │   └── themes/          # Theme management
│   ├── config/              # Configuration loading
│   ├── db/
│   │   ├── migrations/      # SQL migration files
│   │   ├── queries/         # SQLC query files
│   │   ├── schema/          # Master schema
│   │   └── generated/       # SQLC output
│   ├── models/              # Domain models
│   ├── request/             # Request parsing utilities
│   ├── templates/           # Templ components
│   │   └── components/
│   │       ├── courts/      # Calendar components
│   │       └── reservations/ # Booking form components
│   └── testutil/            # Test helpers
├── tests/
│   └── smoke/               # Smoke tests
├── web/
│   ├── static/              # Static assets
│   └── styles/              # Tailwind source CSS
├── assets/
│   └── themes/              # Theme color definitions
├── .github/
│   └── workflows/           # CI/CD workflows
├── Taskfile.yml             # Build system
├── .air.toml                # Hot reload config
└── config.yaml              # App configuration
```

## CI/CD

### PR Checks Workflow

`.github/workflows/pr-checks.yml` runs on pull requests to main:
1. Set up Go with version from go.mod
2. Install task, templ, sqlc
3. Run `task generate`
4. Run `task test:smoke`
```