DROP INDEX IF EXISTS idx_league_matches_scheduled_time;
DROP INDEX IF EXISTS idx_league_matches_reservation_id;
DROP INDEX IF EXISTS idx_league_matches_league_id;
DROP INDEX IF EXISTS idx_league_team_members_user_id;
DROP INDEX IF EXISTS idx_league_team_members_team_id;
DROP INDEX IF EXISTS idx_league_teams_captain_user_id;
DROP INDEX IF EXISTS idx_league_teams_league_id;
DROP INDEX IF EXISTS idx_leagues_facility_id;

DROP TABLE IF EXISTS league_matches;
DROP TABLE IF EXISTS league_team_members;
DROP TABLE IF EXISTS league_teams;
DROP TABLE IF EXISTS leagues;
