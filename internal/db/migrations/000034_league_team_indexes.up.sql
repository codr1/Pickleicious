CREATE UNIQUE INDEX IF NOT EXISTS idx_league_teams_league_id_name ON league_teams(league_id, name);
CREATE INDEX IF NOT EXISTS idx_league_matches_home_team_id ON league_matches(home_team_id);
CREATE INDEX IF NOT EXISTS idx_league_matches_away_team_id ON league_matches(away_team_id);
