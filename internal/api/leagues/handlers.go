// internal/api/leagues/handlers.go
package leagues

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/api/htmx"
	appdb "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

const (
	leagueQueryTimeout  = 5 * time.Second
	facilityIDQueryKey  = "facility_id"
	leagueIDPathKey     = "id"
	teamIDPathKey       = "team_id"
	userIDPathKey       = "user_id"
	leagueDateLayout    = "2006-01-02"
	defaultLeagueStatus = "draft"
	defaultTeamStatus   = "active"
)

var (
	queries *dbgen.Queries
)

type leagueRequest struct {
	FacilityID     *int64 `json:"facilityId"`
	Name           string `json:"name"`
	Format         string `json:"format"`
	StartDate      string `json:"startDate"`
	EndDate        string `json:"endDate"`
	DivisionConfig string `json:"divisionConfig"`
	MinTeamSize    int64  `json:"minTeamSize"`
	MaxTeamSize    int64  `json:"maxTeamSize"`
	RosterLockDate string `json:"rosterLockDate"`
	Status         string `json:"status"`
}

type teamRequest struct {
	Name          string `json:"name"`
	CaptainUserID int64  `json:"captainUserId"`
	Status        string `json:"status"`
}

type leagueInput struct {
	Name           string
	Format         string
	StartDate      time.Time
	EndDate        time.Time
	DivisionConfig string
	MinTeamSize    int64
	MaxTeamSize    int64
	RosterLockDate sql.NullTime
	Status         string
}

type teamInput struct {
	Name          string
	CaptainUserID int64
	Status        string
}

type teamMemberRequest struct {
	UserID      int64 `json:"userId"`
	IsFreeAgent bool  `json:"isFreeAgent"`
}

type assignFreeAgentRequest struct {
	TeamID int64 `json:"teamId"`
}

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(database *appdb.DB) {
	if database == nil {
		return
	}
	queries = database.Queries
}

// GET /leagues
func HandleLeaguesPage(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, err := facilityIDFromQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	leagues, err := q.ListLeaguesByFacility(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to list leagues")
		http.Error(w, "Failed to load leagues", http.StatusInternalServerError)
		return
	}

	activeTheme, err := models.GetActiveTheme(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
		activeTheme = nil
	}

	sessionType := authz.SessionTypeFromContext(r.Context())
	page := layouts.Base(leaguesPageComponent(leagues, facilityID), activeTheme, sessionType)
	if !apiutil.RenderHTMLComponent(r.Context(), w, page, nil, "Failed to render leagues page", "Failed to render page") {
		return
	}
}

// GET /api/v1/leagues
func HandleLeaguesList(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, err := facilityIDFromQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	leagues, err := q.ListLeaguesByFacility(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to list leagues")
		http.Error(w, "Failed to list leagues", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		component := leaguesListComponent(leagues)
		if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render leagues list", "Failed to render list") {
			return
		}
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"leagues": leagues}); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to write leagues response")
	}
}

// POST /api/v1/leagues
func HandleLeagueCreate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	req, err := decodeLeagueRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityID, err := facilityIDFromRequest(r, req.FacilityID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	input, err := parseLeagueRequest(req, defaultLeagueStatus)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.CreateLeague(ctx, dbgen.CreateLeagueParams{
		FacilityID:     facilityID,
		Name:           input.Name,
		Format:         input.Format,
		StartDate:      input.StartDate,
		EndDate:        input.EndDate,
		DivisionConfig: input.DivisionConfig,
		MinTeamSize:    input.MinTeamSize,
		MaxTeamSize:    input.MaxTeamSize,
		RosterLockDate: input.RosterLockDate,
		Status:         input.Status,
	})
	if err != nil {
		if apiutil.IsSQLiteForeignKeyViolation(err) {
			http.Error(w, "Facility not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to create league")
		http.Error(w, "Failed to create league", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		headers := map[string]string{
			"HX-Trigger": "refreshLeaguesList",
		}
		component := leagueDetailComponent(league)
		if !apiutil.RenderHTMLComponent(r.Context(), w, component, headers, "Failed to render league detail", "Failed to render response") {
			return
		}
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, league); err != nil {
		logger.Error().Err(err).Int64("league_id", league.ID).Msg("Failed to write league response")
	}
}

// GET /api/v1/leagues/{id}
func HandleLeagueDetail(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to fetch league")
		http.Error(w, "Failed to fetch league", http.StatusInternalServerError)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, league.FacilityID) {
		return
	}

	if htmx.IsRequest(r) {
		component := leagueDetailComponent(league)
		if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render league detail", "Failed to render response") {
			return
		}
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, league); err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to write league response")
	}
}

// PUT /api/v1/leagues/{id}
func HandleLeagueUpdate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	req, err := decodeLeagueRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	input, err := parseLeagueRequest(req, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to fetch league")
		http.Error(w, "Failed to fetch league", http.StatusInternalServerError)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, league.FacilityID) {
		return
	}

	updated, err := q.UpdateLeague(ctx, dbgen.UpdateLeagueParams{
		ID:             leagueID,
		Name:           input.Name,
		Format:         input.Format,
		StartDate:      input.StartDate,
		EndDate:        input.EndDate,
		DivisionConfig: input.DivisionConfig,
		MinTeamSize:    input.MinTeamSize,
		MaxTeamSize:    input.MaxTeamSize,
		RosterLockDate: input.RosterLockDate,
		Status:         input.Status,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to update league")
		http.Error(w, "Failed to update league", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		headers := map[string]string{
			"HX-Trigger": "refreshLeaguesList",
		}
		component := leagueDetailComponent(updated)
		if !apiutil.RenderHTMLComponent(r.Context(), w, component, headers, "Failed to render league detail", "Failed to render response") {
			return
		}
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, updated); err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to write league response")
	}
}

// DELETE /api/v1/leagues/{id}
func HandleLeagueDelete(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to fetch league")
		http.Error(w, "Failed to fetch league", http.StatusInternalServerError)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, league.FacilityID) {
		return
	}

	affected, err := q.DeleteLeague(ctx, leagueID)
	if err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to delete league")
		http.Error(w, "Failed to delete league", http.StatusInternalServerError)
		return
	}
	if affected == 0 {
		http.Error(w, "League not found", http.StatusNotFound)
		return
	}

	if htmx.IsRequest(r) {
		headers := map[string]string{
			"HX-Trigger": "refreshLeaguesList",
		}
		component := leagueDeleteComponent()
		if !apiutil.RenderHTMLComponent(r.Context(), w, component, headers, "Failed to render delete response", "Failed to render response") {
			return
		}
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"deleted": leagueID}); err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to write league delete response")
	}
}

// POST /api/v1/leagues/{id}/teams
func HandleTeamCreate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	req, err := decodeTeamRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	input, err := parseTeamRequest(req, defaultTeamStatus)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to fetch league")
		http.Error(w, "Failed to fetch league", http.StatusInternalServerError)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, league.FacilityID) {
		return
	}

	if _, err := q.GetUserByID(ctx, input.CaptainUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Captain not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("captain_user_id", input.CaptainUserID).Msg("Failed to fetch captain")
		http.Error(w, "Failed to create team", http.StatusInternalServerError)
		return
	}

	team, err := q.CreateLeagueTeam(ctx, dbgen.CreateLeagueTeamParams{
		LeagueID:      leagueID,
		Name:          input.Name,
		CaptainUserID: input.CaptainUserID,
		Status:        input.Status,
	})
	if err != nil {
		if apiutil.IsSQLiteForeignKeyViolation(err) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to create team")
		http.Error(w, "Failed to create team", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, team); err != nil {
		logger.Error().Err(err).Int64("team_id", team.ID).Msg("Failed to write team response")
	}
}

// GET /api/v1/leagues/{id}/teams
func HandleListLeagueTeams(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to fetch league")
		http.Error(w, "Failed to fetch league", http.StatusInternalServerError)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, league.FacilityID) {
		return
	}

	teams, err := q.ListLeagueTeams(ctx, leagueID)
	if err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to list league teams")
		http.Error(w, "Failed to list teams", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"teams": teams}); err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to write teams response")
	}
}

// GET /api/v1/leagues/{id}/teams/{team_id}
func HandleTeamDetail(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	teamID, err := teamIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid team ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to fetch league")
		http.Error(w, "Failed to fetch league", http.StatusInternalServerError)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, league.FacilityID) {
		return
	}

	team, err := q.GetLeagueTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to fetch team")
		http.Error(w, "Failed to fetch team", http.StatusInternalServerError)
		return
	}
	if team.LeagueID != leagueID {
		http.Error(w, "Team not found", http.StatusNotFound)
		return
	}

	members, err := q.ListTeamMembers(ctx, teamID)
	if err != nil {
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to list team members")
		http.Error(w, "Failed to fetch team members", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{
		"team":    team,
		"members": members,
	}); err != nil {
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to write team response")
	}
}

// PUT /api/v1/leagues/{id}/teams/{team_id}
func HandleTeamUpdate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	teamID, err := teamIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid team ID", http.StatusBadRequest)
		return
	}

	req, err := decodeTeamRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	input, err := parseTeamRequest(req, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to fetch league")
		http.Error(w, "Failed to fetch league", http.StatusInternalServerError)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, league.FacilityID) {
		return
	}

	team, err := q.GetLeagueTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to fetch team")
		http.Error(w, "Failed to update team", http.StatusInternalServerError)
		return
	}
	if team.LeagueID != leagueID {
		http.Error(w, "Team not found", http.StatusNotFound)
		return
	}

	if _, err := q.GetUserByID(ctx, input.CaptainUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Captain not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("captain_user_id", input.CaptainUserID).Msg("Failed to fetch captain")
		http.Error(w, "Failed to update team", http.StatusInternalServerError)
		return
	}

	updated, err := q.UpdateLeagueTeam(ctx, dbgen.UpdateLeagueTeamParams{
		ID:       teamID,
		LeagueID: leagueID,
		Name:     input.Name,
		Status:   input.Status,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to update team")
		http.Error(w, "Failed to update team", http.StatusInternalServerError)
		return
	}

	if input.CaptainUserID != team.CaptainUserID {
		updated, err = q.UpdateTeamCaptain(ctx, dbgen.UpdateTeamCaptainParams{
			ID:            teamID,
			LeagueID:      leagueID,
			CaptainUserID: input.CaptainUserID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Team not found", http.StatusNotFound)
				return
			}
			logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to update team captain")
			http.Error(w, "Failed to update team", http.StatusInternalServerError)
			return
		}
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, updated); err != nil {
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to write team response")
	}
}

// POST /api/v1/leagues/{id}/teams/{team_id}/members
func HandleAddTeamMember(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	teamID, err := teamIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid team ID", http.StatusBadRequest)
		return
	}

	req, err := decodeTeamMemberRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.UserID <= 0 {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to fetch league")
		http.Error(w, "Failed to fetch league", http.StatusInternalServerError)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, league.FacilityID) {
		return
	}

	if rosterLocked(league) {
		http.Error(w, "Roster is locked for this league", http.StatusConflict)
		return
	}

	team, err := q.GetLeagueTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to fetch team")
		http.Error(w, "Failed to add team member", http.StatusInternalServerError)
		return
	}
	if team.LeagueID != leagueID {
		http.Error(w, "Team not found", http.StatusNotFound)
		return
	}

	members, err := q.ListTeamMembers(ctx, teamID)
	if err != nil {
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to list team members")
		http.Error(w, "Failed to add team member", http.StatusInternalServerError)
		return
	}
	if int64(len(members)) >= league.MaxTeamSize {
		http.Error(w, "Team is at max size", http.StatusConflict)
		return
	}

	if _, err := q.GetUserByID(ctx, req.UserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("user_id", req.UserID).Msg("Failed to fetch user")
		http.Error(w, "Failed to add team member", http.StatusInternalServerError)
		return
	}

	member, err := q.AddTeamMember(ctx, dbgen.AddTeamMemberParams{
		LeagueTeamID: teamID,
		UserID:       req.UserID,
		IsFreeAgent:  req.IsFreeAgent,
	})
	if err != nil {
		if apiutil.IsSQLiteUniqueViolation(err) {
			http.Error(w, "Team member already exists", http.StatusConflict)
			return
		}
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to add team member")
		http.Error(w, "Failed to add team member", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, member); err != nil {
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to write team member response")
	}
}

// DELETE /api/v1/leagues/{id}/teams/{team_id}/members/{user_id}
func HandleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	teamID, err := teamIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid team ID", http.StatusBadRequest)
		return
	}

	userID, err := userIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to fetch league")
		http.Error(w, "Failed to fetch league", http.StatusInternalServerError)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, league.FacilityID) {
		return
	}

	if rosterLocked(league) {
		http.Error(w, "Roster is locked for this league", http.StatusConflict)
		return
	}

	team, err := q.GetLeagueTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to fetch team")
		http.Error(w, "Failed to remove team member", http.StatusInternalServerError)
		return
	}
	if team.LeagueID != leagueID {
		http.Error(w, "Team not found", http.StatusNotFound)
		return
	}

	affected, err := q.RemoveTeamMember(ctx, dbgen.RemoveTeamMemberParams{
		LeagueTeamID: teamID,
		UserID:       userID,
	})
	if err != nil {
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to remove team member")
		http.Error(w, "Failed to remove team member", http.StatusInternalServerError)
		return
	}
	if affected == 0 {
		http.Error(w, "Team member not found", http.StatusNotFound)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"removed": userID}); err != nil {
		logger.Error().Err(err).Int64("team_id", teamID).Msg("Failed to write team member response")
	}
}

// GET /api/v1/leagues/{id}/free-agents
func HandleListFreeAgents(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to fetch league")
		http.Error(w, "Failed to fetch league", http.StatusInternalServerError)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, league.FacilityID) {
		return
	}

	freeAgents, err := q.ListFreeAgentsByLeague(ctx, leagueID)
	if err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to list free agents")
		http.Error(w, "Failed to list free agents", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"freeAgents": freeAgents}); err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to write free agents response")
	}
}

// POST /api/v1/leagues/{id}/free-agents/{user_id}/assign
func HandleAssignFreeAgent(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	userID, err := userIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	req, err := decodeAssignFreeAgentRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.TeamID <= 0 {
		http.Error(w, "Invalid team ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), leagueQueryTimeout)
	defer cancel()

	league, err := q.GetLeague(ctx, leagueID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "League not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to fetch league")
		http.Error(w, "Failed to fetch league", http.StatusInternalServerError)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, league.FacilityID) {
		return
	}

	if rosterLocked(league) {
		http.Error(w, "Roster is locked for this league", http.StatusConflict)
		return
	}

	team, err := q.GetLeagueTeam(ctx, req.TeamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("team_id", req.TeamID).Msg("Failed to fetch team")
		http.Error(w, "Failed to assign free agent", http.StatusInternalServerError)
		return
	}
	if team.LeagueID != leagueID {
		http.Error(w, "Team not found", http.StatusNotFound)
		return
	}

	members, err := q.ListTeamMembers(ctx, req.TeamID)
	if err != nil {
		logger.Error().Err(err).Int64("team_id", req.TeamID).Msg("Failed to list team members")
		http.Error(w, "Failed to assign free agent", http.StatusInternalServerError)
		return
	}
	if int64(len(members)) >= league.MaxTeamSize {
		http.Error(w, "Team is at max size", http.StatusConflict)
		return
	}

	assigned, err := q.AssignFreeAgentToTeam(ctx, dbgen.AssignFreeAgentToTeamParams{
		LeagueID:     leagueID,
		LeagueTeamID: req.TeamID,
		UserID:       userID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Free agent not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to assign free agent")
		http.Error(w, "Failed to assign free agent", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, assigned); err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to write free agent response")
	}
}

func decodeLeagueRequest(r *http.Request) (leagueRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req leagueRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return leagueRequest{}, err
	}

	facilityID, err := apiutil.ParseOptionalInt64Field(apiutil.FirstNonEmpty(r.FormValue("facility_id"), r.FormValue("facilityId")), "facility_id")
	if err != nil {
		return leagueRequest{}, err
	}

	minTeamSize, err := apiutil.ParseRequiredInt64Field(apiutil.FirstNonEmpty(r.FormValue("min_team_size"), r.FormValue("minTeamSize")), "min_team_size")
	if err != nil {
		return leagueRequest{}, err
	}

	maxTeamSize, err := apiutil.ParseRequiredInt64Field(apiutil.FirstNonEmpty(r.FormValue("max_team_size"), r.FormValue("maxTeamSize")), "max_team_size")
	if err != nil {
		return leagueRequest{}, err
	}

	return leagueRequest{
		FacilityID:     facilityID,
		Name:           apiutil.FirstNonEmpty(r.FormValue("name")),
		Format:         apiutil.FirstNonEmpty(r.FormValue("format")),
		StartDate:      apiutil.FirstNonEmpty(r.FormValue("start_date"), r.FormValue("startDate")),
		EndDate:        apiutil.FirstNonEmpty(r.FormValue("end_date"), r.FormValue("endDate")),
		DivisionConfig: apiutil.FirstNonEmpty(r.FormValue("division_config"), r.FormValue("divisionConfig")),
		MinTeamSize:    minTeamSize,
		MaxTeamSize:    maxTeamSize,
		RosterLockDate: apiutil.FirstNonEmpty(r.FormValue("roster_lock_date"), r.FormValue("rosterLockDate")),
		Status:         apiutil.FirstNonEmpty(r.FormValue("status")),
	}, nil
}

func decodeTeamRequest(r *http.Request) (teamRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req teamRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return teamRequest{}, err
	}

	captainUserID, err := apiutil.ParseRequiredInt64Field(apiutil.FirstNonEmpty(r.FormValue("captain_user_id"), r.FormValue("captainUserId")), "captain_user_id")
	if err != nil {
		return teamRequest{}, err
	}

	return teamRequest{
		Name:          apiutil.FirstNonEmpty(r.FormValue("name")),
		CaptainUserID: captainUserID,
		Status:        apiutil.FirstNonEmpty(r.FormValue("status")),
	}, nil
}

func decodeTeamMemberRequest(r *http.Request) (teamMemberRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req teamMemberRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return teamMemberRequest{}, err
	}

	userID, err := apiutil.ParseRequiredInt64Field(apiutil.FirstNonEmpty(r.FormValue("user_id"), r.FormValue("userId")), "user_id")
	if err != nil {
		return teamMemberRequest{}, err
	}

	return teamMemberRequest{
		UserID:      userID,
		IsFreeAgent: apiutil.ParseBool(apiutil.FirstNonEmpty(r.FormValue("is_free_agent"), r.FormValue("isFreeAgent"))),
	}, nil
}

func decodeAssignFreeAgentRequest(r *http.Request) (assignFreeAgentRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req assignFreeAgentRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return assignFreeAgentRequest{}, err
	}

	teamID, err := apiutil.ParseRequiredInt64Field(apiutil.FirstNonEmpty(r.FormValue("team_id"), r.FormValue("teamId")), "team_id")
	if err != nil {
		return assignFreeAgentRequest{}, err
	}

	return assignFreeAgentRequest{
		TeamID: teamID,
	}, nil
}

func parseLeagueRequest(req leagueRequest, defaultStatus string) (leagueInput, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return leagueInput{}, fmt.Errorf("name is required")
	}

	format := strings.ToLower(strings.TrimSpace(req.Format))
	if !leagueFormatAllowed(format) {
		return leagueInput{}, fmt.Errorf("format must be singles, doubles, or mixed_doubles")
	}

	startDate, err := parseLeagueDate(req.StartDate)
	if err != nil {
		return leagueInput{}, fmt.Errorf("start_date must be a valid date")
	}

	endDate, err := parseLeagueDate(req.EndDate)
	if err != nil {
		return leagueInput{}, fmt.Errorf("end_date must be a valid date")
	}
	if startDate.After(endDate) {
		return leagueInput{}, fmt.Errorf("start_date must be on or before end_date")
	}

	divisionConfig := strings.TrimSpace(req.DivisionConfig)
	if divisionConfig == "" {
		return leagueInput{}, fmt.Errorf("division_config is required")
	}

	if req.MinTeamSize <= 0 {
		return leagueInput{}, fmt.Errorf("min_team_size must be greater than 0")
	}
	if req.MaxTeamSize <= 0 {
		return leagueInput{}, fmt.Errorf("max_team_size must be greater than 0")
	}
	if req.MinTeamSize > req.MaxTeamSize {
		return leagueInput{}, fmt.Errorf("min_team_size must be less than or equal to max_team_size")
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		status = defaultStatus
	}
	if !leagueStatusAllowed(status) {
		return leagueInput{}, fmt.Errorf("status must be draft, registration, active, completed, or cancelled")
	}

	rosterLockDate, err := parseOptionalLeagueDate(req.RosterLockDate)
	if err != nil {
		return leagueInput{}, fmt.Errorf("roster_lock_date must be a valid date")
	}

	return leagueInput{
		Name:           name,
		Format:         format,
		StartDate:      startDate,
		EndDate:        endDate,
		DivisionConfig: divisionConfig,
		MinTeamSize:    req.MinTeamSize,
		MaxTeamSize:    req.MaxTeamSize,
		RosterLockDate: rosterLockDate,
		Status:         status,
	}, nil
}

func parseTeamRequest(req teamRequest, defaultStatus string) (teamInput, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return teamInput{}, fmt.Errorf("name is required")
	}

	if req.CaptainUserID <= 0 {
		return teamInput{}, fmt.Errorf("captain_user_id must be a positive integer")
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		if defaultStatus == "" {
			return teamInput{}, fmt.Errorf("status is required")
		}
		status = defaultStatus
	}
	if !teamStatusAllowed(status) {
		return teamInput{}, fmt.Errorf("status must be active or inactive")
	}

	return teamInput{
		Name:          name,
		CaptainUserID: req.CaptainUserID,
		Status:        status,
	}, nil
}

func parseLeagueDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("date is required")
	}
	if parsed, err := time.Parse(leagueDateLayout, raw); err == nil {
		return parsed, nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("invalid date")
}

func parseOptionalLeagueDate(raw string) (sql.NullTime, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sql.NullTime{}, nil
	}
	parsed, err := parseLeagueDate(raw)
	if err != nil {
		return sql.NullTime{}, err
	}
	return sql.NullTime{Time: parsed, Valid: true}, nil
}

func leagueFormatAllowed(format string) bool {
	switch format {
	case "singles", "doubles", "mixed_doubles":
		return true
	default:
		return false
	}
}

func leagueStatusAllowed(status string) bool {
	switch status {
	case "draft", "registration", "active", "completed", "cancelled":
		return true
	default:
		return false
	}
}

func teamStatusAllowed(status string) bool {
	switch status {
	case "active", "inactive":
		return true
	default:
		return false
	}
}

func facilityIDFromQuery(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(facilityIDQueryKey))
	if raw == "" {
		return 0, fmt.Errorf("%s is required", facilityIDQueryKey)
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", facilityIDQueryKey)
	}
	return id, nil
}

func facilityIDFromRequest(r *http.Request, fromBody *int64) (int64, error) {
	if fromBody != nil {
		if *fromBody <= 0 {
			return 0, fmt.Errorf("facility_id must be a positive integer")
		}
		return *fromBody, nil
	}
	return facilityIDFromQuery(r)
}

func leagueIDFromRequest(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(leagueIDPathKey))
	if raw == "" {
		return 0, fmt.Errorf("invalid league ID")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid league ID")
	}
	return id, nil
}

func teamIDFromRequest(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(teamIDPathKey))
	if raw == "" {
		return 0, fmt.Errorf("invalid team ID")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid team ID")
	}
	return id, nil
}

func userIDFromRequest(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(userIDPathKey))
	if raw == "" {
		return 0, fmt.Errorf("invalid user ID")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid user ID")
	}
	return id, nil
}

func rosterLocked(league dbgen.League) bool {
	if !league.RosterLockDate.Valid {
		return false
	}
	return !time.Now().Before(league.RosterLockDate.Time)
}

func loadQueries() *dbgen.Queries {
	return queries
}

func leaguesPageComponent(leagues []dbgen.League, facilityID int64) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<div class="space-y-6">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="flex items-center justify-between"><h1 class="text-2xl font-semibold text-gray-900">Leagues</h1>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, fmt.Sprintf(`<div class="text-xs text-gray-500">Facility %d</div></div>`, facilityID)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div id="leagues-list">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, buildLeaguesListHTML(leagues)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `</div></div>`); err != nil {
			return err
		}
		return nil
	})
}

func leaguesListComponent(leagues []dbgen.League) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, buildLeaguesListHTML(leagues))
		return err
	})
}

func leagueDetailComponent(league dbgen.League) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, buildLeagueCardHTML(league))
		return err
	})
}

func leagueDeleteComponent() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<div class="h-full flex items-center justify-center text-gray-500"><p>League deleted.</p></div>`)
		return err
	})
}

func buildLeaguesListHTML(leagues []dbgen.League) string {
	if len(leagues) == 0 {
		return `<div class="rounded border border-dashed p-6 text-center text-sm text-gray-500">No leagues found.</div>`
	}

	var builder strings.Builder
	builder.WriteString(`<div class="grid gap-4">`)
	for _, league := range leagues {
		builder.WriteString(buildLeagueCardHTML(league))
	}
	builder.WriteString(`</div>`)
	return builder.String()
}

func buildLeagueCardHTML(league dbgen.League) string {
	name := html.EscapeString(league.Name)
	format := html.EscapeString(league.Format)
	status := html.EscapeString(league.Status)
	rosterLock := "Not set"
	if league.RosterLockDate.Valid {
		rosterLock = formatLeagueDate(league.RosterLockDate.Time)
	}

	return fmt.Sprintf(
		`<div class="rounded border bg-white p-4 shadow-sm" data-league-id="%d">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<div class="text-lg font-semibold text-gray-900">%s</div>
				<div class="text-xs text-gray-500">ID %d</div>
			</div>
			<dl class="mt-3 grid grid-cols-1 gap-2 text-sm text-gray-700 sm:grid-cols-2">
				<div class="flex items-center justify-between gap-4">
					<dt class="font-medium text-gray-600">Status</dt>
					<dd>%s</dd>
				</div>
				<div class="flex items-center justify-between gap-4">
					<dt class="font-medium text-gray-600">Format</dt>
					<dd>%s</dd>
				</div>
				<div class="flex items-center justify-between gap-4">
					<dt class="font-medium text-gray-600">Dates</dt>
					<dd>%s - %s</dd>
				</div>
				<div class="flex items-center justify-between gap-4">
					<dt class="font-medium text-gray-600">Team size</dt>
					<dd>%d-%d</dd>
				</div>
				<div class="flex items-center justify-between gap-4">
					<dt class="font-medium text-gray-600">Roster lock</dt>
					<dd>%s</dd>
				</div>
			</dl>
		</div>`,
		league.ID,
		name,
		league.ID,
		status,
		format,
		formatLeagueDate(league.StartDate),
		formatLeagueDate(league.EndDate),
		league.MinTeamSize,
		league.MaxTeamSize,
		rosterLock,
	)
}

func formatLeagueDate(date time.Time) string {
	return date.Format("Jan 2, 2006")
}
