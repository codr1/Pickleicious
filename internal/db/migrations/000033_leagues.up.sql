CREATE TABLE IF NOT EXISTS leagues (
    id INTEGER PRIMARY KEY,
    facility_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    format TEXT NOT NULL CHECK (format IN ('singles', 'doubles', 'mixed_doubles')),
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    division_config TEXT NOT NULL,
    min_team_size INTEGER NOT NULL,
    max_team_size INTEGER NOT NULL,
    roster_lock_date DATE,
    status TEXT NOT NULL CHECK (status IN ('draft', 'registration', 'active', 'completed', 'cancelled')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (start_date <= end_date),
    CHECK (min_team_size > 0 AND max_team_size > 0 AND min_team_size <= max_team_size),
    FOREIGN KEY (facility_id) REFERENCES facilities(id)
);

CREATE TABLE IF NOT EXISTS league_teams (
    id INTEGER PRIMARY KEY,
    league_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    captain_user_id INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (league_id) REFERENCES leagues(id) ON DELETE CASCADE,
    FOREIGN KEY (captain_user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS league_team_members (
    id INTEGER PRIMARY KEY,
    league_team_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    is_free_agent BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (league_team_id) REFERENCES league_teams(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id),
    UNIQUE (league_team_id, user_id)
);

CREATE TABLE IF NOT EXISTS league_matches (
    id INTEGER PRIMARY KEY,
    league_id INTEGER NOT NULL,
    home_team_id INTEGER NOT NULL,
    away_team_id INTEGER NOT NULL,
    reservation_id INTEGER,
    scheduled_time DATETIME NOT NULL,
    home_score INTEGER,
    away_score INTEGER,
    status TEXT NOT NULL CHECK (status IN ('scheduled', 'in_progress', 'completed', 'cancelled', 'forfeit')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (home_team_id != away_team_id),
    FOREIGN KEY (league_id) REFERENCES leagues(id) ON DELETE CASCADE,
    FOREIGN KEY (home_team_id) REFERENCES league_teams(id) ON DELETE RESTRICT,
    FOREIGN KEY (away_team_id) REFERENCES league_teams(id) ON DELETE RESTRICT,
    FOREIGN KEY (reservation_id) REFERENCES reservations(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_leagues_facility_id ON leagues(facility_id);
CREATE INDEX IF NOT EXISTS idx_league_teams_league_id ON league_teams(league_id);
CREATE INDEX IF NOT EXISTS idx_league_teams_captain_user_id ON league_teams(captain_user_id);
CREATE INDEX IF NOT EXISTS idx_league_team_members_team_id ON league_team_members(league_team_id);
CREATE INDEX IF NOT EXISTS idx_league_team_members_user_id ON league_team_members(user_id);
CREATE INDEX IF NOT EXISTS idx_league_matches_league_id ON league_matches(league_id);
CREATE INDEX IF NOT EXISTS idx_league_matches_reservation_id ON league_matches(reservation_id);
CREATE INDEX IF NOT EXISTS idx_league_matches_scheduled_time ON league_matches(scheduled_time);
