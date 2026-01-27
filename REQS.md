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
| Authentication | ❌ | ❌ | ✅ Cognito EMAIL_OTP | Logout/reset pending |
| Open Play Rules | #32 | ❌ | ❌ | New feature request |
| **Planned (Section 9)** | ❌ | ❌ | ❌ | 18 feature areas identified |

---

## 0. INFRASTRUCTURE DEBT (Priority)

### 0.0 Member Booking Enhancements (NEXT)

**Context**: Follow-up from member-booking feature (STORY-0021). These are enhancements identified during implementation.

#### 0.0.1 Simple Date Picker for Member Booking

#### 0.0.2 Member Reservation Limits

---

### 0.1 Comprehensive Test Infrastructure

## 1. EXECUTIVE SUMMARY

### 1.1 Product Vision
**Pickleicious** is a comprehensive **multi-tenant SaaS platform for pickleball facility management**. It enables indoor pickleball venues to manage court reservations, member profiles, staff operations, and facility scheduling through a modern web interface optimized for front-desk operations.

### 1.2 Target Market
- Indoor pickleball facilities (dedicated courts)
- Multi-location pickleball chains/franchises
- Recreation centers with pickleball programs
- Private clubs with pickleball amenities

### 1.3 Key Value Propositions
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

### 3.4 Member Billing Information

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


---

## 4. AUTHENTICATION SYSTEM

> **Source:** `internal/api/auth/handlers.go` (TODOs), login templates
> **GitHub Issues:** None (not tracked as stories)

### 4.1 Authentication Architecture


### 4.2 Login UI Flow (HTMX-Powered)

**Step 1: Identifier Entry**
- On blur, checks if identifier belongs to staff with local auth
- If yes, shows "Sign in with staff credentials" link

**Step 2: Code Request**
- Sends verification code via Cognito
- Replaces form with code entry UI


### 4.3 Pending

| Feature | Status |
|---------|--------|
| Password reset | ❌ TODO |

---

## 5. MEMBER MANAGEMENT OPERATIONS

> **Source:** `internal/api/members/handlers.go`, member templates

### 5.1 Member CRUD Operations

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

### 5.2 Member Detail View

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


### 6.7 Open Play Rules (NEW)

> **GitHub Issue #32:** Open play cancellation and auto-scaling


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

**Visual Grid:**
```
         Court 1  Court 2  Court 3  Court 4  Court 5  Court 6  Court 7  Court 8
 6:00   [      ] [      ] [      ] [      ] [      ] [      ] [      ] [      ]
 7:00   [      ] [      ] [      ] [      ] [      ] [      ] [      ] [      ]
 ...
22:00   [      ] [      ] [      ] [      ] [      ] [      ] [      ] [      ]
```


### 6.9 Calendar Interactions (Planned)

These calendar interactions are designed but not yet implemented:

**Views:**
- Week View: One court across a week for spotting patterns
- Month View: Overview of all reservations
- Agenda View: Chronological list of upcoming reservations


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
- [ ] Hover and active states defined
- [ ] Theme preview demonstrates all components

### 7.4 Light/Dark Mode System

> **GitHub Issue #21 [TH3.0]:** Light/Dark Theme Variant System


### 7.5 Theme Preference Management

> **GitHub Issue #22 [TH3.3]:** Theme Preference Management System


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



### 9.4 Cancellation Policies & No-Show Management

> **Implemented:** STORY-0025 (Reservation Cancellation Policies) - merged 2026-01-08

**Current Implementation:**
- Cancellation policies per reservation type (full refund window, partial refund percentage)
- Staff users see confirmation prompt with fee waiver option (lines 707-728 in handlers.go)
- Cancellation logs track all cancellations with policy applied



### 9.5 League & Tournament Management


**Tournament Features:**
| Feature | Description |
|---------|-------------|
| Bracket Types | Single elimination, double elimination, round robin |
| Seeding | Manual or ratings-based seeding |
| Court Assignment | Auto-assign matches to courts |
| Score Entry | Staff or self-reported scores |
| Live Brackets | Real-time bracket updates |
| Prize Management | Track and award prizes |


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

> **Implemented:** STORY-0026 (Member Portal Lesson Booking MVP) - in progress

**Current Implementation (MVP):**
- Members can book 1-hour lessons with teaching pros at their home facility
- Pro unavailability management (pros can block their own time)
- `lesson_min_notice_hours` facility setting (default: 24 hours) - booking must be at least this far in advance
- Lessons count toward max_member_reservations limit

**Lesson Packages:**
| Feature | Description |
|---------|-------------|
| Package Creation | Bundle lessons at discount (e.g., 5 for $X) |
| Package Tracking | Remaining lessons in package |
| Expiration | Package validity period |
| Transferability | Can package be shared/gifted |

<!-- BEGIN WIP: STORY-0043 -->
**Clinics & Classes:**
| Feature | Description |
|---------|-------------|
| Multi-Participant Sessions | Group instruction |
| Skill Level Filtering | Beginner, intermediate, advanced |
| Recurring Classes | Weekly clinics |
| Min/Max Enrollment | Cancel if min not met |
| Waitlist | For popular sessions |
<!-- END WIP -->

**Camps & Programs:**
| Feature | Description |
|---------|-------------|
| Multi-Day Programs | Summer camps, boot camps |
| Age Groups | Junior, adult, senior programs |
| Package Pricing | Full camp vs. daily rate |
| Materials Included | Equipment rental in price |

**Coach Marketplace & Discovery:**

A self-service marketplace where members discover and book coaches directly, reducing front-desk overhead while increasing lesson bookings.

| Instant Booking | Members book directly without staff intervention |
| Rating & Reviews | 5-star ratings with written reviews from past students |
| Response Time Badge | Shows coach's typical response time to booking requests |
| Featured Coaches | Facility can highlight top coaches or new instructors |

**Coach Search & Filtering:**
| Filter | Description |
|--------|-------------|
| Availability | "Available this week", specific date/time |
| Specialty | Serve improvement, strategy, beginner basics, competitive play |
| Price Range | Budget-friendly to premium coaching |
| Rating | Minimum star rating filter |
| Teaching Style | Video analysis, drilling, game-based, technical |
| Language | For multilingual facilities |

**Coach Onboarding:**
| Feature | Description |
|---------|-------------|
| Self-Registration | Coaches apply through portal |
| Credential Verification | Upload certifications for admin review |
| Background Check Integration | Link to third-party verification services |
| Trial Period | New coaches marked as "New" for first 30 days |
| Commission Settings | Per-coach commission rates or flat fees |

**Coach Analytics:**
| Metric | Description |
|--------|-------------|
| Booking Rate | % of available slots booked |
| Student Retention | Repeat booking rate |
| Revenue Generated | Total and per-lesson averages |
| Rating Trends | Rating over time |
| Cancellation Rate | Coach and student cancellations |

### 9.8 Reporting & Analytics Dashboard


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
| Guest Conversion | Guests who become members | Acquisition funnel |

**Operational Reports:**
| Report | Description |
|--------|-------------|
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


**Kiosk Mode:**
| Feature | Description |
|---------|-------------|
| Self-Service Terminal | Dedicated check-in station |
| Waiver Signing | Complete waivers at kiosk |

**Physical Access:**
| Feature | Description |
|---------|-------------|
| Key Fob Integration | Electronic door access |
| Gate Control | Automated gate opening |
| Court Lighting | Auto-on with booking |
| Locker Access | Electronic locker assignment |


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

### 9.19 Player Matchmaking & Social Features

> **Competitive Reference:** PickleUp, Hitch, Playtomic, CourtReserve "Player Matchmaker", DUPR social features
> **Priority:** High - Key differentiator for member retention and community building

Social features transform a facility from a transactional booking platform into a community hub. Players who find regular partners have significantly higher retention rates.

**Player Discovery:**
| Feature | Description |
|---------|-------------|
| Player Profiles | Photo, bio, playing style, availability preferences |
| Skill Level Display | DUPR rating or self-assessed level prominently shown |
| Play Style Tags | "Aggressive", "Strategic", "Social", "Competitive" |
| Availability Status | "Looking to play today", "Available weekends", etc. |
| Home Facility | Primary location for multi-facility orgs |
| Playing History | How long playing, frequency, preferred formats |

**Matchmaking Engine:**
| Feature | Description |
|---------|-------------|
| Skill-Based Matching | Suggest players within 0.5 rating points |
| Schedule Compatibility | Match players with overlapping availability |
| Location Proximity | Prioritize players at same facility |
| Play Style Compatibility | Match complementary or similar styles |
| Gender Preferences | Filter for men's, women's, or mixed play |
| Age Range Filtering | Optional age-based matching |

**Connection & Invitations:**
| Feature | Description |
|---------|-------------|
| Send Play Request | "Want to play Tuesday at 6pm?" with one click |
| Buddy List | Save favorite partners for quick invitations |
| Group Creation | Form regular playing groups (e.g., "Tuesday Night Crew") |
| Broadcast Invite | "I have a court at 4pm, need 3 players" |
| Response Tracking | Yes/No/Maybe with automatic tallying |
| Invite Expiration | Auto-expire unanswered invites |

**Communication:**
| Feature | Description |
|---------|-------------|
| In-App Messaging | Direct messages between members (opt-in) |
| Group Chat | Chat threads for playing groups |
| Activity Feed | See when buddies book courts or join events |
| Privacy Controls | Control who can message you, see your schedule |
| Block/Report | Safety features for unwanted contact |

**Social Visibility:**
| Feature | Description |
|---------|-------------|
| "Who's Playing" | See who else is signed up for open play |
| Court Activity | Live view of who's currently playing |
| Leaderboards | Optional competitive rankings within facility |
| Achievement Badges | "100 games played", "Early Bird", "Social Butterfly" |
| New Member Spotlight | Help new members find their first partners |

**Opt-In Preferences:**
| Setting | Options |
|---------|---------|
| Profile Visibility | Public / Members Only / Private |
| Matchmaking Participation | Active / Passive / Off |
| Contact Preferences | Anyone / Buddies Only / None |
| Activity Sharing | Share play activity with buddies or keep private |

### 9.20 Round Robin & Open Play Engine

> **Competitive Reference:** Pickleheads Round Robin Tool, USAPA Rotation Charts, TopDog, PlayMore AI
> **Priority:** High - Core functionality for recreational facilities
> **Related:** GitHub Issue #32 (Open play cancellation rules)

Open play and round robins are the bread-and-butter of recreational pickleball. A sophisticated rotation engine removes staff burden while ensuring fair, fun play for all participants.

**Session Types:**
| Type | Description |
|------|-------------|
| Traditional Round Robin | Each player/team plays every other player/team once |
| Rotating Partners ("Popcorn") | Partners change each round; maximize mixing |
| King/Queen of the Court | Winners stay, losers rotate off |
| Ladder Play | Skill-based progression up/down courts |
| Swiss System | Match players with similar win records |
| Flex Play | Drop-in/drop-out with dynamic court assignment |

**Rotation Algorithms:**
| Algorithm | Best For | Description |
|-----------|----------|-------------|
| Circle Method | Even players | Classic round robin rotation |
| Balanced Rotation | Mixed skill | Weight matchups by rating differential |
| Random Shuffle | Social play | Randomize each round for variety |
| Skill Clustering | Large groups | Group by skill, rotate within clusters |
| Partner Variety | Doubles | Ensure each player partners with every other |


**Court Assignment:**
| Feature | Description |
|---------|-------------|
| Auto-Assignment | System assigns players to courts each round |
| Court Balancing | Distribute skill levels across courts |
| Rest Rotation | Ensure players get breaks in large groups |
| Bye Handling | Fair distribution when odd number of players |
| Court Preferences | Honor accessibility or skill-level court preferences |

**Scoring & Standings:**
| Feature | Description |
|---------|-------------|
| Score Entry | Self-report via mobile or kiosk |
| Real-Time Standings | Live leaderboard during session |
| Point Differential | Track margin of victory |
| Head-to-Head Records | Personal stats vs. each opponent |
| Session Summary | Email recap with stats after session |
| DUPR Submission | Optional auto-submit to DUPR |

**Timing & Announcements:**
| Feature | Description |
|---------|-------------|
| Round Timer | Configurable game length (e.g., 15 min rounds) |
| Audio Announcements | "Round ending in 2 minutes", "New round starting" |
| Court Assignments Display | Show next matchups on screens or mobile |
| Rotation Alerts | Push notification when your game is ready |
| Overtime Rules | What happens if game runs long |

**Configuration Options:**
| Setting | Options |
|---------|---------|
| Format | Round robin / Rotating partners / King of court |
| Games per Round | 1, 2, or 3 games per matchup |
| Scoring | Rally scoring, traditional, timed |
| Skill Restrictions | Open / Level-restricted (e.g., 3.0-3.5 only) |
| Gender Format | Open / Men's / Women's / Mixed |
| Max Players | Per-session cap with waitlist overflow |

**Staff Tools:**
| Feature | Description |
|---------|-------------|
| Override Matchups | Manual adjustment when needed |
| Force Court Assignment | Place specific players together |
| Pause/Resume | Stop rotation for announcements |
| Print Bracket | Printable rotation schedule |
| Session Templates | Save configurations for recurring sessions |

### 9.21 Challenge Ladder System

> **Competitive Reference:** Global Pickleball Network, TopDog Challenge Ladders, R2 Sports
> **Priority:** High - Self-sustaining engagement with minimal staff oversight

Challenge ladders are player-driven competition systems where participants challenge each other directly, submit their own scores, and rankings update automatically. Once configured, they essentially run themselves.

**Ladder Types:**
| Type | Description |
|------|-------------|
| Open Challenge | Challenge anyone within X positions above you |
| Tiered Ladder | Divisions (A/B/C) with promotion/relegation |
| Points-Based | Accumulate points; rankings by total points |
| ELO-Style | Rating adjusts based on opponent strength |
| Seasonal | Reset periodically with finals for top players |
| Doubles Ladder | Team-based challenge ladder |

**Challenge Rules:**
| Rule | Description |
|------|-------------|
| Challenge Range | How many positions up can you challenge (e.g., 3 spots) |
| Challenge Frequency | Max challenges per week (e.g., 2 per player) |
| Response Deadline | Days to accept/decline challenge (e.g., 3 days) |
| Default Win | Challenged player who doesn't respond loses by default |
| Decline Limits | Max declines before forced match or penalty |
| Re-Challenge Cooldown | Days before challenging same player again |

**Ranking Algorithms:**
| Algorithm | Description |
|-----------|-------------|
| Position Swap | Winner takes loser's position if higher |
| Weighted Swap | Bigger jump for beating higher-ranked opponent |
| Points Accumulation | Win = X points, bonus for upset |
| ELO Rating | Mathematical rating based on expected vs. actual results |
| Activity Decay | Inactive players drift down over time |
| Win Streak Bonus | Extra ranking boost for consecutive wins |
| Head-to-Head Tiebreaker | Direct matchup history breaks ties |

**Match Scheduling:**
| Feature | Description |
|---------|-------------|
| Challenge Request | Send challenge via app with proposed times |
| Availability Sharing | Show your open slots to potential challengers |
| Court Auto-Booking | System books court when match is scheduled |
| Reminder Notifications | Automated reminders before match |
| Reschedule Workflow | Request/approve time changes |
| Match Deadline | Complete match within X days of challenge |

**Score Reporting:**
| Feature | Description |
|---------|-------------|
| Self-Report | Players submit their own scores |
| Dual Confirmation | Both players must confirm result |
| Dispute Resolution | Flag discrepancies for admin review |
| Photo Proof | Optional score sheet photo upload |
| DUPR Sync | Auto-submit verified results to DUPR |

**Standings & Visibility:**
| Feature | Description |
|---------|-------------|
| Live Leaderboard | Real-time ranking display |
| Movement Arrows | Show who's rising/falling |
| Match History | Full history of all ladder matches |
| Player Stats | W/L record, win %, streak, avg margin |
| Recent Activity | Who challenged whom, recent results |
| Inactive Indicator | Flag players who haven't played recently |

**Seasons & Events:**
| Feature | Description |
|---------|-------------|
| Season Length | Define ladder seasons (e.g., quarterly) |
| Season Reset | Full reset or partial position reset |
| Playoff Qualification | Top X players qualify for season-end tournament |
| Prize/Recognition | Badges, prizes for season winners |
| Historical Archives | Past season standings preserved |

**Administration:**
| Feature | Description |
|---------|-------------|
| Ladder Creation Wizard | Easy setup with templates |
| Rule Configuration | All challenge rules adjustable |
| Player Management | Add/remove players, handle disputes |
| Activity Reports | Identify inactive players, popular matchups |
| Announcement System | Post updates to ladder participants |
| Auto-Cleanup | Archive completed seasons, prune inactive players |

### 9.22 AI-Powered Features

> **Competitive Reference:** Wellyx AI marketing, PlayMore AI matchmaking
> **Priority:** Medium - Nice-to-have enhancement layer

AI capabilities can enhance personalization, automate routine decisions, and surface insights that would be impossible to derive manually.

**Smart Matchmaking:**
| Feature | Description |
|---------|-------------|
| Compatibility Scoring | ML model predicts good partner/opponent matches |
| Play Style Analysis | Infer style from game history and behavior |
| Optimal Pairing | Suggest doubles partners for balanced teams |
| Skill Progression Prediction | Estimate player improvement trajectory |
| Churn Risk Detection | Flag members at risk of not returning |

**Intelligent Scheduling:**
| Feature | Description |
|---------|-------------|
| Demand Prediction | Forecast busy times based on historical data |
| Dynamic Pricing Suggestions | Recommend price adjustments for utilization |
| Staff Scheduling Optimization | Suggest staffing levels by predicted demand |
| Court Maintenance Timing | Recommend maintenance windows with least impact |
| Event Timing Recommendations | Best times to schedule leagues, clinics |

**Personalized Recommendations:**
| Feature | Description |
|---------|-------------|
| "Players Like You" | Recommend partners based on similar members |
| Event Suggestions | Personalized event recommendations |
| Lesson Recommendations | Suggest coaches/lessons based on improvement areas |
| Content Personalization | Tailor emails and notifications to interests |
| Optimal Booking Times | Suggest times when preferred courts/partners available |

**Marketing Automation:**
| Feature | Description |
|---------|-------------|
| Smart Segmentation | Auto-segment members by behavior patterns |
| Send Time Optimization | Deliver messages when most likely to engage |
| Subject Line Optimization | A/B test and learn best messaging |
| Win-Back Campaigns | Automated re-engagement for lapsed members |
| Upsell Targeting | Identify members likely to upgrade |

**Operational Intelligence:**
| Feature | Description |
|---------|-------------|
| Anomaly Detection | Flag unusual patterns (fraud, abuse, errors) |
| Review Sentiment Analysis | Categorize feedback as positive/negative/neutral |
| FAQ Auto-Response | Answer common questions automatically |
| Predictive Maintenance | Flag equipment likely to need service |
| Revenue Forecasting | Project future revenue based on trends |

**Natural Language Features:**
| Feature | Description |
|---------|-------------|
| Voice Booking | "Book a court tomorrow at 6pm" |
| Chat Assistant | Answer member questions about schedules, rules |
| Smart Search | Understand natural language queries |
| Automated Summaries | Generate session recaps, performance reports |

### 9.23 Autonomous & Unstaffed Operations

> **Competitive Reference:** PodPlay autonomous tier, Pickle Parlor, Pickleball 365, Rhombus/Latitude Security
> **Priority:** Medium - Significant cost savings for right facility types
> **Best For:** Smaller facilities (2-6 courts), off-peak hours, 24/7 operations

Autonomous operation enables facilities to run with minimal or zero on-site staff, dramatically reducing labor costs while enabling 24/7 access. This model is growing rapidly, with facilities reporting up to 90% reduction in staffing needs.

**Access Control Integration:**
| Feature | Description |
|---------|-------------|
| Smart Lock Integration | RemoteLock, Salto, Kisi, August compatible |
| PIN Code Access | Unique codes per booking or member |
| Mobile Unlock | One-tap door unlock in app |
| QR Code Entry | Scan to enter |
| Geofenced Auto-Unlock | Door unlocks when member approaches |
| Access Scheduling | Door only unlocks during booked time window |
| Emergency Override | Staff can remotely lock/unlock |

**Booking-Triggered Automation:**
| Feature | Description |
|---------|-------------|
| Automatic Door Access | Unlock X minutes before booking starts |
| Lighting Control | Courts lights on/off with bookings |
| HVAC Scheduling | Climate control synced to occupancy |
| Court Power | Ball machines, scoreboards activate with booking |
| Music/Ambiance | Automated audio system control |
| Access Revocation | Auto-lock after booking ends + grace period |

**Remote Monitoring:**
| Feature | Description |
|---------|-------------|
| Camera Integration | Live view of all courts and common areas |
| Motion Detection | Alert on activity outside booked times |
| Occupancy Counting | Track number of people present |
| Incident Recording | Auto-save clips on triggered events |
| Two-Way Audio | Staff can speak to members via speakers |
| Remote Assistance | Video call with support agent |

**Self-Service Operations:**
| Feature | Description |
|---------|-------------|
| Kiosk Check-In | Self-service terminal for walk-ins |
| Equipment Lockers | Smart lockers for rental paddles, balls |
| Vending Integration | Snack/beverage machines |
| Self-Checkout Pro Shop | Scan and pay without staff |
| Waiver Kiosk | Complete waivers on-site |
| Payment Terminal | Accept payments 24/7 |

**Automated Communication:**
| Feature | Description |
|---------|-------------|
| Pre-Arrival Instructions | Email/SMS with access codes, rules |
| Voice Announcements | "Your session ends in 5 minutes" |
| Emergency Broadcasts | Facility-wide audio announcements |
| Violation Warnings | "Please return equipment to designated area" |
| Session Reminders | "Court 3, your next booking starts in 10 minutes" |

**Security & Safety:**
| Feature | Description |
|---------|-------------|
| 24/7 Monitoring Center | Remote security team watches cameras |
| Panic Button | Emergency alert in app and on-site |
| Incident Response | Protocol for security, medical, facility issues |
| Trespasser Detection | Alert on unrecognized individuals |
| Damage Documentation | Photo/video evidence of facility damage |
| Insurance Integration | Incident reports for claims |

**Hybrid Operations:**
| Feature | Description |
|---------|-------------|
| Staffed Hours | Full service during peak times |
| Autonomous Hours | Unstaffed during early morning, late night |
| Transition Alerts | Notify members when switching modes |
| Staff Override | On-call staff can intervene remotely |
| Graduated Rollout | Start autonomous during low-risk hours |

**Member Experience:**
| Feature | Description |
|---------|-------------|
| Tutorial Videos | How to use autonomous features |
| In-App Help | Chat/call support from app |
| Feedback Collection | Rate autonomous experience after visit |
| Issue Reporting | Report problems with photos |
| Lost & Found | Log lost items for later retrieval |

**Financial Impact:**
| Metric | Typical Result |
|--------|----------------|
| Staffing Reduction | Up to 90% reduction |
| Operating Hours | 24/7 vs. 16-18 hours staffed |
| Revenue Increase | 20-40% from extended hours |
| Break-Even Point | Lower revenue threshold for profitability |
| Technology Investment | Higher upfront, lower ongoing |

### 9.24 Video Replay & Digital Scoreboards

> **Competitive Reference:** PodPlay video replays, PlaySight, TopDog digital scoreboards
> **Priority:** Low - Premium feature for differentiation
> **Integration:** DUPR score submission, streaming platforms

Video technology and digital scoreboards create a premium, professional experience that justifies higher pricing and attracts serious players. These features also enable automated score reporting to rating systems like DUPR.

**Digital Scoreboard System:**
| Feature | Description |
|---------|-------------|
| Court-Side Displays | Large screens visible to players and spectators |
| Mobile Scoring | Players update score from phones |
| Physical Buttons | Dedicated scoring buttons on each court |
| Voice Scoring | "Score: 11-9" voice commands |
| Auto-Detection | Computer vision detects points (advanced) |
| Real-Time Sync | All displays update instantly |

**Score Display Features:**
| Feature | Description |
|---------|-------------|
| Player Names | Show player/team names |
| Game Score | Current game score prominently displayed |
| Match Score | Games won in match |
| Server Indicator | Visual cue for serving team |
| Timeout Counter | Track remaining timeouts |
| Game Timer | Optional timed games display |
| Side-Out Indicator | Track side-outs for rally scoring |

**DUPR Integration:**
| Feature | Description |
|---------|-------------|
| DUPR Account Linking | Connect member profiles to DUPR |
| One-Button Submit | Submit match result to DUPR after game |
| Auto-Submit Option | Automatically send all scores to DUPR |
| Match Verification | Both players confirm before submission |
| Rating Display | Show current DUPR ratings on scoreboard |
| Rating Prediction | "If you win, your new rating will be..." |

**Video Recording:**
| Feature | Description |
|---------|-------------|
| Continuous Recording | Record all court activity |
| Per-Court Cameras | Individual camera per court |
| Multi-Angle | Multiple cameras for different views |
| Auto-Start/Stop | Recording synced to booking times |
| Cloud Storage | Secure cloud storage with retention policies |
| Local Backup | On-site backup for reliability |

**Video Replay:**
| Feature | Description |
|---------|-------------|
| Instant Replay | Review last point on court-side display |
| Full Game Access | Watch entire game after session |
| Highlight Clips | Auto-generate highlight moments |
| Slow Motion | Variable speed playback |
| Frame-by-Frame | Precise analysis of technique |
| Mobile Access | Watch replays on phone |

**Sharing & Social:**
| Feature | Description |
|---------|-------------|
| One-Click Sharing | Share clips to social media |
| Branded Overlays | Facility logo/branding on videos |
| Custom Graphics | Player names, scores overlaid |
| Download Options | MP4 download for personal use |
| Privacy Controls | Players control who can access their videos |
| Highlight Reels | Auto-generated best-of compilations |

**Live Streaming:**
| Feature | Description |
|---------|-------------|
| Court Streaming | Live stream any court |
| Multi-Court View | Facility-wide overview stream |
| Public/Private | Control who can view streams |
| Platform Integration | Stream to YouTube, Facebook, Twitch |
| Commentary Support | Audio input for play-by-play |
| Delay Options | Configurable stream delay |

**Player Analysis:**
| Feature | Description |
|---------|-------------|
| Shot Tracking | Track shot types and locations |
| Movement Analysis | Analyze court coverage patterns |
| Performance Stats | Unforced errors, winners, etc. |
| Progress Over Time | Compare games across sessions |
| Coach Review | Share videos with coach for analysis |
| Benchmarking | Compare stats to skill-level averages |

**Hardware Options:**
| Tier | Components | Use Case |
|------|------------|----------|
| Basic | Tablet scorekeeping, no video | Budget-friendly |
| Standard | Fixed cameras, basic scoreboard | Most facilities |
| Premium | Multi-angle, AI tracking, large displays | High-end venues |
| Portable | Mobile camera kits for events | Tournaments |

**Content Management:**
| Feature | Description |
|---------|-------------|
| Automatic Cleanup | Delete old recordings per retention policy |
| Favorite Clips | Members save favorite moments |
| Search | Find games by date, opponent, court |
| Collections | Organize clips into folders |
| Storage Tiers | Member storage limits by membership level |

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

### 9.3 Slide-Out Menu

**Menu Items:**
- Dashboard
- Courts
- Member Management
- Settings

**User Section:**

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

### 11.3 Configuration

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
| Member billing info | ✅ Complete | — |
| Court calendar UI | ✅ Basic | — |
| Navigation menu | ✅ Complete | — |
| Dark mode toggle | ✅ Complete | — |
| Login page UI | ✅ Complete | — |
| Database layer | ✅ Complete | #4, #5 |
| Build system | ✅ Complete | — |

### 13.2 In Progress / TODO

| Feature | Status | GitHub Issue |
|---------|--------|--------------|
| ~~Fix member search~~ | ✅ Fixed | #31 |
| Theme management system | ❌ TODO | #17-22 |
| Localization system | ❌ TODO | #23-28 |
| GitHub Actions CI/CD | ❌ TODO | #29 |
| Open play rules | ❌ TODO | #32 |
| Global search | ❌ TODO | — |
| Transactions table | ❌ TODO | — |

### 13.3 TODO Items (From Code)

| Location | TODO |
|----------|------|
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
| `cognito_config` | Per-org auth settings |

### 14.2 Key Constraints

- `organizations.slug` - UNIQUE
- `facilities.slug` - UNIQUE
- `users.email` - UNIQUE
- `courts(facility_id, court_number)` - UNIQUE
- `operating_hours(facility_id, day_of_week)` - UNIQUE
- `member_photos.member_id` - UNIQUE INDEX

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

### Duplicate Issues (3-8 duplicate 9-16)

Issues 3-8 are duplicates of issues 9-16 (same content, created twice).

---

## 16. FUTURE ROADMAP

### 16.1 Near-Term (Based on TODOs + Bugs)
1. ~~Fix member search (#31)~~ ✅ Done
4. Implement global search
5. Add transactions/billing tables

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
