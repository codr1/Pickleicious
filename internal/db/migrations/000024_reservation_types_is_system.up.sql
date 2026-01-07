PRAGMA foreign_keys = ON;

ALTER TABLE reservation_types
    ADD COLUMN is_system BOOLEAN NOT NULL DEFAULT 0;

INSERT INTO reservation_types (name, description, color, is_system)
VALUES
    ('OPEN_PLAY', 'Open play session', '#2E7D32', true),
    ('GAME', 'Standard game reservation', '#1976D2', true),
    ('PRO_SESSION', 'Pro-led session', '#6A1B9A', true),
    ('EVENT', 'Special event booking', '#F57C00', true),
    ('MAINTENANCE', 'Maintenance block', '#546E7A', true),
    ('LEAGUE', 'League play', '#C62828', true),
    ('LESSON', 'Lesson session', '#00897B', true),
    ('TOURNAMENT', 'Tournament play', '#5E35B1', true),
    ('CLINIC', 'Clinic session', '#8D6E63', true)
ON CONFLICT(name) DO UPDATE
SET description = excluded.description,
    color = excluded.color,
    is_system = excluded.is_system;
