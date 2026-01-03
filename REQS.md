# PICKLEICIOUS - Complete Business Requirements Document

## Requirements Sources

### File-Based Sources
| File | Description |
|------|-------------|
| `tools/auto_stories/stories.yaml` | Main PRD - 50+ user stories with acceptance criteria |
| `assets/stories/stories1.yaml` | Foundational stories (IS, DB, SV categories) |
| `internal/db/schema/schema.sql` | Complete database schema |
| `internal/db/migrations/000001_init_schema.up.sql` | Initial migration |
| `internal/db/queries/members.sql` | Member operations |
| `internal/db/queries/courts.sql` | Court operations |
| `internal/db/queries/schedules.sql` | Operating hours |
| `assets/themes` | 30+ predefined color palettes |
| `web/styles/themes.css` | CSS variable structure |
| `internal/db/migrations/README.md` | Database architecture docs |
| `internal/api/auth/handlers.go` | Auth TODOs |
| `internal/api/nav/handlers.go` | Search TODO |

### GitHub Issues (codr1/pickleicious)
32 total issues tracking stories and bugs. See **Section 15** for complete list.

### Coverage Matrix

| Requirement Area | In GitHub Issues | In YAML Stories | In Code/Schema | Notes |
|-----------------|------------------|-----------------|----------------|-------|
| Infrastructure (IS1-3) | #1, #2, #3, #9, #10, #11 | ✅ | ✅ Implemented | IS1, IS2 closed |
| Database (DB1-3) | #4, #5, #6, #12, #13, #14 | ✅ | ✅ Implemented | Schema complete |
| Server (SV1-2) | #7, #8, #15, #16 | ✅ | ✅ Implemented | Hot reload working |
| Themes (TH0-3) | #17, #18, #19, #20, #21, #22 | ✅ | Partial | CSS vars defined, mgmt TODO |
| Localization (L1-6) | #23, #24, #25, #26, #27, #28 | ✅ | ❌ Not started | Full i18n planned |
| CI/CD | #29 | ❌ | ❌ Not started | GitHub Actions planned |
| Member Management | ❌ | ❌ | ✅ Implemented | Not in issues, but complete |
| Court/Calendar | ❌ | ❌ | ✅ Basic UI | Not in issues |
| Authentication | ❌ | ❌ | Partial (TODOs) | Cognito integration pending |
| Open Play Rules | #32 | ❌ | ❌ | New feature request |
| Search | #31 | ❌ | ✅ Fixed | Bug fixed (merged 2025-12-24) |
| Deleted User Handling | #30 | ❌ | ✅ Implemented | Closed |
| **Planned (Section 9)** | ❌ | ❌ | ❌ | 18 feature areas identified |

---

## 0. INFRASTRUCTURE DEBT (Priority)

### 0.1 Comprehensive Test Infrastructure
**Problem**: Server startup can fail without any test catching it. Current tests only cover handlers in isolation.

**Required**:
- E2E tests that start the actual server binary
- Smoke tests for server startup (db connection, migrations applied, tables exist)
- Integration tests that use real database with migrations
- CI gate that runs smoke tests on every PR

### 0.2 Build System Modernization
**Problem**: CMake/Make build system is overly complex for a Go project and causes confusion (e.g., dead `DB_PATH` variables, multiple indirections).

**Required**:
- Migrate to Go-centric build tool (Mage, Task, or just Makefile with go commands)
- Single source of truth for configuration
- Simpler developer experience: `go run`, `go test`, `go build`
<!-- BEGIN WIP: STORY-0009 -->
- Remove cmake/, CMakeLists.txt complexity
<!-- END WIP -->

---

## 1. EXECUTIVE SUMMARY

### 1.1 Product Vision
**Pickleicious** is a comprehensive **multi-tenant SaaS platform for pickleball facility management**. It enables indoor pickleball venues to manage court reservations, member profiles, staff operations, and facility scheduling through a modern web interface optimized for front-desk operations.

### 1.2 Target Market
- Indoor pickleball facilities (dedicated courts)
- Multi-location pickleball chains/franchises
- Recreation centers with pickleball programs
- Private clubs with pickleball amenities

### 1.3 Key Value Propositions
1. **Rapid member check-in** with photo identification and camera capture
2. **Visual court scheduling** with drag-and-click booking
3. **Multi-facility operations** under a single organizational umbrella
4. **White-label theming** for brand consistency per facility
5. **Future-ready internationalization** for global expansion

---

## 2. ORGANIZATIONAL HIERARCHY

### 2.1 Multi-Tenancy Model

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

### 2.2 Data Model Relationships

| Entity | Parent | Key Attributes |
|--------|--------|----------------|
| **Organization** | — | name, slug (unique), status |
| **Facility** | Organization | name, slug (unique), timezone |
| **User** | — | email, phone, cognito_sub, auth preferences |
| **Member** | User → Facility | profile, membership_level, billing, photo |
| **Staff** | User → Facility | role, home_facility (optional for admins) |
| **Court** | Facility | name, court_number, status |
| **Reservation** | Facility | type, courts, times, participants |

---

## 3. USER MANAGEMENT SYSTEM

> **Source:** `internal/db/schema/schema.sql`, `internal/db/queries/members.sql`
> **GitHub Issues:** None (implemented directly)

### 3.1 User Entity (Authentication Core)

The `users` table is the authentication foundation:

| Field | Type | Description |
|-------|------|-------------|
| `id` | INTEGER PK | Unique user identifier |
| `email` | TEXT UNIQUE | Primary email address |
| `phone` | TEXT | Phone number for SMS auth |
| `cognito_sub` | TEXT | AWS Cognito unique user ID |
| `cognito_status` | TEXT | 'CONFIRMED' or 'UNCONFIRMED' |
| `preferred_auth_method` | TEXT | 'SMS', 'EMAIL', or 'PUSH' |
| `password_hash` | TEXT | For staff local authentication |
| `local_auth_enabled` | BOOLEAN | Staff password login toggle |
| `status` | TEXT | 'active', 'suspended', 'archived' |

### 3.2 Member Entity (Customer Profile)

Complete member record structure:

| Field | Type | Description |
|-------|------|-------------|
| `id` | INTEGER PK | Member identifier |
| `user_id` | FK→users | Links to auth record |
| `first_name` | TEXT NOT NULL | First name |
| `last_name` | TEXT NOT NULL | Last name |
| `photo_url` | TEXT | Legacy photo URL (deprecated) |
| `street_address` | TEXT | Street address |
| `city` | TEXT | City |
| `state` | TEXT | State/province |
| `postal_code` | TEXT | ZIP/postal code |
| `date_of_birth` | TEXT | YYYY-MM-DD format |
| `waiver_signed` | BOOLEAN | Legal waiver acceptance |
| `status` | TEXT | 'active', 'suspended', 'archived', 'deleted' |
| `home_facility_id` | FK→facilities | Primary location |
| `membership_level` | INTEGER | Tier: 0-3+ |

### 3.3 Membership Levels

| Level | Name | Description |
|-------|------|-------------|
| 0 | Unverified Guest | New registrant, identity not confirmed |
| 1 | Verified Guest | Identity confirmed, limited access |
| 2 | Member | Full membership, standard benefits |
| 3+ | Member+ | Premium tiers with additional benefits |

### 3.4 Member Photo System

Photos are stored as BLOBs directly in the database:

| Field | Type | Description |
|-------|------|-------------|
| `member_id` | FK→members | Unique (one photo per member) |
| `data` | BLOB | Raw image bytes |
| `content_type` | TEXT | MIME type (e.g., 'image/jpeg') |
| `size` | INTEGER | File size in bytes |

**Photo Capture Workflow:**
1. Frontend activates webcam via `navigator.mediaDevices.getUserMedia()`
2. Video stream displays in preview element
3. User clicks "Take Photo" → canvas captures frame
4. Frame converted to Base64 data URL
5. Data URL sent in hidden form field `photo_data`
6. Backend decodes Base64 → stores as BLOB
7. Photo served via `/api/v1/members/photo/{member_id}`

**UI States:**
- No photo → Show initials (e.g., "JD" for John Doe)
- New member → Show "+" placeholder
- With photo → Render from `/api/v1/members/photo/{id}`

### 3.5 Member Billing Information

Separate billing table for payment data:

| Field | Type | Description |
|-------|------|-------------|
| `member_id` | FK→members | Unique per member |
| `card_last_four` | TEXT | Last 4 digits of card |
| `card_type` | TEXT | Visa, Mastercard, etc. |
| `billing_address` | TEXT | Billing street address |
| `billing_city` | TEXT | Billing city |
| `billing_state` | TEXT | Billing state |
| `billing_postal_code` | TEXT | Billing postal code |

**Planned:** Transaction history table for all charges, payments, refunds.

### 3.6 Staff Entity

| Field | Type | Description |
|-------|------|-------------|
| `id` | INTEGER PK | Staff identifier |
| `user_id` | FK→users | Links to auth record |
| `first_name` | TEXT | First name |
| `last_name` | TEXT | Last name |
| `home_facility_id` | FK→facilities | NULL for corporate admins |
| `role` | TEXT | Role identifier |

**Staff Roles:**

---

## 4. AUTHENTICATION SYSTEM

> **Source:** `internal/api/auth/handlers.go` (TODOs), login templates
> **GitHub Issues:** None (not tracked as stories)

### 4.1 Authentication Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    AUTHENTICATION FLOW                       │
└─────────────────────────────────────────────────────────────┘
                              │
          ┌───────────────────┴───────────────────┐
          ▼                                       ▼
┌─────────────────────┐               ┌─────────────────────┐
│  MEMBER AUTH        │               │  STAFF AUTH         │
│  (Passwordless)     │               │  (Password + OTP)   │
└─────────────────────┘               └─────────────────────┘
          │                                       │
          ▼                                       ▼
┌─────────────────────┐               ┌─────────────────────┐
│ 1. Enter email/phone│               │ 1. Enter identifier │
│ 2. Cognito sends OTP│               │ 2. Check staff flag │
│ 3. Verify code      │               │ 3a. Password login  │
│ 4. Create session   │               │ 3b. Fallback to OTP │
└─────────────────────┘               └─────────────────────┘
```

### 4.2 Cognito Configuration (Per-Organization)

| Field | Description |
|-------|-------------|
| `pool_id` | Cognito User Pool ID |
| `client_id` | App client ID |
| `client_secret` | App client secret |
| `domain` | Custom domain (e.g., org.pickleadmin.com) |
| `callback_url` | OAuth callback URL |

### 4.3 Login UI Flow (HTMX-Powered)

**Step 1: Identifier Entry**
- On blur, checks if identifier belongs to staff with local auth
- If yes, shows "Sign in with staff credentials" link

**Step 2: Code Request**
- Sends verification code via Cognito
- Replaces form with code entry UI

**Step 3: Code Verification**
- Validates code with Cognito
- Creates session/JWT on success

### 4.4 Implementation Status

| Feature | Status |
|---------|--------|
| Login page UI | ✅ Complete |
| Staff check endpoint | ✅ Complete |
| Cognito initialization | ❌ TODO |
| Code sending | ❌ TODO |
| Code verification | ❌ TODO |
| Session creation | ❌ TODO |
| Password login | ❌ TODO |
| Password reset | ❌ TODO |

---

## 5. MEMBER MANAGEMENT OPERATIONS

> **Source:** `internal/api/members/handlers.go`, member templates
> **GitHub Issues:** #30 (Deleted user re-add - CLOSED), #31 (Search broken - FIXED)

### 5.1 Member List View

**Layout:** Split-panel design
- Left panel (50%): Searchable, paginated member list
- Right panel (50%): Selected member detail or form

**Search Functionality:**
```sql
WHERE m.status <> 'deleted'
  AND (@search_term IS NULL
       OR m.first_name LIKE '%' || @search_term || '%'
       OR m.last_name LIKE '%' || @search_term || '%'
       OR m.email LIKE '%' || @search_term || '%')
ORDER BY m.last_name, m.first_name
```

> **GitHub Issue #31:** ✅ Search fixed (merged 2025-12-24)

**Pagination:**
- Default: 25 per page
- Options: 25, 50, 100
- HTMX-powered infinite scroll ready

### 5.2 Member CRUD Operations

**Create Member:**
1. Click "Add Member" → New form loads in right panel
2. Fill required fields:
   - First name, last name
   - Email (validated, checked for duplicates)
   - Phone (10-20 characters)
   - Date of birth (validated age 0-100)
   - Address fields
3. Optional: Capture photo via webcam
4. Submit triggers waiver send alert
5. On success: Detail view + list refresh

**Validation Rules:**
| Field | Rule |
|-------|------|
| Email | Must contain `@`, max 254 chars |
| Phone | 10-20 characters |
| Postal Code | 5-10 characters |
| DOB | Valid date, age 0-100 years |

**Duplicate Email Handling:**
- 409 Conflict returns `{error: "duplicate_email", member_id: X}`
- User can restore deleted account or modify old email to create new

> **GitHub Issue #30 (CLOSED):** Deleted user re-add conflict handling - implemented

**Update Member:**
1. Click "Edit" on detail view
2. All fields editable except ID
3. Photo can be replaced via webcam
4. Waiver can be re-sent if unsigned
5. Save updates detail view + refreshes list

**Delete Member (Soft Delete):**
```sql
UPDATE members
SET status = 'deleted', updated_at = CURRENT_TIMESTAMP
WHERE id = @id;
```
- Confirmation dialog required
- Sets status to 'deleted' (not physical delete)
- Excluded from normal queries

**Restore Member:**
1. Triggered when creating member with deleted account's email
2. Options:
   - Restore previous account (reactivate)
   - Create new account (prefix old email with `{id}___`)

### 5.3 Member Detail View

**Information Displayed:**
- Photo (or initials placeholder)
- Full name, email, phone
- Date of birth
- Full address
- Status badge (green=active, red=inactive)
- Waiver status
- Membership level (Unverified Guest → Member+)
- Member ID
- Billing info (lazy-loaded)

**Actions Available:**
- Edit member
- Delete member
- Resend waiver (if unsigned)

---

## 6. COURT & RESERVATION SYSTEM

> **Source:** `internal/db/schema/schema.sql`, `internal/templates/components/courts/`
> **GitHub Issues:** #32 (Open play cancellation rules - NEW)

### 6.1 Facility Operating Hours

| Field | Description |
|-------|-------------|
| `facility_id` | FK to facility |
| `day_of_week` | 0=Sunday through 6=Saturday |
| `opens_at` | TIME - Opening time |
| `closes_at` | TIME - Closing time |

Unique constraint: One entry per facility per day.

### 6.2 Court Entity

| Field | Description |
|-------|-------------|
| `facility_id` | FK to facility |
| `name` | Display name (e.g., "Court A") |
| `court_number` | Numeric identifier (unique per facility) |
| `status` | 'active', 'maintenance', etc. |

### 6.3 Reservation Types (Lookup Table)

| Type | Description | Visual |
|------|-------------|--------|
| `GAME` | Regular member play | Standard color |
| `PRO_SESSION` | Lesson with pro | Accent color + pattern |
| `EVENT` | Special events | Custom styling |
| `MAINTENANCE` | Court maintenance | Tertiary + diagonal |
| `LEAGUE` | League play | Primary color |
| `TOURNAMENT` | Tournament blocks | Gradient |

### 6.4 Recurrence Rules

Supports recurring reservations:

| Rule | Description |
|------|-------------|
| `WEEKLY` | Same day/time every week |
| `BIWEEKLY` | Every two weeks |
| `MONTHLY` | Same day of month |

Stored as iCalendar RRULE compatible format.

### 6.5 Reservation Entity

| Field | Type | Description |
|-------|------|-------------|
| `id` | INTEGER PK | Reservation ID |
| `facility_id` | FK | Facility reference |
| `reservation_type_id` | FK | Type lookup |
| `recurrence_rule_id` | FK | NULL if one-time |
| `primary_member_id` | FK | Booking owner |
| `pro_id` | FK | Staff pro (for lessons) |
| `start_time` | DATETIME | Start timestamp |
| `end_time` | DATETIME | End timestamp |
| `is_open_event` | BOOLEAN | Open for sign-ups |
| `teams_per_court` | INTEGER | Team configuration |
| `people_per_team` | INTEGER | Team size |

### 6.6 Multi-Court & Multi-Participant Support

**Junction Tables:**

`reservation_courts`:
- One reservation can span multiple courts
- Example: Tournament uses courts 1-4

`reservation_participants`:
- Multiple members can join a reservation
- Beyond the primary member

### 6.7 Open Play Rules (NEW)

> **GitHub Issue #32:** Open play cancellation and auto-scaling

<!-- BEGIN WIP: STORY-0005 -->
**Requirements:**
- Cancel open play if fewer than X reservations by cutoff time
- Auto-scale number of courts based on signups (not all-or-nothing)
- Rule: Number of players per court for open play
- Ability to auto-scale up if demand increases
<!-- END WIP -->

**Team Configuration:**
- Total number of teams
- People per team
- Teams per court

### 6.8 Calendar UI Requirements

**Current Implementation:**
- 8-court grid display
- Hours: 6:00 AM to 10:00 PM (16 slots)
- Date navigation: Previous/Next/Today
- View modes: Work Week, Week, Month (selector ready)
- Click-to-book: Opens modal at court/time intersection

**Visual Grid:**
```
         Court 1  Court 2  Court 3  Court 4  Court 5  Court 6  Court 7  Court 8
 6:00   [      ] [      ] [      ] [      ] [      ] [      ] [      ] [      ]
 7:00   [      ] [      ] [      ] [      ] [      ] [      ] [      ] [      ]
 ...
22:00   [      ] [      ] [      ] [      ] [      ] [      ] [      ] [      ]
```

**Slot States (Planned):**
| State | Color Treatment |
|-------|-----------------|
| Available | Highlight color, subtle gradient |
| Game | Standard slot color |
| Pro Session | Accent color, distinctive pattern |
| Tournament | Primary→Secondary gradient |
| Maintenance | Tertiary, diagonal stripes |

### 6.9 Calendar Interactions (Planned)

These calendar interactions are designed but not yet implemented:

**Views:**
- Week View: One court across a week for spotting patterns
- Month View: Overview of all reservations
- Agenda View: Chronological list of upcoming reservations

**Interactions:**
- Click empty slot: Create new reservation
- Click existing reservation: View details, edit
- Drag reservation: Reschedule within constraints
- Hover over reservation: Quick preview of participants

**Constraints:**
- Respect operating hours (slots outside hours grayed/non-bookable)
- Prevent double-booking
- Validate against court availability

---

## 7. THEMING SYSTEM

> **Source:** `assets/themes`, `web/styles/themes.css`, `tools/auto_stories/stories.yaml`
> **GitHub Issues:** #17, #18, #19, #20, #21, #22 (TH0.1 through TH3.3)

### 7.1 Theme Architecture

> **GitHub Issue #17 [TH0.1]:** Theme Data Structure and Template Definition

**5-Color Palette System:**

| Variable | Role |
|----------|------|
| `--theme-primary` | Main brand color, headers |
| `--theme-secondary` | Supporting color, backgrounds |
| `--theme-tertiary` | Contrast/accent areas |
| `--theme-accent` | Interactive elements, CTAs |
| `--theme-highlight` | Emphasis, notifications |

**Acceptance Criteria (from #17):**
- [ ] Database schema defined for theme storage
- [ ] CSS template structure created
- [ ] Theme metadata structure defined
- [ ] Validation rules established
- [ ] Documentation templates created
- [ ] Helper functions for theme processing defined
- [ ] Theme switching mechanism specified
- [ ] Default component styles templated

### 7.2 Theme Component Definitions

> **GitHub Issue #18 [TH1.0]:** Theme Component Definitions and Requirements

**Components to Theme:**
- Navigation bars
- Buttons (primary, secondary, tertiary)
- Forms and input elements
- Calendar slots for each state (Available, Booked, Pro session, Tournament, Maintenance)
- Alert messages (success, warning, error, info)
- Text elements (headers, body, links)

**Acceptance Criteria (from #18):**
- [ ] Define color usage guide for each component
<!-- BEGIN WIP: STORY-0007 -->
- [ ] Define contrast requirements for all text elements
<!-- END WIP -->
- [ ] Define hover/active state behaviors
- [ ] Define transition animations
- [ ] Create component preview layout
- [ ] Document accessibility requirements

### 7.3 Pre-Defined Themes (30+)

> **GitHub Issue #19 [TH2.0]:** Simple Theme (Default) Implementation
> **GitHub Issue #20 [TH2.1]:** Core Theme Palette Definitions

| Theme | Primary | Character |
|-------|---------|-----------|
| Simple (Default) | #0B0C10 | Minimal, cyan accents |
| Metal | #3D52A0 | Professional navy/blue |
| Vintage | #244855 | Warm earth tones |
| Cool | #003135 | Ocean-inspired teal |
| Cosmic | #212A31 | Space-inspired dark |
| Artsy | #D79922 | Bold gold/red |
| Elegance | #EDC7B7 | Soft rose/navy |
| Futuristic | #2C3531 | Teal/cream |
| Dynamic | #2F4454 | Dark pink accents |
| Green Pickle | #61892F | Nature green |
| Fresh Pickle | #182628 | Teal/green |
| Purple Dream | #802BB1 | Vibrant purple |
| Desert Pickle | #026670 | Teal/cream |
| Modern Pickle | #25274D | Navy/cyan |
| Flat Style | #00887A | Teal/white |

**Simple Theme Acceptance Criteria (from #19):**
- [ ] Color palette implemented according to specification
- [ ] Navigation using Primary (#0B0C10)
- [ ] Primary buttons using Accent (#66FCF1)
- [ ] Secondary buttons using Secondary (#1F2833)
- [ ] Calendar slot states with proper colors
<!-- BEGIN WIP: STORY-0007 -->
- [ ] All text elements meet WCAG AA contrast
<!-- END WIP -->
- [ ] Hover and active states defined
- [ ] Theme preview demonstrates all components

### 7.4 Light/Dark Mode System

> **GitHub Issue #21 [TH3.0]:** Light/Dark Theme Variant System

**Transformation Rules:**
```typescript
// HSL-based transformation
function transformColor(color: string, mode: 'light' | 'dark'): string {
    const hsl = convertToHSL(color);
    if (mode === 'dark') {
        hsl.lightness = invertLightness(hsl.lightness);
        hsl.saturation *= 0.9; // Slightly desaturate
    }
    return convertToHex(hsl);
}
```

**Acceptance Criteria (from #21):**
- [ ] Generate dark variant from light theme
- [ ] Generate light variant from dark theme
- [ ] Preserve relative contrast relationships
- [ ] Maintain theme's character
<!-- BEGIN WIP: STORY-0007 -->
- [ ] Ensure WCAG compliance in both variants
<!-- END WIP -->
- [ ] Handle backgrounds, text, borders, shadows
- [ ] Support @prefers-color-scheme media query
- [ ] Manual override capability

### 7.5 Theme Preference Management

> **GitHub Issue #22 [TH3.3]:** Theme Preference Management System

**Preference Hierarchy:**
1. User explicit settings (highest priority)
2. System preferences (`prefers-color-scheme`)
3. Facility defaults
4. System defaults

**Acceptance Criteria (from #22):**
- [ ] Store: selected theme, light/dark pref, contrast pref, animation pref
- [ ] Local storage backup
- [ ] Server-side persistence
- [ ] Cross-device sync
- [ ] System preference detection
- [ ] Fallback chain
- [ ] Initial load management

### 7.6 Theme Transitions

```css
.theme-transition-root {
    --theme-transition-duration: 300ms;
    --theme-transition-timing: cubic-bezier(0.4, 0, 0.2, 1);

    transition-property:
        background-color, color, border-color, box-shadow;
    transition-duration: var(--theme-transition-duration);
}

/* Accessibility: Respect reduced motion */
@media (prefers-reduced-motion: reduce) {
    .theme-transition-root {
        --theme-transition-duration: 0ms;
    }
}
```

---

## 8. LOCALIZATION SYSTEM

> **Source:** `tools/auto_stories/stories.yaml`
> **GitHub Issues:** #23, #24, #25, #26, #27, #28 (L1 through L6)
> **Status:** NOT STARTED

### 8.1 Core Infrastructure

> **GitHub Issue #23 [L1]:** Localization Core Infrastructure Setup

**Technology Stack:**
- **Package:** `github.com/nicksnyder/go-i18n/v2`
- **Format:** TOML message files
- **Fallback:** English (always loaded)

**Acceptance Criteria (from #23):**
- [ ] go-i18n package integrated
- [ ] Directory structure created and documented
- [ ] Base English message file created
- [ ] Fall-through to English working
- [ ] Message loading and caching system
- [ ] Hot reload in development

### 8.2 Message Extraction

> **GitHub Issue #24 [L2]:** Message Extraction and Bundle Management

**Directory Structure:**
```
/locales
├── extract/            # Extracted strings
├── active/             # Active translations
└── archive/            # Obsolete translations
```

**Acceptance Criteria (from #24):**
- [ ] Message extraction command working
- [ ] Merge preserves existing translations
- [ ] Duplicate message detection
- [ ] Missing translation reporting
- [ ] Bundle hot-reloading in development
- [ ] Documentation for extraction workflow

### 8.3 Locale Detection

> **GitHub Issue #25 [L3]:** Locale Detection and Switching

**Detection Priority:**
1. URL parameter (`?lang=es`)
2. User preference (logged-in users)
3. Accept-Language header
4. Default: English

**Acceptance Criteria (from #25):**
- [ ] Locale detection hierarchy implemented
- [ ] Switching without page reload
- [ ] Preference persistence (user settings + cookie)
- [ ] Locale middleware
- [ ] URL-based switching
- [ ] Tests for detection logic

### 8.4 Formatting Support

> **GitHub Issue #26 [L4]:** Date, Time, and Number Formatting

| Type | Implementation |
|------|----------------|
| Dates | `golang.org/x/text/date` |
| Times | Locale-aware formatting |
| Numbers | Thousand separators, decimals |
| Currency | Symbol, position, decimals |

**Acceptance Criteria (from #26):**
- [ ] Date formatting respects locale
- [ ] Time formatting respects locale
- [ ] Number formatting respects locale
- [ ] Currency formatting respects locale
- [ ] Helper functions documented
- [ ] Tests for all formatting functions

### 8.5 Translation Management UI

> **GitHub Issue #27 [L5]:** Translation Management Interface

Admin interface features:
- Missing translation highlighting
- Unused translation identification
- Inline editing (HTMX real-time updates)
- Import/export (TOML)
- Cross-translation search

**Acceptance Criteria (from #27):**
- [ ] Translation management UI implemented
- [ ] Missing translations highlighted
- [ ] Unused translations identified
- [ ] Inline editing working
- [ ] Import/export working
- [ ] Search functionality working

### 8.6 Testing Framework

> **GitHub Issue #28 [L6]:** Localization Testing Framework

**Test Types:**
- Unit tests for formatting functions
- Integration tests for locale switching
- Validation tests for translation files
- Performance tests for bundle loading
- Missing translation detection

**Acceptance Criteria (from #28):**
- [ ] Unit tests implemented
- [ ] Integration tests implemented
- [ ] Translation file validation working
- [ ] Performance benchmarks created
- [ ] CI pipeline integration complete

### 8.7 Performance Targets

| Metric | Target |
|--------|--------|
| Core bundle size | < 50KB |
| Cache hit rate | > 98% |
| Time to Interactive impact | None |
| Lazy loading | Route-based, idle-time |

---

## 9. PLANNED REQUIREMENTS (NOT YET IMPLEMENTED)

> **Source:** Industry analysis, competitive research (CourtReserve, PodPlay, Playbypoint, EZFacility)
> **GitHub Issues:** None yet - these are identified gaps
> **Status:** NOT STARTED

This section documents features that are standard in competing pickleball/sports facility management software but are not yet implemented or tracked in Pickleicious.

### 9.1 Payment Processing System

**Payment Gateway Integration:**
| Feature | Description | Priority |
|---------|-------------|----------|
| Stripe/Square Integration | Primary payment processor connection | Critical |
| Credit/Debit Card Processing | Accept all major cards (Visa, MC, Amex, Discover) | Critical |
| ACH/Bank Transfer | Direct bank payments for memberships | High |
| Digital Wallets | Apple Pay, Google Pay support | Medium |

**Recurring Billing:**
| Feature | Description |
|---------|-------------|
| Membership Subscriptions | Automated monthly/annual billing |
| Auto-renewal | Automatic membership renewals with notifications |
| Failed Payment Handling | Retry logic, dunning emails, grace periods |
| Proration | Handle mid-cycle upgrades/downgrades |

**Point of Sale (POS):**
| Feature | Description |
|---------|-------------|
| On-site Transactions | Front desk payment processing |
| Receipt Generation | Digital and printed receipts |
| Cash Handling | Cash drawer management, end-of-day reconciliation |
| Split Payments | Multiple payment methods per transaction |

**Financial Management:**
| Feature | Description |
|---------|-------------|
| Refund Processing | Full and partial refunds |
| Credit System | Account credits for future use |
| Promo Codes/Discounts | Percentage or fixed-amount discounts |
| Payment Plans | Installment options for large purchases |
| Gift Cards | Purchase and redemption |
| Processing Fee Handling | Pass-through or absorb card fees |

**Reporting:**
| Feature | Description |
|---------|-------------|
| Daily Revenue Reports | Sales by category, payment method |
| Settlement Reports | Bank deposit reconciliation |
| Tax Reporting | Sales tax collection and reporting |
| Accounts Receivable | Outstanding balances tracking |

### 9.2 Communication System

**Automated Notifications:**
| Trigger | Channels | Content |
|---------|----------|---------|
| Booking Confirmation | Email, SMS | Reservation details, court, time |
| 24-Hour Reminder | Email, SMS, Push | Upcoming reservation reminder |
<!-- BEGIN WIP: STORY-0005 -->
| Cancellation Notice | Email, SMS | Confirmation of cancellation |
<!-- END WIP -->
| Waitlist Promotion | Email, SMS | Spot opened, action required |
| Payment Receipt | Email | Transaction confirmation |
| Membership Renewal | Email | Upcoming renewal reminder |
| Waiver Expiration | Email | Waiver renewal needed |

**Marketing & Engagement:**
| Feature | Description |
|---------|-------------|
| Email Newsletters | Bulk email campaigns to members |
| SMS Marketing | Text message campaigns (opt-in) |
| Push Notifications | Mobile app alerts |
| Segmented Messaging | Target by membership level, activity, etc. |
| Event Announcements | Promote leagues, tournaments, clinics |

**In-App Communication:**
| Feature | Description |
|---------|-------------|
| Member-to-Member Messaging | Find partners for games (opt-in) |
| Staff-to-Member Messaging | Direct communication channel |
| Announcement Banner | Facility-wide notices |

### 9.3 Waitlist Management

**Core Waitlist Features:**
| Feature | Description |
|---------|-------------|
| Auto-Waitlist | Automatically offer waitlist when court is full |
| Position Tracking | Show member their place in queue |
| Fair Notification | Simultaneous notification to all waitlisted |
| First-Come-First-Served | First to accept gets the slot |
| Expiring Offers | Time limit to accept (e.g., 15 minutes) |
| Auto-Decline | Remove from waitlist if not accepted in time |

**Configuration Options:**
| Setting | Description |
|---------|-------------|
| Max Waitlist Size | Limit waitlist length per slot |
| Notification Window | How long before slot to notify |
| Member Priority | VIP members get first notification |

### 9.4 Cancellation Policies & No-Show Management

**Tiered Cancellation Rules:**
| Window | Policy | Example |
|--------|--------|---------|
| 48+ hours | Full refund/credit | Free cancellation |
| 24-48 hours | Partial refund (50%) | Late cancellation fee |
| < 24 hours | No refund | Forfeited payment |
| No-show | Full charge + penalty | Strike system |

**No-Show Tracking:**
| Feature | Description |
|---------|-------------|
| No-Show Recording | Track members who don't appear |
| Strike System | Accumulate warnings (e.g., 3 strikes = suspension) |
| Penalty Fees | Automatic charge for no-shows |
| Booking Restrictions | Limit future bookings for repeat offenders |
| Grace Period | Minutes allowed for late arrival |

**Configuration:**
| Setting | Description |
|---------|-------------|
| Cancellation Windows | Define time-based refund tiers |
| Penalty Amounts | Set fees per violation |
| Strike Threshold | Number before action taken |
| Check-in Window | How early/late can check in |

### 9.5 League & Tournament Management

**League Features:**
| Feature | Description |
|---------|-------------|
| League Creation | Name, format, schedule, rules |
| Team Management | Create/manage teams, rosters |
| Division Support | Skill-based divisions |
| Schedule Generation | Auto-generate match schedules |
| Standings Tracking | Points, wins, losses, ties |
| Playoff Brackets | Auto-generate from standings |

**Tournament Features:**
| Feature | Description |
|---------|-------------|
| Bracket Types | Single elimination, double elimination, round robin |
| Seeding | Manual or ratings-based seeding |
| Court Assignment | Auto-assign matches to courts |
| Score Entry | Staff or self-reported scores |
| Live Brackets | Real-time bracket updates |
| Prize Management | Track and award prizes |

**Registration:**
| Feature | Description |
|---------|-------------|
| Online Registration | Self-service sign-up |
| Team Registration | Register as team or individual |
| Waitlist | When league is full |
| Payment Integration | Collect fees at registration |
| Skill Verification | DUPR or self-reported level |

### 9.6 DUPR Integration & Player Ratings

**DUPR Sync:**
| Feature | Description |
|---------|-------------|
| Profile Linking | Connect member to DUPR account |
| Rating Display | Show DUPR rating on member profile |
| Auto-Sync | Periodic rating updates |
| Match Submission | Submit match results to DUPR |

**Ratings-Based Features:**
| Feature | Description |
|---------|-------------|
| Event Restrictions | Limit events by rating range (e.g., 3.5-4.0) |
| Skill-Based Matchmaking | Suggest opponents at similar level |
| Division Assignment | Auto-place in appropriate division |
| Rating Verification | Require DUPR for certain events |

**Coach-Assigned Ratings:**
| Feature | Description |
|---------|-------------|
| Staff Rating Override | Pros can assign ratings |
| New Player Assessment | Rate players without DUPR history |
| Rating Appeals | Process for disputing ratings |

### 9.7 Lesson & Program Management

**Lesson Scheduling:**
| Feature | Description |
|---------|-------------|
| Pro Availability | Instructors set available hours |
| Lesson Types | Private, semi-private, group |
| Duration Options | 30, 60, 90 minute lessons |
| Court Auto-Assignment | Book court with lesson |
| Pro Profiles | Bios, specialties, certifications |

**Lesson Packages:**
| Feature | Description |
|---------|-------------|
| Package Creation | Bundle lessons at discount (e.g., 5 for $X) |
| Package Tracking | Remaining lessons in package |
| Expiration | Package validity period |
| Transferability | Can package be shared/gifted |

**Clinics & Classes:**
| Feature | Description |
|---------|-------------|
| Multi-Participant Sessions | Group instruction |
| Skill Level Filtering | Beginner, intermediate, advanced |
| Recurring Classes | Weekly clinics |
| Min/Max Enrollment | Cancel if min not met |
| Waitlist | For popular sessions |

**Camps & Programs:**
| Feature | Description |
|---------|-------------|
| Multi-Day Programs | Summer camps, boot camps |
| Age Groups | Junior, adult, senior programs |
| Package Pricing | Full camp vs. daily rate |
| Materials Included | Equipment rental in price |

### 9.8 Reporting & Analytics Dashboard

**Court Utilization:**
| Metric | Description | Benchmark |
|--------|-------------|-----------|
| Occupancy Rate | % of available slots booked | 70-80% peak |
| Peak Hour Analysis | Busiest times of day/week | Identify patterns |
| Court Comparison | Usage across different courts | Balance load |
| Cancellation Rate | % of bookings cancelled | < 10% |

**Financial Analytics:**
| Metric | Description |
|--------|-------------|
| Revenue by Category | Memberships, lessons, court fees, retail |
| Revenue Trends | Month-over-month, year-over-year |
| Average Transaction | Mean $ per transaction |
| Revenue per Member | ARPM tracking |

**Member Analytics:**
| Metric | Description | Benchmark |
|--------|-------------|-----------|
| Member Retention | % retained year-over-year | 80%+ |
| Churn Rate | Members lost per period | < 20% |
| New Member Growth | Acquisition rate | Track trend |
| Visit Frequency | Average visits per member | Engagement proxy |
| Guest Conversion | Guests who become members | Acquisition funnel |

**Operational Reports:**
| Report | Description |
|--------|-------------|
| Daily Summary | Bookings, revenue, check-ins |
| Staff Performance | Lessons taught, revenue generated |
| Inventory Status | Pro shop stock levels |
| Maintenance Log | Equipment/facility issues |

### 9.9 Booking Rules & Quotas

**Advance Booking Limits:**
| Setting | Description | Example |
|---------|-------------|---------|
| Booking Window | How far ahead can book | 7 days max |
| Member Tier Windows | Premium books earlier | VIP: 14 days, Basic: 3 days |
| Same-Day Booking | Rules for day-of reservations | After 6am only |

**Quotas:**
| Setting | Description | Example |
|---------|-------------|---------|
| Daily Booking Limit | Max reservations per day | 2 per member |
| Weekly Booking Limit | Max per week | 10 per member |
| Concurrent Reservations | Active future bookings | Max 3 at a time |
| Court Time Limit | Max consecutive hours | 2 hours max |

**Buffer Times:**
| Setting | Description |
|---------|-------------|
| Between Bookings | Gap for court maintenance | 15 minutes |
| Setup Time | Time before event starts | 30 minutes |
| Cleanup Time | Time after event ends | 15 minutes |

**Pricing Rules:**
| Setting | Description |
|---------|-------------|
| Peak/Off-Peak Pricing | Higher rates during busy times |
| Member vs. Guest Pricing | Discounts for members |
| Dynamic Pricing | Adjust based on demand |
| Minimum Duration | Shortest bookable slot |

### 9.10 Access Control & Check-In

**Check-In Methods:**
| Method | Description |
|--------|-------------|
| QR Code Scan | Member scans code at kiosk |
| Member Card | Physical card with barcode/NFC |
| Mobile App | Check in via smartphone |
| Front Desk | Staff manual check-in |
| Facial Recognition | Photo-based verification (future) |

**Kiosk Mode:**
| Feature | Description |
|---------|-------------|
| Self-Service Terminal | Dedicated check-in station |
| Waiver Signing | Complete waivers at kiosk |
| Guest Registration | Walk-in guest sign-up |
| Payment Processing | Pay at kiosk |

**Physical Access:**
| Feature | Description |
|---------|-------------|
| Key Fob Integration | Electronic door access |
| Gate Control | Automated gate opening |
| Court Lighting | Auto-on with booking |
| Locker Access | Electronic locker assignment |

**Verification:**
| Feature | Description |
|---------|-------------|
| Photo ID Check | Compare photo to member |
| Waiver Status | Block if waiver expired |
| Payment Status | Block if balance due |
| Membership Status | Verify active membership |

### 9.11 Waiver & Compliance Management

**Digital Waivers:**
| Feature | Description |
|---------|-------------|
| Electronic Signature | Legally binding e-signatures |
| Waiver Templates | Customizable waiver documents |
| Minor Waivers | Parent/guardian signatures |
| Waiver Storage | Secure document retention |

**Waiver Lifecycle:**
| Feature | Description |
|---------|-------------|
| Expiration Tracking | Annual renewal reminders |
| Version Control | Track waiver document changes |
| Re-signing Prompts | Block booking if expired |
| Audit Trail | Who signed what, when |

**Insurance & Compliance:**
| Feature | Description |
|---------|-------------|
| Insurance Upload | Store proof of insurance |
| Certificate Tracking | Instructor certifications |
| Background Checks | Staff verification records |
| Safety Inspections | Facility inspection logs |

### 9.12 Guest & Drop-In Management

**Guest Passes:**
| Feature | Description |
|---------|-------------|
| Guest Pass Types | Day pass, week pass, punch card |
| Guest Pricing | Non-member rates |
| Guest Limits | Max guests per member per month |
| Guest Registration | Quick sign-up flow |

**Drop-In Play:**
| Feature | Description |
|---------|-------------|
| Open Play Sessions | Scheduled drop-in times |
| Skill-Based Sessions | Beginner, intermediate, advanced |
| Sign-Up Visibility | See who else is signed up |
<!-- BEGIN WIP: STORY-0005 -->
| Auto-Court Assignment | Balance player count per court |
<!-- END WIP -->
| Rotation System | Fair play time distribution |

**Guest Conversion:**
| Feature | Description |
|---------|-------------|
| Trial Memberships | Limited-time full access |
| Promotional Offers | First-visit discounts |
| Follow-Up Campaigns | Email after guest visits |
| Conversion Tracking | Guest-to-member analytics |

### 9.13 Pro Shop & Retail

**Inventory Management:**
| Feature | Description |
|---------|-------------|
| Product Catalog | Paddles, balls, apparel, accessories |
| Stock Tracking | Real-time inventory levels |
| Low Stock Alerts | Automatic reorder notifications |
| Vendor Management | Supplier tracking |

**Point of Sale:**
| Feature | Description |
|---------|-------------|
| Barcode Scanning | Quick product lookup |
| Member Discounts | Auto-apply member pricing |
| Gift Cards | Sell and redeem |
| Returns/Exchanges | Process returns |

**Equipment Rental:**
| Feature | Description |
|---------|-------------|
| Paddle Rental | Track equipment out/in |
| Ball Machine Rental | Hourly rental booking |
| Locker Rental | Assign lockers to members |
| Equipment Condition | Track wear and damage |

### 9.14 Mobile App & Member Portal

**Member Self-Service Portal:**
| Feature | Description |
|---------|-------------|
| Online Booking | Reserve courts from any device |
| Booking History | View past and upcoming reservations |
| Payment History | See charges and receipts |
| Profile Management | Update contact info, photo |
| Membership Management | Upgrade, renew, cancel |

**Mobile App Features:**
| Feature | Description |
|---------|-------------|
| Native/PWA App | iOS, Android, or Progressive Web App |
| Push Notifications | Real-time alerts |
| Mobile Check-In | Check in via phone |
| Digital Member Card | QR code for access |
| Offline Mode | View bookings without connection |

**Additional Portal Features:**
| Feature | Description |
|---------|-------------|
| League Sign-Up | Register for leagues/tournaments |
| Lesson Booking | Schedule with pros |
| Waitlist Management | Join/leave waitlists |
| Guest Booking | Reserve for guests |
| Message Center | View communications |

### 9.15 Staff Management

**Scheduling:**
| Feature | Description |
|---------|-------------|
| Shift Management | Create and assign shifts |
| Availability | Staff set available times |
| Time-Off Requests | Request and approve PTO |
| Schedule Publishing | Share schedules with staff |

**Payroll & Compensation:**
| Feature | Description |
|---------|-------------|
| Hours Tracking | Clock in/out |
| Commission Tracking | Pro lesson commissions |
| Tip Management | Process and distribute tips |
| Payroll Export | Integration with payroll systems |

**Performance:**
| Feature | Description |
|---------|-------------|
| Lesson Stats | Lessons taught, revenue |
| Customer Ratings | Member feedback on pros |
| Utilization | % of available time booked |

### 9.16 External Integrations

**Calendar Sync:**
| Integration | Description |
|-------------|-------------|
| Google Calendar | Sync bookings to personal calendar |
| Apple Calendar | iCal feed support |
| Outlook | Microsoft calendar integration |

**Accounting:**
| Integration | Description |
|-------------|-------------|
| QuickBooks | Sync transactions and invoices |
| Xero | Alternative accounting platform |
| Bank Feeds | Direct bank reconciliation |

**Communication:**
| Integration | Description |
|-------------|-------------|
| Twilio | SMS notifications |
| SendGrid | Transactional email |
| Mailchimp | Marketing campaigns |

**Other:**
| Integration | Description |
|-------------|-------------|
| Zapier | Connect to 5000+ apps |
| Webhooks | Real-time event notifications |
| API | Public API for custom integrations |

### 9.17 Family & Group Memberships

**Family Accounts:**
| Feature | Description |
|---------|-------------|
| Household Linking | Connect family members |
| Primary Account Holder | Billing goes to one person |
| Dependent Management | Add/remove family members |
| Shared Benefits | Pool court hours, guest passes |

**Group Memberships:**
| Feature | Description |
|---------|-------------|
| Corporate Memberships | Company-sponsored accounts |
| Group Discounts | Volume pricing |
| Admin Portal | Company admin manages members |
| Billing Options | Individual or consolidated |

### 9.18 Maintenance & Equipment Management

**Facility Maintenance:**
| Feature | Description |
|---------|-------------|
| Work Order System | Submit and track issues |
| Preventive Maintenance | Scheduled maintenance tasks |
| Vendor Management | Track contractors |
| Cost Tracking | Maintenance expenses |

**Equipment Lifecycle:**
| Feature | Description |
|---------|-------------|
| Asset Registry | All equipment tracked |
| Maintenance History | Service records |
| Replacement Planning | End-of-life forecasting |
| Depreciation | Financial tracking |

**Inspections:**
| Feature | Description |
|---------|-------------|
| Safety Checklists | Daily/weekly inspections |
| Issue Reporting | Staff report problems |
| Photo Documentation | Visual issue records |
| Compliance Tracking | Regulatory requirements |

---

## 10. NAVIGATION & UI FRAMEWORK

> **Source:** `internal/templates/components/nav/`, `internal/templates/layouts/`
> **GitHub Issues:** None

### 9.1 Application Layout

```
┌─────────────────────────────────────────────────────────────┐
│  [☰]                    [Search...]              [◐] [🔔]  │  ← Top Nav (fixed)
├─────────────────────────────────────────────────────────────┤
│                                                              │
│                     MAIN CONTENT                             │
│                     (pt-16 for nav)                          │
│                                                              │
└─────────────────────────────────────────────────────────────┘

┌──────────────┐
│   SLIDE-OUT  │  ← Menu (z-50, -translate-x-full → 0)
│    MENU      │
│              │
│  Dashboard   │
│  Courts      │
│  Members     │
│  ──────────  │
│  [Avatar]    │
│  Settings    │
└──────────────┘
```

### 9.2 Top Navigation Components

- **Menu Toggle:** Opens slide-out navigation
- **Global Search:** Debounced search (500ms) - ✅ Fixed (#31)
- **Theme Toggle:** Light/dark mode switch
- **Notifications:** Bell icon - TODO

### 9.3 Slide-Out Menu

**Menu Items:**
- Dashboard
- Courts
- Member Management
- Settings

**User Section:**
- Avatar placeholder
- Name display
- Email display
- Settings link

### 9.4 HTMX Patterns Used

| Pattern | Usage |
|---------|-------|
| `hx-get` | Load content fragments |
| `hx-post` | Form submissions |
| `hx-put` | Update operations |
| `hx-delete` | Delete operations |
| `hx-trigger` | Custom events, delays |
| `hx-target` | Where to swap content |
| `hx-swap` | How to swap (innerHTML, outerHTML) |
| `HX-Trigger` | Server-sent events |
| `HX-Retarget` | Dynamic target change |
| `HX-Reswap` | Dynamic swap method |

---

## 11. TECHNICAL ARCHITECTURE

### 11.1 Technology Stack

**Backend:**
| Component | Technology |
|-----------|------------|
| Language | Go 1.22+ (toolchain 1.23.1) |
| HTTP Server | Standard library `net/http` |
| Templating | Templ (type-safe HTML) |
| Database | SQLite (via mattn/go-sqlite3) |
| Query Generation | SQLC |
| Migrations | golang-migrate |
| Logging | Zerolog |
| Auth | AWS Cognito SDK v2 |
| UUIDs | google/uuid |
| Config | YAML + godotenv |

**Frontend:**
| Component | Technology |
|-----------|------------|
| Interactivity | HTMX 1.9.10 |
| Styling | Tailwind CSS 3.4 |
| Build | PostCSS + autoprefixer |
| Camera | MediaDevices API |

### 11.2 Project Structure

```
pickleicious/
├── cmd/
│   ├── server/              # Main application
│   ├── dbtools/migrate/     # Migration tool
│   └── tools/dbmigrate/     # Alternative migration tool
├── internal/
│   ├── api/                 # HTTP handlers
│   │   ├── middleware.go    # Logging, recovery, request ID
│   │   ├── auth/            # Authentication handlers
│   │   ├── courts/          # Court/calendar handlers
│   │   ├── members/         # Member CRUD handlers
│   │   └── nav/             # Navigation handlers
│   ├── config/              # Configuration loading
│   ├── db/
│   │   ├── db.go            # Database wrapper
│   │   ├── migrations/      # SQL migration files
│   │   ├── queries/         # SQLC query files
│   │   ├── schema/          # Master schema
│   │   └── generated/       # SQLC output
│   ├── models/              # Domain models
│   └── templates/
│       ├── layouts/         # Base HTML layout
│       └── components/      # Templ components
├── tools/
│   ├── auto_stories/        # GitHub issue generator
│   └── svg_tools/           # SVG utilities
├── web/
│   ├── static/js/           # Client-side JavaScript
│   └── styles/              # Tailwind source CSS
├── assets/
│   ├── stories/             # Story YAML files
│   └── themes/              # Theme color definitions
├── .air.toml                # Hot reload config
└── config.yaml              # App configuration
```

### 11.3 Build System (Taskfile)

**Environments:**
- `build:prod` (minified binary flags)

**Key Tasks:**
| Target | Description |
|--------|-------------|
| `generate` | Compile .templ files and sqlc queries |
| `css` | Build CSS from Tailwind |
| `db:migrate` | Run migrations |
| `build` | Build server binary |
| `build:prod` | Build server binary for production |
| `dev` | Run development server |

### 11.4 Configuration

**config.yaml:**
```yaml
app:
  name: "Pickleicious"
  environment: "development"
  port: 8080
  base_url: "http://localhost:8080"

database:
  driver: "sqlite"  # or "turso"
  filename: "build/db/pickleicious.db"

features:
  enable_metrics: false
  enable_tracing: false
  enable_debug: true
```

**Environment Variables:**
| Variable | Purpose |
|----------|---------|
| `APP_SECRET_KEY` | Application secret |
| `DATABASE_AUTH_TOKEN` | Turso auth token |
| `GITHUB_API_KEY` | For auto_stories tool |
| `STATIC_DIR` | Override static file path |

### 11.5 Middleware Stack

```go
handler := api.ChainMiddleware(
    router,
    api.WithLogging,      // Request/response logging
    api.WithRecovery,     // Panic recovery
    api.WithRequestID,    // UUID per request
    api.WithContentType,  // Default Accept header
)
```

### 11.6 API Routes

| Method | Path | Handler |
|--------|------|---------|
| GET | `/` | Base layout |
| GET | `/health` | Health check |
| GET | `/api/v1/nav/menu` | Load menu HTML |
| GET | `/api/v1/nav/menu/close` | Clear menu |
| GET | `/api/v1/nav/search` | Global search (TODO) |
| GET | `/members` | Members page |
| GET | `/api/v1/members` | List members |
| POST | `/api/v1/members` | Create member |
| GET | `/api/v1/members/search` | Search members |
| GET | `/api/v1/members/new` | New member form |
| GET | `/api/v1/members/{id}` | Member detail |
| GET | `/api/v1/members/{id}/edit` | Edit form |
| PUT | `/api/v1/members/{id}` | Update member |
| DELETE | `/api/v1/members/{id}` | Delete member |
| GET | `/api/v1/members/{id}/billing` | Billing info |
| GET | `/api/v1/members/photo/{id}` | Member photo |
| POST | `/api/v1/members/restore` | Restore/create decision |
| GET | `/courts` | Courts page |
| GET | `/api/v1/courts/calendar` | Calendar view |

---

## 12. CI/CD & INFRASTRUCTURE

> **Source:** GitHub Issue #29
> **GitHub Issues:** #9, #29

### 12.1 GitHub Repository Setup

> **GitHub Issue #9 [IS1]:** GitHub Repository Setup (partially complete)

**Acceptance Criteria:**
- [x] Repository created with README.md
- [x] .gitignore configured for Go, JS, SQLite
- [x] Branch protection rules established
- [ ] Basic GitHub Actions workflow configured (#29)
- [x] Project board setup with Kanban structure

### 12.2 GitHub Actions (Planned)

> **GitHub Issue #29:** Basic GitHub Actions workflow

**Planned Workflows:**

1. **Check Tests**
   - Run on push to main and PRs
   - `npm test` / `go test`

2. **Dependency Management**
   - Weekly vulnerability audit
   - `npm audit` / `go mod verify`

3. **Release Automation**
   - Trigger on version tags (v*.*.*)
   - Auto-create GitHub releases

4. **Documentation**
   - Deploy auto-generated docs to gh-pages

5. **Notifications**
   - Slack notifications on workflow completion

---

## 13. IMPLEMENTATION STATUS

### 13.1 Completed Features

| Feature | Status | GitHub Issue |
|---------|--------|--------------|
| Member CRUD | ✅ Complete | — |
| Member search/pagination | ✅ Complete | #31 (fixed) |
| Member photos | ✅ Complete | — |
| Member billing info | ✅ Complete | — |
| Member restoration | ✅ Complete | #30 (closed) |
| Court calendar UI | ✅ Basic | — |
| Navigation menu | ✅ Complete | — |
| Dark mode toggle | ✅ Complete | — |
| Login page UI | ✅ Complete | — |
| Database layer | ✅ Complete | #4, #5 |
| Build system | ✅ Complete | — |
| GitHub repo setup | ✅ Complete | #1 (closed) |
| Dev environment docs | ✅ Complete | #2 (closed) |

### 13.2 In Progress / TODO

| Feature | Status | GitHub Issue |
|---------|--------|--------------|
| ~~Fix member search~~ | ✅ Fixed | #31 |
| Cognito auth integration | ❌ TODO | — |
| Theme management system | ❌ TODO | #17-22 |
| Localization system | ❌ TODO | #23-28 |
| GitHub Actions CI/CD | ❌ TODO | #29 |
| Open play rules | ❌ TODO | #32 |
| Global search | ❌ TODO | — |
| Transactions table | ❌ TODO | — |

### 13.3 TODO Items (From Code)

| Location | TODO |
|----------|------|
| auth/handlers.go:106 | Cognito client initialization |
| auth/handlers.go:109 | Cognito code sending |
| auth/handlers.go:140 | Cognito verification |
| auth/handlers.go:143 | Update cognito_status in DB |
| auth/handlers.go:145 | Session/JWT setup |
| auth/handlers.go:169 | Password verification |
| auth/handlers.go:176 | Password reset flow |
| auth/handlers.go:190 | Cognito code resending |
| nav/handlers.go:20 | Search functionality |
| schema.sql:78 | Transactions table |
| schema.sql:79 | Products table |

---

## 14. DATABASE SCHEMA REFERENCE

### 14.1 Complete Table List

| Table | Purpose |
|-------|---------|
| `organizations` | Top-level tenant entities |
| `facilities` | Physical locations |
| `operating_hours` | Per-facility schedules |
| `users` | Authentication records |
| `members` | Customer profiles |
| `member_billing` | Payment information |
| `member_photos` | Photo BLOB storage |
| `staff` | Employee records |
| `courts` | Court definitions |
| `reservation_types` | Booking type lookup |
| `recurrence_rules` | Recurring patterns |
| `reservations` | Booking records |
| `reservation_courts` | Multi-court junction |
| `reservation_participants` | Multi-member junction |
| `cognito_config` | Per-org auth settings |

### 14.2 Key Constraints

- `organizations.slug` - UNIQUE
- `facilities.slug` - UNIQUE
- `users.email` - UNIQUE
- `courts(facility_id, court_number)` - UNIQUE
- `operating_hours(facility_id, day_of_week)` - UNIQUE
- `member_photos.member_id` - UNIQUE INDEX
- `reservation_courts(reservation_id, court_id)` - UNIQUE
- `reservation_participants(reservation_id, member_id)` - UNIQUE

---

## 15. GITHUB ISSUES (Complete List)

### Open Issues

| # | Title | Labels | Category |
|---|-------|--------|----------|
| 32 | Open play cancellation rules | — | Feature |
| 29 | GitHub Actions workflow | — | CI/CD |
| 28 | [L6] Localization Testing Framework | localization, testing | L |
| 27 | [L5] Translation Management Interface | admin, localization | L |
| 26 | [L4] Date, Time, Number Formatting | formatting, localization | L |
| 25 | [L3] Locale Detection and Switching | localization, user-experience | L |
| 24 | [L2] Message Extraction and Bundle Management | localization, tooling | L |
| 23 | [L1] Localization Core Infrastructure | infrastructure, localization | L |
| 22 | [TH3.3] Theme Preference Management | themes, user-preferences | TH |
| 21 | [TH3.0] Light/Dark Theme Variant System | themes, dark-mode | TH |
| 20 | [TH2.1] Core Theme Palette Definitions | themes, design-system | TH |
| 19 | [TH2.0] Simple Theme Implementation | themes, default-theme | TH |
| 18 | [TH1.0] Theme Component Definitions | themes, design-system | TH |
| 17 | [TH0.1] Theme Data Structure | themes, infrastructure | TH |
| 16 | [SV2] Hot Reload Setup | server, development | SV |
| 15 | [SV1] Basic Go Server | infrastructure, server | SV |
| 14 | [DB3] Default Theme Data | database, themes | DB |
| 13 | [DB2] Migration System | database, infrastructure | DB |
| 12 | [DB1] SQLite Schema Design | database, design | DB |
| 11 | [IS3] Project Directory Structure | infrastructure, setup | IS |
| 10 | [IS2] Dev Environment Docs | documentation, setup | IS |
| 9 | [IS1] GitHub Repo Setup | infrastructure, setup | IS |

### Closed Issues

| # | Title | Labels | Resolution |
|---|-------|--------|------------|
| 31 | Search is broken | — | Fixed (2025-12-24) |
| 30 | Deleted user re-add conflict | — | Implemented |
| 2 | [IS2] Dev Environment Docs | documentation, setup | Completed |
| 1 | [IS1] GitHub Repo Setup | infrastructure, setup | Completed |

### Duplicate Issues (3-8 duplicate 9-16)

Issues 3-8 are duplicates of issues 9-16 (same content, created twice).

---

## 16. FUTURE ROADMAP

### 16.1 Near-Term (Based on TODOs + Bugs)
1. ~~Fix member search (#31)~~ ✅ Done
2. Complete Cognito authentication integration
3. Implement global search
4. Add transactions/billing tables

### 16.2 Medium-Term (Based on GitHub Issues)
1. Theme management system (#17-22)
2. Localization system (#23-28)
3. GitHub Actions CI/CD (#29)
4. Open play rules (#32)

### 16.3 Long-Term (Based on Section 9 - Planned Requirements)
1. Payment processing system (9.1)
2. Communication system (9.2)
3. Waitlist management (9.3)
4. Cancellation policies & no-show tracking (9.4)
5. League & tournament management (9.5)
6. DUPR integration (9.6)
7. Lesson & program management (9.7)
8. Reporting & analytics dashboard (9.8)
9. Mobile app / member portal (9.14)
10. Turso cloud database migration
11. Multi-facility reporting

---

*Document generated from codebase analysis and GitHub issues.*
*Last updated: December 2024*
*Repository: github.com/codr1/pickleicious*
