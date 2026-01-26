package leagues

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	appdb "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	leaguescheduler "github.com/codr1/Pickleicious/internal/leagues"
)

const leagueReservationTypeName = "LEAGUE"
const defaultMatchDuration = time.Hour

// POST /api/v1/leagues/{id}/schedule/generate
func HandleGenerateSchedule(w http.ResponseWriter, r *http.Request) {
	req, err := decodeScheduleRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	handleScheduleGeneration(w, r, false, req)
}

// POST /api/v1/leagues/{id}/schedule/regenerate
func HandleRegenerateSchedule(w http.ResponseWriter, r *http.Request) {
	req, err := decodeScheduleRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	handleScheduleGeneration(w, r, true, req)
}

type scheduleRequest struct {
	PreserveCourts       bool `json:"preserveCourts"`
	MatchDurationMinutes int  `json:"matchDurationMinutes"`
}

func decodeScheduleRequest(r *http.Request) (scheduleRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req scheduleRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return scheduleRequest{}, err
	}
	preserve, err := parseOptionalBool(apiutil.FirstNonEmpty(r.FormValue("preserve_courts"), r.FormValue("preserveCourts")))
	if err != nil {
		return scheduleRequest{}, err
	}
	matchDurationMinutes, err := parseOptionalInt(apiutil.FirstNonEmpty(r.FormValue("match_duration_minutes"), r.FormValue("matchDurationMinutes")))
	if err != nil {
		return scheduleRequest{}, err
	}
	return scheduleRequest{PreserveCourts: preserve, MatchDurationMinutes: matchDurationMinutes}, nil
}

func parseOptionalBool(raw string) (bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, nil
	}
	switch strings.ToLower(raw) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value")
	}
}

func parseOptionalInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid integer value")
	}
	return value, nil
}

func handleScheduleGeneration(w http.ResponseWriter, r *http.Request, regenerate bool, req scheduleRequest) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	database := loadDB()
	if q == nil || database == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	leagueID, err := leagueIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid league ID", http.StatusBadRequest)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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

	if !regenerate {
		existing, err := q.ListLeagueMatches(ctx, leagueID)
		if err != nil {
			logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to check existing schedule")
			http.Error(w, "Failed to check existing schedule", http.StatusInternalServerError)
			return
		}
		if len(existing) > 0 {
			http.Error(w, "Schedule already exists for this league", http.StatusConflict)
			return
		}
	}

	teams, err := q.ListLeagueTeams(ctx, leagueID)
	if err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to load league teams")
		http.Error(w, "Failed to load league teams", http.StatusInternalServerError)
		return
	}
	teams = filterActiveTeams(teams)
	if len(teams) < 2 {
		http.Error(w, "At least two active teams are required", http.StatusBadRequest)
		return
	}

	courts, err := q.ListCourts(ctx, league.FacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", league.FacilityID).Msg("Failed to load courts")
		http.Error(w, "Failed to load courts", http.StatusInternalServerError)
		return
	}
	courts = filterActiveCourts(courts)
	if len(courts) == 0 {
		http.Error(w, "No active courts available for scheduling", http.StatusBadRequest)
		return
	}

	hours, err := q.GetFacilityHours(ctx, league.FacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", league.FacilityID).Msg("Failed to load operating hours")
		http.Error(w, "Failed to load operating hours", http.StatusInternalServerError)
		return
	}

	reservationType, err := q.GetReservationTypeByName(ctx, leagueReservationTypeName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Reservation type not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Msg("Failed to load reservation type")
		http.Error(w, "Failed to load reservation type", http.StatusInternalServerError)
		return
	}

	var preferredCourts map[string]int64
	var existingMatches []dbgen.ListLeagueMatchesWithReservationsRow
	if regenerate {
		existingMatches, err = q.ListLeagueMatchesWithReservations(ctx, leagueID)
		if err != nil {
			logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to load existing schedule")
			http.Error(w, "Failed to load existing schedule", http.StatusInternalServerError)
			return
		}
		if req.PreserveCourts {
			preferredCourts = buildPreferredCourtMap(existingMatches)
		}
	}

	matchDuration := defaultMatchDuration
	if req.MatchDurationMinutes < 0 {
		http.Error(w, "Match duration must be positive", http.StatusBadRequest)
		return
	}
	if req.MatchDurationMinutes > 0 {
		matchDuration = time.Duration(req.MatchDurationMinutes) * time.Minute
	}

	schedule, err := leaguescheduler.GenerateRoundRobinSchedule(leagueID, teams, league.StartDate, league.EndDate, courts, hours, matchDuration)
	if err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to generate schedule")
		http.Error(w, "Unable to generate schedule with the current league settings", http.StatusBadRequest)
		return
	}
	if req.PreserveCourts && len(preferredCourts) > 0 {
		schedule = applyPreferredCourts(schedule, preferredCourts, courts)
	}

	createdMatches := make([]dbgen.LeagueMatch, 0, len(schedule))
	peoplePerTeam := peoplePerTeamFromFormat(league.Format)

	err = database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		if regenerate {
			if err := deleteExistingSchedule(ctx, qtx, league.FacilityID, leagueID, existingMatches); err != nil {
				return err
			}
		}

		for _, match := range schedule {
			if err := apiutil.EnsureCourtsAvailable(ctx, qtx, league.FacilityID, 0, match.StartTime, match.EndTime, []int64{match.Court.ID}); err != nil {
				var availErr apiutil.AvailabilityError
				if errors.As(err, &availErr) {
					return apiutil.HandlerError{Status: http.StatusConflict, Message: "Court unavailable for scheduled match", Err: err}
				}
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check court availability", Err: err}
			}
			teamsPerCourt := int64(2)
			reservation, err := qtx.CreateReservation(ctx, dbgen.CreateReservationParams{
				FacilityID:        league.FacilityID,
				ReservationTypeID: reservationType.ID,
				RecurrenceRuleID:  sql.NullInt64{},
				PrimaryUserID:     sql.NullInt64{},
				CreatedByUserID:   user.ID,
				ProID:             sql.NullInt64{},
				OpenPlayRuleID:    sql.NullInt64{},
				StartTime:         match.StartTime,
				EndTime:           match.EndTime,
				IsOpenEvent:       false,
				TeamsPerCourt:     apiutil.ToNullInt64(&teamsPerCourt),
				PeoplePerTeam:     apiutil.ToNullInt64(&peoplePerTeam),
			})
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to create reservation", Err: err}
			}
			if err := qtx.AddReservationCourt(ctx, dbgen.AddReservationCourtParams{
				ReservationID: reservation.ID,
				CourtID:       match.Court.ID,
			}); err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to assign courts", Err: err}
			}
			created, err := qtx.CreateLeagueMatch(ctx, dbgen.CreateLeagueMatchParams{
				LeagueID:      leagueID,
				HomeTeamID:    match.HomeTeam.ID,
				AwayTeamID:    match.AwayTeam.ID,
				ReservationID: sql.NullInt64{Int64: reservation.ID, Valid: true},
				ScheduledTime: match.StartTime,
				HomeScore:     sql.NullInt64{},
				AwayScore:     sql.NullInt64{},
				Status:        "scheduled",
			})
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to create league match", Err: err}
			}
			createdMatches = append(createdMatches, created)
		}
		return nil
	})
	if err != nil {
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			logger.Error().Err(herr.Err).Int64("league_id", leagueID).Msg(herr.Message)
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to generate schedule")
		http.Error(w, "Failed to generate schedule", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, map[string]any{"matches": createdMatches}); err != nil {
		logger.Error().Err(err).Int64("league_id", leagueID).Msg("Failed to write schedule response")
	}
}

func deleteExistingSchedule(ctx context.Context, qtx *dbgen.Queries, facilityID int64, leagueID int64, matches []dbgen.ListLeagueMatchesWithReservationsRow) error {
	reservationIDs := make(map[int64]struct{})
	for _, match := range matches {
		if match.ReservationID.Valid {
			reservationIDs[match.ReservationID.Int64] = struct{}{}
		}
	}
	if _, err := qtx.DeleteLeagueMatchesByLeagueID(ctx, leagueID); err != nil {
		return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to delete league matches", Err: err}
	}
	for reservationID := range reservationIDs {
		if err := qtx.DeleteReservationParticipantsByReservationID(ctx, reservationID); err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to remove reservation participants", Err: err}
		}
		if err := qtx.DeleteReservationCourtsByReservationID(ctx, reservationID); err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to remove reservation courts", Err: err}
		}
		if _, err := qtx.DeleteReservation(ctx, dbgen.DeleteReservationParams{ID: reservationID, FacilityID: facilityID}); err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to delete reservation", Err: err}
		}
	}
	return nil
}

func buildPreferredCourtMap(matches []dbgen.ListLeagueMatchesWithReservationsRow) map[string]int64 {
	preferred := make(map[string]int64)
	for _, match := range matches {
		courtID, ok := parseNullableInt64(match.CourtID)
		if !ok {
			continue
		}
		key := matchKey(match.HomeTeamID, match.AwayTeamID)
		preferred[key] = courtID
	}
	return preferred
}

func parseNullableInt64(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case int64:
		return v, true
	case int32:
		return int64(v), true
	case int:
		return int64(v), true
	case int16:
		return int64(v), true
	case int8:
		return int64(v), true
	case uint64:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint:
		return int64(v), true
	case []byte:
		if len(v) == 0 {
			return 0, false
		}
		parsed, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case string:
		if v == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case sql.NullInt64:
		if !v.Valid {
			return 0, false
		}
		return v.Int64, true
	case *int64:
		if v == nil {
			return 0, false
		}
		return *v, true
	default:
		return 0, false
	}
}

func applyPreferredCourts(schedule []leaguescheduler.ScheduledMatch, preferred map[string]int64, courts []dbgen.Court) []leaguescheduler.ScheduledMatch {
	courtOrder := make([]int64, 0, len(courts))
	courtLookup := make(map[int64]dbgen.Court, len(courts))
	for _, court := range courts {
		courtOrder = append(courtOrder, court.ID)
		courtLookup[court.ID] = court
	}
	result := make([]leaguescheduler.ScheduledMatch, len(schedule))
	copy(result, schedule)

	groups := make(map[time.Time][]int)
	for idx, match := range result {
		groups[match.StartTime] = append(groups[match.StartTime], idx)
	}

	times := make([]time.Time, 0, len(groups))
	for t := range groups {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	for _, startTime := range times {
		indices := groups[startTime]
		available := make(map[int64]struct{}, len(courtOrder))
		for _, courtID := range courtOrder {
			available[courtID] = struct{}{}
		}

		assignedPreferred := make(map[int]struct{})
		for _, idx := range indices {
			match := result[idx]
			key := matchKey(match.HomeTeam.ID, match.AwayTeam.ID)
			if courtID, ok := preferred[key]; ok {
				if _, exists := available[courtID]; exists {
					if court, ok := courtLookup[courtID]; ok {
						match.Court = court
						result[idx] = match
						delete(available, courtID)
						assignedPreferred[idx] = struct{}{}
						continue
					}
				}
			}
		}

		for _, idx := range indices {
			if _, ok := assignedPreferred[idx]; ok {
				continue
			}
			match := result[idx]
			for _, courtID := range courtOrder {
				if _, ok := available[courtID]; ok {
					match.Court = courtLookup[courtID]
					result[idx] = match
					delete(available, courtID)
					break
				}
			}
		}
	}

	return result
}

func matchKey(teamA, teamB int64) string {
	if teamA > teamB {
		teamA, teamB = teamB, teamA
	}
	return fmt.Sprintf("%d:%d", teamA, teamB)
}

func filterActiveTeams(teams []dbgen.LeagueTeam) []dbgen.LeagueTeam {
	active := make([]dbgen.LeagueTeam, 0, len(teams))
	for _, team := range teams {
		if strings.EqualFold(team.Status, "active") {
			active = append(active, team)
		}
	}
	return active
}

func filterActiveCourts(courts []dbgen.Court) []dbgen.Court {
	active := make([]dbgen.Court, 0, len(courts))
	for _, court := range courts {
		if strings.EqualFold(court.Status, "active") {
			active = append(active, court)
		}
	}
	return active
}

func peoplePerTeamFromFormat(format string) int64 {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "singles":
		return 1
	case "doubles", "mixed_doubles":
		return 2
	default:
		return 2
	}
}
