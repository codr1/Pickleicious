PRAGMA foreign_keys = ON;

DELETE FROM reservation_types
WHERE name IN (
    'GAME',
    'PRO_SESSION',
    'EVENT',
    'MAINTENANCE',
    'LEAGUE'
);
