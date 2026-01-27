PRAGMA foreign_keys = OFF;

CREATE TABLE league_matches_new (
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
    CHECK (home_score IS NULL OR home_score >= 0),
    CHECK (away_score IS NULL OR away_score >= 0),
    FOREIGN KEY (league_id) REFERENCES leagues(id) ON DELETE CASCADE,
    FOREIGN KEY (home_team_id) REFERENCES league_teams(id) ON DELETE RESTRICT,
    FOREIGN KEY (away_team_id) REFERENCES league_teams(id) ON DELETE RESTRICT,
    FOREIGN KEY (reservation_id) REFERENCES reservations(id) ON DELETE SET NULL
);

INSERT INTO league_matches_new (
    id,
    league_id,
    home_team_id,
    away_team_id,
    reservation_id,
    scheduled_time,
    home_score,
    away_score,
    status,
    created_at,
    updated_at
)
SELECT
    id,
    league_id,
    home_team_id,
    away_team_id,
    reservation_id,
    scheduled_time,
    home_score,
    away_score,
    status,
    created_at,
    updated_at
FROM league_matches;

DROP TABLE league_matches;
ALTER TABLE league_matches_new RENAME TO league_matches;

CREATE INDEX IF NOT EXISTS idx_league_matches_league_id ON league_matches(league_id);
CREATE INDEX IF NOT EXISTS idx_league_matches_reservation_id ON league_matches(reservation_id);
CREATE INDEX IF NOT EXISTS idx_league_matches_scheduled_time ON league_matches(scheduled_time);
CREATE INDEX IF NOT EXISTS idx_league_matches_home_team_id ON league_matches(home_team_id);
CREATE INDEX IF NOT EXISTS idx_league_matches_away_team_id ON league_matches(away_team_id);

PRAGMA foreign_keys = ON;
