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
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (organization_id) REFERENCES organizations(id)
);

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


------ USERS --------
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT UNIQUE,
    phone TEXT,
    cognito_sub TEXT,                       -- Cognito's unique user ID
    cognito_status TEXT CHECK (cognito_status IN ('CONFIRMED', 'UNCONFIRMED')),
    preferred_auth_method TEXT,             -- e.g. 'SMS', 'EMAIL', or 'PUSH'
    password_hash TEXT,                     -- For staff local auth
    local_auth_enabled BOOLEAN NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',  -- e.g. 'active', 'suspended', 'archived'
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

------ MEMBERS --------
CREATE TABLE members (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    photo_url TEXT,
    street_address TEXT,
    city TEXT,
    state TEXT,
    postal_code TEXT,
    date_of_birth TEXT NOT NULL,            -- stored as YYYY-MM-DD
    waiver_signed BOOLEAN NOT NULL,         -- already exists (maps to TOS acceptance, etc.)
    status TEXT NOT NULL DEFAULT 'active',  -- e.g. 'active', 'suspended', 'archived'
    
    home_facility_id INTEGER,               -- The facility this member calls "home"
    membership_level INTEGER NOT NULL DEFAULT 0,  -- 0=Unverified Guest, 1=Verified Guest, 2=Member, 3+=Member+
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (home_facility_id) REFERENCES facilities(id)
);


-- TODO: WE are going to need a table called transactions that will contain every single transaction. 
--       We will also need a table called products that will contain common products and the various fields 
--         and formulas to calculate a sum.  For example -Game, will have hours so it is easy for the frontdesk
--         to bill for thing

CREATE TABLE member_billing (
    id INTEGER PRIMARY KEY,
    member_id INTEGER NOT NULL,
    card_last_four TEXT,
    card_type TEXT,
    billing_address TEXT,
    billing_city TEXT,
    billing_state TEXT,
    billing_postal_code TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (member_id) REFERENCES members(id)
);



CREATE TABLE member_photos (
    id INTEGER PRIMARY KEY,
    member_id INTEGER NOT NULL,
    data BLOB NOT NULL,
    content_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (member_id) REFERENCES members(id)
);

CREATE UNIQUE INDEX idx_member_photos_member_id ON member_photos(member_id);

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
    primary_member_id INTEGER,         -- if there's a responsible member (e.g. for a game)
    pro_id INTEGER,                    -- if it's a pro session
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
    FOREIGN KEY (primary_member_id)   REFERENCES members(id),
    FOREIGN KEY (pro_id)              REFERENCES staff(id)
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
--     Tracks which members are signed up for each reservation (beyond the primary_member).
CREATE TABLE reservation_participants (
    id INTEGER PRIMARY KEY,
    reservation_id INTEGER NOT NULL,
    member_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (reservation_id) REFERENCES reservations(id),
    FOREIGN KEY (member_id)      REFERENCES members(id),
    UNIQUE (reservation_id, member_id)
);

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
