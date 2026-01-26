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

-- name: GetLeagueWithFacilityTimezone :one
SELECT l.id, l.facility_id, l.name, l.format, l.start_date, l.end_date, l.division_config,
    l.min_team_size, l.max_team_size, l.roster_lock_date, l.status, l.created_at, l.updated_at,
    f.timezone AS facility_timezone
FROM leagues l
JOIN facilities f ON f.id = l.facility_id
WHERE l.id = @id;

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
  AND league_id = @league_id
RETURNING id, league_id, home_team_id, away_team_id, reservation_id,
    scheduled_time, home_score, away_score, status, created_at, updated_at;

-- name: GetLeagueMatch :one
SELECT id, league_id, home_team_id, away_team_id, reservation_id,
    scheduled_time, home_score, away_score, status, created_at, updated_at
FROM league_matches
WHERE id = @id
  AND league_id = @league_id;

-- name: GetLeagueStandingsData :many
SELECT lt.id AS team_id,
    lt.name AS team_name,
    lm.id AS match_id,
    lm.home_team_id,
    lm.away_team_id,
    lm.home_score,
    lm.away_score
FROM league_teams lt
LEFT JOIN league_matches lm
    ON lm.league_id = lt.league_id
    AND lm.status = 'completed'
    AND (lm.home_team_id = lt.id OR lm.away_team_id = lt.id)
WHERE lt.league_id = @league_id
ORDER BY lt.name, lm.scheduled_time;

-- name: ListLeagueMatches :many
SELECT id, league_id, home_team_id, away_team_id, reservation_id,
    scheduled_time, home_score, away_score, status, created_at, updated_at
FROM league_matches
WHERE league_id = @league_id
ORDER BY scheduled_time;

-- name: ListLeagueMatchesWithReservations :many
SELECT lm.id,
    lm.league_id,
    lm.home_team_id,
    lm.away_team_id,
    lm.reservation_id,
    lm.scheduled_time,
    lm.home_score,
    lm.away_score,
    lm.status,
    lm.created_at,
    lm.updated_at,
    r.start_time,
    r.end_time,
    CASE
        WHEN COUNT(rc.court_id) = 0 THEN NULL
        ELSE MIN(rc.court_id)
    END AS court_id
FROM league_matches lm
LEFT JOIN reservations r ON r.id = lm.reservation_id
LEFT JOIN reservation_courts rc ON rc.reservation_id = r.id
WHERE lm.league_id = @league_id
GROUP BY lm.id,
    lm.league_id,
    lm.home_team_id,
    lm.away_team_id,
    lm.reservation_id,
    lm.scheduled_time,
    lm.home_score,
    lm.away_score,
    lm.status,
    lm.created_at,
    lm.updated_at,
    r.start_time,
    r.end_time
ORDER BY lm.scheduled_time;

-- name: DeleteLeagueMatchesByLeagueID :execrows
DELETE FROM league_matches
WHERE league_id = @league_id;

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
