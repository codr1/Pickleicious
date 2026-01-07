-- internal/db/schema/schema.sql
PRAGMA foreign_keys = ON;

------ ORGANIZATIONS ------
CREATE TABLE organizations (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

------ FACILITY ------
CREATE TABLE facilities (
    id INTEGER PRIMARY KEY,
    organization_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    timezone TEXT NOT NULL,
    active_theme_id INTEGER,
    max_advance_booking_days INTEGER NOT NULL DEFAULT 7,
    max_member_reservations INTEGER NOT NULL DEFAULT 30,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    FOREIGN KEY (active_theme_id) REFERENCES themes(id)
);

CREATE INDEX idx_facilities_active_theme_id ON facilities(active_theme_id);

CREATE TABLE operating_hours (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    day_of_week INTEGER NOT NULL,
    opens_at TIME NOT NULL,  -- 0=Sunday, 1=Monday
    closes_at TIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    UNIQUE(facility_id, day_of_week)
);


------ USERS (consolidated: auth + member + staff) --------
CREATE TABLE users (
    id INTEGER PRIMARY KEY,

    -- Auth fields
    email TEXT UNIQUE,
    phone TEXT,
    cognito_sub TEXT,                       -- Cognito's unique user ID
    cognito_status TEXT CHECK (cognito_status IN ('CONFIRMED', 'UNCONFIRMED')),
    preferred_auth_method TEXT,             -- e.g. 'SMS', 'EMAIL', or 'PUSH'
    password_hash TEXT,                     -- For staff local auth
    local_auth_enabled BOOLEAN NOT NULL DEFAULT 0,

    -- Profile fields (shared)
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    photo_url TEXT,
    street_address TEXT,
    city TEXT,
    state TEXT,
    postal_code TEXT,
    home_facility_id INTEGER,               -- The facility this user calls "home"

    -- Role flags
    is_member BOOLEAN NOT NULL DEFAULT 0,
    is_staff BOOLEAN NOT NULL DEFAULT 0,

    -- Member-specific fields
    date_of_birth TEXT NOT NULL DEFAULT '',  -- stored as YYYY-MM-DD
    waiver_signed BOOLEAN NOT NULL DEFAULT 0,
    membership_level INTEGER NOT NULL DEFAULT 0,  -- 0=Unverified Guest, 1=Verified Guest, 2=Member, 3+=Member+

    -- Staff-specific fields (nullable if not staff)
    staff_role TEXT,                        -- 'admin', 'manager', 'desk', 'pro', etc.

    -- Common
    status TEXT NOT NULL DEFAULT 'active',  -- e.g. 'active', 'suspended', 'archived'
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (home_facility_id) REFERENCES facilities(id)
);


-- TODO: WE are going to need a table called transactions that will contain every single transaction.
--       We will also need a table called products that will contain common products and the various fields
--         and formulas to calculate a sum.  For example -Game, will have hours so it is easy for the frontdesk
--         to bill for thing

CREATE TABLE user_billing (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL UNIQUE,
    card_last_four TEXT,
    card_type TEXT,
    billing_address TEXT,
    billing_city TEXT,
    billing_state TEXT,
    billing_postal_code TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE user_photos (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    data BLOB NOT NULL,
    content_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE UNIQUE INDEX idx_user_photos_user_id ON user_photos(user_id);

--------- Staff ---------
CREATE TABLE staff (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    home_facility_id INTEGER,       -- can be NULL for corporate-level Admins
    role TEXT NOT NULL,             -- 'admin', 'manager', 'desk', 'pro', etc.
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (home_facility_id) REFERENCES facilities(id)
);


--------- Courts ----------
CREATE TABLE courts (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    court_number INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    UNIQUE(facility_id, court_number)
);

------ THEMES ------
CREATE TABLE themes (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER,
    name TEXT NOT NULL,
    is_system BOOLEAN NOT NULL DEFAULT 0,
    primary_color TEXT NOT NULL,
    secondary_color TEXT NOT NULL,
    tertiary_color TEXT NOT NULL,
    accent_color TEXT NOT NULL,
    highlight_color TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (
        (facility_id IS NULL AND is_system = 1)
        OR (facility_id IS NOT NULL AND is_system = 0)
    ),
    FOREIGN KEY (facility_id) REFERENCES facilities(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_themes_system_name ON themes(name) WHERE facility_id IS NULL;
CREATE UNIQUE INDEX idx_themes_facility_name ON themes(facility_id, name) WHERE facility_id IS NOT NULL;

------ OPEN PLAY RULES ------
CREATE TABLE open_play_rules (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    min_participants INTEGER NOT NULL DEFAULT 4,
    max_participants_per_court INTEGER NOT NULL DEFAULT 8,
    cancellation_cutoff_minutes INTEGER NOT NULL DEFAULT 60,
    auto_scale_enabled BOOLEAN NOT NULL DEFAULT 1,
    min_courts INTEGER NOT NULL DEFAULT 1,
    max_courts INTEGER NOT NULL DEFAULT 4,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (min_participants > 0),
    CHECK (max_participants_per_court > 0),
    CHECK (min_courts > 0),
    CHECK (max_courts > 0),
    CHECK (min_courts <= max_courts),
    CHECK (min_participants <= max_participants_per_court * min_courts),
    FOREIGN KEY (facility_id) REFERENCES facilities(id)
);

CREATE INDEX idx_open_play_rules_facility_id ON open_play_rules(facility_id);

------ OPEN PLAY SESSIONS ------
CREATE TABLE open_play_sessions (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    open_play_rule_id INTEGER NOT NULL,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    status TEXT NOT NULL DEFAULT 'scheduled',
    current_court_count INTEGER NOT NULL DEFAULT 0,
    auto_scale_override BOOLEAN,
    cancelled_at DATETIME,
    cancellation_reason TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('scheduled', 'cancelled', 'completed')),
    CHECK (current_court_count >= 0),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (open_play_rule_id) REFERENCES open_play_rules(id)
);

CREATE INDEX idx_open_play_sessions_facility_id ON open_play_sessions(facility_id);
CREATE INDEX idx_open_play_sessions_rule_id ON open_play_sessions(open_play_rule_id);
CREATE INDEX idx_open_play_sessions_start_time ON open_play_sessions(start_time);
CREATE INDEX idx_open_play_sessions_status ON open_play_sessions(status);

------ STAFF NOTIFICATIONS ------
CREATE TABLE staff_notifications (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    notification_type TEXT NOT NULL,
    message TEXT NOT NULL,
    related_session_id INTEGER,
    read BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (notification_type IN ('scale_up', 'scale_down', 'cancelled')),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (related_session_id) REFERENCES open_play_sessions(id)
);

CREATE INDEX idx_staff_notifications_facility_id ON staff_notifications(facility_id);
CREATE INDEX idx_staff_notifications_related_session_id ON staff_notifications(related_session_id);
CREATE INDEX idx_staff_notifications_read ON staff_notifications(read);

------ OPEN PLAY AUDIT LOG ------
CREATE TABLE open_play_audit_log (
    id INTEGER PRIMARY KEY,
    session_id INTEGER NOT NULL,
    action TEXT NOT NULL,
    before_state TEXT,
    after_state TEXT,
    reason TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (action IN ('scale_up', 'scale_down', 'cancelled', 'auto_scale_override', 'auto_scale_rule_disabled', 'participant_added', 'participant_removed')),
    FOREIGN KEY (session_id) REFERENCES open_play_sessions(id)
);

CREATE INDEX idx_open_play_audit_log_session_id ON open_play_audit_log(session_id);
CREATE INDEX idx_open_play_audit_log_created_at ON open_play_audit_log(created_at);


--------- Reservations ---------

-- Reservation Types (lookup table)
--    Acts like an enum for reservation categories.
CREATE TABLE reservation_types (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,  -- e.g. 'GAME', 'PRO_SESSION', 'EVENT', 'MAINTENANCE', 'LEAGUE', etc.
    description TEXT,           -- optional: describe this type in detail
    color TEXT,                 -- optional: store a default color code like '#FF0000'
    is_system BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Recurrence Rules (lookup table)
--    Manages possible recurrence patterns (e.g. weekly, monthly).
CREATE TABLE recurrence_rules (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,  -- e.g. 'WEEKLY', 'MONTHLY', 'BIWEEKLY'
    rule_definition TEXT,       -- e.g. iCalendar RRULE or custom logic
    description TEXT,           -- human-readable explanation
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Reservations
--     Main table for all booking blocks: Games, Pro Sessions, Events, Maintenance, League, etc.
CREATE TABLE reservations (
    id INTEGER PRIMARY KEY,

    facility_id INTEGER NOT NULL,
    reservation_type_id INTEGER NOT NULL,
    recurrence_rule_id INTEGER,        -- null if it's not recurring
    primary_user_id INTEGER,           -- if there's a responsible user (e.g. for a game)
    pro_id INTEGER,                    -- if it's a pro session (FK to staff)
    open_play_rule_id INTEGER,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,

    -- For events/leagues: open vs closed; #teams; #people/team
    is_open_event BOOLEAN NOT NULL DEFAULT 0,
    teams_per_court INTEGER,
    people_per_team INTEGER,

    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (facility_id)         REFERENCES facilities(id),
    FOREIGN KEY (reservation_type_id) REFERENCES reservation_types(id),
    FOREIGN KEY (recurrence_rule_id)  REFERENCES recurrence_rules(id),
    FOREIGN KEY (primary_user_id)     REFERENCES users(id),
    FOREIGN KEY (pro_id)              REFERENCES staff(id),
    -- No ON DELETE action: deletion is intentionally blocked when reservations exist.
    FOREIGN KEY (open_play_rule_id)   REFERENCES open_play_rules(id)
);

-- Reservation Courts (junction table)
--     Allows one reservation to block multiple courts (e.g., an event).
CREATE TABLE reservation_courts (
    id INTEGER PRIMARY KEY,
    reservation_id INTEGER NOT NULL,
    court_id INTEGER NOT NULL,
    FOREIGN KEY (reservation_id) REFERENCES reservations(id),
    FOREIGN KEY (court_id)       REFERENCES courts(id),
    UNIQUE (reservation_id, court_id)
);

-- Reservation Participants (junction table)
--     Tracks which users are signed up for each reservation (beyond the primary_user).
CREATE TABLE reservation_participants (
    id INTEGER PRIMARY KEY,
    reservation_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (reservation_id) REFERENCES reservations(id),
    FOREIGN KEY (user_id)        REFERENCES users(id),
    UNIQUE (reservation_id, user_id)
);

------ FACILITY VISITS ------
CREATE TABLE facility_visits (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    facility_id INTEGER NOT NULL,
    check_in_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    check_out_time DATETIME,
    checked_in_by_staff_id INTEGER,
    activity_type TEXT CHECK (activity_type IS NULL OR activity_type IN ('court_reservation', 'open_play', 'league')),
    related_reservation_id INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (checked_in_by_staff_id) REFERENCES users(id),
    FOREIGN KEY (related_reservation_id) REFERENCES reservations(id)
);

CREATE INDEX idx_facility_visits_facility_id ON facility_visits(facility_id);
CREATE INDEX idx_facility_visits_user_id ON facility_visits(user_id);
CREATE INDEX idx_facility_visits_check_in_time ON facility_visits(check_in_time);

------ COGNITO CONFIG ------
CREATE TABLE cognito_config (
    id INTEGER PRIMARY KEY,
    organization_id INTEGER NOT NULL,
    pool_id TEXT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    domain TEXT NOT NULL,           -- e.g., organization.pickleadmin.com
    callback_url TEXT NOT NULL,     -- e.g., https://organization.pickleadmin.com/auth/callback
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    UNIQUE(organization_id)
);
