PRAGMA foreign_keys = ON;

ALTER TABLE organizations
    ADD COLUMN cross_facility_visit_packs BOOLEAN NOT NULL DEFAULT 0;

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
