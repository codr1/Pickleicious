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
| Email | AWS SES v2 |
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

```
┌─────────────────────────────────────────────────────────────┐
│                      ORGANIZATION                            │
│  (Corporate entity - e.g., "PicklePlex Holdings")           │
│  - Custom Cognito configuration                              │
│  - Custom domain (organization.pickleadmin.com)             │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│   FACILITY A    │ │   FACILITY B    │ │   FACILITY C    │
│ (Location 1)    │ │ (Location 2)    │ │ (Location 3)    │
│ - Own courts    │ │ - Own courts    │ │ - Own courts    │
│ - Own hours     │ │ - Own hours     │ │ - Own hours     │
│ - Own theme     │ │ - Own theme     │ │ - Own theme     │
└─────────────────┘ └─────────────────┘ └─────────────────┘
```

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
- Booking configuration (max_advance_booking_days, max_member_reservations, lesson_min_notice_hours)

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
| Pro | Teaching professional, assigned to lessons and clinics, manage own unavailability |

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
| cognito_config | Legacy (unused - auth via env vars) |

### Reservation System

| Table | Purpose |
|-------|---------|
| reservation_types | Booking type lookup (system types: GAME, OPEN_PLAY, PRO_SESSION, EVENT, MAINTENANCE, LEAGUE, LESSON, TOURNAMENT, CLINIC) |
| recurrence_rules | Recurring patterns (WEEKLY, BIWEEKLY, MONTHLY) |
| reservations | Booking records (includes created_by_user_id to track who created the reservation) |
| reservation_courts | Multi-court junction |
| reservation_participants | Multi-member junction |
| reservation_cancellations | Cancellation log: reservation_id, cancelled_by_user_id, cancelled_at, refund_percentage_applied, fee_waived, hours_before_start |
| cancellation_policy_tiers | Per-facility refund tiers: facility_id, min_hours_before, refund_percentage |
| pro_unavailability | Time blocks when pros are unavailable for lessons |

### Check-in System

| Table | Purpose |
|-------|---------|
| facility_visits | Tracks member arrivals: user_id, facility_id, check_in_time, check_out_time (nullable), checked_in_by_staff_id, activity_type, related_reservation_id |

### Open Play System

| Table | Purpose |
|-------|---------|
| open_play_rules | Configuration for open play sessions |
| open_play_sessions | Individual open play session instances |
| staff_notifications | Staff notification storage (includes lesson_cancelled type with target_staff_id) |
| audit_log | Audit trail for automated decisions |

### Waitlist System

| Table | Purpose |
|-------|---------|
| waitlist_config | Per-facility waitlist settings (max size, notification mode, offer expiry) |
| waitlists | Waitlist entries tracking slot interest |
| waitlist_offers | Time-limited offers to waitlisted members when slots become available |

### Visit Pack System

| Table | Purpose |
|-------|---------|
| visit_pack_types | Pack definitions per facility (name, price, visit count, validity) |
| visit_packs | Purchased packs owned by users (visits remaining, expiration) |
| visit_pack_redemptions | Log of pack usage (links pack, facility, optional reservation) |

### Lesson Package System

| Table | Purpose |
|-------|---------|
| lesson_package_types | Package definitions per facility (name, price, lesson count, validity) |
| lesson_packages | Purchased packages owned by users (lessons remaining, expiration) |
| lesson_package_redemptions | Log of package usage (links package, facility, reservation) |

### Clinic System

| Table | Purpose |
|-------|---------|
| clinic_types | Clinic templates per facility (name, description, min/max participants, price, status) |
| clinic_sessions | Scheduled clinic instances (links clinic_type, pro, times, enrollment_status) |
| clinic_enrollments | Member enrollments (user_id, clinic_session_id, status: enrolled/waitlisted/cancelled) |

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

## Staff Management

Staff records are managed through the `/staff` page. Admins and managers can create, edit, and deactivate staff members.

### Staff List

The staff list shows all staff with name, email, role, and home facility. The list can be filtered by:
- **Facility**: Show only staff assigned to a specific facility
- **Role**: Filter by admin, manager, desk, or pro
- **Search**: Text search across name and email

### Staff Creation

Creating new staff requires:
- First name and last name (required)
- Email (required, must be unique)
- Phone (optional)
- Role (admin, manager, desk, pro)
- Home facility (optional - NULL for corporate-level staff)
- Local authentication toggle

The form reuses the photo capture component from the members module (camera.js, Base64 upload pattern).

### Staff Editing

Admins and managers can:
- Update staff profile information
- Change staff role
- Transfer staff between facilities (update home_facility_id)
- Enable/disable local password authentication

### Staff Deactivation

Staff deactivation performs a soft delete by setting user status to 'inactive'. When deactivating a staff member who has future assigned pro sessions:

1. System queries for future reservations where staff is the primary user
2. If sessions exist, a modal presents options:
   - **Reassign**: Transfer sessions to another pro at the same facility
   - **Cancel**: Delete the affected sessions
   - **Abort**: Cancel the deactivation
3. A confirmation hash prevents race conditions if sessions change between modal display and confirmation

Sessions are validated again at confirmation time. If the session count changes, the operation aborts and forces a fresh decision.

### Pro Unavailability

Teaching pros (staff with role='pro') can mark themselves as unavailable for specific time blocks. These blocks prevent members from booking lessons during those times.

#### Unavailability Management

Pros access unavailability management at `/staff/unavailability`. The interface allows:

- **View blocks**: List of all future unavailability blocks for the logged-in pro
- **Create block**: Add new unavailability with start time, end time, and optional reason
- **Delete block**: Remove an unavailability block (only own blocks)

#### Unavailability Rules

| Rule | Description |
|------|-------------|
| Pro-scoped | Pros can only manage their own unavailability |
| Future only | Start time must be in the future when creating |
| Valid range | End time must be after start time |
| No overlap validation | System allows overlapping blocks (they combine to block time) |

#### Effect on Lesson Availability

When members view available lesson slots:
1. System calculates slots from facility operating hours
2. Subtracts pro's existing reservations (where pro_id = selected pro)
3. Subtracts pro's unavailability blocks
4. Remaining slots are shown as available

---

## Court Reservations

Courts can be reserved for different purposes, and the system handles each appropriately.

### Reservation Types (System)

System reservation types are seeded on database creation and protected from deletion. User-defined types can be created but system types cannot be removed.

| Type | Description | Multi-Court | Participants |
|------|-------------|-------------|--------------|
| GAME | Member books court for themselves and friends | Optional | Primary + guests |
| PRO_SESSION | Teaching pro assigned for lesson/clinic | Optional | Pro + students |
| EVENT | Larger gathering, tournament | Yes | Teams with rosters |
| OPEN_PLAY | Drop-in rotation session | Yes | Dynamic signup |
| MAINTENANCE | Blocks court from booking | No | None |
| LEAGUE | Recurring competitive play | Yes | Team rosters |
| LESSON | One-on-one or small group instruction | Optional | Pro + students |
| TOURNAMENT | Competitive tournament play | Yes | Registered players |
| CLINIC | Group instructional session | Optional | Instructor + participants |

### Reservation Structure

Each reservation captures:

- **Facility and court(s)**: Where it happens
- **Time window**: Start and end datetime
- **Type**: What kind of reservation
- **Primary user**: Who's responsible
- **Created by user**: Who created the reservation (tracks staff-created bookings)
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

**Staff Lesson Booking (`/api/v1/staff/lessons/booking/new`)**
- Accessed via "Book Lesson" action on courts calendar (staff only)
- Facility selector for org-level staff (home_facility_id=NULL)
- Pro selection dropdown showing active pros at selected facility
- Date picker with available lesson slots for selected pro
- Member search scoped to selected facility only
- Creates PRO_SESSION reservation with pro_id, member as primary_user
- Enforces lesson_min_notice_hours facility setting
- Counts toward member's max_member_reservations limit
- Staff can view pro's upcoming lesson schedule before booking

### Calendar Display

- Courts shown as columns, hours as rows
- Reservations rendered as colored blocks positioned by time
- Block color determined by reservation_type.color
- Clicking empty cell opens quick booking form
- Clicking reservation block opens edit form
- Date navigation with query parameter `?date=YYYY-MM-DD`
- "Book Lesson" action button available for staff users

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

Reservations created by staff display a small "Staff" badge on the calendar block, distinguishing staff-booked lessons from member self-bookings.

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

### Staff Cancellation with Fee Waiver

When staff cancel a reservation that falls within a penalty window (refund < 100%):

1. System returns HTTP 409 with cancellation penalty details
2. Staff prompted: "This cancellation incurs a fee. Waive cancellation fee?" with Yes/No options
3. If staff selects "Waive fee", reservation cancelled with 100% refund regardless of policy
4. If staff selects "No", reservation cancelled with policy-determined refund
5. Cancellation logged with fee_waived flag and applied refund percentage

Only staff can waive fees; members always receive the policy-determined refund.

---

## Cancellation Policies

Facilities can configure time-based cancellation policies that determine refund percentages based on how early a reservation is cancelled.

### Policy Tiers

Each tier specifies:

| Field | Description |
|-------|-------------|
| min_hours_before | Minimum hours before reservation start for this tier to apply |
| refund_percentage | Refund percentage (0-100) when this tier matches |
| reservation_type_id | Optional: Apply tier only to specific reservation type (e.g., PRO_SESSION, GAME) |

Tiers are ordered by `min_hours_before` descending. The system first looks for a tier matching the specific reservation type, then falls back to default (NULL type) tiers if no type-specific tier applies.

### Example Configuration

| Min Hours Before | Refund % | Meaning |
|------------------|----------|---------|
| 48 | 100 | Cancel 48+ hours ahead: full refund |
| 24 | 50 | Cancel 24-47 hours ahead: 50% refund |
| 0 | 0 | Cancel <24 hours ahead: no refund |

### Default Behavior

If no policy tiers are configured for a facility, 100% refund applies at any time (preserves pre-policy behavior).

### Admin Interface

Staff access the cancellation policy page at `/admin/cancellation-policy?facility_id=X`. The interface displays:

- Reservation type filter dropdown (Court Reservations/GAME, Lessons/PRO_SESSION)
- Form to add new tiers (min hours, refund percentage, optional reservation type)
- List of existing tiers with inline editing
- Auto-save on field changes with 300ms debounce
- Delete confirmation for each tier

### Validation Rules

| Field | Rule |
|-------|------|
| min_hours_before | Must be >= 0, unique per facility + reservation_type combination |
| refund_percentage | Must be 0-100 |
| reservation_type_id | Must reference valid reservation type if provided |

### Cancellation Logging

All cancellations are logged to `reservation_cancellations` with:

- Timestamp of cancellation
- Who cancelled (user ID)
- Refund percentage applied
- Whether fee was waived (staff only)
- Hours before reservation start at time of cancellation

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

## Group Clinics

Clinics are scheduled group instruction sessions led by a teaching pro. Unlike open play (drop-in rotation) or lessons (one-on-one), clinics are structured group classes with defined participant limits and pricing.

### Clinic Types

Clinic types are templates that define the characteristics of a clinic offering:

| Field | Description |
|-------|-------------|
| name | Display name (e.g., "Beginner Dinking Clinic") |
| description | Optional detailed description |
| min_participants | Minimum enrollment for session to run |
| max_participants | Maximum capacity |
| price_cents | Price per participant in cents |
| status | draft, active, inactive, archived |

Only clinic types with status "active" are available for member enrollment.

### Clinic Sessions

A clinic session is a scheduled instance of a clinic type:

| Field | Description |
|-------|-------------|
| clinic_type_id | Reference to the clinic template |
| pro_id | Teaching pro assigned to run the session |
| start_time | Session start datetime |
| end_time | Session end datetime |
| enrollment_status | open or closed |
| created_by_user_id | Staff who created the session |

Sessions with enrollment_status "open" accept new enrollments. Staff can close enrollment manually.

### Enrollment States

Member enrollments track participation with these statuses:

| Status | Description |
|--------|-------------|
| enrolled | Confirmed participant (within max_participants) |
| waitlisted | Overflow beyond max_participants, auto-promoted on cancellation |
| cancelled | Member cancelled their enrollment |

### Enrollment Flow

1. Member views available clinics at their home facility
2. System shows sessions with enrollment_status "open" and start_time in future
3. Member enrolls in a session
4. If enrolled_count < max_participants: status = "enrolled"
5. If enrolled_count >= max_participants: status = "waitlisted"
6. Enrollment counts toward member's max_member_reservations limit

### Waitlist Promotion

When an enrolled member cancels:

1. First waitlisted member (by enrollment time) is auto-promoted to "enrolled"
2. Promoted member moves from waitlist to confirmed roster
3. If cancellation drops enrolled count below min_participants, staff notification is created

### Staff Management

Staff can manage clinics through the admin interface:

| Operation | Description |
|-----------|-------------|
| Create clinic type | Define new clinic template for facility |
| List clinic types | View all clinic templates at facility |
| Create session | Schedule a clinic instance with pro and times |
| Update session | Modify session details (pro, times, enrollment status) |
| Cancel session | Remove session (deletes session record) |
| View roster | See enrolled and waitlisted members |

### Clinic Constraints

| Constraint | Rule |
|------------|------|
| Facility scope | Clinic types and sessions belong to one facility |
| Pro validation | Pro must have role "pro" and be active at the facility |
| Clinic type status | Only "active" types shown to members |
| Future sessions only | Members can only enroll in sessions with start_time in future |
| Enrollment status | Must be "open" to accept new enrollments |
| Advance notice | Respects facility's lesson_min_notice_hours setting |
| Reservation limit | Enrollments count toward max_member_reservations |

### Staff Notifications

Staff are notified when enrollment drops below minimum:

- Notification type: `clinic_enrollment_below_minimum`
- Message includes clinic name, date/time, and current vs minimum enrollment
- Created when an enrolled member cancels and remaining count < min_participants

---

## Waitlist System

When courts are fully booked, members can join a waitlist to be notified if a slot becomes available. The waitlist system handles automatic notifications when reservations are cancelled.

### Waitlist Entry

Members join a waitlist for a specific time slot:

| Field | Description |
|-------|-------------|
| facility_id | Facility where slot is desired |
| target_date | Date of the desired slot |
| target_start_time | Start time (HH:MM:SS format) |
| target_end_time | End time (HH:MM:SS format) |
| target_court_id | Optional: Specific court preference (NULL = any court) |
| position | Queue position (auto-assigned, incrementing per slot) |
| status | pending, notified, expired, fulfilled |

### Waitlist Configuration

Each facility can configure waitlist behavior:

| Setting | Default | Description |
|---------|---------|-------------|
| max_waitlist_size | 0 (unlimited) | Maximum entries per slot |
| notification_mode | broadcast | How to notify waitlisted members |
| offer_expiry_minutes | 30 | How long a slot offer remains valid |
| notification_window_minutes | 0 (unlimited) | Only notify if slot starts within this window |

### Notification Modes

| Mode | Behavior |
|------|----------|
| broadcast | All waitlisted members for the slot are notified simultaneously |
| sequential | Members are notified one at a time in queue order; next member notified only after previous offer expires |

### Waitlist Offers

When a reservation is cancelled, the system creates offers for waitlisted members:

1. Matches waitlist entries by facility, date, and time range
2. Includes entries targeting any court (NULL) and the specific cancelled court(s)
3. Respects notification_window_minutes (if configured, only notifies if slot starts within window)
4. Creates `waitlist_offers` records with expiration time
5. Updates waitlist entry status to 'notified'

### Offer Lifecycle

| Status | Description |
|--------|-------------|
| pending | Offer created, awaiting member action |
| accepted | Member booked the slot |
| expired | Offer timed out without action |

For sequential mode, when an offer expires:
1. Current waitlist entry marked as 'expired'
2. Next member in queue receives new offer
3. Process continues until slot is filled or waitlist exhausted

### Scheduled Jobs

The waitlist system runs two scheduled jobs:

| Job | Interval | Purpose |
|-----|----------|---------|
| Expire Offers | 1 minute | Expires pending offers past their expiry time, advances sequential queues |
| Cleanup Past | 1 hour | Removes waitlist entries for past time slots |

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/waitlist` | List member's waitlist entries at facility |
| POST | `/api/v1/waitlist` | Join waitlist for a slot |
| DELETE | `/api/v1/waitlist/{id}` | Leave waitlist |
| POST | `/api/v1/waitlist/config` | Update facility waitlist configuration (staff) |
| GET | `/api/v1/staff/waitlist` | View all waitlist entries for facility (staff) |
| GET | `/member/waitlist` | Member portal waitlist view |

### Member Portal Integration

Members can view their waitlist entries from the portal. The booking form offers waitlist signup when no slots are available for the selected date.

### Constraints

| Rule | Enforcement |
|------|-------------|
| One entry per slot | Cannot join same slot twice (unique constraint) |
| Future slots only | Cannot waitlist for past times |
| Same-day times | Start and end must be on the same date |
| End after start | End time must be after start time |
| Facility access | Member must have access to the facility |

---

## Visit Packs

Visit packs allow facilities to sell prepaid visit bundles to guests and low-tier members. A member purchases a pack (e.g., "10-Visit Pack") and redeems visits when booking courts.

### Visit Pack Types

Each facility defines visit pack types available for purchase:

| Field | Description |
|-------|-------------|
| name | Display name (e.g., "10-Visit Pack") |
| price_cents | Price in cents |
| visit_count | Number of visits included |
| valid_days | Days until expiration from purchase |
| status | active or inactive |

Staff create and manage pack types through the `/admin/visit-packs` admin page.

### Visit Packs

When a pack is sold, a visit_pack record is created:

| Field | Description |
|-------|-------------|
| pack_type_id | Reference to the pack type |
| user_id | Owner of the pack |
| purchase_date | When the pack was purchased |
| expires_at | Calculated from purchase_date + valid_days |
| visits_remaining | Decrements on each redemption |
| status | active, expired, or depleted |

### Redemption Flow

Visit packs are redeemed when guests/low-tier members book courts:

1. Member with membership_level <= 1 opens booking form
2. System loads member's active visit packs (not expired, visits_remaining > 0)
3. Booking form displays dropdown to select a pack
4. On booking submission, a visit is decremented from the selected pack
5. A redemption record is created linking pack, facility, and reservation

### Cross-Facility Redemption

Organizations can enable `cross_facility_visit_packs` to allow packs purchased at one facility to be redeemed at any facility within the organization. When disabled (default), packs can only be redeemed at the facility where they were purchased.

### Visit Pack Constraints

| Constraint | Rule |
|------------|------|
| Eligibility | Only membership_level <= 1 can use visit packs |
| Pack Status | Must be 'active' (not expired or depleted) |
| Expiration | Checked at redemption time against current timestamp |
| Visits | Must have visits_remaining > 0 |
| Facility | Must match pack's facility or cross_facility_visit_packs enabled |
| Pack Type Limit | Maximum 1000 pack types per facility |

### Admin Operations

| Operation | Endpoint | Notes |
|-----------|----------|-------|
| List pack types | GET `/api/v1/visit-pack-types` | Requires facility_id |
| Create pack type | POST `/api/v1/visit-pack-types` | Staff only |
| Update pack type | PUT `/api/v1/visit-pack-types/{id}` | Staff only |
| Deactivate pack type | DELETE `/api/v1/visit-pack-types/{id}` | Soft deactivate |
| Sell pack | POST `/api/v1/visit-packs` | Staff creates pack for user |
| List user packs | GET `/api/v1/users/{id}/visit-packs` | Staff or self |

---

## Lesson Packages

Lesson packages allow facilities to sell prepaid lesson bundles to members. A member purchases a package (e.g., "5-Lesson Pack") and lessons are automatically redeemed when booking PRO_SESSION reservations.

### Lesson Package Types

Each facility defines lesson package types available for purchase:

| Field | Description |
|-------|-------------|
| name | Display name (e.g., "5-Lesson Pack") |
| price_cents | Price in cents |
| lesson_count | Number of lessons included |
| valid_days | Days until expiration from purchase |
| status | active or inactive |

Staff create and manage package types through the `/admin/lesson-packages` admin page.

### Lesson Packages

When a package is sold, a lesson_package record is created:

| Field | Description |
|-------|-------------|
| pack_type_id | Reference to the package type |
| user_id | Owner of the package |
| purchase_date | When the package was purchased |
| expires_at | Calculated from purchase_date + valid_days |
| lessons_remaining | Decrements on each redemption |
| status | active, expired, or depleted |

### Redemption Flow

Lesson packages are automatically redeemed when members book lessons:

1. Member books a lesson (PRO_SESSION reservation)
2. System checks for eligible packages (active, not expired, lessons_remaining > 0)
3. If eligible package exists, a lesson is decremented from the oldest eligible package
4. A redemption record is created linking package, facility, and reservation
5. Staff-booked lessons also trigger automatic redemption

### Cancellation Handling

When a lesson reservation is cancelled:

1. System looks up any lesson_package_redemptions linked to the reservation
2. For each redemption, the lesson is restored to the package (if not expired and below max)
3. Redemption records are deleted
4. Expired packages or packages already at max lessons skip restoration

### Lesson Package Constraints

| Constraint | Rule |
|------------|------|
| Facility scope | Packages are scoped to the facility where purchased |
| Package Status | Must be 'active' (not expired or depleted) |
| Expiration | Checked at booking time against current timestamp |
| Lessons | Must have lessons_remaining > 0 |
| Auto-redemption | Happens automatically during lesson booking |
| Package Type Limit | Maximum 1000 package types per facility |

### Admin Operations

| Operation | Endpoint | Notes |
|-----------|----------|-------|
| List package types | GET `/api/v1/lesson-package-types` | Requires facility_id |
| Create package type | POST `/api/v1/lesson-package-types` | Staff only |
| Update package type | PUT `/api/v1/lesson-package-types/{id}` | Staff only |
| Deactivate package type | DELETE `/api/v1/lesson-package-types/{id}` | Soft deactivate |
| Sell package | POST `/api/v1/lesson-packages` | Staff creates package for user |
| List user packages | GET `/api/v1/users/{id}/lesson-packages` | Staff or self |

---

## Reporting Dashboard

Staff can view facility metrics through `/admin/dashboard`. The dashboard displays utilization and activity metrics for a configurable date range.

### Dashboard Metrics

| Metric | Description |
|--------|-------------|
| Court Utilization Rate | Booked court hours / available court hours |
| Scheduled Reservations | Future reservations in date range |
| Bookings by Type | Reservation counts grouped by type (Court, Lesson, etc.) |
| Cancellation Rate | Cancelled / total reservations, with refund percentage |
| Check-in Count | Total check-ins in date range |

### Date Range Options

| Preset | Description |
|--------|-------------|
| today | Current day only |
| last_7_days | Past 7 days including today |
| last_30_days | Past 30 days including today (default) |
| this_month | First of current month to today |
| this_year | January 1 to today |
| custom | Explicit start_date and end_date |

Custom date ranges use `YYYY-MM-DD` format. The `date_range` parameter accepts either a preset name or a `YYYY-MM-DD to YYYY-MM-DD` format.

### Facility Selection

- Staff with `home_facility_id` see only their facility
- Org-level staff (no home facility) see a facility selector
- Org-level staff can select "All Facilities" (facility_id=0) for aggregate view

### Authorization

Dashboard access requires staff authentication. Staff can only view metrics for their assigned facility, or all facilities if they have no home facility assignment.

---

## League Management

Leagues enable facilities to organize competitive team play over a season. A league defines a time period, team roster rules, and generates a match schedule for participating teams.

### League Lifecycle

| Status | Description |
|--------|-------------|
| draft | Initial state, configuring league settings |
| registration | Teams can register and rosters are open |
| active | Season in progress, rosters locked (if configured) |
| completed | Season ended, standings finalized |

### League Configuration

| Field | Description |
|-------|-------------|
| name | League display name |
| format | Match format (e.g., "singles", "doubles") |
| start_date | Season start date |
| end_date | Season end date |
| division_config | JSON configuration for divisions |
| min_team_size | Minimum players per team roster |
| max_team_size | Maximum players per team roster |
| roster_lock_date | Optional date after which rosters cannot change |

### Teams

Teams belong to a single league. Each team has a captain (user_id) responsible for roster management.

| Field | Description |
|-------|-------------|
| name | Team name (unique within league) |
| captain_user_id | User responsible for team |
| status | active or inactive |

Team names must be unique within a league. Captain assignment can be changed but requires the new captain to exist as a user.

### Team Members

| Field | Description |
|-------|-------------|
| league_team_id | Team the member belongs to |
| user_id | The player |
| is_free_agent | Whether member was assigned as a free agent |

Members can belong to one team per league. The captain does not automatically become a team member—they must be explicitly added.

### Free Agents

Players can register as free agents for a league without joining a team. Staff can assign free agents to teams needing additional players.

| Operation | Description |
|-----------|-------------|
| List free agents | View unassigned players in a league |
| Assign to team | Move free agent to a team roster |

### Roster Lock

When `roster_lock_date` is set and that date passes (evaluated in the facility's timezone), roster modifications are blocked:

- Cannot add team members
- Cannot remove team members
- Cannot assign free agents to teams

### Schedule Generation

The scheduler generates round-robin matches for all teams:

| Feature | Description |
|---------|-------------|
| Round-robin | Each team plays every other team |
| Home/away | Alternating home and away designations |
| Bye handling | Odd team counts get bye weeks |
| Regeneration | Can regenerate schedule (clears existing matches) |

Matches are created with status "scheduled" and include home_team_id, away_team_id, and scheduled_date.

### Match Results

| Field | Description |
|-------|-------------|
| home_score | Points scored by home team |
| away_score | Points scored by away team |
| status | scheduled, in_progress, completed |

Results can only be recorded for matches in "scheduled" or "in_progress" status. Recording a result sets status to "completed".

### Standings

Standings are calculated from completed matches:

| Metric | Description |
|--------|-------------|
| Wins | Matches where team scored higher |
| Losses | Matches where team scored lower |
| Points For | Total points scored |
| Points Against | Total points conceded |
| Point Differential | Points for minus points against |

Teams are ranked by: wins (desc), point differential (desc), points for (desc).

### Standings Export

Standings can be exported to CSV format for offline analysis or distribution. The export includes rank, team name, matches played, wins, losses, and point statistics.

### League Constraints

| Constraint | Rule |
|------------|------|
| Facility scope | Leagues belong to one facility |
| Unique team names | Team names unique within a league |
| Captain validation | Captain must exist as a user |
| Team size limits | Roster respects min/max team size |
| Single team per league | A user can only be on one team per league |
| Roster lock enforcement | Roster changes blocked after lock date |
| Match result validation | Scores must be non-negative |

### League Operations

| Operation | Endpoint | Notes |
|-----------|----------|-------|
| List leagues | GET `/api/v1/leagues` | Requires facility_id |
| Create league | POST `/api/v1/leagues` | Staff only |
| Get league | GET `/api/v1/leagues/{id}` | League detail |
| Update league | PUT `/api/v1/leagues/{id}` | Staff only |
| Delete league | DELETE `/api/v1/leagues/{id}` | Staff only |
| List teams | GET `/api/v1/leagues/{id}/teams` | Teams in league |
| Create team | POST `/api/v1/leagues/{id}/teams` | Staff only |
| Get team | GET `/api/v1/leagues/{id}/teams/{team_id}` | Team with members |
| Update team | PUT `/api/v1/leagues/{id}/teams/{team_id}` | Staff only |
| Add member | POST `/api/v1/leagues/{id}/teams/{team_id}/members` | Respects roster lock |
| Remove member | DELETE `/api/v1/leagues/{id}/teams/{team_id}/members/{user_id}` | Respects roster lock |
| List free agents | GET `/api/v1/leagues/{id}/free-agents` | Unassigned players |
| Assign free agent | POST `/api/v1/leagues/{id}/free-agents/{user_id}/assign` | Respects roster lock |
| Generate schedule | POST `/api/v1/leagues/{id}/schedule/generate` | Creates matches |
| Regenerate schedule | POST `/api/v1/leagues/{id}/schedule/regenerate` | Clears and recreates |
| Record result | PUT `/api/v1/leagues/{id}/matches/{match_id}/result` | Updates match |
| Get standings | GET `/api/v1/leagues/{id}/standings` | Calculated standings |
| Export standings | GET `/api/v1/leagues/{id}/standings/export` | CSV download |

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

### System Themes

System themes are pre-defined color schemes available to all facilities. They are embedded in the application binary and seeded to the database via migration.

**Definition File:** `assets/themes`

The themes file uses a simple line-based format:
```
Theme Name
#HEXCOLOR  (primary)
#HEXCOLOR  (secondary)
#HEXCOLOR  (tertiary)
#HEXCOLOR  (accent)
#HEXCOLOR  (highlight)
```

One theme can be marked as default by appending ` DEFAULT` to its name (e.g., `Simple DEFAULT`). The default theme is used when a facility has no active theme selected.

**Seeding:** Migration `000045_seed_system_themes.up.sql` inserts all system themes with `facility_id = NULL` and `is_system = 1`. Uses `ON CONFLICT` to make re-running idempotent - existing themes are updated, not duplicated.

**Parser:** `internal/db/themes_parser.go` reads the embedded file, validates each theme against the model validation rules, and exposes `DefaultSystemThemeName()` for runtime access.

**Current themes:** 34 pre-defined themes including Simple (default), Metal, Vintage, Cool, Cosmic, Artsy, Elegance, Futuristic, Innovative, Dynamic, and others.

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

### Dark Mode

The UI supports light and dark color schemes, toggled via the sun/moon icon in the top navigation.

**Behavior:**
- Click the toggle: page switches between light and dark themes
- Preference persists across page refreshes via localStorage
- Initial load respects system preference (`prefers-color-scheme`) when no explicit preference is set
- Toggle icon shows moon in light mode, sun in dark mode

**Implementation:**
- Tailwind configured for class-based dark mode (`darkMode: 'class'`)
- JavaScript adds/removes `dark` class on `<html>` element
- CSS custom properties transform automatically via `themes.css`:
  - Light: `--background`, `--foreground`, `--muted`, `--muted-foreground`, `--border`
  - Dark: inverted values applied when `.dark` class present
- Templates use semantic token classes (`bg-background`, `text-foreground`, `border-border`, `bg-muted`, `text-muted-foreground`) instead of hardcoded Tailwind colors
- No `dark:` variant additions required in templates - theme colors invert via CSS custom properties

**Semantic Tokens:**

| Token | Light Value | Dark Value | Usage |
|-------|-------------|------------|-------|
| `background` | white | gray-900 | Page and card backgrounds |
| `foreground` | gray-900 | gray-50 | Primary text |
| `muted` | gray-50 | gray-800 | Subtle backgrounds |
| `muted-foreground` | gray-500 | gray-400 | Secondary text |
| `border` | gray-200 | gray-700 | Borders and dividers |

### Theme Operations

**Create**: Define colors for a facility. Name must be unique within that facility's scope. Name validation: alphanumeric, spaces, hyphens, parentheses.

**Clone**: Copy an existing theme (system or facility) as a starting point. Useful for taking "Classic Court" and tweaking one color.

**Edit**: Modify colors. System re-validates accessibility on save.

**Delete**: Remove a theme. Cannot delete if it's the active theme for any facility.

**Set Active**: Assign a theme to a facility. Takes effect immediately on next page load.

---

## Operating Hours

Facilities define when they're open each day of the week. These hours constrain when courts can be reserved and when open play sessions can run.

### Admin Interface

Staff access the operating hours page at `/admin/operating-hours?facility_id=X`. The interface displays a 7-day weekly grid where each day shows:

- **Day label**: Sunday through Saturday (0-6 internally)
- **Closed toggle**: Checkbox to mark the facility as closed
- **Time inputs**: Opens at and closes at fields with AM/PM format

### Time Format

Times are displayed and accepted in 12-hour AM/PM format (e.g., "8:00 AM", "9:30 PM"). Internally stored as 24-hour HH:MM strings. The UI provides a datalist with 30-minute increments for quick selection.

### Default Hours

New facilities or days with no hours configured display default hours of 8:00 AM - 9:00 PM. When a closed day is toggled back to open, these defaults pre-populate if no previous hours existed.

### Saving Behavior

Changes save automatically via HTMX PUT requests with a 300ms debounce. Each day saves independently to `/api/v1/operating-hours/{day_of_week}`. Visual feedback ("Saved" indicator) confirms successful updates.

### Validation Rules

| Rule | Behavior |
|------|----------|
| opens_at before closes_at | Required - rejects invalid order |
| Time format | Accepts HH:MM or H:MM AM/PM |
| Closed day | Deletes hours from database |

### Authorization

Only authenticated staff with facility access can view or edit operating hours. Uses the same facility-scoped authorization as other admin pages.

### Booking Configuration

The operating hours page includes a booking configuration section for facility-wide member booking settings:

| Setting | Default | Description |
|---------|---------|-------------|
| max_advance_booking_days | 7 | How far in advance members can book courts |
| max_member_reservations | 30 | Maximum active future reservations per member |
| lesson_min_notice_hours | 24 | Minimum hours in advance lessons must be booked |

Settings save via POST to `/api/v1/facility-settings`. All values must be positive integers.

---

## Tier Booking Windows

Facilities can configure different advance booking windows based on membership level, allowing premium members to book courts further in advance than standard members.

### Configuration

Tier booking is disabled by default. When enabled via the toggle on `/admin/booking-windows`, each membership tier can have its own `max_advance_days` setting.

| Field | Description |
|-------|-------------|
| tier_booking_enabled | Facility-level toggle to enable tier-based booking windows |
| membership_level | Tier level (0=Unverified Guest, 1=Verified Guest, 2=Member, 3=Member+) |
| max_advance_days | Maximum days in advance this tier can book (1-364) |

### Behavior

When tier booking is enabled:
1. System looks up the member's membership_level
2. Finds matching tier_booking_window for facility + membership_level
3. Uses that tier's max_advance_days for booking validation
4. Falls back to facility's default max_advance_booking_days if no tier window defined

When tier booking is disabled, all members use the facility's default max_advance_booking_days regardless of membership level.

### Admin Interface

Staff access the booking windows page at `/admin/booking-windows?facility_id=X`. The interface displays:

- Toggle to enable/disable tier booking for the facility
- Grid showing all four tiers (0-3) with their current advance booking days
- Each tier shows label, current setting, and default indicator
- Inline editing with auto-save

### Database Schema

| Table | Description |
|-------|-------------|
| member_tier_booking_windows | Per-tier booking window settings |

| Column | Table | Description |
|--------|-------|-------------|
| tier_booking_enabled | facilities | Toggle for tier-based booking windows |
| membership_level | member_tier_booking_windows | Tier level (0-3) |
| max_advance_days | member_tier_booking_windows | Days in advance this tier can book |

### Constraints

| Constraint | Rule |
|------------|------|
| Facility scope | Windows are scoped to individual facilities |
| Tier range | membership_level must be 0-3 |
| Days range | max_advance_days must be 1-364 |
| Unique per facility | One window per facility + membership_level combination |

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/booking-windows` | Booking windows admin page |
| GET | `/api/v1/booking-windows` | List booking windows for facility |
| POST | `/api/v1/booking-windows/{tier}` | Create tier window |
| PUT | `/api/v1/booking-windows/{tier}` | Update tier window |
| DELETE | `/api/v1/booking-windows/{tier}` | Delete tier window |
| POST | `/api/v1/tier-booking/toggle` | Enable/disable tier booking |
| POST | `/api/v1/tier-booking/windows` | Bulk save tier windows |

---

## Authentication and Access

### Session Management

Two cookie-based session mechanisms operate in parallel:

| Cookie | Purpose | Used By |
|--------|---------|---------|
| `pickleicious_session` | In-memory token-based session | Staff local login |
| `pickleicious_auth` | HMAC-signed JSON payload | Member login (dev bypass or Cognito) |

Session characteristics:
- 8-hour TTL for both session types
- HttpOnly, SameSite=Lax cookies
- Secure flag enabled in non-development environments
- In-memory session store with 15-minute cleanup interval
- Single active session per user (previous sessions cleared on new login)

### Logout

The logout endpoint (`POST /api/v1/auth/logout`) handles both staff and member sessions:

1. Clears `pickleicious_auth` cookie (member sessions)
2. Clears `pickleicious_session` cookie and removes in-memory token (staff sessions)
3. Returns `HX-Redirect` header based on session type:
   - Member sessions redirect to `/member/login`
   - Staff sessions redirect to `/login`

Logout buttons appear in:
- Staff slide-out menu (User Section) for authenticated staff
- Member portal navigation header for authenticated members

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

### Password Reset Flow

Staff and members can reset passwords via `/api/v1/auth/reset-password` (initiate) and `/api/v1/auth/confirm-reset-password` (confirm):

**Initiation (`POST /api/v1/auth/reset-password`):**
1. User submits email or phone identifier
2. System validates user exists before burning OTP quota
3. Cognito sends reset code via email/SMS

**Confirmation (`POST /api/v1/auth/confirm-reset-password`):**
1. Validate password complexity (min 8 chars, uppercase, symbol)
2. Record OTP verify attempt (rate limited)
3. Confirm reset with Cognito
4. If user has `local_auth_enabled=true`, sync new password hash to local DB
5. Clear rate limit attempts on success

**Password Requirements:**
- Minimum 8 characters
- At least one uppercase letter
- At least one symbol (punctuation or symbol character)

**Dual-Auth Sync:**
Users with both Cognito and local auth enabled have their password hash updated in both systems. If the local update fails after Cognito succeeds, an error is returned instructing the user to contact support (Cognito password is already changed).

### Dev Mode Bypass

When `config.App.Environment == "development"`:
- Credentials `dev@test.local` / `devpass` bypass database lookup
- Creates authenticated session with `IsStaff=true`
- Optional `facility_id` parameter sets `HomeFacilityID`
- Warning logged: "Dev mode staff login bypass used"
- Completely disabled in non-development environments

### Member Authentication

Members authenticate via AWS Cognito OTP (email or SMS):

| Method | Flow |
|--------|------|
| Email OTP | Cognito sends 6-digit code to member's email |
| SMS OTP | Cognito sends 6-digit code to member's phone |
| Dev bypass | Code `123456` with session `dev-session` in development mode |

Authentication is optional for walk-in check-ins. Staff can check someone in without the member having an account - useful for first-time guests or those who forgot their phone.

**Flow:**
1. Member enters email OR phone number on `/member/login`
2. System detects identifier type and calls Cognito `InitiateAuth` with `USER_AUTH` flow
3. Cognito sends 6-digit code via email or SMS based on identifier
4. Member enters code, system calls `RespondToAuthChallenge` with appropriate challenge type
5. On success, `pickleicious_auth` cookie is set with member info

**Phone Number Format:**
- Accepted: `(555) 123-4567`, `555-123-4567`, `5551234567`, `+15551234567`
- Backend normalizes to E.164 format (`+1XXXXXXXXXX`) before sending to Cognito
- 10-digit numbers assumed to be US (+1)

### Cognito Configuration

Single shared AWS Cognito User Pool configured via environment variables:

| Variable | Description |
|----------|-------------|
| `COGNITO_POOL_ID` | User Pool ID (e.g., `us-east-2_XXXXXXXXX`) |
| `COGNITO_CLIENT_ID` | App Client ID (public, no secret) |

Configuration loaded from `.env` file (gitignored). See `.env.example` for template.

**User Pool Setup:**
- Alias attributes: email, phone_number (users can sign in with either)
- Auto-verified attributes: email, phone_number
- Allowed first auth factors: EMAIL_OTP, SMS_OTP, PASSWORD
- SMS via SNS with IAM role `CognitoSMSRole`
- App client: No secret (public client), explicit auth flows: USER_AUTH, REFRESH_TOKEN

**Member Sync:**
When a new member is created via the admin UI, a corresponding Cognito user is automatically created with email and phone (if provided), both marked as verified. No welcome email/SMS is sent.

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

### Staff Management Authorization

Staff management operations require the requester to have admin or manager role. Authorization is further scoped:

| Requester | Can Manage |
|-----------|------------|
| Corporate admin (HomeFacilityID=NULL) | All staff at any facility |
| Facility admin/manager | Only staff at their facility |
| Desk or pro staff | No staff management access |

Role assignment restrictions:
- Facility-scoped managers cannot assign corporate admin roles
- Cannot manage staff at other facilities

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
| GET | `/api/v1/auth/check-staff` | Check if identifier belongs to staff |
| POST | `/api/v1/auth/send-code` | Send Cognito OTP code |
| POST | `/api/v1/auth/verify-code` | Verify Cognito OTP code |
| POST | `/api/v1/auth/resend-code` | Resend OTP code |
| POST | `/api/v1/auth/staff-login` | Staff local password login |
| POST | `/api/v1/auth/reset-password` | Password reset flow |
| POST | `/api/v1/auth/standard-login` | Standard member login |
| POST | `/api/v1/auth/logout` | Logout (clears session, redirects to login) |

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
| GET | `/api/v1/members/{id}/visits` | Member visit history (last 10 visits) |
| GET | `/api/v1/members/photo/{id}` | Member photo |
| POST | `/api/v1/members/restore` | Restore/create decision |

### Member Portal

| Method | Path | Description |
|--------|------|-------------|
| GET | `/member` | Member portal page |
| GET | `/member/reservations` | Member reservations list (HTMX partial) |
| POST | `/member/reservations` | Create member booking |
| DELETE | `/member/reservations/{id}` | Cancel member reservation |
| GET | `/member/booking/new` | Booking form modal |
| GET | `/member/booking/slots` | Reload available slots for selected date |
| GET | `/member/lessons/new` | Lesson booking form |
| GET | `/member/lessons/slots` | Reload lesson slots for selected pro/date |
| GET | `/member/lessons/pros` | List pros available for lessons |
| GET | `/member/lessons/pros/{id}/slots` | Get available lesson slots for a pro |
| POST | `/member/lessons` | Create lesson booking |
| GET | `/member/openplay` | List upcoming open play sessions |
| POST | `/member/openplay/{id}` | Sign up for open play session |
| DELETE | `/member/openplay/{id}` | Cancel open play signup |
| GET | `/member/clinics` | List available clinics at home facility |
| POST | `/member/clinics/{id}/enroll` | Enroll in clinic session |
| DELETE | `/member/clinics/{id}/enroll` | Cancel clinic enrollment |
| GET | `/api/v1/member/reservations/widget` | Reservations widget data |

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

### Leagues

| Method | Path | Description |
|--------|------|-------------|
| GET | `/leagues` | Leagues page |
| GET | `/api/v1/leagues` | List leagues by facility |
| POST | `/api/v1/leagues` | Create league |
| GET | `/api/v1/leagues/{id}` | League detail |
| PUT | `/api/v1/leagues/{id}` | Update league |
| DELETE | `/api/v1/leagues/{id}` | Delete league |
| GET | `/api/v1/leagues/{id}/teams` | List teams in league |
| POST | `/api/v1/leagues/{id}/teams` | Create team |
| GET | `/api/v1/leagues/{id}/teams/{team_id}` | Team detail with members |
| PUT | `/api/v1/leagues/{id}/teams/{team_id}` | Update team |
| POST | `/api/v1/leagues/{id}/teams/{team_id}/members` | Add team member |
| DELETE | `/api/v1/leagues/{id}/teams/{team_id}/members/{user_id}` | Remove team member |
| GET | `/api/v1/leagues/{id}/free-agents` | List free agents |
| POST | `/api/v1/leagues/{id}/free-agents/{user_id}/assign` | Assign free agent to team |
| POST | `/api/v1/leagues/{id}/schedule/generate` | Generate match schedule |
| POST | `/api/v1/leagues/{id}/schedule/regenerate` | Regenerate schedule |
| PUT | `/api/v1/leagues/{id}/matches/{match_id}/result` | Record match result |
| GET | `/api/v1/leagues/{id}/standings` | Get league standings |
| GET | `/api/v1/leagues/{id}/standings/export` | Export standings CSV |

### Clinics

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/clinic-types` | List clinic types by facility |
| POST | `/api/v1/clinic-types` | Create clinic type |
| GET | `/api/v1/clinics` | List clinic sessions by facility |
| POST | `/api/v1/clinics` | Create clinic session |
| PUT | `/api/v1/clinics/{id}` | Update clinic session |
| DELETE | `/api/v1/clinics/{id}` | Cancel clinic session |
| GET | `/api/v1/clinics/{id}/roster` | View clinic enrollment roster |

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

### Operating Hours

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/operating-hours` | Operating hours admin page |
| PUT | `/api/v1/operating-hours/{day_of_week}` | Update hours for a day (0=Sunday through 6=Saturday) |
| POST | `/api/v1/facility-settings` | Update facility booking configuration |

### Cancellation Policy

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/cancellation-policy` | Cancellation policy admin page |
| GET | `/api/v1/cancellation-policy/tiers` | List policy tiers (supports reservation_type_id filter) |
| POST | `/api/v1/cancellation-policy/tiers` | Create policy tier |
| PUT | `/api/v1/cancellation-policy/tiers/{id}` | Update policy tier |
| DELETE | `/api/v1/cancellation-policy/tiers/{id}` | Delete policy tier |

### Waitlist

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/waitlist` | List member's waitlist entries at facility |
| POST | `/api/v1/waitlist` | Join waitlist for a slot |
| DELETE | `/api/v1/waitlist/{id}` | Leave waitlist |
| POST | `/api/v1/waitlist/config` | Update facility waitlist configuration (staff) |
| GET | `/api/v1/staff/waitlist` | View all waitlist entries for facility (staff) |
| GET | `/member/waitlist` | Member portal waitlist entries |

### Visit Packs

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/visit-packs` | Visit pack types admin page |
| GET | `/api/v1/visit-pack-types` | List visit pack types for facility |
| POST | `/api/v1/visit-pack-types` | Create visit pack type |
| PUT | `/api/v1/visit-pack-types/{id}` | Update visit pack type |
| DELETE | `/api/v1/visit-pack-types/{id}` | Deactivate visit pack type |
| POST | `/api/v1/visit-packs` | Sell visit pack to user (staff only) |
| GET | `/api/v1/users/{id}/visit-packs` | List user's active visit packs |

### Lesson Packages

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/lesson-packages` | Lesson package types admin page |
| GET | `/api/v1/lesson-package-types` | List lesson package types for facility |
| POST | `/api/v1/lesson-package-types` | Create lesson package type |
| PUT | `/api/v1/lesson-package-types/{id}` | Update lesson package type |
| DELETE | `/api/v1/lesson-package-types/{id}` | Deactivate lesson package type |
| POST | `/api/v1/lesson-packages` | Sell lesson package to user (staff only) |
| GET | `/api/v1/users/{id}/lesson-packages` | List user's active lesson packages |

### Dashboard

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/dashboard` | Reporting dashboard page |
| GET | `/api/v1/dashboard/metrics` | Dashboard metrics partial (HTMX) |

### Check-in

| Method | Path | Description |
|--------|------|-------------|
| GET | `/checkin` | Check-in page with search and arrivals list |
| GET | `/api/v1/checkin/search` | Search members for check-in |
| POST | `/api/v1/checkin` | Record member check-in |
| POST | `/api/v1/checkin/activity` | Update visit activity type after check-in |

### Staff

| Method | Path | Description |
|--------|------|-------------|
| GET | `/staff` | Staff management page |
| GET | `/staff/notifications/{id}` | Staff notification detail page |
| GET | `/api/v1/staff` | List staff (filterable by facility_id, role, search) |
| GET | `/api/v1/staff/new` | New staff form |
| GET | `/api/v1/staff/{id}` | Staff detail |
| GET | `/api/v1/staff/{id}/edit` | Edit staff form |
| POST | `/api/v1/staff` | Create staff |
| PUT | `/api/v1/staff/{id}` | Update staff |
| POST | `/api/v1/staff/{id}/deactivate` | Deactivate staff (soft delete via user status) |
| GET | `/staff/unavailability` | Pro unavailability page (pros only) |
| POST | `/staff/unavailability` | Create unavailability block |
| DELETE | `/staff/unavailability/{id}` | Delete unavailability block |
| GET | `/api/v1/staff/lessons/booking/new` | Staff lesson booking form |
| GET | `/api/v1/staff/lessons/booking/slots` | Available lesson slots for pro/date |
| GET | `/api/v1/staff/lessons/schedule` | Pro's upcoming lesson schedule |
| POST | `/api/v1/staff/lessons/booking` | Create lesson (form submission) |
| POST | `/api/v1/staff/lessons` | Create lesson (JSON API) |
| GET | `/api/v1/staff/members/search` | Search members scoped to facility |

### Notifications

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/notifications` | List notifications panel HTML |
| GET | `/api/v1/notifications/count` | Unread count badge HTML |
| GET | `/api/v1/notifications/close` | Close panel (returns empty string) |
| PUT | `/api/v1/notifications/{id}/read` | Mark notification as read |

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

- Fixed top navigation with menu toggle, search, dark mode toggle (sun/moon icon), notifications
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
| HX-Trigger | Server-sent events (e.g., calendar-refresh, refreshMemberReservations) |
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
| `db:seed` | Reset database and populate with test data | db:reset |
| `db:snapshot` | Save database snapshot (usage: `task db:snapshot -- name`) | - |
| `db:restore` | Restore database from snapshot (usage: `task db:restore -- name`) | - |
| `db:snapshots` | List available database snapshots | - |

### Test Tasks

| Task | Description | Dependencies |
|------|-------------|--------------|
| `test` | Run all Go tests (unit, smoke, integration) | - |
| `test:unit` | Run unit tests (exclude smoke/integration tags) | generate |
| `test:smoke` | Run smoke tests | generate |
| `test:integration` | Run integration tests | generate |

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
│   │   ├── cancellationpolicy/ # Cancellation policy management
│   │   ├── checkin/         # Front desk check-in
│   │   ├── courts/          # Court/calendar
│   │   ├── htmx/            # HTMX helpers
│   │   ├── member/          # Member portal handlers
│   │   ├── members/         # Member CRUD (staff-facing)
│   │   ├── nav/             # Navigation
│   │   ├── notifications/   # Staff notifications
│   │   ├── openplay/        # Open play rules
│   │   ├── operatinghours/  # Operating hours management
│   │   ├── reservations/    # Reservation CRUD
│   │   ├── staff/           # Staff management
│   │   ├── themes/          # Theme management
│   │   └── waitlist/        # Waitlist management
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
│   │       ├── cancellationpolicy/ # Cancellation policy UI
│   │       ├── checkin/         # Check-in page and cards
│   │       ├── courts/          # Calendar components
│   │       ├── member/          # Member portal and booking UI
│   │       ├── members/         # Member list and visit history
│   │       ├── notifications/   # Notification panel and badge
│   │       ├── operatinghours/  # Operating hours UI
│   │       ├── reservations/    # Booking form components
│   │       ├── staff/           # Staff management UI
│   │       └── waitlist/        # Waitlist UI components
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

## Member Portal

Members with verified accounts (membership_level >= 1) can access a self-service portal at `/member`. The portal requires member authentication via the `RequireMemberSession` middleware.

### Portal Features

| Feature | Description |
|---------|-------------|
| Reservations List | View upcoming and past reservations at home facility |
| Court Booking | Book available courts at home facility |
| Lesson Booking | Book lessons with teaching pros at home facility |
| Open Play Signup | Sign up for and cancel open play sessions at home facility |
| Reservation Cancellation | Cancel own upcoming reservations (courts and lessons) |

### Member Booking

Members can book courts through a booking form accessible from the portal:

- **Date Selection**: Three-dropdown date picker (year, month, day) for selecting booking date
- **Slot Selection**: Shows available 1-hour time slots based on facility operating hours
- **Court Selection**: Lists active courts at the member's home facility
- **Availability Check**: Validates court availability before creating reservation
- **Automatic Participant**: Member is added as primary_user_id and participant
- **Default Type**: Reservations use type 'GAME'

#### Date Picker

The booking form includes an inline date picker with three dropdowns:
- **Year**: Current year and next year
- **Month**: 1-12 (all months)
- **Day**: Adjusts dynamically based on selected month/year (28-31 days)

Changing any dropdown triggers an HTMX request to `/member/booking/slots` to reload available time slots for the selected date. The date picker pre-selects today's date on initial load.

### Booking Constraints (Courts)

| Constraint | Rule |
|------------|------|
| Facility | Must be member's home facility |
| Membership Level | Must be >= 1 (verified) |
| Duration | Minimum 1 hour |
| Timing | Start time must be in the future |
| Advance Booking | Date must be within facility's max_advance_booking_days (default: 7) |
| Reservation Limit | Member cannot exceed max_member_reservations active future bookings |
| Single Court | Members book one court at a time |

### Visit Pack Usage

Members with membership_level <= 1 (guests) can use visit packs when booking courts:

- **Pack Selection**: Booking form shows dropdown of active visit packs if member has any
- **Pack Display**: Shows pack ID, visits remaining, and expiration date
- **Redemption**: When pack selected, one visit is redeemed atomically with reservation creation
- **Validation**: Pack must be active, not expired, have visits remaining, and be valid at the facility

If no visit pack is selected, the booking proceeds without pack redemption.

### Lesson Booking

Members can book lessons with teaching pros through a dedicated booking interface:

- **Pro Selection**: View list of pros available at home facility (staff with role='pro')
- **Pro Display**: Each pro shows name and initials placeholder (photo if available)
- **Date Selection**: Same three-dropdown date picker as court booking
- **Slot Selection**: Available 1-hour time slots based on pro availability
- **Availability Calculation**: Facility operating hours minus pro's existing reservations and unavailability blocks

#### Lesson Booking Flow

1. Member selects "Book a Lesson" from portal
2. Pro list displays teaching pros at member's home facility
3. Selecting a pro loads available time slots for the selected date
4. Member picks a slot and confirms booking
5. System creates PRO_SESSION reservation with pro_id set

#### Lesson Constraints

| Constraint | Rule |
|------------|------|
| Facility | Must be member's home facility |
| Duration | Fixed at 1 hour |
| Advance Notice | Must be at least lesson_min_notice_hours (default: 24) in advance |
| Advance Booking | Date must be within facility's max_advance_booking_days |
| Reservation Limit | Counts toward max_member_reservations (same as GAME reservations) |
| Pro Availability | Slot must not conflict with pro's reservations or unavailability blocks |

#### Lesson Reservation Details

When a lesson is booked:
- `reservation_type_id` = PRO_SESSION
- `pro_id` = selected pro's staff ID
- `primary_user_id` = booking member's user ID
- Member is added to `reservation_participants`
- Displays on staff court calendar with PRO_SESSION styling

### Open Play Signup

Members can view and sign up for upcoming open play sessions at their home facility:

- **Session List**: Shows scheduled open play sessions with rule name, date/time, current participant count vs minimum required
- **Signup**: Single-click signup adds member as participant to the session's OPEN_PLAY reservation
- **Cancel Signup**: Members can cancel their signup before the session starts, subject to cancellation cutoff rules
- **Refresh**: Session list auto-refreshes after signup/cancel via `refreshMemberOpenPlay` trigger

#### Open Play Display

Each session card shows:
- Rule name (e.g., "Morning Open Play")
- Date and time range
- Current signup count and minimum required
- Session status badge (scheduled/cancelled)
- Sign Up or Cancel button based on participation status

#### Open Play Constraints

| Constraint | Rule |
|------------|------|
| Facility | Sessions shown for member's home facility only |
| Session Status | Must be 'scheduled' (not cancelled) |
| Timing | Session start time must be in the future |
| Signup Limit | Counts toward max_member_reservations (same as GAME and LESSON) |
| Capacity | Cannot sign up if session is full (participants >= max_participants_per_court * courts) |
| No Duplicates | Cannot sign up twice for the same session |
| Cancellation Cutoff | Must cancel before rule's cancellation_cutoff_minutes before session start |

#### Signup Process

1. Member clicks "Sign up" on a session card
2. System validates constraints in transaction
3. Creates reservation_participant record linking member to OPEN_PLAY reservation
4. Returns HTTP 201, triggers `refreshMemberReservations` and `refreshMemberOpenPlay`
5. Session appears in "Your reservations" as type 'Open Play'

#### Cancel Signup Process

1. Member clicks "Cancel" on a session they've signed up for
2. System validates cancellation cutoff hasn't passed
3. Removes reservation_participant record in transaction
4. Returns HTTP 204, triggers reservation and open play list refresh

### Reservation Limits

The system enforces a per-member limit on active future reservations:

- Counts GAME, PRO_SESSION, and OPEN_PLAY type reservations where member is participant
- Excludes LEAGUE and TOURNAMENT types from the count
- Staff-created reservations (where creator differs from primary_user) do not count against member limit
- When limit is reached, returns HTTP 409 with message: "You have reached the maximum of X active reservations"

### Reservation Cancellation

Members can cancel their own reservations with these restrictions:

- Must be the `primary_user_id` on the reservation
- Reservation must be in the future
- Refund percentage determined by facility's cancellation policy tiers
- Courts and participants removed in transaction
- Cancellation logged with refund percentage applied and hours before start

If no cancellation policy is configured for the facility, 100% refund applies.

#### Full Refund Cancellations

When the applicable refund percentage is 100%, cancellation proceeds immediately without confirmation.

#### Partial Refund Cancellations

When refund is less than 100%, the system requires explicit member confirmation:

1. Initial `DELETE /member/reservations/{id}` returns HTTP 409 with penalty details
2. Frontend displays a styled confirmation modal showing:
   - Fee percentage and refund percentage
   - Reservation details (date, time, court)
   - Facility name
   - 10-minute countdown timer
3. Modal includes "Cancel Reservation" and "Keep Reservation" buttons
4. Member must confirm via second request with `confirm=true` parameter

**Modal Behavior:**

| Action | Result |
|--------|--------|
| Click "Cancel Reservation" | Sends `DELETE /member/reservations/{id}?confirm=true&penalty_calculated_at=...&hours_before_start=...` |
| Click "Keep Reservation" | Modal closes, reservation unchanged |
| Timer expires (10 minutes) | Modal closes automatically, reservation unchanged |
| Close modal (X button) | Modal closes, reservation unchanged |

**Penalty Recalculation:**

The modal includes `penalty_calculated_at` and `hours_before_start` parameters to detect tier boundary crossings:

1. If confirmation arrives after 10-minute window expires, backend recalculates penalty
2. If recalculated tier differs from original (e.g., crossed from 50% to 25% refund), returns new 409 with updated penalty
3. If tier unchanged, proceeds with cancellation at originally displayed percentage

This prevents members from seeing one penalty, waiting, and receiving a different (better) refund.

### HTMX Integration

| Trigger | Action |
|---------|--------|
| `refreshMemberReservations` | Reloads reservations list after booking/cancellation |
| `refreshMemberOpenPlay` | Reloads open play sessions list after signup/cancel |

---

## Front Desk Check-in

Staff use a dedicated check-in interface at `/checkin` for fast facility arrivals. The page is optimized for high-volume check-ins with minimal clicks.

### Check-in Page Layout

```
+-----------------------------------+------------------+
|          Search Box               |   Today's        |
|    (large, auto-focus)            |   Arrivals       |
+-----------------------------------+                  |
|                                   |   - John D 8:15  |
|    Search Results                 |   - Jane S 8:22  |
|    +---------------------------+  |   - Bob K 8:30   |
|    | Photo | Name  | Check In |  |   ...            |
|    +---------------------------+  |                  |
|    | Photo | Name  | Check In |  |                  |
|    +---------------------------+  |                  |
|                                   |                  |
+-----------------------------------+------------------+
```

### Check-in Workflow

1. **Search**: Staff types member name, email, or phone in search box
2. **Results**: Matching members display as cards with photo, name, membership level, and status badges
3. **Status Badges**:
   - Red badge: Waiver not signed - blocks check-in
   - Yellow badge: Membership level 0 (unverified guest) - blocks check-in
4. **Check In**: Click button to record arrival
5. **Success**: Shows member photo, confirmation, and today's activities

### Blocking and Override

Check-in is blocked for members with issues:

| Issue | Badge | Message |
|-------|-------|---------|
| Waiver unsigned | Red | "Waiver missing. Ask the member to sign the waiver before check-in." |
| Membership unverified | Yellow | "Membership is unverified. Confirm membership status before check-in." |

Staff can override blocks with confirmation. The override flag is passed to the check-in request, allowing staff discretion for exceptional cases.

### Activity Selection

After successful check-in, staff can select what the member is arriving for:

| Activity Type | Description |
|---------------|-------------|
| court_reservation | Member has a court booking |
| open_play | Joining an open play session |
| league | League match participation |

The activity selection updates the visit record via `/api/v1/checkin/activity`. Activities display from the member's today's schedule at this facility (court reservations, open play signups, league matches).

### Today's Arrivals

The right panel shows all facility arrivals for the current day:
- Member name and check-in time
- Updates automatically after each check-in
- Sorted by most recent first

### Visit History

Member detail view (existing `/api/v1/members/{id}`) includes visit history showing the last 10 visits with:
- Check-in timestamp
- Facility name
- Activity type (if recorded)

### Multiple Check-ins

The system allows multiple check-ins per day without warning. This supports members who:
- Arrive for morning open play, leave, return for evening league
- Step out for lunch and return
- Attend multiple sessions throughout the day

### Authorization

Check-in is facility-scoped:
- Staff can only check in members at their home facility
- Requires `is_staff=true` authentication
- Facility ID passed via query parameter

---

## Staff Notifications

Staff receive in-app notifications for operational events generated by the open play engine (session cancellations, court scaling). The notification bell icon in the top navigation displays for staff users only.

### Notification Panel

The bell icon triggers an HTMX-loaded dropdown panel showing:
- Unread count badge (refreshes on page load and after marking as read)
- List of recent notifications (up to 25)
- Each notification displays: message, timestamp, type badge, read/unread status
- Empty state when no notifications exist

Clicking a notification marks it as read via PUT request and refreshes the panel.

### Notification Types

| Type | Badge Color | Example |
|------|-------------|---------|
| scale_up | Blue | "Evening Open Play scaled from 2 to 3 courts" |
| scale_down | Yellow | "Morning Open Play scaled from 4 to 2 courts" |
| cancelled | Red | "Morning Open Play cancelled - only 2 signups (min: 4)" |
| lesson_cancelled | Orange | "Lesson cancelled: John Smith (2024-01-15 10:00 - 11:00)" |

### Lesson Cancellation Notifications

When a member cancels a PRO_SESSION (lesson) reservation, the system notifies the assigned pro:

- Notification includes member name, date, and time
- Only triggers for member-initiated cancellations (not staff cancellations)
- Uses `target_staff_id` to route notification to specific pro
- Pros see these in their notification panel filtered by their staff ID

### Facility Scoping

Notifications are scoped by facility:
- Staff with `home_facility_id` see only their facility's notifications
- Corporate-level staff (`home_facility_id = NULL`) see all notifications

---

## Email Notifications

Members receive email notifications for key booking events via AWS SES. Emails are sent asynchronously to avoid blocking request handling.

### Configuration

Email functionality requires four environment variables:

| Variable | Description |
|----------|-------------|
| `SES_ACCESS_KEY_ID` | AWS IAM access key with SES permissions |
| `SES_SECRET_ACCESS_KEY` | AWS IAM secret key |
| `SES_REGION` | AWS region for SES (e.g., `us-east-1`) |
| `SES_SENDER` | Default "from" address (must be verified in SES) |

If any variable is missing, email features are disabled and a warning is logged at startup.

### Email Types

| Email | Trigger | Recipients |
|-------|---------|------------|
| Confirmation | Member books court, lesson, or open play | Booking member |
| Cancellation | Reservation is cancelled | All participants + primary user |
| Reminder | Scheduled job before reservation start | Primary user |

### Confirmation Emails

Sent immediately when a member creates a booking:

- **Court Reservation**: Subject "Game Reservation Confirmed - {Facility}"
- **Lesson Booking**: Subject "Pro Session Confirmed - {Facility}"
- **Open Play Signup**: Subject "Open Play Signup Confirmed - {Facility}"

Email body includes:
- Reservation type and facility name
- Date and time (in facility timezone)
- Court assignment
- Cancellation policy summary

### Cancellation Emails

Sent when a reservation is cancelled (by member or staff):

- Subject: "{Reservation Type} Cancelled - {Facility}"
- Sent to all participants and the primary user (deduplicated)
- Includes refund percentage or "Fee waived" if applicable

### Reminder Emails

A scheduled job runs every 15 minutes to send upcoming reservation reminders:

- Subject: "Upcoming {Reservation Type} Reminder - {Facility}"
- Sent to the primary user of each reservation
- Reminder timing is configurable per organization (default: 24 hours before)

### Sender Address Resolution

The "from" address is resolved in order:
1. Facility-specific `email_from_address` if configured
2. Organization-level `email_from_address` if configured
3. System default `SES_SENDER` environment variable

All sender addresses must be verified in AWS SES.

### Database Schema

| Column | Table | Description |
|--------|-------|-------------|
| email_from_address | organizations | Organization-level sender override |
| email_from_address | facilities | Facility-level sender override |
| reminder_hours_before | organizations | Hours before reservation to send reminder (default: 24) |
| reminder_hours_before | facilities | Facility-level reminder timing override |

### Constraints

| Rule | Enforcement |
|------|-------------|
| Verified sender | SES identity verification checked on startup |
| Valid email format | Recipient email validated before sending |
| Graceful degradation | Email failures are logged but don't fail requests |

### Planned Extensions

- SMS notifications via SNS
- Push notifications (mobile app)
- Waitlist slot availability alerts
- Membership renewal reminders

---

## Implementation Status

### Operational Today

| Feature | Status | Notes |
|---------|--------|-------|
| Member CRUD | Complete | Create, list, search, edit, soft delete, restore |
| Member Photos | Complete | Base64 upload, BLOB storage, MediaDevices API |
| Member Search | Complete | Name, email, phone - instant results |
| Theme Management | Complete | Create, edit, clone, delete, set active, 34 system themes seeded |
| Theme Accessibility | Complete | WCAG AA contrast validation (3.0 ratio) |
| Open Play Rules | Complete | Full CRUD with constraint validation |
| Open Play Sessions | Partial | Session tracking, participant management |
| Court Calendar | Complete | Day view with reservations, date navigation |
| Reservations | Complete | CRUD, multi-court, participants, conflict detection |
| Staff Local Login | Complete | Bcrypt, rate limiting, timing attack mitigation |
| Authorization | Complete | Facility-scoped access, admin override |
| Operating Hours | Complete | Admin UI, per-day CRUD, default hours, HTMX updates |
| Staff Management | Complete | CRUD, facility-scoped authorization, deactivation with session handling |
| Staff Lesson Booking | Complete | Book lessons for members from calendar, facility selector for org-level staff, staff badge on calendar |
| Staff Notifications | Complete | Bell icon, dropdown panel, unread badge, mark-as-read, facility scoping |
| Member Portal | Complete | Self-service portal, court booking, lesson booking, reservation cancellation |
| Pro Unavailability | Complete | Pros can block time, affects lesson availability |
| Check-in Flow | Complete | Search, check-in, activity selection, arrivals list, visit history |
| Cancellation Policies | Complete | Per-facility refund tiers, reservation type-specific policies, staff fee waiver, cancellation logging |
| Waitlist Management | Complete | Join/leave waitlist, slot notifications on cancellation, configurable notification modes |
| Lesson Cancellation Notifications | Complete | Pros notified when members cancel lessons |
| Email Notifications | Complete | SES integration, confirmation/cancellation/reminder emails |
| Tier Booking Windows | Complete | Per-tier advance booking days, admin UI, membership-based enforcement |
| Visit Pack Management | Complete | Pack type CRUD, pack sales, redemption at booking, cross-facility support |
| Lesson Package Management | Complete | Package type CRUD, package sales, auto-redemption at lesson booking, cancellation restore |
| League Management | Complete | League CRUD, team management, roster controls, schedule generation, match results, standings |
| Reporting Dashboard | Complete | Utilization metrics, booking counts by type, cancellation rates, check-in counts, date range filtering |

### Partial Implementation

| Feature | Status | Notes |
|---------|--------|-------|
| Cognito Auth | Complete | EMAIL_OTP and SMS_OTP via single shared User Pool |
| Open Play Enforcement | Scheduled | gocron job configured, evaluation logic partial |
| Password Reset | Complete | Cognito reset + local hash sync for dual-auth users |
| Recurrence Rules | Schema only | Tables exist, not used in handlers |

### Not Yet Started

| Feature | Notes |
|---------|-------|
| SMS Notifications | SNS delivery for reservation alerts |
| Payment Processing | Stripe/Square integration |
| Financial Reporting | Revenue tracking, payment reconciliation |
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
| **League** | Competitive season with teams and scheduled matches |
| **Theme** | Color scheme for facility branding |
| **Waiver** | Liability agreement required for play |
| **Waitlist** | Queue for members awaiting slot availability |
| **Soft Delete** | Mark as deleted but preserve history |
