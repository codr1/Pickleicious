-- Seed Data for Development/Testing
-- ============================================================================
--
-- USAGE:
--   task dev                      # Run server in dev mode (OTP bypass: 123456)
--   task staging                  # Run server in staging mode (real Cognito SMS)
--   task db:seed                  # Reset DB and apply seed data
--   task db:snapshot -- name      # Save current DB state
--   task db:restore -- name       # Restore from snapshot
--   task db:snapshots             # List available snapshots
--
-- TEST CREDENTIALS (Staff):
--   admin@pickle.test    / admin123     (admin, Downtown Club)
--   manager@pickle.test  / manager123   (manager, Downtown Club)
--   desk@pickle.test     / desk123      (desk, Downtown Club)
--   pro@pickle.test      / pro123       (pro, Downtown Club)
--   westside@pickle.test / westside123  (manager, Westside Center)
--   metro@pickle.test    / metro123     (admin, Metro Courts)
--
-- TEST CREDENTIALS (Members):
--   Member login uses OTP bypass in dev mode.
--   Enter any member email OR phone, then use code: 123456
--   Examples:
--     alice.j@email.test / code: 123456
--     3024422842 / code: 123456 (Alice's phone)
--
-- IMPORTANT NOTES:
--   - This script is DESTRUCTIVE. Run only via `task db:seed` which resets first.
--   - Reservation/session dates are relative to current time (datetime('now')).
--     If you snapshot and restore later, dates will be stale relative to "now".
--   - Wrapped in a transaction. Partial failures will rollback completely.
--
-- ============================================================================

PRAGMA foreign_keys = ON;

BEGIN TRANSACTION;

--------------------------------------------------------------------------------
-- ORGANIZATIONS
--------------------------------------------------------------------------------
INSERT INTO organizations (id, name, slug, status) VALUES
    (1, 'Pickle Paradise', 'pickle', 'active'),
    (2, 'Metro Pickleball', 'metro', 'active');

--------------------------------------------------------------------------------
-- FACILITIES
--------------------------------------------------------------------------------
INSERT INTO facilities (id, organization_id, name, slug, timezone, max_advance_booking_days, max_member_reservations) VALUES
    (1, 1, 'Downtown Club', 'downtown-club', 'America/New_York', 14, 30),
    (2, 1, 'Westside Center', 'westside-center', 'America/New_York', 7, 20),
    (3, 2, 'Metro Courts', 'metro-courts', 'America/Chicago', 7, 15);

--------------------------------------------------------------------------------
-- COURTS
--------------------------------------------------------------------------------
-- Downtown Club: 8 courts
INSERT INTO courts (id, facility_id, name, court_number, status) VALUES
    (1, 1, 'Court 1', 1, 'active'),
    (2, 1, 'Court 2', 2, 'active'),
    (3, 1, 'Court 3', 3, 'active'),
    (4, 1, 'Court 4', 4, 'active'),
    (5, 1, 'Court 5', 5, 'active'),
    (6, 1, 'Court 6', 6, 'active'),
    (7, 1, 'Court 7', 7, 'active'),
    (8, 1, 'Court 8', 8, 'active'),
    -- Westside Center: 6 courts
    (9, 2, 'Court 1', 1, 'active'),
    (10, 2, 'Court 2', 2, 'active'),
    (11, 2, 'Court 3', 3, 'active'),
    (12, 2, 'Court 4', 4, 'active'),
    (13, 2, 'Court 5', 5, 'active'),
    (14, 2, 'Court 6', 6, 'active'),
    -- Metro Courts: 4 courts
    (15, 3, 'Court 1', 1, 'active'),
    (16, 3, 'Court 2', 2, 'active'),
    (17, 3, 'Court 3', 3, 'active'),
    (18, 3, 'Court 4', 4, 'active');

--------------------------------------------------------------------------------
-- OPERATING HOURS (6 AM - 10 PM weekdays, 7 AM - 9 PM weekends)
--------------------------------------------------------------------------------
INSERT INTO operating_hours (facility_id, day_of_week, opens_at, closes_at) VALUES
    -- Downtown Club
    (1, 0, '07:00', '21:00'),  -- Sunday
    (1, 1, '06:00', '22:00'),  -- Monday
    (1, 2, '06:00', '22:00'),  -- Tuesday
    (1, 3, '06:00', '22:00'),  -- Wednesday
    (1, 4, '06:00', '22:00'),  -- Thursday
    (1, 5, '06:00', '22:00'),  -- Friday
    (1, 6, '07:00', '21:00'),  -- Saturday
    -- Westside Center
    (2, 0, '07:00', '21:00'),
    (2, 1, '06:00', '22:00'),
    (2, 2, '06:00', '22:00'),
    (2, 3, '06:00', '22:00'),
    (2, 4, '06:00', '22:00'),
    (2, 5, '06:00', '22:00'),
    (2, 6, '07:00', '21:00'),
    -- Metro Courts
    (3, 0, '07:00', '21:00'),
    (3, 1, '06:00', '22:00'),
    (3, 2, '06:00', '22:00'),
    (3, 3, '06:00', '22:00'),
    (3, 4, '06:00', '22:00'),
    (3, 5, '06:00', '22:00'),
    (3, 6, '07:00', '21:00');

--------------------------------------------------------------------------------
-- USERS: Staff (IDs 1-6) and Members (IDs 7-26)
-- Passwords are bcrypt hashed at cost 10
--------------------------------------------------------------------------------
INSERT INTO users (id, email, phone, first_name, last_name, is_staff, is_member, local_auth_enabled, password_hash, home_facility_id, staff_role, membership_level, waiver_signed, date_of_birth, status) VALUES
    -- STAFF (IDs 1-6)
    -- Downtown Club staff
    (1, 'admin@pickle.test', NULL, 'Alice', 'Admin', 1, 0, 1,
     '$2a$10$.8ErO9m3l68arU3uAZsU.em9AniyogKti3vRdKunBjnmep9i3znCa',
     1, 'admin', 0, 0, '', 'active'),
    (2, 'manager@pickle.test', NULL, 'Mike', 'Manager', 1, 0, 1,
     '$2a$10$qnFmsdJgdAllcOOPSgfZZO6Ba2YQCf74GaidG0uUb8lWmb3wBdXaW',
     1, 'manager', 0, 0, '', 'active'),
    (3, 'desk@pickle.test', NULL, 'Dana', 'Desk', 1, 0, 1,
     '$2a$10$o1J1bqxdLULo5bsoQe7P7.5ekDZaZG/na8J8zdLLlnM2zXAP4C2.K',
     1, 'desk', 0, 0, '', 'active'),
    (4, 'pro@pickle.test', NULL, 'Pete', 'Pro', 1, 0, 1,
     '$2a$10$VLCgMNuT9pWoZJsKzENk5OG5bslqBlNlw2dSabquxmOQTkS5Ht7Vm',
     1, 'pro', 0, 0, '', 'active'),
    -- Westside Center staff
    (5, 'westside@pickle.test', NULL, 'Wendy', 'West', 1, 0, 1,
     '$2a$10$SPf51VUbR0c38oS1HaiUXOXFC9ga8UHeKy.SCmtBJHFzS3zh5t4PS',
     2, 'manager', 0, 0, '', 'active'),
    -- Metro Courts staff
    (6, 'metro@pickle.test', NULL, 'Marcus', 'Metro', 1, 0, 1,
     '$2a$10$Tc7j2wAuOjBnXnMyGbbBHunernez/YlhUeTa3ix18zs3IbV.wGl4K',
     3, 'admin', 0, 0, '', 'active'),

    -- MEMBERS (IDs 7-26)
    -- Members use OTP auth (local_auth_enabled=0), phone numbers in E.164 format
    -- Downtown Club members (IDs 7-14)
    (7, 'alice.j@email.test', '+13024422842', 'Alice', 'Johnson', 0, 1, 0,
     NULL,
     1, NULL, 2, 1, '1985-03-15', 'active'),
    (8, 'bob.s@email.test', '+13025550102', 'Bob', 'Smith', 0, 1, 0,
     NULL,
     1, NULL, 3, 1, '1978-07-22', 'active'),
    (9, 'carol.w@email.test', '+13025550103', 'Carol', 'Williams', 0, 1, 0,
     NULL,
     1, NULL, 2, 1, '1990-11-08', 'active'),
    (10, 'david.b@email.test', '+13025550104', 'David', 'Brown', 0, 1, 0,
     NULL,
     1, NULL, 1, 0, '1982-05-30', 'active'),
    (11, 'emma.d@email.test', '+13025550105', 'Emma', 'Davis', 0, 1, 0,
     NULL,
     1, NULL, 2, 1, '1995-09-12', 'active'),
    (12, 'frank.m@email.test', '+13025550106', 'Frank', 'Miller', 0, 1, 0,
     NULL,
     1, NULL, 0, 0, '1970-01-25', 'active'),
    (13, 'grace.w@email.test', '+13025550107', 'Grace', 'Wilson', 0, 1, 0,
     NULL,
     1, NULL, 3, 1, '1988-04-18', 'active'),
    (14, 'henry.m@email.test', '+13025550108', 'Henry', 'Moore', 0, 1, 0,
     NULL,
     1, NULL, 2, 1, '1975-12-03', 'active'),
    -- Westside Center members (IDs 15-20)
    (15, 'ivy.t@email.test', '+13025550201', 'Ivy', 'Taylor', 0, 1, 0,
     NULL,
     2, NULL, 2, 1, '1992-06-20', 'active'),
    (16, 'jack.a@email.test', '+13025550202', 'Jack', 'Anderson', 0, 1, 0,
     NULL,
     2, NULL, 1, 1, '1980-08-14', 'active'),
    (17, 'kate.t@email.test', '+13025550203', 'Kate', 'Thomas', 0, 1, 0,
     NULL,
     2, NULL, 3, 1, '1987-02-28', 'active'),
    (18, 'leo.j@email.test', '+13025550204', 'Leo', 'Jackson', 0, 1, 0,
     NULL,
     2, NULL, 2, 0, '1993-10-05', 'active'),
    (19, 'mia.w@email.test', '+13025550205', 'Mia', 'White', 0, 1, 0,
     NULL,
     2, NULL, 2, 1, '1991-07-17', 'active'),
    (20, 'noah.h@email.test', '+13025550206', 'Noah', 'Harris', 0, 1, 0,
     NULL,
     2, NULL, 1, 1, '1983-11-22', 'active'),
    -- Metro Courts members (IDs 21-26)
    (21, 'olivia.m@email.test', '+13025550301', 'Olivia', 'Martin', 0, 1, 0,
     NULL,
     3, NULL, 2, 1, '1989-04-10', 'active'),
    (22, 'paul.g@email.test', '+13025550302', 'Paul', 'Garcia', 0, 1, 0,
     NULL,
     3, NULL, 3, 1, '1976-09-08', 'active'),
    (23, 'quinn.m@email.test', '+13025550303', 'Quinn', 'Martinez', 0, 1, 0,
     NULL,
     3, NULL, 1, 0, '1994-01-30', 'active'),
    (24, 'ruby.r@email.test', '+13025550304', 'Ruby', 'Robinson', 0, 1, 0,
     NULL,
     3, NULL, 2, 1, '1986-06-15', 'active'),
    (25, 'sam.c@email.test', '+13025550305', 'Sam', 'Clark', 0, 1, 0,
     NULL,
     3, NULL, 2, 1, '1979-03-25', 'active'),
    (26, 'tina.l@email.test', '+13025550306', 'Tina', 'Lewis', 0, 1, 0,
     NULL,
     3, NULL, 0, 0, '1997-12-01', 'active');

--------------------------------------------------------------------------------
-- STAFF RECORDS (linked to user IDs 1-6)
--------------------------------------------------------------------------------
INSERT INTO staff (id, user_id, first_name, last_name, home_facility_id, role) VALUES
    (1, 1, 'Alice', 'Admin', 1, 'admin'),
    (2, 2, 'Mike', 'Manager', 1, 'manager'),
    (3, 3, 'Dana', 'Desk', 1, 'desk'),
    (4, 4, 'Pete', 'Pro', 1, 'pro'),
    (5, 5, 'Wendy', 'West', 2, 'manager'),
    (6, 6, 'Marcus', 'Metro', 3, 'admin');

--------------------------------------------------------------------------------
-- THEMES (custom per facility; system themes added by migrations)
--------------------------------------------------------------------------------
INSERT INTO themes (id, facility_id, name, is_system, primary_color, secondary_color, tertiary_color, accent_color, highlight_color) VALUES
    -- Downtown Club themes
    (10, 1, 'Downtown Dark', 0, '#1a1a2e', '#16213e', '#0f3460', '#e94560', '#ff6b6b'),
    (11, 1, 'Downtown Light', 0, '#f8f9fa', '#e9ecef', '#dee2e6', '#228be6', '#40c057'),
    -- Westside Center themes
    (12, 2, 'Westside Sunset', 0, '#2d3436', '#636e72', '#b2bec3', '#fdcb6e', '#e17055'),
    (13, 2, 'Westside Ocean', 0, '#0c2461', '#1e3799', '#4a69bd', '#0be881', '#00d2d3'),
    -- Metro Courts themes
    (14, 3, 'Metro Modern', 0, '#2c3e50', '#34495e', '#7f8c8d', '#e74c3c', '#f39c12'),
    (15, 3, 'Metro Fresh', 0, '#ecf0f1', '#bdc3c7', '#95a5a6', '#27ae60', '#3498db');

--------------------------------------------------------------------------------
-- OPEN PLAY RULES
--------------------------------------------------------------------------------
INSERT INTO open_play_rules (id, facility_id, name, min_participants, max_participants_per_court, cancellation_cutoff_minutes, auto_scale_enabled, min_courts, max_courts) VALUES
    -- Downtown Club
    (1, 1, 'Morning Open Play', 4, 8, 60, 1, 1, 2),
    (2, 1, 'Midday Open Play', 4, 8, 60, 1, 1, 3),
    (3, 1, 'Evening Open Play', 6, 8, 120, 1, 2, 4),
    (4, 1, 'Weekend Open Play', 8, 8, 180, 1, 2, 4),
    -- Westside Center
    (5, 2, 'Morning Open Play', 4, 8, 60, 1, 1, 2),
    (6, 2, 'Midday Open Play', 4, 8, 60, 1, 1, 2),
    (7, 2, 'Evening Open Play', 4, 8, 90, 1, 1, 3),
    (8, 2, 'Weekend Open Play', 6, 8, 120, 1, 2, 3),
    -- Metro Courts
    (9, 3, 'Morning Open Play', 4, 8, 60, 1, 1, 2),
    (10, 3, 'Evening Open Play', 4, 8, 90, 1, 1, 2),
    (11, 3, 'Weekend Open Play', 4, 8, 120, 1, 1, 2);

--------------------------------------------------------------------------------
-- RESERVATIONS
-- Using relative dates: datetime('now', '+/-N days', 'start of day', '+H hours')
-- Note: Dates are computed at seed time. Snapshots will have stale dates.
--------------------------------------------------------------------------------

-- Past reservations (last 2 weeks) - Downtown Club
-- reservation_type_id: 1=OPEN_PLAY, 2=GAME, 3=PRO_SESSION, 4=EVENT, 5=MAINTENANCE, 6=LEAGUE
INSERT INTO reservations (id, facility_id, reservation_type_id, primary_user_id, start_time, end_time) VALUES
    (1, 1, 2, 7, datetime('now', '-14 days', 'start of day', '+9 hours'), datetime('now', '-14 days', 'start of day', '+10 hours')),
    (2, 1, 2, 8, datetime('now', '-14 days', 'start of day', '+10 hours'), datetime('now', '-14 days', 'start of day', '+11 hours')),
    (3, 1, 2, 9, datetime('now', '-13 days', 'start of day', '+14 hours'), datetime('now', '-13 days', 'start of day', '+15 hours')),
    (4, 1, 2, 10, datetime('now', '-12 days', 'start of day', '+18 hours'), datetime('now', '-12 days', 'start of day', '+19 hours')),
    (5, 1, 2, 11, datetime('now', '-11 days', 'start of day', '+9 hours'), datetime('now', '-11 days', 'start of day', '+10 hours')),
    (6, 1, 2, 7, datetime('now', '-10 days', 'start of day', '+11 hours'), datetime('now', '-10 days', 'start of day', '+12 hours')),
    (7, 1, 2, 12, datetime('now', '-9 days', 'start of day', '+16 hours'), datetime('now', '-9 days', 'start of day', '+17 hours')),
    (8, 1, 2, 13, datetime('now', '-8 days', 'start of day', '+10 hours'), datetime('now', '-8 days', 'start of day', '+11 hours')),
    (9, 1, 2, 14, datetime('now', '-7 days', 'start of day', '+15 hours'), datetime('now', '-7 days', 'start of day', '+16 hours')),
    (10, 1, 2, 7, datetime('now', '-6 days', 'start of day', '+9 hours'), datetime('now', '-6 days', 'start of day', '+10 hours'));

-- Pro sessions (past) - pro_id references staff.id (Pete Pro = staff ID 4)
INSERT INTO reservations (id, facility_id, reservation_type_id, primary_user_id, pro_id, start_time, end_time) VALUES
    (11, 1, 3, 8, 4, datetime('now', '-13 days', 'start of day', '+10 hours'), datetime('now', '-13 days', 'start of day', '+11 hours')),
    (12, 1, 3, 9, 4, datetime('now', '-10 days', 'start of day', '+14 hours'), datetime('now', '-10 days', 'start of day', '+15 hours')),
    (13, 1, 3, 13, 4, datetime('now', '-7 days', 'start of day', '+11 hours'), datetime('now', '-7 days', 'start of day', '+12 hours'));

-- Current/upcoming reservations (today and next 2 weeks) - Downtown Club
INSERT INTO reservations (id, facility_id, reservation_type_id, primary_user_id, start_time, end_time) VALUES
    (20, 1, 2, 7, datetime('now', 'start of day', '+10 hours'), datetime('now', 'start of day', '+11 hours')),
    (21, 1, 2, 8, datetime('now', 'start of day', '+14 hours'), datetime('now', 'start of day', '+15 hours')),
    (22, 1, 2, 9, datetime('now', '+1 days', 'start of day', '+9 hours'), datetime('now', '+1 days', 'start of day', '+10 hours')),
    (23, 1, 2, 10, datetime('now', '+1 days', 'start of day', '+18 hours'), datetime('now', '+1 days', 'start of day', '+19 hours')),
    (24, 1, 2, 11, datetime('now', '+2 days', 'start of day', '+11 hours'), datetime('now', '+2 days', 'start of day', '+12 hours')),
    (25, 1, 2, 12, datetime('now', '+3 days', 'start of day', '+16 hours'), datetime('now', '+3 days', 'start of day', '+17 hours')),
    (26, 1, 2, 13, datetime('now', '+4 days', 'start of day', '+10 hours'), datetime('now', '+4 days', 'start of day', '+11 hours')),
    (27, 1, 2, 14, datetime('now', '+5 days', 'start of day', '+15 hours'), datetime('now', '+5 days', 'start of day', '+16 hours')),
    (28, 1, 2, 7, datetime('now', '+6 days', 'start of day', '+9 hours'), datetime('now', '+6 days', 'start of day', '+10 hours')),
    (29, 1, 2, 8, datetime('now', '+7 days', 'start of day', '+14 hours'), datetime('now', '+7 days', 'start of day', '+15 hours'));

-- Upcoming pro sessions
INSERT INTO reservations (id, facility_id, reservation_type_id, primary_user_id, pro_id, start_time, end_time) VALUES
    (30, 1, 3, 9, 4, datetime('now', '+2 days', 'start of day', '+10 hours'), datetime('now', '+2 days', 'start of day', '+11 hours')),
    (31, 1, 3, 11, 4, datetime('now', '+4 days', 'start of day', '+14 hours'), datetime('now', '+4 days', 'start of day', '+15 hours')),
    (32, 1, 3, 13, 4, datetime('now', '+6 days', 'start of day', '+11 hours'), datetime('now', '+6 days', 'start of day', '+12 hours'));

-- Westside Center reservations (members 15-20)
INSERT INTO reservations (id, facility_id, reservation_type_id, primary_user_id, start_time, end_time) VALUES
    (40, 2, 2, 15, datetime('now', '-5 days', 'start of day', '+10 hours'), datetime('now', '-5 days', 'start of day', '+11 hours')),
    (41, 2, 2, 16, datetime('now', '-3 days', 'start of day', '+14 hours'), datetime('now', '-3 days', 'start of day', '+15 hours')),
    (42, 2, 2, 17, datetime('now', 'start of day', '+16 hours'), datetime('now', 'start of day', '+17 hours')),
    (43, 2, 2, 18, datetime('now', '+1 days', 'start of day', '+10 hours'), datetime('now', '+1 days', 'start of day', '+11 hours')),
    (44, 2, 2, 19, datetime('now', '+3 days', 'start of day', '+18 hours'), datetime('now', '+3 days', 'start of day', '+19 hours')),
    (45, 2, 2, 20, datetime('now', '+5 days', 'start of day', '+11 hours'), datetime('now', '+5 days', 'start of day', '+12 hours'));

-- Metro Courts reservations (members 21-26)
INSERT INTO reservations (id, facility_id, reservation_type_id, primary_user_id, start_time, end_time) VALUES
    (50, 3, 2, 21, datetime('now', '-4 days', 'start of day', '+9 hours'), datetime('now', '-4 days', 'start of day', '+10 hours')),
    (51, 3, 2, 22, datetime('now', '-2 days', 'start of day', '+15 hours'), datetime('now', '-2 days', 'start of day', '+16 hours')),
    (52, 3, 2, 23, datetime('now', 'start of day', '+11 hours'), datetime('now', 'start of day', '+12 hours')),
    (53, 3, 2, 24, datetime('now', '+2 days', 'start of day', '+17 hours'), datetime('now', '+2 days', 'start of day', '+18 hours')),
    (54, 3, 2, 25, datetime('now', '+4 days', 'start of day', '+10 hours'), datetime('now', '+4 days', 'start of day', '+11 hours')),
    (55, 3, 2, 26, datetime('now', '+6 days', 'start of day', '+14 hours'), datetime('now', '+6 days', 'start of day', '+15 hours'));

-- League reservations
INSERT INTO reservations (id, facility_id, reservation_type_id, start_time, end_time, is_open_event, teams_per_court, people_per_team) VALUES
    (60, 1, 6, datetime('now', '+7 days', 'start of day', '+18 hours'), datetime('now', '+7 days', 'start of day', '+21 hours'), 0, 2, 4),
    (61, 2, 6, datetime('now', '+8 days', 'start of day', '+18 hours'), datetime('now', '+8 days', 'start of day', '+21 hours'), 0, 2, 4);

-- Events
INSERT INTO reservations (id, facility_id, reservation_type_id, start_time, end_time, is_open_event) VALUES
    (70, 1, 4, datetime('now', '+10 days', 'start of day', '+9 hours'), datetime('now', '+10 days', 'start of day', '+17 hours'), 1),
    (71, 3, 4, datetime('now', '+12 days', 'start of day', '+10 hours'), datetime('now', '+12 days', 'start of day', '+16 hours'), 1);

-- Maintenance
INSERT INTO reservations (id, facility_id, reservation_type_id, start_time, end_time) VALUES
    (80, 1, 5, datetime('now', '+14 days', 'start of day', '+6 hours'), datetime('now', '+14 days', 'start of day', '+8 hours'));

--------------------------------------------------------------------------------
-- RESERVATION COURTS (link reservations to courts)
--------------------------------------------------------------------------------
INSERT INTO reservation_courts (reservation_id, court_id) VALUES
    -- Past Downtown reservations (1-10) on courts 1-8
    (1, 1), (2, 2), (3, 3), (4, 4), (5, 5), (6, 6), (7, 7), (8, 8),
    (9, 1), (10, 2),
    -- Past pro sessions (11-13)
    (11, 3), (12, 4), (13, 5),
    -- Current/upcoming Downtown (20-29)
    (20, 1), (21, 2), (22, 3), (23, 4), (24, 5), (25, 6), (26, 7), (27, 8),
    (28, 1), (29, 2),
    -- Upcoming pro sessions (30-32)
    (30, 3), (31, 4), (32, 5),
    -- Westside (40-45) on courts 9-14
    (40, 9), (41, 10), (42, 11), (43, 12), (44, 13), (45, 14),
    -- Metro (50-55) on courts 15-18
    (50, 15), (51, 16), (52, 17), (53, 18), (54, 15), (55, 16),
    -- League uses multiple courts
    (60, 1), (60, 2), (60, 3), (60, 4),
    (61, 9), (61, 10), (61, 11),
    -- Event uses multiple courts
    (70, 1), (70, 2), (70, 3), (70, 4), (70, 5), (70, 6),
    (71, 15), (71, 16), (71, 17), (71, 18),
    -- Maintenance
    (80, 1), (80, 2);

--------------------------------------------------------------------------------
-- RESERVATION PARTICIPANTS (additional players beyond primary_user)
--------------------------------------------------------------------------------
INSERT INTO reservation_participants (reservation_id, user_id) VALUES
    -- Downtown games with extra participants
    (1, 8), (1, 9), (1, 10),
    (5, 8), (5, 12),
    (20, 9), (20, 11),
    (22, 7), (22, 8), (22, 10),
    -- Westside participants
    (42, 16), (42, 18),
    (43, 15), (43, 19),
    -- Metro participants
    (52, 22), (52, 24),
    (53, 21), (53, 25);

--------------------------------------------------------------------------------
-- OPEN PLAY SESSIONS
--------------------------------------------------------------------------------
-- Past sessions (completed)
INSERT INTO open_play_sessions (id, facility_id, open_play_rule_id, start_time, end_time, status, current_court_count) VALUES
    (1, 1, 1, datetime('now', '-7 days', 'start of day', '+7 hours'), datetime('now', '-7 days', 'start of day', '+9 hours'), 'completed', 2),
    (2, 1, 2, datetime('now', '-7 days', 'start of day', '+11 hours'), datetime('now', '-7 days', 'start of day', '+14 hours'), 'completed', 2),
    (3, 1, 3, datetime('now', '-6 days', 'start of day', '+17 hours'), datetime('now', '-6 days', 'start of day', '+20 hours'), 'completed', 3),
    (4, 1, 1, datetime('now', '-5 days', 'start of day', '+7 hours'), datetime('now', '-5 days', 'start of day', '+9 hours'), 'completed', 1),
    (5, 2, 5, datetime('now', '-5 days', 'start of day', '+7 hours'), datetime('now', '-5 days', 'start of day', '+9 hours'), 'completed', 2),
    (6, 3, 9, datetime('now', '-4 days', 'start of day', '+7 hours'), datetime('now', '-4 days', 'start of day', '+9 hours'), 'completed', 1);

-- Cancelled session
INSERT INTO open_play_sessions (id, facility_id, open_play_rule_id, start_time, end_time, status, current_court_count, cancelled_at, cancellation_reason) VALUES
    (7, 1, 2, datetime('now', '-3 days', 'start of day', '+11 hours'), datetime('now', '-3 days', 'start of day', '+14 hours'), 'cancelled', 0, datetime('now', '-3 days', 'start of day', '+10 hours'), 'Insufficient participants');

-- Upcoming sessions (scheduled)
INSERT INTO open_play_sessions (id, facility_id, open_play_rule_id, start_time, end_time, status, current_court_count) VALUES
    (10, 1, 1, datetime('now', '+1 days', 'start of day', '+7 hours'), datetime('now', '+1 days', 'start of day', '+9 hours'), 'scheduled', 1),
    (11, 1, 2, datetime('now', '+1 days', 'start of day', '+11 hours'), datetime('now', '+1 days', 'start of day', '+14 hours'), 'scheduled', 2),
    (12, 1, 3, datetime('now', '+1 days', 'start of day', '+17 hours'), datetime('now', '+1 days', 'start of day', '+20 hours'), 'scheduled', 2),
    (13, 1, 4, datetime('now', '+2 days', 'start of day', '+8 hours'), datetime('now', '+2 days', 'start of day', '+12 hours'), 'scheduled', 3),
    (14, 2, 5, datetime('now', '+1 days', 'start of day', '+7 hours'), datetime('now', '+1 days', 'start of day', '+9 hours'), 'scheduled', 1),
    (15, 2, 7, datetime('now', '+1 days', 'start of day', '+17 hours'), datetime('now', '+1 days', 'start of day', '+20 hours'), 'scheduled', 2),
    (16, 3, 9, datetime('now', '+1 days', 'start of day', '+7 hours'), datetime('now', '+1 days', 'start of day', '+9 hours'), 'scheduled', 1),
    (17, 3, 10, datetime('now', '+1 days', 'start of day', '+17 hours'), datetime('now', '+1 days', 'start of day', '+20 hours'), 'scheduled', 1);

--------------------------------------------------------------------------------
-- FACILITY VISITS (check-ins)
-- checked_in_by_staff_id references users.id (staff users 1-6)
--------------------------------------------------------------------------------
INSERT INTO facility_visits (id, user_id, facility_id, check_in_time, check_out_time, checked_in_by_staff_id, activity_type, related_reservation_id) VALUES
    (1, 7, 1, datetime('now', '-7 days', 'start of day', '+9 hours'), datetime('now', '-7 days', 'start of day', '+10 hours', '+30 minutes'), 3, 'court_reservation', 9),
    (2, 8, 1, datetime('now', '-7 days', 'start of day', '+7 hours'), datetime('now', '-7 days', 'start of day', '+9 hours'), 3, 'open_play', NULL),
    (3, 9, 1, datetime('now', '-6 days', 'start of day', '+17 hours'), datetime('now', '-6 days', 'start of day', '+20 hours'), 3, 'open_play', NULL),
    (4, 15, 2, datetime('now', '-5 days', 'start of day', '+10 hours'), datetime('now', '-5 days', 'start of day', '+11 hours', '+15 minutes'), 5, 'court_reservation', 40),
    (5, 21, 3, datetime('now', '-4 days', 'start of day', '+9 hours'), datetime('now', '-4 days', 'start of day', '+10 hours'), 6, 'court_reservation', 50);

COMMIT;

--------------------------------------------------------------------------------
-- SUMMARY
--------------------------------------------------------------------------------
-- Organizations: 2
-- Facilities: 3 (Downtown 8 courts, Westside 6 courts, Metro 4 courts)
-- Courts: 18 total
-- Users: 26 (6 staff + 20 members)
-- Staff: 6 (IDs 1-6)
-- Members: 20 (IDs 7-26)
-- Themes: 6 custom (+ system themes from migrations)
-- Open Play Rules: 11
-- Reservations: 43
-- Open Play Sessions: 15
-- Facility Visits: 5
