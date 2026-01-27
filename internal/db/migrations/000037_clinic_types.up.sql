CREATE TABLE IF NOT EXISTS clinic_types (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    min_participants INTEGER NOT NULL,
    max_participants INTEGER NOT NULL,
    price_cents INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (min_participants > 0),
    CHECK (max_participants > 0),
    CHECK (min_participants <= max_participants),
    CHECK (price_cents >= 0),
    CHECK (status IN ('draft', 'active', 'inactive', 'archived')),
    FOREIGN KEY (facility_id) REFERENCES facilities(id)
);

CREATE INDEX IF NOT EXISTS idx_clinic_types_facility_id ON clinic_types(facility_id);
