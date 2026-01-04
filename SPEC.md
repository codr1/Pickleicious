# Pickleicious Specification

Pickleicious is a management system for pickleball facilities. It handles the daily operations of running a club: tracking who walks through the door, managing court reservations, running open play sessions, and giving each facility its own branded experience.

It operates as a multi-tenant SaaS platform, enabling indoor pickleball venues to manage court reservations, member profiles, staff operations, and facility scheduling through a modern web interface optimized for front-desk operations.

---

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

---

## System Architecture

The application follows a layered architecture with clear separation between HTTP handling, business logic, and data access.

```
                                    +------------------+
                                    |   HTTP Client    |
                                    |  (Browser/HTMX)  |
                                    +--------+---------+
                                             |
                                    +--------v---------+
                                    |   Middleware     |
                                    | Logging/Recovery |
                                    | RequestID/CORS   |
                                    +--------+---------+
                                             |
         +-----------------------------------+-----------------------------------+
         |                    |                    |                    |        |
+--------v-------+  +--------v-------+  +--------v-------+  +--------v-------+  |
|    /members    |  |    /courts     |  |    /themes     |  | /open-play-... |  |
|    Handlers    |  |    Handlers    |  |    Handlers    |  |    Handlers    |  |
+--------+-------+  +--------+-------+  +--------+-------+  +--------+-------+  |
         |                    |                    |                    |        |
         +-----------------------------------+-----------------------------------+
                                             |
                                    +--------v---------+
                                    |     Models       |
                                    |  (Business Logic)|
                                    +--------+---------+
                                             |
                                    +--------v---------+
                                    |   sqlc Queries   |
                                    | (Type-safe SQL)  |
                                    +--------+---------+
                                             |
                                    +--------v---------+
                                    |     SQLite       |
                                    +------------------+
```

### Technology Choices

| Layer | Technology | Why |
|-------|------------|-----|
| HTTP Server | `net/http` | Standard library, no framework lock-in, sufficient for our needs |
| Templates | `templ` | Type-safe HTML at compile time, catches errors before runtime |
| SQL | `sqlc` | Generates Go from SQL, catches schema drift at compile time |
| Database | SQLite | Embedded, zero ops, perfect for single-facility deployments |
| Frontend | HTMX | Server-rendered with interactivity, no JavaScript build pipeline |
| CSS | Tailwind | Utility-first, works with CSS variables for theming |
| Build | Taskfile | YAML task runner, simpler than Make for our needs |

### Design Principles

**Server-Rendered with Interactivity**: The UI is server-rendered HTML enhanced with HTMX. This keeps the system fast and simple - no complex JavaScript build pipeline, no client-side state management. Interactions feel instant because only the affected fragment updates, not the whole page.

**Type Safety End-to-End**: SQL queries generate typed Go code. Templates are type-checked at compile time. This catches errors early and makes refactoring safe. If a database column is renamed, the compiler finds every place it's used.

**Facilities as Boundaries**: Most data is scoped to a facility. This keeps queries fast (indexed by facility_id) and access control simple (staff see their facility). Organization-level views aggregate across facilities when needed.

**Graceful Degradation**: If something fails, the system degrades gracefully. Can't load the member photo? Show a placeholder. Theme not found? Use defaults. Search returns no results? Clear message with suggestion to create new member.

---

## The Domain

### Entity Relationships

```
                              +----------------+
                              | organizations  |
                              +-------+--------+
                                      |
                                      | 1:N
                                      v
+----------------+            +----------------+            +----------------+
| cognito_config |----1:1----|   facilities   |----1:N----| operating_hours|
+----------------+            +-------+--------+            +----------------+
                                      |
              +------------+----------+----------+------------+
              |            |          |          |            |
              v            v          v          v            v
        +--------+   +--------+  +--------+  +---------+  +--------+
        | courts |   | themes |  | users  |  | open_   |  | reserv-|
        +--------+   +--------+  +---+----+  | play_   |  | ations |
                                     |       | rules   |  +---+----+
              +----------+-----------+       +---------+      |
              |          |           |                        |
              v          v           v                   +----+----+
        +--------+  +--------+  +--------+               |         |
        | user_  |  | user_  |  | staff  |          +----v---+ +---v----+
        | photos |  | billing|  +--------+          |reserv- | |reserv- |
        +--------+  +--------+                      |_courts | |_partic.|
                                                    +--------+ +--------+
```

### Facilities and Organizations

A pickleball business may operate multiple facilities under one organization. Each facility is a physical location with its own courts, operating hours, staff, and members. Think of the organization as the business entity (Ace Pickleball Inc.) and facilities as individual clubs (Ace Pickleball Downtown, Ace Pickleball Westside).

Each facility operates in its own timezone and sets its own hours. A facility might be open 6am-10pm on weekdays but only 8am-6pm on Sundays. These hours constrain when courts can be reserved and when open play sessions can run.

**Organization** contains:
- Name and unique slug for URLs
- Status (active/inactive)
- Optional Cognito configuration for SSO

**Facility** contains:
- Parent organization reference
- Name, slug, timezone
- Active theme selection
- Operating hours per day of week

### Courts

Courts are the fundamental bookable resource. Each facility has a numbered set of courts (Court 1, Court 2, etc.). Courts can be active or temporarily offline for maintenance, repairs, or private events.

The system tracks court availability across all reservation types. A court can only have one reservation at any given time - no double-booking is possible.

**Court** contains:
- Parent facility reference
- Name and court number (unique per facility)
- Status: active, maintenance, offline

### People in the System

The system recognizes two overlapping roles: members and staff. A person can be both - the club pro who also plays recreationally, or the manager who's also a paying member.

```
+------------------+
|      users       |
+------------------+
| Identity         |
| - email (unique) |
| - phone          |
| - name           |
+------------------+
| Authentication   |
| - cognito_sub    |
| - password_hash  |
| - local_auth_on  |
+------------------+
| Role Flags       |
| - is_member      |-----> Member fields apply
| - is_staff       |-----> Staff fields apply
+------------------+
| Member Fields    |
| - membership_lvl |
| - date_of_birth  |
| - waiver_signed  |
+------------------+
| Staff Fields     |
| - staff_role     |
| - home_facility  |
+------------------+
```

#### Members

Members are the customers. They range from first-time guests to premium members:

| Level | Name | Description |
|-------|------|-------------|
| 0 | Unverified Guest | Just signed up, hasn't verified email/phone. Can be checked in manually but can't self-serve. |
| 1 | Verified Guest | Confirmed identity. Can sign up for open play, book courts if facility allows. |
| 2 | Member | Paying member with full privileges. |
| 3+ | Member+ | Premium tiers with additional benefits defined per facility. |

Each member has a profile: name, contact info, address, photo, date of birth (required for waivers and age-restricted events), and waiver status. The system enforces that waivers must be signed before participation in play.

**Member-specific fields:**
- Date of birth (stored as YYYY-MM-DD text)
- Waiver signed status and timestamp
- Membership level (0-3+)
- Billing information (separate table)
- Photo (binary blob, separate table)

#### Staff

Staff operate the facility. Roles include:

| Role | Capabilities |
|------|--------------|
| Admin | Full system access, manage other staff, configure facility settings |
| Manager | Day-to-day operations, manage members and reservations, run reports |
| Desk | Check members in, handle walk-ins, process payments |
| Pro | Teaching professional, assigned to lessons and clinics |

Staff can belong to a specific facility (the front desk person at Downtown) or operate at the organization level (the owner who oversees all locations).

**Staff-specific fields:**
- Role (admin, manager, desk, pro)
- Home facility (NULL for organization-level)
- Local authentication enabled flag
- Password hash for local login

---

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

---

## Database Schema

### Core Entities

| Table | Purpose |
|-------|---------|
| organizations | Top-level tenant entities |
| facilities | Physical locations |
| operating_hours | Per-facility schedules |
| users | Authentication records |
| members | Customer profiles (users with is_member=1) |
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

---

## Member Management

### The Front Desk Experience

The front desk needs to quickly find any member, see their status, and take action. This is the primary workflow for staff.

```
+------------------+     +------------------+     +------------------+
|  Search Box      |     |  Results List    |     |  Member Card     |
|  "john" or       +---->|  Shows matches   +---->|  Photo, name,    |
|  "555-1234"      |     |  with photos     |     |  level, status   |
+------------------+     +------------------+     +------------------+
                                                          |
                              +---------------------------+
                              |
              +---------------+---------------+---------------+
              |               |               |               |
              v               v               v               v
        +----------+   +----------+   +----------+   +----------+
        | Check In |   |   Edit   |   |  Billing |   |  Delete  |
        +----------+   +----------+   +----------+   +----------+
```

Search works across name, email, and phone - typing "john" shows all Johns, "555-1234" finds that phone number. Results appear instantly with HTMX partial updates.

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

### Member Registration Flow

When a new person walks in, desk staff can create them on the spot:

```
+------------------+     +------------------+     +------------------+
|  /members/new    |     |  Validate Form   |     |  Check Email     |
|  (form display)  +---->|  - Required      +---->|  Uniqueness      |
+------------------+     |  - DOB format    |     +--------+---------+
                         |  - Age 0-100     |              |
                         +------------------+              v
                                                  +--------+---------+
                                                  |   Conflict?      |
                                                  +--------+---------+
                                                    |              |
                                              No    |              | Yes
                                                    v              v
                                            +-------+------+  +----+--------+
                                            | INSERT user  |  | Show Error  |
                                            | is_member=1  |  | OR Restore  |
                                            | level=0      |  | Dialog      |
                                            +------+-------+  +-------------+
                                                   |
                                                   v
                                            +------+-------+
                                            | Has photo?   |
                                            +------+-------+
                                              |          |
                                        Yes   |          | No
                                              v          |
                                       +------+------+   |
                                       | UPSERT      |   |
                                       | user_photos |   |
                                       +------+------+   |
                                              |          |
                                              +----+-----+
                                                   |
                                                   v
                                            +------+-------+
                                            | Return HTML  |
                                            | HX-Trigger:  |
                                            | refreshList  |
                                            +--------------+
```

**Required fields**: First name, last name, email (unique among members)

**Optional fields**: Phone, address, date of birth, waiver status, photo

**Photo capture**: A photo can be taken right there via webcam or phone camera using the browser MediaDevices API. The image is captured to canvas, converted to Base64, and stored as a binary blob in the database.

### Validation Rules

| Field | Rule |
|-------|------|
| Email | Required, must be unique among is_member=1 users |
| Phone | 10-20 characters if provided |
| Postal code | 5-10 characters if provided |
| Date of birth | Valid YYYY-MM-DD, calculated age must be 0-100 |
| Name | First and last name required, trimmed |

### Soft Deletion and Restoration

Members are never truly deleted. When someone's membership lapses or they request removal:

1. Status changes from 'active' to 'deleted'
2. They disappear from normal searches
3. Their history is preserved (reservations, payments, waivers)
4. Referential integrity maintained

If they return months later, the system offers to restore their account when someone tries to create a duplicate email. All their history comes back.

---

## Court Reservations

Courts can be reserved for different purposes, and the system handles each appropriately.

### Reservation Types (Seeded)

| Type | Description | Multi-Court | Participants |
|------|-------------|-------------|--------------|
| GAME | Member books court for themselves and friends | Optional | Primary + guests |
| PRO_SESSION | Teaching pro assigned for lesson/clinic | Optional | Pro + students |
| EVENT | Larger gathering, tournament | Yes | Teams with rosters |
| OPEN_PLAY | Drop-in rotation session | Yes | Dynamic signup |
| MAINTENANCE | Blocks court from booking | No | None |
| LEAGUE | Recurring competitive play | Yes | Team rosters |

### Reservation Structure

Each reservation captures:

- **Facility and court(s)**: Where it happens
- **Time window**: Start and end datetime
- **Type**: What kind of reservation
- **Primary user**: Who's responsible
- **Participants**: Who's playing (junction table)
- **Recurrence**: One-time or repeating pattern
- **Open play rule**: For open play sessions, which rules apply

Multi-court reservations are supported through a junction table. A tournament might need all 8 courts for a Saturday. An event might need courts 1-4 while leaving 5-8 for regular bookings.

### Booking Workflows

**Quick Booking (`/api/v1/courts/booking/new`)**
- Single court selection (pre-filled from calendar click)
- Start/end time with 1-hour default duration
- Reservation type dropdown
- Optional primary user (member search)
- Validates: start < end, minimum 1-hour duration, no double-booking

**Event Booking (`/api/v1/events/booking/new`)**
- Multi-court selection (checkboxes)
- Participant management (add/remove members)
- Extended options for events
- Same validation as quick booking

### Calendar Display

- Courts shown as columns, hours as rows
- Reservations rendered as colored blocks positioned by time
- Block color determined by reservation_type.color
- Clicking empty cell opens quick booking form
- Clicking reservation block opens edit form
- Date navigation with query parameter `?date=YYYY-MM-DD`

### Visual Indicators

Reservations use these colors by type:

| Color | Reservation Type |
|-------|------------------|
| Blue | Regular game booking |
| Green | Open play session |
| Orange | Pro session/lesson |
| Purple | Event/tournament |
| Gray | Maintenance block |
| Red | League play |

### Recurrence Patterns

Reservations can be one-time or recurring:

| Pattern | Example |
|---------|---------|
| Weekly | Tuesday night league, every week |
| Biweekly | Beginner clinic, every other Saturday |
| Monthly | Club tournament, first Sunday of month |

The recurrence rule stores the pattern definition (compatible with iCalendar RRULE format), and the system generates individual reservation instances.

Note: Recurrence rules are defined in schema but not yet implemented in handlers.

### Validation Rules

- Start time must be before end time
- Minimum duration: 1 hour
- Court must be available (no overlapping reservations)
- Facility must exist and user must have access
- Conflict errors shown inline with red border styling (409 response)

---

## Open Play Sessions

Open play is the heart of recreational pickleball. Members show up during designated hours, sign in, and rotate through games with whoever else is there. Unlike reserved court time, open play is drop-in - you don't need a group, you just show up and play.

### How Open Play Works

1. Facility defines open play time slots (e.g., "Morning Open Play, 8am-12pm")
2. Members sign up in advance or walk in
3. System tracks signup count
4. At cutoff time, system evaluates: enough players?
5. If yes, session runs. If no, session cancels and courts release.
6. During session, players rotate based on facility rules

### Open Play Rules

Each facility configures rules that govern session behavior:

```
+------------------+     +------------------+     +------------------+
|  Create Rule     |     |  Attach to       |     |  Session         |
|  (min/max        +---->|  Reservation     +---->|  Approaches      |
|  participants)   |     |  open_play_      |     |  Cutoff          |
+------------------+     |  rule_id         |     +--------+---------+
                         +------------------+              |
                                                           v
                                                  +--------+---------+
                                                  | Count Signups    |
                                                  | (reservation_    |
                                                  | participants)    |
                                                  +--------+---------+
                                                           |
                                              +------------+------------+
                                              |                         |
                                     < min    |                         | >= min
                                              v                         v
                                      +-------+--------+        +-------+--------+
                                      | Cancel Session |        | If auto_scale  |
                                      | Release Courts |        | enabled:       |
                                      +----------------+        | Adjust courts  |
                                                                +----------------+
```

**Rule parameters:**

| Parameter | Default | Purpose |
|-----------|---------|---------|
| min_participants | 4 | Below this at cutoff = cancel |
| max_participants_per_court | 8 | Capacity per court for rotation |
| cancellation_cutoff_minutes | 60 | When to evaluate go/no-go |
| auto_scale_enabled | true | Dynamically adjust court count |
| min_courts | 1 | Never scale below this |
| max_courts | 4 | Never scale above this |

**Validation constraints:**
- All numeric values must be > 0
- min_courts must be <= max_courts
- min_participants must be <= max_participants_per_court * min_courts

### Auto-Scaling Logic

When auto_scale_enabled is true, the system adjusts court allocation based on signups:

```
Signups: 6   Courts needed: ceil(6/8) = 1
Signups: 12  Courts needed: ceil(12/8) = 2
Signups: 20  Courts needed: ceil(20/8) = 3
Signups: 35  Courts needed: ceil(35/8) = 5 -> capped at max_courts (4)
```

Scaling respects court availability. If the session needs 3 courts but only 2 are free, it uses 2 and logs the constraint.

### Open Play Enforcement Engine

Open play sessions are managed by a gocron scheduler that:
- Evaluates sessions at configured intervals
- Auto-scales courts based on participant count
- Notifies staff of important events
- Logs all automated decisions to audit_log

### Auto-Scale Override

Staff can toggle auto-scaling for individual sessions via PUT `/api/v1/open-play-sessions/{id}/auto-scale`. Options:
- Override for current session only
- Disable for entire rule (affects future sessions)

All overrides are logged to the audit_log.

### Staff Notifications

When a session is cancelled or courts are reallocated, staff receive in-app notifications:

- "Morning Open Play cancelled - only 2 signups (minimum: 4)"
- "Evening Open Play scaled from 2 to 3 courts - 22 participants"

This allows staff to inform affected members and adjust operations.

---

## Theming and Branding

Each facility can have its own visual identity. The member check-in screen at Downtown looks different from Westside because they chose different themes.

### Theme Hierarchy

```
+-------------------+
|   System Themes   |  Pre-built, available to all
|  (facility=NULL)  |
+--------+----------+
         |
         | Facility can use OR
         v
+--------+----------+
|  Facility Themes  |  Custom, only for this facility
| (facility=set)    |
+--------+----------+
         |
         | Facility selects active theme
         v
+--------+----------+
|   Active Theme    |  Applied to all pages
| (facility FK)     |
+-------------------+
```

### Theme Application Flow

```
+------------------+     +------------------+     +------------------+
|  Page Request    |     |  Extract         |     |  Load Active     |
|  ?facility_id=X  +---->|  facility_id     +---->|  Theme           |
+------------------+     +------------------+     +--------+---------+
                                                          |
                                                          v
                                                  +-------+--------+
                                                  | facilities     |
                                                  | .active_theme_ |
                                                  | id             |
                                                  +-------+--------+
                                                          |
                                              +-----------+-----------+
                                              |                       |
                                        Set   |                       | NULL
                                              v                       v
                                      +-------+--------+      +-------+--------+
                                      | SELECT theme   |      | Use Default    |
                                      | WHERE id=X     |      | Theme()        |
                                      +-------+--------+      +-------+--------+
                                              |                       |
                                              +-----------+-----------+
                                                          |
                                                          v
                                                  +-------+--------+
                                                  | Generate CSS   |
                                                  | Variables      |
                                                  +-------+--------+
                                                          |
                                                          v
                                                  +-------+--------+
                                                  | Inject into    |
                                                  | <head> <style> |
                                                  +-------+--------+
                                                          |
                                                          v
                                                  +-------+--------+
                                                  | Tailwind uses  |
                                                  | var(--theme-*) |
                                                  +----------------+
```

### Color Palette

A theme defines five colors:

| Color | Purpose | Default |
|-------|---------|---------|
| Primary | Main brand color, headers | #1f2937 (dark gray) |
| Secondary | Complementary, borders | #e5e7eb (light gray) |
| Tertiary | Backgrounds, subtle areas | #f9fafb (near white) |
| Accent | CTAs, links, buttons | #2563eb (blue) |
| Highlight | Success states, confirmations | #16a34a (green) |

**Generated CSS:**
```css
:root {
    --theme-primary: #1f2937;
    --theme-secondary: #e5e7eb;
    --theme-tertiary: #f9fafb;
    --theme-accent: #2563eb;
    --theme-highlight: #16a34a;
}
```

Tailwind classes reference these variables: `bg-[var(--theme-primary)]`, `text-[var(--theme-accent)]`

### Accessibility Requirements

Colors must meet WCAG AA contrast standards:

- Text must have 3.0+ contrast ratio against backgrounds
- System validates by testing each color against black (#000000) and white (#FFFFFF)
- Uses relative luminance formula per WCAG 2.0 specification
- Rejects themes that would produce unreadable text

### Theme Operations

**Create**: Define colors for a facility. Name must be unique within that facility's scope. Name validation: alphanumeric, spaces, hyphens, parentheses.

**Clone**: Copy an existing theme (system or facility) as a starting point. Useful for taking "Classic Court" and tweaking one color.

**Edit**: Modify colors. System re-validates accessibility on save.

**Delete**: Remove a theme. Cannot delete if it's the active theme for any facility.

**Set Active**: Assign a theme to a facility. Takes effect immediately on next page load.

---

## Authentication and Access

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

### Member Authentication

Members can authenticate in multiple ways:

| Method | Flow |
|--------|------|
| Email code | System sends 6-digit code, member enters it |
| SMS code | Code sent to phone number |
| Cognito SSO | Redirects to AWS Cognito hosted UI |

Authentication is optional for walk-in check-ins. Staff can check someone in without the member having an account - useful for first-time guests or those who forgot their phone.

Note: Cognito integration handlers exist but SDK integration is not yet complete (marked TODO in code).

### Cognito Organization Integration

Organizations can integrate their AWS Cognito user pool for SSO:

- Pool ID and client credentials stored per organization (cognito_config table)
- Callback URLs for OAuth flow
- Members authenticate once, access all organization systems

### Auth Middleware

The `WithAuth` middleware:
1. Attempts to load user from session token or auth cookie
2. If valid, attaches `AuthUser` to request context via `authz.ContextWithUser`
3. Proceeds to next handler regardless of auth status (endpoints enforce their own requirements)

---

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

### Protected Endpoints

All facility-scoped endpoints enforce authorization:
- Theme management (6 endpoints)
- Open play rules (6 endpoints)
- Court calendar and booking
- Reservations CRUD

Authorization failures are logged with facility_id and user_id.

### Error Codes

| Code | Description |
|------|-------------|
| 401 | Unauthenticated - no valid session |
| 403 | Forbidden - authenticated but lacks permission |

---

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
| GET | `/api/v1/open-play-sessions/{id}/participants` | List participants |
| POST | `/api/v1/open-play-sessions/{id}/participants` | Add participant |
| DELETE | `/api/v1/open-play-sessions/{id}/participants/{user_id}` | Remove participant |
| PUT | `/api/v1/open-play-sessions/{id}/auto-scale` | Toggle auto-scale override |

### Themes

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/themes` | Themes admin page |
| GET | `/api/v1/themes` | List themes |
| POST | `/api/v1/themes` | Create theme |
| GET | `/api/v1/themes/new` | New theme form |
| GET | `/api/v1/themes/{id}` | Theme detail |
| PUT | `/api/v1/themes/{id}` | Update theme |
| DELETE | `/api/v1/themes/{id}` | Delete theme |
| POST | `/api/v1/themes/{id}/clone` | Clone theme |
| PUT | `/api/v1/facilities/{id}/theme` | Set facility active theme |

---

## Request/Response Patterns

### HTMX Integration

All endpoints detect HTMX requests via `HX-Request: true` header:

- **HTMX request**: Returns HTML fragment, sets trigger headers
- **Direct request**: Returns full page or JSON

**Response headers for dynamic updates:**
```
HX-Trigger: refreshMembersList    # Fire client-side event
HX-Retarget: #member-detail       # Change swap target
HX-Reswap: innerHTML              # Change swap strategy
HX-Redirect: /members             # Full page redirect
```

### Error Handling

| HTTP Code | Meaning |
|-----------|---------|
| 200 | Success (GET, PUT) |
| 201 | Created (POST) |
| 204 | Deleted (DELETE with HTMX) |
| 400 | Validation error |
| 401 | Unauthenticated |
| 403 | Forbidden |
| 404 | Not found |
| 405 | Method not allowed |
| 409 | Conflict (duplicate, double-booking) |
| 500 | Server error |
| 501 | Not implemented |

Validation errors return plain text messages suitable for display.

---

## Middleware Stack

Every request passes through middleware in order:

```
Request
   |
   v
+------------------+
| WithLogging      |  Logs: method, path, status, duration, request_id
+--------+---------+
         |
         v
+--------+---------+
| WithRecovery     |  Catches panics, logs stack trace, returns 500
+--------+---------+
         |
         v
+--------+---------+
| WithRequestID    |  Generates UUID, adds to context + X-Request-ID header
+--------+---------+
         |
         v
+--------+---------+
| WithAuth         |  Loads auth session into context
+--------+---------+
         |
         v
+--------+---------+
| WithContentType  |  Sets default Accept: text/html
+--------+---------+
         |
         v
     Handler
```

Every response includes `X-Request-ID` for tracing issues through logs.

---

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

---

## Configuration

### config.yaml

```yaml
app:
  name: "Pickleicious"
  environment: "development"    # development | production
  port: 8080
  base_url: "http://localhost:8080"
  secret_key: "your-secret-key-here"  # Required for auth cookie signing

database:
  driver: "sqlite"              # sqlite | turso
  filename: "build/db/pickleicious.db"

features:
  enable_metrics: false
  enable_tracing: false
  enable_debug: true

open_play:
  enforcement_interval: "5m"
```

### Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| APP_SECRET_KEY | Application secret (required for auth cookie signing) | - |
| DATABASE_AUTH_TOKEN | Turso cloud auth | - |
| STATIC_DIR | Static file location | build/bin/static |

### Validation

Config validation enforces:
- `app.port` required
- `app.secret_key` required
- `database.driver` required

---

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

### Build Artifacts

```
build/
  bin/
    server              # Compiled Go binary
    static/             # CSS, JS, images
  db/
    pickleicious.db     # SQLite database file
```

---

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

---

## Shared Utilities

### apiutil Package

Common handler utilities in `internal/api/apiutil`:
- `DecodeJSON` - Strict JSON decoding with unknown field rejection
- `WriteJSON` - JSON response writing
- `RequireFacilityAccess` - Authorization check with logging
- `FieldError` - Field-level validation error
- `HandlerError` - HTTP error with status code

### htmx Package

HTMX helper utilities in `internal/api/htmx`:
- `IsRequest(r)` - Check if request is HTMX
- Response header helpers

### request Package

Request parsing utilities in `internal/request`:
- `ParseFacilityID` - Parse facility_id from string
- `FacilityIDFromBookingRequest` - Extract facility_id from query or form

### testutil Package

Test helpers in `internal/testutil`:
- `NewTestDB` - Create test database with migrations applied

---

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
│   │   ├── htmx/            # HTMX helpers
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

---

## Business Rules

### Waiver Requirements

Members must have a signed waiver before participating in play:

- `waiver_signed` boolean tracks current status
- Staff see visual indicator when checking in someone who needs to sign
- System can block participation until waiver is complete (configurable)

### Age Restrictions

Date of birth enables age-based features:

| Use Case | Age Check |
|----------|-----------|
| Junior programs | Under 18 |
| Senior events | 55+, 65+, 70+ |
| Alcohol service | 21+ verification |

Age is validated at entry time - system rejects future dates and ages over 120.

### Capacity Management

Each reservation type has capacity limits enforced at signup time:

| Resource | Constraint |
|----------|------------|
| Court | One reservation per time slot |
| Open play | max_participants_per_court x courts |
| Event | teams x people_per_team |
| Lesson | Student limit per pro |

Overbooking is not possible. Attempting to exceed capacity returns an error.

### Soft Deletion

Members are never truly deleted:

1. Status changes to 'deleted'
2. Hidden from searches
3. History preserved (reservations, payments)
4. Referential integrity maintained
5. Can be restored later

---

## Notifications

### Current State

Notifications are **not yet implemented**. The top navigation has a placeholder bell icon but no notification system exists. Staff notifications table exists in schema.

### Planned: Staff Notifications (In-App)

When implemented, staff will see alerts for operational events:

| Event | Example |
|-------|---------|
| Open play cancelled | "Morning Open Play cancelled - only 2 signups (min: 4)" |
| Courts reallocated | "Evening Open Play scaled 2->3 courts (22 participants)" |
| New registration | "New member: John Smith - waiver pending" |
| Capacity warning | "Saturday Open Play at 90% capacity (36/40)" |

### Planned: Member Communications

Planned delivery channels: email, SMS, push notifications (mobile app)

- Reservation confirmations
- Session cancellation notices
- Waitlist openings
- Upcoming reservation reminders
- Membership renewal reminders

---

## Implementation Status

### Operational Today

| Feature | Status | Notes |
|---------|--------|-------|
| Member CRUD | Complete | Create, list, search, edit, soft delete, restore |
| Member Photos | Complete | Base64 upload, BLOB storage, MediaDevices API |
| Member Search | Complete | Name, email, phone - instant results |
| Theme Management | Complete | Create, edit, clone, delete, set active |
| Theme Accessibility | Complete | WCAG AA contrast validation (3.0 ratio) |
| Open Play Rules | Complete | Full CRUD with constraint validation |
| Open Play Sessions | Partial | Session tracking, participant management |
| Court Calendar | Complete | Day view with reservations, date navigation |
| Reservations | Complete | CRUD, multi-court, participants, conflict detection |
| Staff Local Login | Complete | Bcrypt, rate limiting, timing attack mitigation |
| Authorization | Complete | Facility-scoped access, admin override |

### Partial Implementation

| Feature | Status | Notes |
|---------|--------|-------|
| Cognito Auth | Framework only | Handlers exist, SDK integration TODO |
| Open Play Enforcement | Scheduled | gocron job configured, evaluation logic partial |
| Password Reset | Not implemented | Returns 501 |
| Staff Management | Schema only | Tables exist, no handlers |
| Operating Hours | Schema only | Tables exist, not used in handlers |
| Recurrence Rules | Schema only | Tables exist, not used in handlers |

### Not Yet Started

| Feature | Notes |
|---------|-------|
| Check-in Flow | The tap-to-arrive workflow |
| Member Notifications | Email/SMS delivery |
| Payment Processing | Stripe/Square integration |
| Reporting | Usage stats, financials |
| Mobile App | Native iOS/Android |

---

## Tech Stack Philosophy

**HTMX-First**: Server-rendered HTML with HTMX for interactivity. No JavaScript build pipeline, no client-side state management. The server is the source of truth, and HTMX handles partial page updates. JavaScript is only used when absolutely necessary for UX (camera capture, color pickers).

**Type Safety End-to-End**: sqlc generates Go from SQL. templ generates Go from HTML templates. Errors caught at compile time, not runtime.

**Embedded Database**: SQLite for zero-ops single-facility deployments. Turso (edge-distributed SQLite) available for multi-region scaling without changing application code.

---

## Terminology

| Term | Definition |
|------|------------|
| **Facility** | Physical location with courts, a club |
| **Organization** | Business entity owning one or more facilities |
| **Member** | Customer using the facility (guest through premium) |
| **Staff** | Employee operating the facility |
| **Court** | Bookable playing surface |
| **Reservation** | Time block on one or more courts |
| **Open Play** | Drop-in session with rotation rules |
| **Theme** | Color scheme for facility branding |
| **Waiver** | Liability agreement required for play |
| **Soft Delete** | Mark as deleted but preserve history |
