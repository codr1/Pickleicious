-- migrations/000001_initial_schema.up.sql
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
    day_of_week INTEGER NOT NULL, -- 0 = Sunday, 6 = Saturday
    opens_at TIME NOT NULL,
    closes_at TIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    UNIQUE(facility_id, day_of_week)
);

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

-- Seed default facility and operating hours
INSERT INTO facilities (name, slug, timezone) 
VALUES ('Main Facility', 'main', 'America/New_York');

-- Default operating hours (6 AM to 10 PM)
INSERT INTO operating_hours (facility_id, day_of_week, opens_at, closes_at)
SELECT 
    1 as facility_id,
    day_of_week,
    '06:00:00' as opens_at,
    '22:00:00' as closes_at
FROM (
    SELECT 0 as day_of_week UNION SELECT 1 UNION SELECT 2 
    UNION SELECT 3 UNION SELECT 4 UNION SELECT 5 UNION SELECT 6
);

-- Insert 8 default courts
INSERT INTO courts (facility_id, name, court_number)
SELECT 
    1 as facility_id,
    'Court ' || court_number as name,
    court_number
FROM (
    SELECT 1 as court_number UNION SELECT 2 UNION SELECT 3 
    UNION SELECT 4 UNION SELECT 5 UNION SELECT 6 
    UNION SELECT 7 UNION SELECT 8
);
