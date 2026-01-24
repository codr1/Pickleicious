// internal/api/waitlist/handlers.go
package waitlist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	appdb "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/request"
)

var (
	queries     *dbgen.Queries
	store       *appdb.DB
	queriesOnce sync.Once
)

const (
	waitlistQueryTimeout        = 5 * time.Second
	waitlistTimeLayout          = "15:04:05"
	waitlistDateTimeLocalLayout = "2006-01-02T15:04"
	waitlistDateTimeLayout      = "2006-01-02 15:04"
	waitlistDateTimeSecLayout   = "2006-01-02 15:04:05"
)

const (
	waitlistStatusPending   = "pending"
	waitlistStatusExpired   = "expired"
	waitlistStatusFulfilled = "fulfilled"
)

type waitlistTimeRange struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type waitlistJoinRequest struct {
	FacilityID int64             `json:"facility_id"`
	CourtID    *int64            `json:"court_id,omitempty"`
	StartTime  string            `json:"start_time"`
	EndTime    string            `json:"end_time"`
	TimeRange  waitlistTimeRange `json:"time_range"`
}

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(database *appdb.DB) {
	if database == nil {
		log.Warn().Msg("waitlist.InitHandlers called with nil database")
		return
	}
	queriesOnce.Do(func() {
		queries = database.Queries
		store = database
	})
}

func loadQueries() *dbgen.Queries {
	return queries
}

func loadDB() *appdb.DB {
	return store
}

// POST /api/v1/waitlist
func HandleWaitlistJoin(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	database := loadDB()
	if q == nil || database == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	req, err := decodeWaitlistJoinRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityID, err := resolveFacilityID(r, req.FacilityID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	startValue, endValue := waitlistRequestTimes(req)
	startTime, endTime, err := parseWaitlistTimes(startValue, endValue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !endTime.After(startTime) {
		http.Error(w, "end_time must be after start_time", http.StatusBadRequest)
		return
	}
	if !sameWaitlistDate(startTime, endTime) {
		http.Error(w, "start_time and end_time must be on the same date", http.StatusBadRequest)
		return
	}

	targetDate := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
	now := time.Now().In(startTime.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if targetDate.Before(today) {
		http.Error(w, "start_time must be today or later", http.StatusBadRequest)
		return
	}
	targetStartTime := startTime.Format(waitlistTimeLayout)
	targetEndTime := endTime.Format(waitlistTimeLayout)

	targetCourtID, err := parseWaitlistCourtID(req.CourtID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), waitlistQueryTimeout)
	defer cancel()

	exists, err := q.FacilityExists(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to validate facility")
		http.Error(w, "Failed to validate facility", http.StatusInternalServerError)
		return
	}
	if exists == 0 {
		http.Error(w, "Facility not found", http.StatusNotFound)
		return
	}

	if targetCourtID.Valid {
		court, err := q.GetCourt(ctx, targetCourtID.Int64)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Court not found", http.StatusNotFound)
				return
			}
			logger.Error().Err(err).Int64("court_id", targetCourtID.Int64).Msg("Failed to load court")
			http.Error(w, "Failed to load court", http.StatusInternalServerError)
			return
		}
		if court.FacilityID != facilityID {
			http.Error(w, "Court not found", http.StatusNotFound)
			return
		}
	}

	targetCourtParam := sql.NullInt64{}
	if targetCourtID.Valid {
		targetCourtParam = targetCourtID
	}

	var created dbgen.Waitlist
	const maxInsertAttempts = 3
	for attempt := 1; attempt <= maxInsertAttempts; attempt++ {
		err = database.RunInTx(ctx, func(txDB *appdb.DB) error {
			existing, err := txDB.Queries.ListWaitlistsForSlot(ctx, dbgen.ListWaitlistsForSlotParams{
				FacilityID:      facilityID,
				TargetDate:      targetDate,
				TargetStartTime: targetStartTime,
				TargetEndTime:   targetEndTime,
				TargetCourtID:   targetCourtParam,
			})
			if err != nil {
				return fmt.Errorf("list waitlists: %w", err)
			}
			for _, entry := range existing {
				if entry.UserID == user.ID && entry.Status != waitlistStatusExpired && entry.Status != waitlistStatusFulfilled {
					return apiutil.HandlerError{Status: http.StatusConflict, Message: "Already on waitlist for this slot"}
				}
			}

			maxWaitlistSize, err := loadMaxWaitlistSize(ctx, txDB.Queries, facilityID)
			if err != nil {
				return fmt.Errorf("load waitlist config: %w", err)
			}
			if maxWaitlistSize > 0 && int64(len(existing)) >= maxWaitlistSize {
				return apiutil.HandlerError{Status: http.StatusConflict, Message: "Waitlist is full for this slot"}
			}

			created, err = txDB.Queries.CreateWaitlistEntry(ctx, dbgen.CreateWaitlistEntryParams{
				FacilityID:      facilityID,
				UserID:          user.ID,
				TargetCourtID:   targetCourtID,
				TargetDate:      targetDate,
				TargetStartTime: targetStartTime,
				TargetEndTime:   targetEndTime,
				Status:          waitlistStatusPending,
			})
			if err != nil {
				return fmt.Errorf("create waitlist entry: %w", err)
			}
			return nil
		})
		if err == nil {
			break
		}
		var handlerErr apiutil.HandlerError
		if errors.As(err, &handlerErr) {
			http.Error(w, handlerErr.Message, handlerErr.Status)
			return
		}
		if apiutil.IsSQLiteUniqueViolation(err) {
			if attempt < maxInsertAttempts {
				continue
			}
			http.Error(w, "Waitlist updated, please try again", http.StatusConflict)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to create waitlist entry")
		http.Error(w, "Failed to create waitlist entry", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, created); err != nil {
		logger.Error().Err(err).Int64("waitlist_id", created.ID).Msg("Failed to write waitlist response")
		return
	}
}

// GET /api/v1/waitlist
func HandleWaitlistList(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), waitlistQueryTimeout)
	defer cancel()

	waitlists, err := q.ListWaitlistsByUserAndFacility(ctx, dbgen.ListWaitlistsByUserAndFacilityParams{
		UserID:     user.ID,
		FacilityID: facilityID,
	})
	if err != nil {
		logger.Error().Err(err).Int64("user_id", user.ID).Msg("Failed to list waitlists")
		http.Error(w, "Failed to load waitlists", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, waitlists); err != nil {
		logger.Error().Err(err).Int64("user_id", user.ID).Msg("Failed to write waitlist response")
		return
	}
}

// DELETE /api/v1/waitlist/{id}
func HandleWaitlistLeave(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	waitlistID, err := waitlistIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), waitlistQueryTimeout)
	defer cancel()

	entry, err := q.GetWaitlistEntry(ctx, dbgen.GetWaitlistEntryParams{
		ID:         waitlistID,
		FacilityID: facilityID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Waitlist entry not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("waitlist_id", waitlistID).Msg("Failed to load waitlist entry")
		http.Error(w, "Failed to remove waitlist entry", http.StatusInternalServerError)
		return
	}

	if entry.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	_, err = q.DeleteWaitlistEntry(ctx, dbgen.DeleteWaitlistEntryParams{
		ID:         waitlistID,
		FacilityID: facilityID,
	})
	if err != nil {
		logger.Error().Err(err).Int64("waitlist_id", waitlistID).Msg("Failed to delete waitlist entry")
		http.Error(w, "Failed to remove waitlist entry", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/staff/waitlist
func HandleStaffWaitlistView(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if !authz.IsStaff(user) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	facilityID, err := staffFacilityIDFromRequest(r, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), waitlistQueryTimeout)
	defer cancel()

	waitlists, err := q.ListWaitlistsByFacility(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to list waitlists")
		http.Error(w, "Failed to load waitlists", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, waitlists); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to write waitlist response")
		return
	}
}

// POST /api/v1/waitlist/config
func HandleWaitlistConfigUpdate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if !authz.IsStaff(user) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	facilityID, err := apiutil.ParseRequiredInt64Field(r.FormValue("facility_id"), "facility_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	maxWaitlistSize, err := parseNonNegativeInt64Field(r.FormValue("max_waitlist_size"), "max_waitlist_size")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	notificationMode := strings.ToLower(strings.TrimSpace(r.FormValue("notification_mode")))
	if notificationMode == "" {
		notificationMode = "broadcast"
	}
	switch notificationMode {
	case "broadcast", "sequential":
	default:
		http.Error(w, "notification_mode must be broadcast or sequential", http.StatusBadRequest)
		return
	}

	offerExpiryMinutes, err := parseNonNegativeInt64Field(r.FormValue("offer_expiry_minutes"), "offer_expiry_minutes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	notificationWindowMinutes, err := parseNonNegativeInt64Field(r.FormValue("notification_window_minutes"), "notification_window_minutes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), waitlistQueryTimeout)
	defer cancel()

	_, err = q.UpsertWaitlistConfig(ctx, dbgen.UpsertWaitlistConfigParams{
		FacilityID:                facilityID,
		MaxWaitlistSize:           maxWaitlistSize,
		NotificationMode:          notificationMode,
		OfferExpiryMinutes:        offerExpiryMinutes,
		NotificationWindowMinutes: notificationWindowMinutes,
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to update waitlist config")
		http.Error(w, "Failed to update waitlist configuration", http.StatusInternalServerError)
		return
	}

	apiutil.WriteHTMLFeedback(w, http.StatusOK, "Waitlist configuration updated.")
}

func decodeWaitlistJoinRequest(r *http.Request) (waitlistJoinRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req waitlistJoinRequest
		if err := apiutil.DecodeJSON(r, &req); err != nil {
			return waitlistJoinRequest{}, err
		}
		return req, nil
	}

	if err := r.ParseForm(); err != nil {
		return waitlistJoinRequest{}, fmt.Errorf("invalid form data")
	}

	req := waitlistJoinRequest{}
	facilityID, err := apiutil.ParseRequiredInt64Field(r.FormValue("facility_id"), "facility_id")
	if err != nil {
		return waitlistJoinRequest{}, err
	}
	req.FacilityID = facilityID

	courtID, err := apiutil.ParseOptionalInt64Field(r.FormValue("court_id"), "court_id")
	if err != nil {
		return waitlistJoinRequest{}, err
	}
	req.CourtID = courtID

	req.StartTime = strings.TrimSpace(r.FormValue("start_time"))
	req.EndTime = strings.TrimSpace(r.FormValue("end_time"))
	return req, nil
}

func waitlistRequestTimes(req waitlistJoinRequest) (string, string) {
	startTime := apiutil.FirstNonEmpty(req.TimeRange.StartTime, req.StartTime)
	endTime := apiutil.FirstNonEmpty(req.TimeRange.EndTime, req.EndTime)
	return startTime, endTime
}

func parseWaitlistTimes(startValue, endValue string) (time.Time, time.Time, error) {
	startTime, err := parseWaitlistTime(startValue, "start_time")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	endTime, err := parseWaitlistTime(endValue, "end_time")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return startTime, endTime, nil
}

func parseWaitlistTime(value, field string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, apiutil.FieldError{Field: field, Reason: "is required"}
	}

	layouts := []string{time.RFC3339, waitlistDateTimeLocalLayout, waitlistDateTimeLayout, waitlistDateTimeSecLayout}
	for _, layout := range layouts {
		if layout == time.RFC3339 {
			parsed, err := time.Parse(layout, value)
			if err == nil {
				return parsed, nil
			}
			continue
		}
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, apiutil.FieldError{Field: field, Reason: "must be a valid datetime"}
}

func parseWaitlistCourtID(courtID *int64) (sql.NullInt64, error) {
	if courtID == nil {
		return sql.NullInt64{Valid: false}, nil
	}
	if *courtID <= 0 {
		return sql.NullInt64{}, fmt.Errorf("court_id must be a positive integer")
	}
	return sql.NullInt64{Int64: *courtID, Valid: true}, nil
}

func parseNonNegativeInt64Field(raw string, field string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", field)
	}
	return value, nil
}

func sameWaitlistDate(startTime, endTime time.Time) bool {
	return startTime.Year() == endTime.Year() &&
		startTime.Month() == endTime.Month() &&
		startTime.Day() == endTime.Day()
}

func resolveFacilityID(r *http.Request, payloadFacilityID int64) (int64, error) {
	if facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id")); ok {
		if payloadFacilityID != 0 && payloadFacilityID != facilityID {
			return 0, fmt.Errorf("facility_id mismatch between query and payload")
		}
		return facilityID, nil
	}

	if payloadFacilityID <= 0 {
		return 0, fmt.Errorf("facility_id is required")
	}
	return payloadFacilityID, nil
}

func facilityIDFromRequest(r *http.Request) (int64, error) {
	facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id"))
	if !ok {
		return 0, fmt.Errorf("facility_id is required")
	}
	return facilityID, nil
}

func staffFacilityIDFromRequest(r *http.Request, user *authz.AuthUser) (int64, error) {
	if user != nil && user.HomeFacilityID != nil {
		return *user.HomeFacilityID, nil
	}
	return facilityIDFromRequest(r)
}

func waitlistIDFromRequest(r *http.Request) (int64, error) {
	pathID := strings.TrimSpace(r.PathValue("id"))
	if pathID == "" {
		return 0, fmt.Errorf("invalid waitlist ID")
	}
	id, err := strconv.ParseInt(pathID, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid waitlist ID")
	}
	return id, nil
}

func loadMaxWaitlistSize(ctx context.Context, q *dbgen.Queries, facilityID int64) (int64, error) {
	config, err := q.GetWaitlistConfig(ctx, facilityID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return config.MaxWaitlistSize, nil
}
