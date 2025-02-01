PRAGMA foreign_keys = ON;

------ FACILITY ------
CREATE TABLE facilities (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    timezone TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
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

------ MEMBERS --------
CREATE TABLE members (
    id INTEGER PRIMARY KEY,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    email TEXT UNIQUE,
    phone TEXT,
    photo_url TEXT,
    street_address TEXT,
    city TEXT,
    state TEXT,
    postal_code TEXT,
    date_of_birth TEXT NOT NULL,
    waiver_signed BOOLEAN NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    home_facility_id INTEGER,
    membership_level INTEGER NOT NULL DEFAULT 0,
    cognito_sub TEXT,
    preferred_auth_method TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (home_facility_id) REFERENCES facilities(id)
);

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

------ STAFF -------
CREATE TABLE staff (
    id INTEGER PRIMARY KEY,
    home_facility_id INTEGER,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    email TEXT UNIQUE,
    phone TEXT,
    role TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    cognito_sub TEXT,
    preferred_auth_method TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
    primary_member_id INTEGER,
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
    FOREIGN KEY (primary_member_id) REFERENCES members(id),
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
    member_id INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (reservation_id) REFERENCES reservations(id),
    FOREIGN KEY (member_id) REFERENCES members(id),
    UNIQUE (reservation_id, member_id)
); 