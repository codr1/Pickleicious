// internal/api/clinics/handlers.go
package clinics

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
)

var (
	queries     *dbgen.Queries
	store       *appdb.DB
	queriesOnce sync.Once
)

const (
	clinicQueryTimeout      = 5 * time.Second
	timeLayoutDatetimeLocal = "2006-01-02T15:04"
)

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(database *appdb.DB) {
	if database == nil {
		log.Fatal().Msg("clinics.InitHandlers called with nil database")
	}
	queriesOnce.Do(func() {
		queries = database.Queries
		store = database
	})
}

type clinicTypeRequest struct {
	FacilityID      int64   `json:"facilityId"`
	Name            string  `json:"name"`
	Description     *string `json:"description"`
	MinParticipants int64   `json:"minParticipants"`
	MaxParticipants int64   `json:"maxParticipants"`
	PriceCents      int64   `json:"priceCents"`
	Status          string  `json:"status"`
}

type clinicSessionRequest struct {
	FacilityID       int64  `json:"facilityId"`
	ClinicTypeID     int64  `json:"clinicTypeId"`
	ProID            int64  `json:"proId"`
	StartTime        string `json:"startTime"`
	EndTime          string `json:"endTime"`
	EnrollmentStatus string `json:"enrollmentStatus"`
}

// POST /api/v1/clinic-types
func HandleClinicTypeCreate(w http.ResponseWriter, r *http.Request) {
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

	req, err := decodeClinicTypeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateClinicTypeRequest(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityID, err := resolveFacilityID(r, req.FacilityID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.FacilityID = facilityID

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), clinicQueryTimeout)
	defer cancel()

	exists, err := facilityExists(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to validate facility")
		http.Error(w, "Failed to validate facility", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Facility not found", http.StatusNotFound)
		return
	}

	created, err := q.CreateClinicType(ctx, dbgen.CreateClinicTypeParams{
		FacilityID:      facilityID,
		Name:            req.Name,
		Description:     toNullString(req.Description),
		MinParticipants: req.MinParticipants,
		MaxParticipants: req.MaxParticipants,
		PriceCents:      req.PriceCents,
		Status:          req.Status,
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to create clinic type")
		http.Error(w, "Failed to create clinic type", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, created); err != nil {
		logger.Error().Err(err).Int64("clinic_type_id", created.ID).Msg("Failed to write clinic type response")
		return
	}
}

// GET /api/v1/clinic-types?facility_id=...
func HandleClinicTypeList(w http.ResponseWriter, r *http.Request) {
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

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), clinicQueryTimeout)
	defer cancel()

	types, err := q.ListClinicTypesByFacility(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to list clinic types")
		http.Error(w, "Failed to list clinic types", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, types); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to write clinic types response")
		return
	}
}

// POST /api/v1/clinics
func HandleClinicSessionCreate(w http.ResponseWriter, r *http.Request) {
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

	req, err := decodeClinicSessionRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityID, err := resolveFacilityID(r, req.FacilityID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.FacilityID = facilityID

	if err := validateClinicSessionRequest(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	startTime, endTime, err := parseClinicTimes(req.StartTime, req.EndTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), clinicQueryTimeout)
	defer cancel()

	exists, err := facilityExists(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to validate facility")
		http.Error(w, "Failed to validate facility", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Facility not found", http.StatusNotFound)
		return
	}

	if err := ensureClinicType(ctx, q, facilityID, req.ClinicTypeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Clinic type not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to validate clinic type")
		http.Error(w, "Failed to validate clinic type", http.StatusInternalServerError)
		return
	}

	if err := ensureProForFacility(ctx, q, facilityID, req.ProID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Pro not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created, err := q.CreateClinicSession(ctx, dbgen.CreateClinicSessionParams{
		ClinicTypeID:     req.ClinicTypeID,
		FacilityID:       facilityID,
		ProID:            req.ProID,
		StartTime:        startTime,
		EndTime:          endTime,
		EnrollmentStatus: req.EnrollmentStatus,
		CreatedByUserID:  user.ID,
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to create clinic session")
		http.Error(w, "Failed to create clinic session", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, created); err != nil {
		logger.Error().Err(err).Int64("clinic_session_id", created.ID).Msg("Failed to write clinic session response")
		return
	}
}

// GET /api/v1/clinics?facility_id=...
func HandleClinicSessionList(w http.ResponseWriter, r *http.Request) {
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

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), clinicQueryTimeout)
	defer cancel()

	sessions, err := q.ListClinicSessionsByFacility(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to list clinic sessions")
		http.Error(w, "Failed to list clinic sessions", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, sessions); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to write clinic sessions response")
		return
	}
}

// PUT /api/v1/clinics/{id}
func HandleClinicSessionUpdate(w http.ResponseWriter, r *http.Request) {
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

	clinicID, err := clinicSessionIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid clinic ID", http.StatusBadRequest)
		return
	}

	req, err := decodeClinicSessionRequest(r)
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

	if err := validateFacilityMatch(r, req.FacilityID, facilityID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), clinicQueryTimeout)
	defer cancel()

	_, err = q.GetClinicSession(ctx, dbgen.GetClinicSessionParams{
		ID:         clinicID,
		FacilityID: facilityID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Clinic session not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("clinic_session_id", clinicID).Msg("Failed to fetch clinic session")
		http.Error(w, "Failed to fetch clinic session", http.StatusInternalServerError)
		return
	}
	req.FacilityID = facilityID

	if err := validateClinicSessionRequest(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	startTime, endTime, err := parseClinicTimes(req.StartTime, req.EndTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := ensureClinicType(ctx, q, facilityID, req.ClinicTypeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Clinic type not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to validate clinic type")
		http.Error(w, "Failed to validate clinic type", http.StatusInternalServerError)
		return
	}

	if err := ensureProForFacility(ctx, q, facilityID, req.ProID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Pro not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	updated, err := q.UpdateClinicSession(ctx, dbgen.UpdateClinicSessionParams{
		ID:               clinicID,
		ClinicTypeID:     req.ClinicTypeID,
		FacilityID:       facilityID,
		ProID:            req.ProID,
		StartTime:        startTime,
		EndTime:          endTime,
		EnrollmentStatus: req.EnrollmentStatus,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Clinic session not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("clinic_session_id", clinicID).Msg("Failed to update clinic session")
		http.Error(w, "Failed to update clinic session", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, updated); err != nil {
		logger.Error().Err(err).Int64("clinic_session_id", clinicID).Msg("Failed to write clinic session response")
		return
	}
}

// GET /api/v1/clinics/{id}/roster?facility_id=...
func HandleClinicRoster(w http.ResponseWriter, r *http.Request) {
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

	clinicID, err := clinicSessionIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid clinic ID", http.StatusBadRequest)
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

	ctx, cancel := context.WithTimeout(r.Context(), clinicQueryTimeout)
	defer cancel()

	if _, err := q.GetClinicSession(ctx, dbgen.GetClinicSessionParams{
		ID:         clinicID,
		FacilityID: facilityID,
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Clinic session not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("clinic_session_id", clinicID).Msg("Failed to fetch clinic session")
		http.Error(w, "Failed to fetch clinic session", http.StatusInternalServerError)
		return
	}

	roster, err := q.ListEnrollmentsForClinic(ctx, dbgen.ListEnrollmentsForClinicParams{
		ClinicSessionID: clinicID,
		FacilityID:      facilityID,
	})
	if err != nil {
		logger.Error().Err(err).Int64("clinic_session_id", clinicID).Msg("Failed to list clinic roster")
		http.Error(w, "Failed to list clinic roster", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, roster); err != nil {
		logger.Error().Err(err).Int64("clinic_session_id", clinicID).Msg("Failed to write clinic roster response")
		return
	}
}

// DELETE /api/v1/clinics/{id}?facility_id=...
func HandleClinicCancel(w http.ResponseWriter, r *http.Request) {
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

	clinicID, err := clinicSessionIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid clinic ID", http.StatusBadRequest)
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

	ctx, cancel := context.WithTimeout(r.Context(), clinicQueryTimeout)
	defer cancel()

	deleted, err := q.DeleteClinicSession(ctx, dbgen.DeleteClinicSessionParams{
		ID:         clinicID,
		FacilityID: facilityID,
	})
	if err != nil {
		logger.Error().Err(err).Int64("clinic_session_id", clinicID).Msg("Failed to cancel clinic session")
		http.Error(w, "Failed to cancel clinic session", http.StatusInternalServerError)
		return
	}
	if deleted == 0 {
		http.Error(w, "Clinic session not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func decodeClinicTypeRequest(r *http.Request) (clinicTypeRequest, error) {
	var req clinicTypeRequest
	if err := apiutil.DecodeJSON(r, &req); err != nil {
		return req, err
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Status = strings.TrimSpace(req.Status)
	if req.Status == "" {
		req.Status = "draft"
	}
	if req.Description != nil {
		trimmed := strings.TrimSpace(*req.Description)
		if trimmed == "" {
			req.Description = nil
		} else {
			req.Description = &trimmed
		}
	}
	return req, nil
}

func validateClinicTypeRequest(req *clinicTypeRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.MinParticipants <= 0 {
		return fmt.Errorf("minParticipants must be greater than 0")
	}
	if req.MaxParticipants <= 0 {
		return fmt.Errorf("maxParticipants must be greater than 0")
	}
	if req.MinParticipants > req.MaxParticipants {
		return fmt.Errorf("minParticipants must be less than or equal to maxParticipants")
	}
	if req.PriceCents < 0 {
		return fmt.Errorf("priceCents must be at least 0")
	}
	if !validClinicTypeStatus(req.Status) {
		return fmt.Errorf("status must be one of: draft, active, inactive, archived")
	}
	return nil
}

func decodeClinicSessionRequest(r *http.Request) (clinicSessionRequest, error) {
	var req clinicSessionRequest
	if err := apiutil.DecodeJSON(r, &req); err != nil {
		return req, err
	}
	req.EnrollmentStatus = strings.TrimSpace(req.EnrollmentStatus)
	if req.EnrollmentStatus == "" {
		req.EnrollmentStatus = "open"
	}
	req.StartTime = strings.TrimSpace(req.StartTime)
	req.EndTime = strings.TrimSpace(req.EndTime)
	return req, nil
}

func validateClinicSessionRequest(req *clinicSessionRequest) error {
	if req.ClinicTypeID <= 0 {
		return fmt.Errorf("clinicTypeId must be a positive integer")
	}
	if req.ProID <= 0 {
		return fmt.Errorf("proId must be a positive integer")
	}
	if req.StartTime == "" {
		return fmt.Errorf("startTime is required")
	}
	if req.EndTime == "" {
		return fmt.Errorf("endTime is required")
	}
	if !validEnrollmentStatus(req.EnrollmentStatus) {
		return fmt.Errorf("enrollmentStatus must be one of: open, closed")
	}
	return nil
}

func validClinicTypeStatus(status string) bool {
	switch status {
	case "draft", "active", "inactive", "archived":
		return true
	default:
		return false
	}
}

func validEnrollmentStatus(status string) bool {
	switch status {
	case "open", "closed":
		return true
	default:
		return false
	}
}

func parseClinicTimes(startValue, endValue string) (time.Time, time.Time, error) {
	startTime, err := parseClinicTime(startValue)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	endTime, err := parseClinicTime(endValue)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !endTime.After(startTime) {
		return time.Time{}, time.Time{}, fmt.Errorf("endTime must be after startTime")
	}
	return startTime, endTime, nil
}

func parseClinicTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("time value is required")
	}
	layouts := []string{timeLayoutDatetimeLocal, time.RFC3339}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time format")
}

func ensureClinicType(ctx context.Context, q *dbgen.Queries, facilityID, clinicTypeID int64) error {
	_, err := q.GetClinicType(ctx, dbgen.GetClinicTypeParams{
		ID:         clinicTypeID,
		FacilityID: facilityID,
	})
	return err
}

func ensureProForFacility(ctx context.Context, q *dbgen.Queries, facilityID, proID int64) error {
	pro, err := q.GetStaffByID(ctx, proID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(pro.Role, "pro") {
		return sql.ErrNoRows
	}
	if !pro.HomeFacilityID.Valid || pro.HomeFacilityID.Int64 != facilityID {
		return sql.ErrNoRows
	}
	if !isActiveStaffStatus(pro.UserStatus) {
		return sql.ErrNoRows
	}
	return nil
}

func isActiveStaffStatus(status string) bool {
	return strings.EqualFold(status, "active")
}

func facilityIDFromRequest(r *http.Request) (int64, error) {
	value := strings.TrimSpace(r.URL.Query().Get("facility_id"))
	if value == "" {
		return 0, fmt.Errorf("facility_id is required")
	}
	facilityID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || facilityID <= 0 {
		return 0, fmt.Errorf("facility_id must be a positive integer")
	}
	return facilityID, nil
}

func resolveFacilityID(r *http.Request, payloadFacilityID int64) (int64, error) {
	queryID := strings.TrimSpace(r.URL.Query().Get("facility_id"))
	if queryID != "" {
		queryValue, err := strconv.ParseInt(queryID, 10, 64)
		if err != nil || queryValue <= 0 {
			return 0, fmt.Errorf("facility_id must be a positive integer")
		}
		if payloadFacilityID != 0 && payloadFacilityID != queryValue {
			return 0, fmt.Errorf("facility_id mismatch between query and payload")
		}
		return queryValue, nil
	}

	if payloadFacilityID <= 0 {
		return 0, fmt.Errorf("facility_id is required")
	}
	return payloadFacilityID, nil
}

func validateFacilityMatch(r *http.Request, payloadFacilityID, expectedFacilityID int64) error {
	queryID := strings.TrimSpace(r.URL.Query().Get("facility_id"))
	if queryID != "" {
		queryValue, err := strconv.ParseInt(queryID, 10, 64)
		if err != nil || queryValue <= 0 {
			return fmt.Errorf("facility_id must be a positive integer")
		}
		if queryValue != expectedFacilityID {
			return fmt.Errorf("facility_id mismatch between clinic session and request")
		}
	}
	if payloadFacilityID != 0 && payloadFacilityID != expectedFacilityID {
		return fmt.Errorf("facility_id mismatch between clinic session and payload")
	}
	return nil
}

func facilityExists(ctx context.Context, q *dbgen.Queries, facilityID int64) (bool, error) {
	count, err := q.FacilityExists(ctx, facilityID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func clinicSessionIDFromRequest(r *http.Request) (int64, error) {
	value := strings.TrimSpace(r.PathValue("id"))
	if value == "" {
		return 0, fmt.Errorf("invalid clinic ID")
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid clinic ID")
	}
	return id, nil
}

func toNullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: trimmed, Valid: true}
}

func loadQueries() *dbgen.Queries {
	return queries
}
