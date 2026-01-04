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
| reservation_types | Booking type lookup (GAME, PRO_SESSION, EVENT, MAINTENANCE, LEAGUE, OPEN_PLAY) |
| recurrence_rules | Recurring patterns (WEEKLY, BIWEEKLY, MONTHLY) |
| reservations | Booking records |
| reservation_courts | Multi-court junction |
| reservation_participants | Multi-member junction |

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
| POST | `/api/v1/auth/reset-password` | Initiate password reset (Cognito ForgotPassword) |
| POST | `/api/v1/auth/confirm-reset-password` | Complete password reset with code |
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
| GET | `/api/v1/courts/calendar` | Calendar view fragment |
| GET | `/api/v1/courts/booking/new` | Quick booking modal form |

### Reservations

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/reservations` | List reservations by date range |
| POST | `/api/v1/reservations` | Create reservation |
| GET | `/api/v1/reservations/{id}/edit` | Edit form |
| PUT | `/api/v1/reservations/{id}` | Update reservation |
| DELETE | `/api/v1/reservations/{id}` | Delete reservation |
| GET | `/api/v1/events/booking/new` | Event booking form (multi-court) |

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

## Reservation System

### Reservation Types

Seeded via migration with default colors:

| Type | Color | Purpose |
|------|-------|---------|
| OPEN_PLAY | #2E7D32 (green) | Open play session |
| GAME | #1976D2 (blue) | Standard game reservation |
| PRO_SESSION | #6A1B9A (purple) | Pro-led session |
| EVENT | #F57C00 (orange) | Special event booking |
| MAINTENANCE | #546E7A (gray) | Maintenance block |
| LEAGUE | #C62828 (red) | League play |

### Reservation SQLC Queries

| Query | Description |
|-------|-------------|
| CreateReservation | Insert new reservation |
| GetReservation | Fetch by ID with facility check |
| GetReservationByID | Fetch by ID only |
| ListReservationsByDateRange | List reservations overlapping time range |
| ListReservationCourtsByDateRange | List court assignments for reservations in range |
| UpdateReservation | Update reservation details |
| DeleteReservation | Remove reservation |
| AddReservationCourt | Assign court to reservation |
| RemoveReservationCourt | Remove court from reservation |
| ListCourtsForReservation | List courts assigned to reservation |
| AddParticipant | Add user to reservation |
| RemoveParticipant | Remove user from reservation |
| ListParticipantsForReservation | List users in reservation |
| GetReservationType | Fetch type by ID |
| ListReservationTypes | List all types |

### Booking Workflows

**Quick Booking** (`/api/v1/courts/booking/new`):
- Modal form triggered by clicking calendar cell
- Pre-fills court and time from click location
- Single court selection
- Minimum 1-hour duration enforced

**Event Booking** (`/api/v1/events/booking/new`):
- Full-page form for complex bookings
- Multi-court selection
- Participant management
- Extended options (open event, teams per court, people per team)

### Calendar Display

The courts calendar (`/api/v1/courts/calendar`) displays:
- Hourly grid rows (configurable start/end hours)
- Court columns
- Reservation blocks positioned by start_time/end_time
- Color-coded by reservation type
- Click empty cell opens quick booking form
- Click existing reservation opens edit form

### Validation Rules

| Rule | Description |
|------|-------------|
| start_time < end_time | Start must precede end |
| Minimum duration | 1 hour default |
| Facility exists | facility_id must be valid |
| No double-booking | Courts cannot overlap in time |
| Facility authorization | Staff can only book for their home facility (admin unrestricted) |

### Conflict Detection

Before creating/updating reservations:
1. Query existing reservations overlapping the time range
2. Check if any requested courts are already assigned
3. Return 409 Conflict with field-specific error if overlap found

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

1. User enters email/phone and organization_id
2. `/api/v1/auth/send-code` calls Cognito InitiateAuth with CUSTOM_AUTH flow
3. Cognito sends OTP via user's preferred_auth_method (EMAIL or SMS)
4. `/api/v1/auth/verify-code` calls Cognito RespondToAuthChallenge with session + code
5. On success, cognito_status updated to 'CONFIRMED' in users table
6. Stateless JWT created and returned as HTTP-only Secure cookie

OTP codes expire after 5 minutes (configured in Cognito User Pool).

### Password Reset Flow

1. User requests reset via `/api/v1/auth/reset-password`
2. Cognito ForgotPassword API sends reset code via preferred_auth_method
3. User submits code + new password to `/api/v1/auth/confirm-reset-password`
4. Cognito ConfirmForgotPassword validates and updates password
5. User redirected to login page on success

### Cognito Client (`internal/cognito`)

| Function | Purpose |
|----------|---------|
| NewClient | Initialize Cognito client from per-org config |
| InitiateCustomAuth | Start CUSTOM_AUTH flow, send OTP |
| RespondToAuthChallenge | Verify OTP code, return auth result |
| ForgotPassword | Initiate password reset flow |
| ConfirmForgotPassword | Complete reset with code + new password |

### Cognito Error Handling

| Error | HTTP Status | User Message |
|-------|-------------|--------------|
| TooManyRequestsException | 429 | "Too many requests. Please try again in a few minutes." |
| NotAuthorizedException | 401 | "Invalid credentials" or "Invalid verification code" |
| CodeMismatchException | 401 | "Invalid verification code" |
| ExpiredCodeException | 403 | "Verification code expired" |
| InvalidPasswordException | 400 | "Password does not meet requirements" |
| UserNotFoundException | 401 | Silent (no user enumeration) |

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

## Shared Utilities

### apiutil Package (`internal/api/apiutil`)

| Function | Purpose |
|----------|---------|
| DecodeJSON | Decode JSON request body with unknown field rejection |
| WriteJSON | Encode and write JSON response |
| RequireFacilityAccess | Check facility authorization, write error response if denied |
| FieldError | Structured field validation error |
| HandlerError | HTTP status + message wrapper for transaction rollback |

### request Package (`internal/request`)

| Function | Purpose |
|----------|---------|
| ParseFacilityID | Parse facility_id from string, return (id, ok) |
| FacilityIDFromBookingRequest | Extract facility_id from query or form |

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
| HX-Trigger | Server-sent events (e.g., `refreshCourtsCalendar`) |
| HX-Redirect | Server-initiated redirects (used after login) |

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
│   │   ├── openplay/        # Open play sessions
│   │   ├── reservations/    # Reservation CRUD
│   │   └── themes/          # Facility themes
│   ├── cognito/             # AWS Cognito client wrapper
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
│   │       └── reservations/ # Booking form templates
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
