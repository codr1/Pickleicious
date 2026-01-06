// internal/api/checkin/handlers.go
package checkin

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	"github.com/codr1/Pickleicious/internal/request"
	checkintempl "github.com/codr1/Pickleicious/internal/templates/components/checkin"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

const (
	checkinQueryTimeout  = 5 * time.Second
	defaultCheckinLimit  = 10
	maxCheckinLimit      = 50
	membershipBlockLevel = int64(0)
)

var (
	queries     *dbgen.Queries
	queriesOnce sync.Once
)

type checkinSearchCard struct {
	ID              int64  `json:"id"`
	FirstName       string `json:"firstName"`
	LastName        string `json:"lastName"`
	Email           string `json:"email"`
	PhotoURL        string `json:"photoUrl"`
	PhotoID         *int64 `json:"photoId,omitempty"`
	WaiverSigned    bool   `json:"waiverSigned"`
	MembershipLevel int64  `json:"membershipLevel"`
}

type checkinSearchResponse struct {
	Members []checkinSearchCard `json:"members"`
}

type checkinRequest struct {
	UserID               int64  `json:"userId"`
	FacilityID           int64  `json:"facilityId"`
	ActivityType         string `json:"activityType"`
	RelatedReservationID *int64 `json:"relatedReservationId"`
	Override             bool   `json:"override"`
}

type checkinBlockResponse struct {
	Status          string `json:"status"`
	Reason          string `json:"reason"`
	Badge           string `json:"badge"`
	WaiverSigned    bool   `json:"waiverSigned"`
	MembershipLevel int64  `json:"membershipLevel"`
}

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(q *dbgen.Queries) {
	if q == nil {
		return
	}
	queriesOnce.Do(func() {
		queries = q
	})
}

func loadQueries() *dbgen.Queries {
	return queries
}

// /checkin
func HandleCheckinPage(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id"))
	if !ok {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}
	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	var activeTheme *models.Theme
	ctx, cancel := context.WithTimeout(r.Context(), checkinQueryTimeout)
	defer cancel()

	activeTheme, err := models.GetActiveTheme(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
		activeTheme = nil
	}

	arrivals, err := listTodayVisitsWithMembers(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load arrivals for check-in")
		arrivals = nil
	}

	component := checkintempl.CheckinLayout(facilityID, []checkintempl.CheckinMember{}, arrivals)

	sessionType := authz.SessionTypeFromContext(r.Context())
	page := layouts.Base(component, activeTheme, sessionType)
	if err := page.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render check-in page")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

// /api/v1/checkin/search
func HandleCheckinSearch(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id"))
	if !ok {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}
	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	searchTerm := strings.TrimSpace(r.URL.Query().Get("q"))
	if searchTerm == "" {
		if isHTMXRequest(r) {
			component := checkintempl.CheckinMembersList([]checkintempl.CheckinMember{}, facilityID)
			apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render empty check-in search results", "Failed to render search results")
			return
		}
		if err := apiutil.WriteJSON(w, http.StatusOK, checkinSearchResponse{Members: []checkinSearchCard{}}); err != nil {
			logger.Error().Err(err).Msg("Failed to write empty check-in search response")
		}
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))
	offset := parseOffset(r.URL.Query().Get("offset"))

	ctx, cancel := context.WithTimeout(r.Context(), checkinQueryTimeout)
	defer cancel()

	rows, err := q.ListMembers(ctx, dbgen.ListMembersParams{
		Limit:      limit,
		Offset:     offset,
		SearchTerm: sql.NullString{String: searchTerm, Valid: true},
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to search members for check-in")
		http.Error(w, "Failed to search members", http.StatusInternalServerError)
		return
	}

	cards := make([]checkinSearchCard, 0, len(rows))
	for _, row := range rows {
		var photoID *int64
		if row.PhotoID.Valid {
			value := row.PhotoID.Int64
			photoID = &value
		}

		cards = append(cards, checkinSearchCard{
			ID:              row.ID,
			FirstName:       row.FirstName,
			LastName:        row.LastName,
			Email:           nullString(row.Email),
			PhotoURL:        nullString(row.PhotoUrl),
			PhotoID:         photoID,
			WaiverSigned:    row.WaiverSigned,
			MembershipLevel: row.MembershipLevel,
		})
	}

	if isHTMXRequest(r) {
		component := checkintempl.CheckinMembersList(checkintempl.NewCheckinMembers(rows), facilityID)
		apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render check-in search results", "Failed to render search results")
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, checkinSearchResponse{Members: cards}); err != nil {
		logger.Error().Err(err).Msg("Failed to write check-in search response")
		return
	}
}

// /api/v1/checkin
func HandleCheckin(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	req, err := decodeCheckinRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.UserID <= 0 {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}
	if req.FacilityID <= 0 {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}
	if req.RelatedReservationID != nil && *req.RelatedReservationID <= 0 {
		http.Error(w, "Related reservation ID must be a positive integer", http.StatusBadRequest)
		return
	}

	activityType, err := normalizeActivityType(req.ActivityType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, req.FacilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), checkinQueryTimeout)
	defer cancel()

	member, err := q.GetMemberByID(ctx, req.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Member not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("user_id", req.UserID).Msg("Failed to fetch member for check-in")
		http.Error(w, "Failed to fetch member", http.StatusInternalServerError)
		return
	}

	if !req.Override {
		if !member.WaiverSigned {
			blocked := checkinBlockResponse{
				Status:          "blocked",
				Reason:          "waiver_unsigned",
				Badge:           "red",
				WaiverSigned:    member.WaiverSigned,
				MembershipLevel: member.MembershipLevel,
			}
			if isHTMXRequest(r) {
				renderCheckinBlocked(r.Context(), w, blocked, member)
				return
			}
			respondCheckinBlocked(w, blocked)
			return
		}
		if member.MembershipLevel == membershipBlockLevel {
			blocked := checkinBlockResponse{
				Status:          "blocked",
				Reason:          "membership_unverified",
				Badge:           "yellow",
				WaiverSigned:    member.WaiverSigned,
				MembershipLevel: member.MembershipLevel,
			}
			if isHTMXRequest(r) {
				renderCheckinBlocked(r.Context(), w, blocked, member)
				return
			}
			respondCheckinBlocked(w, blocked)
			return
		}
	}

	checkedInByStaffID := sql.NullInt64{Int64: user.ID, Valid: true}
	relatedReservationID := sql.NullInt64{Valid: false}
	if req.RelatedReservationID != nil {
		relatedReservationID = sql.NullInt64{Int64: *req.RelatedReservationID, Valid: true}
	}

	visit, err := q.CreateFacilityVisit(ctx, dbgen.CreateFacilityVisitParams{
		UserID:               req.UserID,
		FacilityID:           req.FacilityID,
		CheckOutTime:         sql.NullTime{},
		CheckedInByStaffID:   checkedInByStaffID,
		ActivityType:         activityType,
		RelatedReservationID: relatedReservationID,
	})
	if err != nil {
		logger.Error().Err(err).Int64("user_id", req.UserID).Int64("facility_id", req.FacilityID).Msg("Failed to create facility visit")
		if apiutil.IsSQLiteForeignKeyViolation(err) {
			http.Error(w, "Invalid related record", http.StatusBadRequest)
			return
		}
		http.Error(w, "Failed to check in member", http.StatusInternalServerError)
		return
	}

	if isHTMXRequest(r) {
		visits, err := listTodayVisitsByUser(ctx, q, req.UserID)
		if err != nil {
			logger.Error().Err(err).Int64("user_id", req.UserID).Msg("Failed to load visits for check-in success")
			visits = nil
		}
		arrivals, err := listTodayVisitsWithMembers(ctx, q, req.FacilityID)
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", req.FacilityID).Msg("Failed to refresh arrivals list after check-in")
			arrivals = nil
		}
		component := checkintempl.CheckinSuccessResponse(
			checkintempl.NewCheckinMemberFromMember(member),
			checkintempl.NewFacilityVisits(visits),
			arrivals,
		)
		renderHTMLWithStatus(r.Context(), w, component, http.StatusCreated, "Failed to render check-in success response", "Failed to render check-in success")
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, visit); err != nil {
		logger.Error().Err(err).Int64("visit_id", visit.ID).Msg("Failed to write check-in response")
		return
	}
}

func parseLimit(raw string) int64 {
	limit, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || limit <= 0 {
		return defaultCheckinLimit
	}
	if limit > maxCheckinLimit {
		return maxCheckinLimit
	}
	return limit
}

func parseOffset(raw string) int64 {
	offset, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func normalizeActivityType(activityType string) (sql.NullString, error) {
	activityType = strings.TrimSpace(activityType)
	if activityType == "" {
		return sql.NullString{Valid: false}, nil
	}

	switch activityType {
	case "court_reservation", "open_play", "league":
		return sql.NullString{String: activityType, Valid: true}, nil
	default:
		return sql.NullString{}, errors.New("activity_type must be one of: court_reservation, open_play, league")
	}
}

func respondCheckinBlocked(w http.ResponseWriter, response checkinBlockResponse) {
	if err := apiutil.WriteJSON(w, http.StatusConflict, response); err != nil {
		log.Error().Err(err).Msg("Failed to write blocked check-in response")
	}
}

func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func decodeCheckinRequest(r *http.Request) (checkinRequest, error) {
	var req checkinRequest
	if apiutil.IsJSONRequest(r) {
		if err := apiutil.DecodeJSON(r, &req); err != nil {
			return req, err
		}
		return req, nil
	}

	if err := r.ParseForm(); err != nil {
		return req, err
	}

	userID, err := parseRequiredInt64Field(apiutil.FirstNonEmpty(r.FormValue("userId"), r.FormValue("user_id")), "userId")
	if err != nil {
		return req, err
	}
	facilityID, err := parseRequiredInt64Field(apiutil.FirstNonEmpty(r.FormValue("facilityId"), r.FormValue("facility_id")), "facilityId")
	if err != nil {
		return req, err
	}

	relatedReservationID, err := apiutil.ParseOptionalInt64Field(
		apiutil.FirstNonEmpty(r.FormValue("relatedReservationId"), r.FormValue("related_reservation_id")),
		"relatedReservationId",
	)
	if err != nil {
		return req, err
	}

	req.UserID = userID
	req.FacilityID = facilityID
	req.ActivityType = apiutil.FirstNonEmpty(r.FormValue("activityType"), r.FormValue("activity_type"))
	req.RelatedReservationID = relatedReservationID
	req.Override = parseBool(r.FormValue("override"))
	return req, nil
}

func parseRequiredInt64Field(raw string, field string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", field)
	}
	return value, nil
}

func parseBool(raw string) bool {
	raw = strings.TrimSpace(raw)
	return raw == "1" || strings.EqualFold(raw, "true") || strings.EqualFold(raw, "yes")
}

func listTodayVisitsWithMembers(ctx context.Context, q *dbgen.Queries, facilityID int64) ([]checkintempl.FacilityVisit, error) {
	start, end := todayRange(time.Now())
	rows, err := q.ListTodayVisitsByFacility(ctx, dbgen.ListTodayVisitsByFacilityParams{
		FacilityID: facilityID,
		TodayStart: start,
		TodayEnd:   end,
	})
	if err != nil {
		return nil, err
	}

	membersByID := make(map[int64]dbgen.GetMemberByIDRow)
	visits := make([]checkintempl.FacilityVisit, 0, len(rows))
	for _, visit := range rows {
		member, ok := membersByID[visit.UserID]
		if !ok {
			var err error
			member, err = q.GetMemberByID(ctx, visit.UserID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					visits = append(visits, checkintempl.NewFacilityVisit(visit))
					continue
				}
				return nil, err
			}
			membersByID[visit.UserID] = member
		}
		visits = append(visits, checkintempl.NewFacilityVisitWithMember(visit, member))
	}

	return visits, nil
}

func listTodayVisitsByUser(ctx context.Context, q *dbgen.Queries, userID int64) ([]dbgen.FacilityVisit, error) {
	rows, err := q.ListRecentVisitsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	start, end := todayRange(time.Now())
	visits := make([]dbgen.FacilityVisit, 0, len(rows))
	for _, visit := range rows {
		if !visit.CheckInTime.Before(start) && visit.CheckInTime.Before(end) {
			visits = append(visits, visit)
		}
	}
	return visits, nil
}

func todayRange(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start, start.Add(24 * time.Hour)
}

func renderCheckinBlocked(ctx context.Context, w http.ResponseWriter, response checkinBlockResponse, member dbgen.GetMemberByIDRow) {
	component := checkintempl.CheckinBlocked(
		checkintempl.NewCheckinMemberFromMember(member),
		checkinBlockMessage(response.Reason),
		response.Badge,
	)
	renderHTMLWithStatus(ctx, w, component, http.StatusConflict, "Failed to render blocked check-in response", "Failed to render blocked check-in response")
}

func checkinBlockMessage(reason string) string {
	switch reason {
	case "waiver_unsigned":
		return "Waiver missing. Ask the member to sign the waiver before check-in."
	case "membership_unverified":
		return "Membership is unverified. Confirm membership status before check-in."
	default:
		return "Unable to check in this member."
	}
}

func renderHTMLWithStatus(ctx context.Context, w http.ResponseWriter, component templ.Component, status int, logMsg string, errMsg string) {
	logger := log.Ctx(ctx)
	var buf bytes.Buffer
	if err := component.Render(ctx, &buf); err != nil {
		logger.Error().Err(err).Msg(logMsg)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		logger.Error().Err(err).Msg("Failed to write response")
	}
}
