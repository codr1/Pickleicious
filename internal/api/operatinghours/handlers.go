// internal/api/operatinghours/handlers.go
package operatinghours

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

	"github.com/a-h/templ"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	operatinghourstempl "github.com/codr1/Pickleicious/internal/templates/components/operatinghours"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

const (
	operatingHoursQueryTimeout = 5 * time.Second
	facilityIDQueryKey         = "facility_id"
	dayOfWeekParam             = "day_of_week"
	defaultOpensAt             = "08:00"
	defaultClosesAt            = "21:00"
)

var (
	queries     *dbgen.Queries
	queriesOnce sync.Once
)

type operatingHoursRequest struct {
	FacilityID *int64 `json:"facilityId"`
	OpensAt    string `json:"opensAt"`
	ClosesAt   string `json:"closesAt"`
	IsClosed   bool   `json:"isClosed"`
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

// GET /admin/operating-hours
func HandleOperatingHoursPage(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), operatingHoursQueryTimeout)
	defer cancel()

	facility, err := q.GetFacilityByID(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load facility booking config")
		http.Error(w, "Failed to load facility booking config", http.StatusInternalServerError)
		return
	}

	hours, err := q.GetFacilityHours(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to fetch operating hours")
		http.Error(w, "Failed to load operating hours", http.StatusInternalServerError)
		return
	}
	if len(hours) == 0 {
		hours = defaultOperatingHours(facilityID)
	}

	activeTheme, err := models.GetActiveTheme(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
		activeTheme = nil
	}

	sessionType := authz.SessionTypeFromContext(r.Context())
	bookingConfig := operatinghourstempl.BookingConfigData{
		FacilityID:            facilityID,
		MaxAdvanceBookingDays: facility.MaxAdvanceBookingDays,
		MaxMemberReservations: facility.MaxMemberReservations,
	}
	page := layouts.Base(operatingHoursPageComponent(facilityID, hours, bookingConfig), activeTheme, sessionType)
	if !apiutil.RenderHTMLComponent(r.Context(), w, page, nil, "Failed to render operating hours page", "Failed to render page") {
		return
	}
}

// PUT /api/v1/operating-hours/{day_of_week}
func HandleOperatingHoursUpdate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	dayOfWeek, err := dayOfWeekFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req, err := decodeOperatingHoursRequest(r)
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

	ctx, cancel := context.WithTimeout(r.Context(), operatingHoursQueryTimeout)
	defer cancel()

	if req.IsClosed {
		_, err := q.DeleteOperatingHours(ctx, dbgen.DeleteOperatingHoursParams{
			FacilityID: facilityID,
			DayOfWeek:  dayOfWeek,
		})
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", facilityID).Int64("day_of_week", dayOfWeek).Msg("Failed to delete operating hours")
			http.Error(w, "Failed to update operating hours", http.StatusInternalServerError)
			return
		}
		if apiutil.IsJSONRequest(r) {
			if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true}); err != nil {
				logger.Error().Err(err).Int64("facility_id", facilityID).Int64("day_of_week", dayOfWeek).Msg("Failed to write operating hours response")
			}
		} else {
			apiutil.WriteHTMLFeedback(w, http.StatusOK, "Operating hours cleared.")
		}
		return
	}

	opensAtRaw := strings.TrimSpace(req.OpensAt)
	closesAtRaw := strings.TrimSpace(req.ClosesAt)
	if opensAtRaw == "" && closesAtRaw == "" {
		count, err := q.OperatingHoursExists(ctx, dbgen.OperatingHoursExistsParams{
			FacilityID: facilityID,
			DayOfWeek:  dayOfWeek,
		})
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", facilityID).Int64("day_of_week", dayOfWeek).Msg("Failed to check existing operating hours")
			http.Error(w, "Failed to update operating hours", http.StatusInternalServerError)
			return
		}
		if count == 0 {
			opensAtRaw = defaultOpensAt
			closesAtRaw = defaultClosesAt
		}
	}

	opensAt, opensTime, err := parseOperatingTime(opensAtRaw, "opens_at")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	closesAt, closesTime, err := parseOperatingTime(closesAtRaw, "closes_at")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !opensTime.Before(closesTime) {
		http.Error(w, "opens_at must be before closes_at", http.StatusBadRequest)
		return
	}

	updated, err := q.UpsertOperatingHours(ctx, dbgen.UpsertOperatingHoursParams{
		FacilityID: facilityID,
		DayOfWeek:  dayOfWeek,
		OpensAt:    opensAt,
		ClosesAt:   closesAt,
	})
	if err != nil {
		if apiutil.IsSQLiteForeignKeyViolation(err) {
			http.Error(w, "Facility not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Int64("day_of_week", dayOfWeek).Msg("Failed to upsert operating hours")
		http.Error(w, "Failed to update operating hours", http.StatusInternalServerError)
		return
	}

	if apiutil.IsJSONRequest(r) {
		if err := apiutil.WriteJSON(w, http.StatusOK, updated); err != nil {
			logger.Error().Err(err).Int64("facility_id", facilityID).Int64("day_of_week", dayOfWeek).Msg("Failed to write operating hours response")
		}
		return
	}
	apiutil.WriteHTMLFeedback(w, http.StatusOK, "Operating hours saved.")
}

// POST /api/v1/facility-settings
func HandleFacilitySettingsUpdate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	facilityID, err := parsePositiveInt64Field(r.FormValue("facility_id"), "facility_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	maxAdvanceDays, err := parsePositiveInt64Field(r.FormValue("max_advance_booking_days"), "max_advance_booking_days")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	maxMemberReservations, err := parsePositiveInt64Field(r.FormValue("max_member_reservations"), "max_member_reservations")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), operatingHoursQueryTimeout)
	defer cancel()

	_, err = q.UpdateFacilityBookingConfig(ctx, dbgen.UpdateFacilityBookingConfigParams{
		ID:                    facilityID,
		MaxAdvanceBookingDays: maxAdvanceDays,
		MaxMemberReservations: maxMemberReservations,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Warn().Int64("facility_id", facilityID).Msg("Facility not found for booking config update")
			http.Error(w, "Facility not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to update facility booking config")
		http.Error(w, "Failed to update booking configuration", http.StatusInternalServerError)
		return
	}

	apiutil.WriteHTMLFeedback(w, http.StatusOK, "Booking configuration updated.")
}

func operatingHoursPageComponent(facilityID int64, hours []dbgen.OperatingHour, bookingConfig operatinghourstempl.BookingConfigData) templ.Component {
	hoursByDay := make(map[int64]dbgen.OperatingHour, len(hours))
	for _, hour := range hours {
		hoursByDay[hour.DayOfWeek] = hour
	}

	days := make([]operatinghourstempl.DayHours, 0, 7)
	for day := int64(0); day < 7; day++ {
		hour, ok := hoursByDay[day]
		entry := operatinghourstempl.DayHours{
			DayOfWeek: day,
			IsClosed:  !ok,
		}
		if ok {
			entry.OpensAt = formatTimeValue(hour.OpensAt)
			entry.ClosesAt = formatTimeValue(hour.ClosesAt)
		}
		days = append(days, entry)
	}

	return operatinghourstempl.OperatingHoursLayout(facilityID, days, bookingConfig)
}

func defaultOperatingHours(facilityID int64) []dbgen.OperatingHour {
	hours := make([]dbgen.OperatingHour, 0, 7)
	for day := int64(0); day < 7; day++ {
		hours = append(hours, dbgen.OperatingHour{
			FacilityID: facilityID,
			DayOfWeek:  day,
			OpensAt:    defaultOpensAt,
			ClosesAt:   defaultClosesAt,
		})
	}
	return hours
}

func parseOperatingTime(raw string, field string) (string, time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", time.Time{}, fmt.Errorf("%s is required", field)
	}
	parsed, err := time.Parse("15:04", raw)
	if err != nil {
		parsed, err = time.Parse("3:04 PM", strings.ToUpper(raw))
		if err != nil {
			return "", time.Time{}, fmt.Errorf("%s must be in HH:MM or H:MM AM/PM format", field)
		}
	}
	return parsed.Format("15:04"), parsed, nil
}

func parseOptionalBool(raw string) (bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, nil
	}
	switch strings.ToLower(raw) {
	case "true", "1", "on", "yes":
		return true, nil
	case "false", "0", "off", "no":
		return false, nil
	default:
		return false, fmt.Errorf("is_closed must be true or false")
	}
}

func parsePositiveInt64Field(raw string, field string) (int64, error) {
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

func dayOfWeekFromRequest(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(dayOfWeekParam))
	if raw == "" {
		return 0, fmt.Errorf("invalid day_of_week")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 || value > 6 {
		return 0, fmt.Errorf("day_of_week must be between 0 and 6")
	}
	return value, nil
}

func decodeOperatingHoursRequest(r *http.Request) (operatingHoursRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req operatingHoursRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return operatingHoursRequest{}, err
	}

	facilityID, err := apiutil.ParseOptionalInt64Field(apiutil.FirstNonEmpty(r.FormValue("facility_id"), r.FormValue("facilityId")), "facility_id")
	if err != nil {
		return operatingHoursRequest{}, err
	}

	isClosed, err := parseOptionalBool(apiutil.FirstNonEmpty(r.FormValue("is_closed"), r.FormValue("isClosed")))
	if err != nil {
		return operatingHoursRequest{}, err
	}

	return operatingHoursRequest{
		FacilityID: facilityID,
		OpensAt:    apiutil.FirstNonEmpty(r.FormValue("opens_at"), r.FormValue("opensAt")),
		ClosesAt:   apiutil.FirstNonEmpty(r.FormValue("closes_at"), r.FormValue("closesAt")),
		IsClosed:   isClosed,
	}, nil
}

func loadQueries() *dbgen.Queries {
	return queries
}

func formatTimeValue(value interface{}) string {
	switch typed := value.(type) {
	case time.Time:
		return typed.Format("15:04")
	case []byte:
		return string(typed)
	case string:
		return typed
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}
