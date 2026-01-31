PRAGMA foreign_keys = OFF;

CREATE TABLE organizations_new (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    email_from_address TEXT,
    cross_facility_visit_packs BOOLEAN NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO organizations_new (
    id,
    name,
    slug,
    email_from_address,
    cross_facility_visit_packs,
    status,
    created_at,
    updated_at
)
SELECT
    id,
    name,
    slug,
    email_from_address,
    cross_facility_visit_packs,
    status,
    created_at,
    updated_at
FROM organizations;

DROP TABLE organizations;
ALTER TABLE organizations_new RENAME TO organizations;

PRAGMA foreign_keys = ON;
