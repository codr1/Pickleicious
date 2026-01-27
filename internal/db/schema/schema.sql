-- internal/db/schema/schema.sql
PRAGMA foreign_keys = ON;

------ ORGANIZATIONS ------
CREATE TABLE organizations (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    cross_facility_visit_packs BOOLEAN NOT NULL DEFAULT 0,
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
    lesson_min_notice_hours INTEGER NOT NULL DEFAULT 24,
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

CREATE TABLE pro_unavailability (
    id INTEGER PRIMARY KEY,
    pro_id INTEGER NOT NULL,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    reason TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (start_time < end_time),
    FOREIGN KEY (pro_id) REFERENCES staff(id) ON DELETE CASCADE
);

CREATE INDEX idx_pro_unavailability_pro_id ON pro_unavailability(pro_id);
CREATE INDEX idx_pro_unavailability_start_time ON pro_unavailability(start_time);
CREATE INDEX idx_pro_unavailability_end_time ON pro_unavailability(end_time);


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
    CHECK (start_time < end_time),
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
    related_reservation_id INTEGER,
    target_staff_id INTEGER,
    read BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (notification_type IN ('scale_up', 'scale_down', 'cancelled', 'lesson_cancelled')),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (related_session_id) REFERENCES open_play_sessions(id),
    FOREIGN KEY (related_reservation_id) REFERENCES reservations(id),
    FOREIGN KEY (target_staff_id) REFERENCES staff(id)
);

CREATE INDEX idx_staff_notifications_facility_id ON staff_notifications(facility_id);
CREATE INDEX idx_staff_notifications_related_session_id ON staff_notifications(related_session_id);
CREATE INDEX idx_staff_notifications_related_reservation_id ON staff_notifications(related_reservation_id);
CREATE INDEX idx_staff_notifications_read ON staff_notifications(read);
CREATE INDEX idx_staff_notifications_target_staff_id ON staff_notifications(target_staff_id);

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
    primary_user_id INTEGER,           -- who the reservation is for (e.g. member booking)
    created_by_user_id INTEGER NOT NULL, -- who created the reservation (staff/member)
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

    CHECK (start_time < end_time),
    FOREIGN KEY (facility_id)         REFERENCES facilities(id),
    FOREIGN KEY (reservation_type_id) REFERENCES reservation_types(id),
    FOREIGN KEY (recurrence_rule_id)  REFERENCES recurrence_rules(id),
    FOREIGN KEY (primary_user_id)     REFERENCES users(id),
    FOREIGN KEY (created_by_user_id)  REFERENCES users(id),
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

------ WAITLISTS ------
CREATE TABLE waitlist_config (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    max_waitlist_size INTEGER NOT NULL DEFAULT 0,
    notification_mode TEXT NOT NULL CHECK (notification_mode IN ('broadcast', 'sequential')),
    offer_expiry_minutes INTEGER NOT NULL DEFAULT 0,
    notification_window_minutes INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    UNIQUE (facility_id)
);

CREATE TABLE waitlists (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    target_court_id INTEGER,
    target_date DATE NOT NULL,
    target_start_time TIME NOT NULL,
    target_end_time TIME NOT NULL,
    position INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'notified', 'expired', 'fulfilled')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (target_start_time < target_end_time),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (target_court_id) REFERENCES courts(id)
);

CREATE TABLE waitlist_offers (
    id INTEGER PRIMARY KEY,
    waitlist_id INTEGER NOT NULL,
    offered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'accepted', 'expired')),
    FOREIGN KEY (waitlist_id) REFERENCES waitlists(id) ON DELETE CASCADE
);

CREATE INDEX idx_waitlists_facility_id ON waitlists(facility_id);
CREATE INDEX idx_waitlists_slot ON waitlists(facility_id, target_date, target_start_time, target_end_time);
CREATE UNIQUE INDEX idx_waitlists_slot_position_unique
    ON waitlists(facility_id, target_date, target_start_time, target_end_time, COALESCE(target_court_id, -1), position);
CREATE INDEX idx_waitlists_user_id ON waitlists(user_id);
CREATE INDEX idx_waitlists_target_date ON waitlists(target_date);
CREATE INDEX idx_waitlists_status ON waitlists(status);
CREATE INDEX idx_waitlist_offers_waitlist_id ON waitlist_offers(waitlist_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_waitlists_unique_active_user_slot
    ON waitlists(facility_id, target_date, target_start_time, target_end_time, COALESCE(target_court_id, -1), user_id)
    WHERE status IN ('pending', 'notified');
CREATE INDEX idx_waitlist_offers_status ON waitlist_offers(status);
CREATE INDEX idx_waitlist_offers_expires_at ON waitlist_offers(expires_at);

------ LEAGUES ------
CREATE TABLE leagues (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    format TEXT NOT NULL CHECK (format IN ('singles', 'doubles', 'mixed_doubles')),
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    division_config TEXT NOT NULL,
    min_team_size INTEGER NOT NULL,
    max_team_size INTEGER NOT NULL,
    roster_lock_date DATE,
    status TEXT NOT NULL CHECK (status IN ('draft', 'registration', 'active', 'completed', 'cancelled')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (start_date <= end_date),
    CHECK (min_team_size > 0 AND max_team_size > 0 AND min_team_size <= max_team_size),
    FOREIGN KEY (facility_id) REFERENCES facilities(id)
);

CREATE TABLE league_teams (
    id INTEGER PRIMARY KEY,
    league_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    captain_user_id INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (league_id) REFERENCES leagues(id) ON DELETE CASCADE,
    FOREIGN KEY (captain_user_id) REFERENCES users(id) ON DELETE RESTRICT,
    UNIQUE (league_id, name)
);

CREATE TABLE league_team_members (
    id INTEGER PRIMARY KEY,
    league_team_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    is_free_agent BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (league_team_id) REFERENCES league_teams(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id),
    UNIQUE (league_team_id, user_id)
);

CREATE TABLE league_matches (
    id INTEGER PRIMARY KEY,
    league_id INTEGER NOT NULL,
    home_team_id INTEGER NOT NULL,
    away_team_id INTEGER NOT NULL,
    reservation_id INTEGER,
    scheduled_time DATETIME NOT NULL,
    home_score INTEGER,
    away_score INTEGER,
    status TEXT NOT NULL CHECK (status IN ('scheduled', 'in_progress', 'completed', 'cancelled', 'forfeit')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (home_team_id != away_team_id),
    CHECK (home_score IS NULL OR home_score >= 0),
    CHECK (away_score IS NULL OR away_score >= 0),
    FOREIGN KEY (league_id) REFERENCES leagues(id) ON DELETE CASCADE,
    FOREIGN KEY (home_team_id) REFERENCES league_teams(id) ON DELETE RESTRICT,
    FOREIGN KEY (away_team_id) REFERENCES league_teams(id) ON DELETE RESTRICT,
    FOREIGN KEY (reservation_id) REFERENCES reservations(id) ON DELETE SET NULL
);

CREATE INDEX idx_leagues_facility_id ON leagues(facility_id);
CREATE INDEX idx_league_teams_league_id ON league_teams(league_id);
CREATE INDEX idx_league_teams_captain_user_id ON league_teams(captain_user_id);
CREATE INDEX idx_league_team_members_team_id ON league_team_members(league_team_id);
CREATE INDEX idx_league_team_members_user_id ON league_team_members(user_id);
CREATE INDEX idx_league_matches_league_id ON league_matches(league_id);
CREATE INDEX idx_league_matches_reservation_id ON league_matches(reservation_id);
CREATE INDEX idx_league_matches_scheduled_time ON league_matches(scheduled_time);
CREATE INDEX idx_league_matches_home_team_id ON league_matches(home_team_id);
CREATE INDEX idx_league_matches_away_team_id ON league_matches(away_team_id);

------ RESERVATION CANCELLATIONS ------
CREATE TABLE reservation_cancellations (
    id INTEGER PRIMARY KEY,
    reservation_id INTEGER NOT NULL,
    cancelled_by_user_id INTEGER NOT NULL,
    cancelled_at DATETIME NOT NULL,
    refund_percentage_applied INTEGER NOT NULL,
    fee_waived BOOLEAN NOT NULL DEFAULT 0,
    hours_before_start INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (refund_percentage_applied >= 0 AND refund_percentage_applied <= 100),
    CHECK (hours_before_start >= 0),
    FOREIGN KEY (reservation_id) REFERENCES reservations(id),
    FOREIGN KEY (cancelled_by_user_id) REFERENCES users(id)
);

CREATE INDEX idx_reservation_cancellations_reservation_id ON reservation_cancellations(reservation_id);
CREATE INDEX idx_reservation_cancellations_cancelled_by_user_id ON reservation_cancellations(cancelled_by_user_id);
CREATE INDEX idx_reservation_cancellations_cancelled_at ON reservation_cancellations(cancelled_at);
CREATE INDEX idx_reservations_created_by_user_id ON reservations(created_by_user_id);

------ CANCELLATION POLICIES ------
CREATE TABLE cancellation_policy_tiers (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    reservation_type_id INTEGER,
    min_hours_before INTEGER NOT NULL,
    refund_percentage INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (min_hours_before >= 0),
    CHECK (refund_percentage >= 0 AND refund_percentage <= 100),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (reservation_type_id) REFERENCES reservation_types(id)
);

CREATE INDEX idx_cancellation_policy_tiers_facility_id ON cancellation_policy_tiers(facility_id);
CREATE UNIQUE INDEX uniq_cancellation_policy_tiers_default
    ON cancellation_policy_tiers(facility_id, min_hours_before)
    WHERE reservation_type_id IS NULL;
CREATE UNIQUE INDEX uniq_cancellation_policy_tiers_type
    ON cancellation_policy_tiers(facility_id, reservation_type_id, min_hours_before)
    WHERE reservation_type_id IS NOT NULL;

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

------ VISIT PACKS ------
CREATE TABLE visit_pack_types (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
    visit_count INTEGER NOT NULL CHECK (visit_count > 0),
    valid_days INTEGER NOT NULL CHECK (valid_days > 0),
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('active', 'inactive')),
    FOREIGN KEY (facility_id) REFERENCES facilities(id)
);

CREATE INDEX idx_visit_pack_types_facility_id ON visit_pack_types(facility_id);
CREATE INDEX idx_visit_pack_types_status ON visit_pack_types(status);

CREATE TABLE visit_packs (
    id INTEGER PRIMARY KEY,
    pack_type_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    purchase_date DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    visits_remaining INTEGER NOT NULL CHECK (visits_remaining >= 0),
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('active', 'expired', 'depleted')),
    FOREIGN KEY (pack_type_id) REFERENCES visit_pack_types(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX idx_visit_packs_pack_type_id ON visit_packs(pack_type_id);
CREATE INDEX idx_visit_packs_user_id ON visit_packs(user_id);
CREATE INDEX idx_visit_packs_status ON visit_packs(status);
CREATE INDEX idx_visit_packs_expires_at ON visit_packs(expires_at);
CREATE INDEX idx_visit_packs_user_status_expires ON visit_packs(user_id, status, expires_at);

CREATE TABLE visit_pack_redemptions (
    id INTEGER PRIMARY KEY,
    visit_pack_id INTEGER NOT NULL,
    facility_id INTEGER NOT NULL,
    redeemed_at DATETIME NOT NULL,
    reservation_id INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (visit_pack_id) REFERENCES visit_packs(id),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (reservation_id) REFERENCES reservations(id)
);

CREATE INDEX idx_visit_pack_redemptions_visit_pack_id ON visit_pack_redemptions(visit_pack_id);
CREATE INDEX idx_visit_pack_redemptions_facility_id ON visit_pack_redemptions(facility_id);
CREATE INDEX idx_visit_pack_redemptions_reservation_id ON visit_pack_redemptions(reservation_id);
CREATE INDEX idx_visit_pack_redemptions_redeemed_at ON visit_pack_redemptions(redeemed_at);

CREATE TRIGGER visit_pack_types_limit_insert
BEFORE INSERT ON visit_pack_types
WHEN (
    SELECT COUNT(*)
    FROM visit_pack_types
    WHERE facility_id = NEW.facility_id
) >= 1000
BEGIN
    SELECT RAISE(ABORT, 'visit pack type limit exceeded');
END;

CREATE TRIGGER visit_pack_types_limit_update
BEFORE UPDATE OF facility_id ON visit_pack_types
WHEN NEW.facility_id IS NOT OLD.facility_id
AND (
    SELECT COUNT(*)
    FROM visit_pack_types
    WHERE facility_id = NEW.facility_id
) >= 1000
BEGIN
    SELECT RAISE(ABORT, 'visit pack type limit exceeded');
END;

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
