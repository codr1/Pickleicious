# Pickleicious Specification

Pickleicious is a management system for pickleball facilities. It handles the daily operations of running a club: tracking who walks through the door, managing court reservations, running open play sessions, and giving each facility its own branded experience.

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

**Photo capture**: A photo can be taken right there via webcam or phone camera. The image is base64-encoded and stored as a binary blob. Photos are used for visual identification on future visits.

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

### Reservation Types

| Type | Description | Multi-Court | Participants |
|------|-------------|-------------|--------------|
| Game | Member books court for themselves and friends | Optional | Primary + guests |
| Pro Session | Teaching pro assigned for lesson/clinic | Optional | Pro + students |
| Event | Larger gathering, tournament | Yes | Teams with rosters |
| Open Play | Drop-in rotation session | Yes | Dynamic signup |
| Maintenance | Blocks court from booking | No | None |
| League | Recurring competitive play | Yes | Team rosters |

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

### Recurrence Patterns

Reservations can be one-time or recurring:

| Pattern | Example |
|---------|---------|
| Weekly | Tuesday night league, every week |
| Biweekly | Beginner clinic, every other Saturday |
| Monthly | Club tournament, first Sunday of month |

The recurrence rule stores the pattern definition (compatible with iCalendar RRULE format), and the system generates individual reservation instances.

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

**Create**: Define colors for a facility. Name must be unique within that facility's scope.

**Clone**: Copy an existing theme (system or facility) as a starting point. Useful for taking "Classic Court" and tweaking one color.

**Edit**: Modify colors. System re-validates accessibility on save.

**Delete**: Remove a theme. Cannot delete if it's the active theme for any facility.

**Set Active**: Assign a theme to a facility. Takes effect immediately on next page load.

---

## Authentication and Access

### Member Authentication

Members can authenticate in multiple ways:

| Method | Flow |
|--------|------|
| Email code | System sends 6-digit code, member enters it |
| SMS code | Code sent to phone number |
| Cognito SSO | Redirects to AWS Cognito hosted UI |

Authentication is optional for walk-in check-ins. Staff can check someone in without the member having an account - useful for first-time guests or those who forgot their phone.

### Staff Authentication

Staff have local accounts with passwords, separate from member authentication. A pro might:
1. Log in as staff to manage their lesson schedule
2. Check themselves in as a member for open play

Staff authentication is required for administrative functions. No anonymous access to member data, financial information, or system configuration.

### Authorization Model

Access is scoped to facilities:

| Role | Scope |
|------|-------|
| Desk Staff | Own facility only |
| Manager | Own facility only |
| Admin | All facilities in organization |
| Pro | Own schedule + assigned facility |

A desk staff member at Downtown cannot see member data from Westside unless they have organization-level access.

### Cognito Integration

Organizations can integrate their AWS Cognito user pool for SSO:

- Pool ID and client credentials stored per organization
- Callback URLs for OAuth flow
- Members authenticate once, access all organization systems

---

## Scheduling and Calendars

### Current Implementation

The court calendar displays an 8-court grid for a single day, showing time slots from 6am to 10pm.

**UI Elements:**
- Date navigation (previous/next day buttons)
- View selector dropdown (Work Week/Week/Month - UI only, not yet functional)
- 8-column grid with hourly time slots

**What Exists:**
- Basic day grid display
- Date shown in header
- Click-to-book triggers HTMX request (handler not yet wired)

**What Does Not Exist Yet:**
- Actual reservations displayed on calendar
- Week/Month view rendering
- Booking modal and creation flow
- Drag-to-reschedule
- Hover previews

The calendar templates and handlers are scaffolded at `internal/templates/components/courts/calendar.templ` and `internal/api/courts/handlers.go`.

### Planned Visual Indicators

When reservations are implemented, they will use these colors:

| Color | Reservation Type |
|-------|------------------|
| Blue | Regular game booking |
| Green | Open play session |
| Orange | Pro session/lesson |
| Purple | Event/tournament |
| Gray | Maintenance block |
| Red | League play |

---

## Notifications

### Current State

Notifications are **not yet implemented**. The top navigation has a placeholder bell icon but no notification system exists.

### Planned: Staff Notifications (In-App)

When implemented, staff will see alerts for operational events:

| Event | Example |
|-------|---------|
| Open play cancelled | "Morning Open Play cancelled - only 2 signups (min: 4)" |
| Courts reallocated | "Evening Open Play scaled 2→3 courts (22 participants)" |
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
| Open play | max_participants_per_court × courts |
| Event | teams × people_per_team |
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
| 404 | Not found |
| 405 | Method not allowed |
| 409 | Conflict (duplicate) |
| 500 | Server error |

Validation errors return plain text messages suitable for display.

---

## Middleware Chain

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
| WithContentType  |  Sets default Accept: text/html
+--------+---------+
         |
         v
     Handler
```

Every response includes `X-Request-ID` for tracing issues through logs.

---

## Configuration

### Application Config (config.yaml)

```yaml
app:
  name: "Pickleicious"
  environment: "development"    # development | production
  port: 8080
  base_url: "http://localhost:8080"

database:
  driver: "sqlite"              # sqlite | turso
  filename: "build/db/pickleicious.db"

features:
  enable_metrics: false
  enable_tracing: false
  enable_debug: true
```

### Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| STATIC_DIR | Static file location | build/bin/static |
| DATABASE_AUTH_TOKEN | Turso cloud auth | - |
| APP_SECRET_KEY | Signing key | - |

---

## Build System

### Taskfile Commands

| Command | Purpose |
|---------|---------|
| `task generate` | Run templ + sqlc code generation |
| `task build` | Build server binary |
| `task build:prod` | Production build (stripped) |
| `task test` | Run Go tests |
| `task css` | Build Tailwind CSS |
| `task db:migrate` | Apply database migrations |
| `task db:reset` | Reset database to clean state |
| `task clean` | Remove build artifacts |
| `task dev` | Development with hot reload |

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

## Implementation Status

### Operational Today

| Feature | Status | Notes |
|---------|--------|-------|
| Member CRUD | Complete | Create, list, search, edit, soft delete, restore |
| Member Photos | Complete | Base64 upload, BLOB storage |
| Member Search | Complete | Name, email, phone - instant results |
| Theme Management | Complete | Create, edit, clone, delete, set active |
| Theme Accessibility | Complete | WCAG AA contrast validation |
| Open Play Rules | Complete | Full CRUD with constraint validation |
| Court Calendar | Partial | Display works, booking not wired |

### Scaffolded (Schema Ready)

| Feature | Status | Notes |
|---------|--------|-------|
| Reservations | Schema only | Tables exist, no handlers |
| Recurring Events | Schema only | Recurrence rules defined |
| Participant Tracking | Schema only | Junction tables ready |
| Member Billing | Schema only | Table exists, no processing |

### Not Yet Started

| Feature | Notes |
|---------|-------|
| Check-in Flow | The tap-to-arrive workflow |
| Open Play Enforcement | Automated cancellation and scaling |
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
