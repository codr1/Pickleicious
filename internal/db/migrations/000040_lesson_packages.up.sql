PRAGMA foreign_keys = ON;

------ LESSON PACKAGES ------
CREATE TABLE lesson_package_types (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
    lesson_count INTEGER NOT NULL CHECK (lesson_count > 0),
    valid_days INTEGER NOT NULL CHECK (valid_days > 0),
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('active', 'inactive')),
    FOREIGN KEY (facility_id) REFERENCES facilities(id)
);

CREATE INDEX idx_lesson_package_types_facility_id ON lesson_package_types(facility_id);
CREATE INDEX idx_lesson_package_types_status ON lesson_package_types(status);

CREATE TABLE lesson_packages (
    id INTEGER PRIMARY KEY,
    pack_type_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    purchase_date DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    lessons_remaining INTEGER NOT NULL CHECK (lessons_remaining >= 0),
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('active', 'expired', 'depleted')),
    FOREIGN KEY (pack_type_id) REFERENCES lesson_package_types(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX idx_lesson_packages_pack_type_id ON lesson_packages(pack_type_id);
CREATE INDEX idx_lesson_packages_user_id ON lesson_packages(user_id);
CREATE INDEX idx_lesson_packages_status ON lesson_packages(status);
CREATE INDEX idx_lesson_packages_expires_at ON lesson_packages(expires_at);
CREATE INDEX idx_lesson_packages_user_status_expires ON lesson_packages(user_id, status, expires_at);

CREATE TABLE lesson_package_redemptions (
    id INTEGER PRIMARY KEY,
    lesson_package_id INTEGER NOT NULL,
    facility_id INTEGER NOT NULL,
    redeemed_at DATETIME NOT NULL,
    reservation_id INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (lesson_package_id) REFERENCES lesson_packages(id),
    FOREIGN KEY (facility_id) REFERENCES facilities(id),
    FOREIGN KEY (reservation_id) REFERENCES reservations(id)
);

CREATE INDEX idx_lesson_package_redemptions_lesson_package_id ON lesson_package_redemptions(lesson_package_id);
CREATE INDEX idx_lesson_package_redemptions_facility_id ON lesson_package_redemptions(facility_id);
CREATE INDEX idx_lesson_package_redemptions_reservation_id ON lesson_package_redemptions(reservation_id);
CREATE INDEX idx_lesson_package_redemptions_redeemed_at ON lesson_package_redemptions(redeemed_at);

CREATE TRIGGER lesson_package_types_limit_insert
BEFORE INSERT ON lesson_package_types
WHEN (
    SELECT COUNT(*)
    FROM lesson_package_types
    WHERE facility_id = NEW.facility_id
) >= 1000
BEGIN
    SELECT RAISE(ABORT, 'lesson package type limit exceeded');
END;

CREATE TRIGGER lesson_package_types_limit_update
BEFORE UPDATE OF facility_id ON lesson_package_types
WHEN NEW.facility_id IS NOT OLD.facility_id
AND (
    SELECT COUNT(*)
    FROM lesson_package_types
    WHERE facility_id = NEW.facility_id
) >= 1000
BEGIN
    SELECT RAISE(ABORT, 'lesson package type limit exceeded');
END;
