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
    opens_at TIME NOT NULL,
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
    cognito_sub TEXT,
    cognito_status TEXT CHECK (cognito_status IN ('CONFIRMED', 'UNCONFIRMED')),
    preferred_auth_method TEXT,
    password_hash TEXT,
    local_auth_enabled BOOLEAN NOT NULL DEFAULT 0,

    -- Profile fields (shared)
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    photo_url TEXT,
    street_address TEXT,
    city TEXT,
    state TEXT,
    postal_code TEXT,
    home_facility_id INTEGER,

    -- Role flags
    is_member BOOLEAN NOT NULL DEFAULT 0,
    is_staff BOOLEAN NOT NULL DEFAULT 0,

    -- Member-specific fields
    date_of_birth TEXT NOT NULL DEFAULT '',
    waiver_signed BOOLEAN NOT NULL DEFAULT 0,
    membership_level INTEGER NOT NULL DEFAULT 0,

    -- Staff-specific fields (nullable if not staff)
    staff_role TEXT,

    -- Common
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (home_facility_id) REFERENCES facilities(id)
);

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
    home_facility_id INTEGER,
    role TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (home_facility_id) REFERENCES facilities(id)
);

------ COURTS -------
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

------ RESERVATIONS -------
CREATE TABLE reservation_types (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    color TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE recurrence_rules (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    rule_definition TEXT,
    description TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE reservations (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    reservation_type_id INTEGER NOT NULL,
    recurrence_rule_id INTEGER,
    primary_user_id INTEGER,
    pro_id INTEGER,
    start_time DATETIME NOT NULL,
    end_time DATETIME NOT NULL,
    is_open_event BOOLEAN NOT NULL DEFAULT 0,
    teams_per_court INTEGER,
    people_per_team INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (reservation_type_id) REFERENCES reservation_types(id),
    FOREIGN KEY (recurrence_rule_id) REFERENCES recurrence_rules(id),
    FOREIGN KEY (primary_user_id) REFERENCES users(id),
    FOREIGN KEY (pro_id) REFERENCES staff(id)
);

CREATE TABLE reservation_courts (
    id INTEGER PRIMARY KEY,
    reservation_id INTEGER NOT NULL,
    court_id INTEGER NOT NULL,
    FOREIGN KEY (reservation_id) REFERENCES reservations(id),
    FOREIGN KEY (court_id) REFERENCES courts(id),
    UNIQUE (reservation_id, court_id)
);

CREATE TABLE reservation_participants (
    id INTEGER PRIMARY KEY,
    reservation_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (reservation_id) REFERENCES reservations(id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    UNIQUE (reservation_id, user_id)
);

------ COGNITO CONFIG ------
CREATE TABLE cognito_config (
    id INTEGER PRIMARY KEY,
    organization_id INTEGER NOT NULL,
    pool_id TEXT NOT NULL,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    domain TEXT NOT NULL,
    callback_url TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
    UNIQUE(organization_id)
);
