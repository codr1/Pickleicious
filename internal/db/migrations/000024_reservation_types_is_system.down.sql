PRAGMA foreign_keys = OFF;

CREATE TABLE reservation_types_new (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    color TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO reservation_types_new (
    id,
    name,
    description,
    color,
    created_at,
    updated_at
)
SELECT
    id,
    name,
    description,
    color,
    created_at,
    updated_at
FROM reservation_types;

DROP TABLE reservation_types;
ALTER TABLE reservation_types_new RENAME TO reservation_types;

PRAGMA foreign_keys = ON;
