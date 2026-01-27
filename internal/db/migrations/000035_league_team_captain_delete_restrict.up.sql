PRAGMA foreign_keys = OFF;

CREATE TABLE league_teams_new (
    id INTEGER PRIMARY KEY,
    league_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    captain_user_id INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'inactive')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (league_id) REFERENCES leagues(id) ON DELETE CASCADE,
    FOREIGN KEY (captain_user_id) REFERENCES users(id) ON DELETE RESTRICT,
    UNIQUE (league_id, name)
);

INSERT INTO league_teams_new (
    id,
    league_id,
    name,
    captain_user_id,
    status,
    created_at,
    updated_at
)
SELECT
    id,
    league_id,
    name,
    captain_user_id,
    status,
    created_at,
    updated_at
FROM league_teams;

DROP TABLE league_teams;
ALTER TABLE league_teams_new RENAME TO league_teams;

CREATE UNIQUE INDEX IF NOT EXISTS idx_league_teams_league_id_name ON league_teams(league_id, name);
CREATE INDEX IF NOT EXISTS idx_league_teams_league_id ON league_teams(league_id);
CREATE INDEX IF NOT EXISTS idx_league_teams_captain_user_id ON league_teams(captain_user_id);

PRAGMA foreign_keys = ON;
