-- internal/db/queries/leagues.sql

-- name: CreateLeague :one
INSERT INTO leagues (
    facility_id,
    name,
    format,
    start_date,
    end_date,
    division_config,
    min_team_size,
    max_team_size,
    roster_lock_date,
    status
) VALUES (
    @facility_id,
    @name,
    @format,
    @start_date,
    @end_date,
    @division_config,
    @min_team_size,
    @max_team_size,
    @roster_lock_date,
    @status
)
RETURNING id, facility_id, name, format, start_date, end_date, division_config,
    min_team_size, max_team_size, roster_lock_date, status, created_at, updated_at;

-- name: GetLeague :one
SELECT id, facility_id, name, format, start_date, end_date, division_config,
    min_team_size, max_team_size, roster_lock_date, status, created_at, updated_at
FROM leagues
WHERE id = @id;

-- name: ListLeaguesByFacility :many
SELECT id, facility_id, name, format, start_date, end_date, division_config,
    min_team_size, max_team_size, roster_lock_date, status, created_at, updated_at
FROM leagues
WHERE facility_id = @facility_id
ORDER BY start_date DESC, name;

-- name: CreateLeagueTeam :one
INSERT INTO league_teams (
    league_id,
    name,
    captain_user_id,
    status
) VALUES (
    @league_id,
    @name,
    @captain_user_id,
    @status
)
RETURNING id, league_id, name, captain_user_id, status, created_at, updated_at;

-- name: GetLeagueTeam :one
SELECT id, league_id, name, captain_user_id, status, created_at, updated_at
FROM league_teams
WHERE id = @id;

-- name: ListLeagueTeams :many
SELECT id, league_id, name, captain_user_id, status, created_at, updated_at
FROM league_teams
WHERE league_id = @league_id
ORDER BY name;

-- name: AddTeamMember :one
INSERT INTO league_team_members (
    league_team_id,
    user_id,
    is_free_agent
) VALUES (
    @league_team_id,
    @user_id,
    @is_free_agent
)
RETURNING id, league_team_id, user_id, is_free_agent, created_at;

-- name: ListTeamMembers :many
SELECT id, league_team_id, user_id, is_free_agent, created_at
FROM league_team_members
WHERE league_team_id = @league_team_id
ORDER BY created_at;

-- name: CreateLeagueMatch :one
INSERT INTO league_matches (
    league_id,
    home_team_id,
    away_team_id,
    reservation_id,
    scheduled_time,
    home_score,
    away_score,
    status
) VALUES (
    @league_id,
    @home_team_id,
    @away_team_id,
    @reservation_id,
    @scheduled_time,
    @home_score,
    @away_score,
    @status
)
RETURNING id, league_id, home_team_id, away_team_id, reservation_id,
    scheduled_time, home_score, away_score, status, created_at, updated_at;

-- name: UpdateMatchResult :one
UPDATE league_matches
SET home_score = @home_score,
    away_score = @away_score,
    status = @status,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING id, league_id, home_team_id, away_team_id, reservation_id,
    scheduled_time, home_score, away_score, status, created_at, updated_at;

-- name: ListLeagueMatches :many
SELECT id, league_id, home_team_id, away_team_id, reservation_id,
    scheduled_time, home_score, away_score, status, created_at, updated_at
FROM league_matches
WHERE league_id = @league_id
ORDER BY scheduled_time;
