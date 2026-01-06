// internal/api/checkin/handlers.go
package checkin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	"github.com/codr1/Pickleicious/internal/request"
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

	var activeTheme *models.Theme
	if facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id")); ok {
		if !apiutil.RequireFacilityAccess(w, r, facilityID) {
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), checkinQueryTimeout)
		defer cancel()

		var err error
		activeTheme, err = models.GetActiveTheme(ctx, q, facilityID)
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
			activeTheme = nil
		}
	}

	sessionType := authz.SessionTypeFromContext(r.Context())
	page := layouts.Base(nil, activeTheme, sessionType)
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

	var req checkinRequest
	if err := apiutil.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
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
			respondCheckinBlocked(w, checkinBlockResponse{
				Status:          "blocked",
				Reason:          "waiver_unsigned",
				Badge:           "red",
				WaiverSigned:    member.WaiverSigned,
				MembershipLevel: member.MembershipLevel,
			})
			return
		}
		if member.MembershipLevel == membershipBlockLevel {
			respondCheckinBlocked(w, checkinBlockResponse{
				Status:          "blocked",
				Reason:          "membership_unverified",
				Badge:           "yellow",
				WaiverSigned:    member.WaiverSigned,
				MembershipLevel: member.MembershipLevel,
			})
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
