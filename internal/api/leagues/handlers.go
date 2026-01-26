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
	leagueDateLayout    = "2006-01-02"
	defaultLeagueStatus = "draft"
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
