PRAGMA foreign_keys = ON;

------ RESERVATION TYPES ------
INSERT INTO reservation_types (name, description, color)
VALUES
    ('OPEN_PLAY', 'Open play session', '#2E7D32'),
    ('GAME', 'Standard game reservation', '#1976D2'),
    ('PRO_SESSION', 'Pro-led session', '#6A1B9A'),
    ('EVENT', 'Special event booking', '#F57C00'),
    ('MAINTENANCE', 'Maintenance block', '#546E7A'),
    ('LEAGUE', 'League play', '#C62828')
-- OPEN_PLAY is originally seeded in 000002; keep it updated but don't delete it in down.
ON CONFLICT(name) DO UPDATE
SET description = excluded.description,
    color = excluded.color;
