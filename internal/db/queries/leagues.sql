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

-- name: UpdateLeague :one
UPDATE leagues
SET name = @name,
    format = @format,
    start_date = @start_date,
    end_date = @end_date,
    division_config = @division_config,
    min_team_size = @min_team_size,
    max_team_size = @max_team_size,
    roster_lock_date = @roster_lock_date,
    status = @status,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
RETURNING id, facility_id, name, format, start_date, end_date, division_config,
    min_team_size, max_team_size, roster_lock_date, status, created_at, updated_at;

-- name: DeleteLeague :execrows
DELETE FROM leagues
WHERE id = @id;

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

-- name: UpdateLeagueTeam :one
UPDATE league_teams
SET name = @name,
    status = @status,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND league_id = @league_id
RETURNING id, league_id, name, captain_user_id, status, created_at, updated_at;

-- name: UpdateTeamCaptain :one
UPDATE league_teams
SET captain_user_id = @captain_user_id,
    updated_at = CURRENT_TIMESTAMP
WHERE id = @id
  AND league_id = @league_id
RETURNING id, league_id, name, captain_user_id, status, created_at, updated_at;

-- name: RemoveTeamMember :execrows
DELETE FROM league_team_members
WHERE league_team_id = @league_team_id
  AND user_id = @user_id;

-- name: ListFreeAgentsByLeague :many
SELECT ltm.id,
    ltm.league_team_id,
    ltm.user_id,
    u.first_name,
    u.last_name,
    u.photo_url,
    ltm.created_at
FROM league_team_members ltm
JOIN league_teams lt ON lt.id = ltm.league_team_id
JOIN users u ON u.id = ltm.user_id
WHERE lt.league_id = @league_id
  AND ltm.is_free_agent = 1
ORDER BY u.last_name, u.first_name;

-- name: AssignFreeAgentToTeam :one
UPDATE league_team_members
SET league_team_id = @league_team_id,
    is_free_agent = 0
WHERE user_id = @user_id
  AND is_free_agent = 1
  AND league_team_id IN (
    SELECT id FROM league_teams WHERE league_teams.league_id = @league_id
  )
  AND EXISTS (
    SELECT 1 FROM league_teams lt WHERE lt.id = @league_team_id AND lt.league_id = @league_id
  )
RETURNING id, league_team_id, user_id, is_free_agent, created_at;
