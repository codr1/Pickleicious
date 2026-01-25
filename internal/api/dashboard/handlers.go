// internal/api/dashboard/handlers.go
package dashboard

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	appdb "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	"github.com/codr1/Pickleicious/internal/request"
	dashboardtempl "github.com/codr1/Pickleicious/internal/templates/components/dashboard"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

const (
	dashboardQueryTimeout = 5 * time.Second
	dashboardDateLayout   = "2006-01-02"
	defaultRangeDays      = 30
	dateRangeToday        = "today"
	dateRangeLast7Days    = "last_7_days"
	dateRangeLast30Days   = "last_30_days"
	dateRangeThisMonth    = "this_month"
	dateRangeThisYear     = "this_year"
	dateRangeCustom       = "custom"
)

var (
	queries     *dbgen.Queries
	queriesOnce sync.Once
)

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(database *appdb.DB) {
	if database == nil {
		log.Warn().Msg("InitHandlers called with nil database; dashboard handlers will be unavailable")
		return
	}
	queriesOnce.Do(func() {
		queries = database.Queries
	})
}

// HandleDashboardPage renders the dashboard page for GET /admin/dashboard.
func HandleDashboardPage(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	facilityID, err := resolveFacilityID(r, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireDashboardAccess(user, facilityID); err != nil {
		respondAccessError(w, r, facilityID, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dashboardQueryTimeout)
	defer cancel()

	facilityName := ""
	facilityLoc := time.Local
	if facilityID > 0 {
		facility, err := q.GetFacilityByID(ctx, facilityID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Facility not found", http.StatusNotFound)
				return
			}
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load facility")
			http.Error(w, "Failed to load facility", http.StatusInternalServerError)
			return
		}
		facilityName = facility.Name
		facilityLoc = loadFacilityLocation(logger, facility.Timezone)
	} else {
		facilityName = "All Facilities"
	}

	startTime, endTime, dateRange, dateRangePreset, startDate, endDate, err := parseDateRange(r, facilityLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	granularity, err := parseGranularity(r.URL.Query().Get("granularity"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := buildDashboardData(ctx, q, facilityID, facilityName, startTime, endTime, dateRange, granularity)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to build dashboard data")
		http.Error(w, "Failed to load dashboard", http.StatusInternalServerError)
		return
	}

	data.DateRangePreset = dateRangePreset
	data.StartDate = startDate
	data.EndDate = endDate

	showFacilitySelector := user != nil && user.IsStaff && user.HomeFacilityID == nil
	if showFacilitySelector {
		facilities, err := q.ListFacilities(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to load facilities for dashboard")
			http.Error(w, "Failed to load facilities", http.StatusInternalServerError)
			return
		}
		data.ShowFacilitySelector = true
		data.Facilities = make([]dashboardtempl.FacilityOption, 0, len(facilities))
		for _, facility := range facilities {
			data.Facilities = append(data.Facilities, dashboardtempl.FacilityOption{
				ID:   facility.ID,
				Name: facility.Name,
			})
		}
	}

	var activeTheme *models.Theme
	if facilityID > 0 {
		var themeErr error
		activeTheme, themeErr = models.GetActiveTheme(ctx, q, facilityID)
		if themeErr != nil {
			logger.Error().Err(themeErr).Int64("facility_id", facilityID).Msg("Failed to load active theme")
			activeTheme = nil
		}
	}

	sessionType := authz.SessionTypeFromContext(r.Context())
	page := layouts.Base(dashboardtempl.DashboardLayout(data), activeTheme, sessionType)
	if !apiutil.RenderHTMLComponent(r.Context(), w, page, nil, "Failed to render dashboard page", "Failed to render page") {
		return
	}
}

// HandleDashboardMetrics returns the dashboard metrics partial for GET /api/v1/dashboard/metrics.
func HandleDashboardMetrics(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	facilityID, err := resolveFacilityID(r, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := requireDashboardAccess(user, facilityID); err != nil {
		respondAccessError(w, r, facilityID, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dashboardQueryTimeout)
	defer cancel()

	facilityName := ""
	facilityLoc := time.Local
	if facilityID > 0 {
		facility, err := q.GetFacilityByID(ctx, facilityID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Facility not found", http.StatusNotFound)
				return
			}
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load facility")
			http.Error(w, "Failed to load facility", http.StatusInternalServerError)
			return
		}
		facilityName = facility.Name
		facilityLoc = loadFacilityLocation(logger, facility.Timezone)
	} else {
		facilityName = "All Facilities"
	}

	startTime, endTime, dateRange, dateRangePreset, startDate, endDate, err := parseDateRange(r, facilityLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	granularity, err := parseGranularity(r.URL.Query().Get("granularity"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := buildDashboardData(ctx, q, facilityID, facilityName, startTime, endTime, dateRange, granularity)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to build dashboard metrics")
		http.Error(w, "Failed to load dashboard metrics", http.StatusInternalServerError)
		return
	}

	data.DateRangePreset = dateRangePreset
	data.StartDate = startDate
	data.EndDate = endDate

	component := dashboardtempl.DashboardMetrics(data)
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render dashboard metrics", "Failed to render metrics") {
		return
	}
}

func buildDashboardData(ctx context.Context, q *dbgen.Queries, facilityID int64, facilityName string, startTime time.Time, endTime time.Time, dateRange string, granularity string) (dashboardtempl.DashboardData, error) {
	bookings, err := q.CountReservationsByTypeInRange(ctx, dbgen.CountReservationsByTypeInRangeParams{
		FacilityID: facilityID,
		StartTime:  startTime,
		EndTime:    endTime,
	})
	if err != nil {
		return dashboardtempl.DashboardData{}, err
	}

	reservationTypes, err := q.ListReservationTypes(ctx)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Failed to load reservation types")
	}
	typeNameByID := make(map[int64]string, len(reservationTypes))
	for _, reservationType := range reservationTypes {
		typeNameByID[reservationType.ID] = reservationType.Name
	}

	bookingsByType := make([]dashboardtempl.BookingTypeCount, 0, len(bookings))
	for _, booking := range bookings {
		bookingsByType = append(bookingsByType, dashboardtempl.BookingTypeCount{
			TypeID:   booking.ReservationTypeID,
			TypeName: typeNameByID[booking.ReservationTypeID],
			Count:    booking.ReservationCount,
		})
	}

	checkinsCount, err := q.CountCheckinsByFacilityInRange(ctx, dbgen.CountCheckinsByFacilityInRangeParams{
		FacilityID: facilityID,
		StartTime:  startTime,
		EndTime:    endTime,
	})
	if err != nil {
		return dashboardtempl.DashboardData{}, err
	}

	cancellationMetrics, err := q.GetCancellationMetricsInRange(ctx, dbgen.GetCancellationMetricsInRangeParams{
		FacilityID: facilityID,
		StartTime:  startTime,
		EndTime:    endTime,
	})
	if err != nil {
		return dashboardtempl.DashboardData{}, err
	}

	availableHours, err := q.GetAvailableCourtHours(ctx, dbgen.GetAvailableCourtHoursParams{
		StartTime:  startTime,
		EndTime:    endTime,
		FacilityID: facilityID,
	})
	if err != nil {
		return dashboardtempl.DashboardData{}, err
	}

	bookedHours, err := q.GetBookedCourtHours(ctx, dbgen.GetBookedCourtHoursParams{
		StartTime:  startTime,
		EndTime:    endTime,
		FacilityID: facilityID,
	})
	if err != nil {
		return dashboardtempl.DashboardData{}, err
	}

	utilizationRate := 0.0
	if availableHours > 0 {
		utilizationRate = bookedHours / availableHours
		if utilizationRate > 1.0 {
			utilizationRate = 1.0
		}
	}

	comparisonTime := time.Now().UTC()
	reservationStatusCounts, err := q.CountScheduledVsCompletedReservations(ctx, dbgen.CountScheduledVsCompletedReservationsParams{
		FacilityID:     facilityID,
		StartTime:      startTime,
		EndTime:        endTime,
		ComparisonTime: comparisonTime,
	})
	if err != nil {
		return dashboardtempl.DashboardData{}, err
	}

	var scheduledCount int64
	for _, statusCount := range reservationStatusCounts {
		if statusCount.ReservationStatus == "scheduled" {
			scheduledCount = statusCount.ReservationCount
			break
		}
	}

	return dashboardtempl.DashboardData{
		FacilityID:      facilityID,
		FacilityName:    facilityName,
		DateRange:       dateRange,
		UtilizationRate: utilizationRate,
		ScheduledCount:  scheduledCount,
		BookingsByType:  bookingsByType,
		CancellationMetrics: dashboardtempl.CancellationMetrics{
			Count:                 cancellationMetrics.CancellationsCount,
			TotalReservations:     cancellationMetrics.TotalReservations,
			Rate:                  cancellationMetrics.CancellationRate,
			TotalRefundPercentage: cancellationMetrics.TotalRefundPercentage,
		},
		CheckinCount: checkinsCount,
		Granularity:  granularity,
	}, nil
}

func parseDateRange(r *http.Request, loc *time.Location) (time.Time, time.Time, string, string, string, string, error) {
	query := r.URL.Query()
	rangeRaw := strings.TrimSpace(query.Get("date_range"))
	preset := strings.ToLower(rangeRaw)
	startRaw := strings.TrimSpace(query.Get("start_date"))
	endRaw := strings.TrimSpace(query.Get("end_date"))

	if rangeRaw != "" && strings.Contains(rangeRaw, "to") && !isKnownDateRangePreset(preset) {
		parts := strings.SplitN(rangeRaw, "to", 2)
		if len(parts) != 2 {
			return time.Time{}, time.Time{}, "", "", "", "", fmt.Errorf("date_range must be in YYYY-MM-DD to YYYY-MM-DD format")
		}
		startRaw = strings.TrimSpace(parts[0])
		endRaw = strings.TrimSpace(parts[1])
		preset = ""
	}

	if preset != "" && preset != dateRangeCustom {
		startDate, endDate := presetDateRange(preset, loc)
		if startDate.IsZero() || endDate.IsZero() {
			return time.Time{}, time.Time{}, "", "", "", "", fmt.Errorf("invalid date_range")
		}
		return startDate, endDate.AddDate(0, 0, 1), formatDateRange(startDate, endDate), preset, formatDate(startDate), formatDate(endDate), nil
	}

	if startRaw == "" && endRaw == "" {
		if preset == dateRangeCustom {
			return time.Time{}, time.Time{}, "", "", "", "", fmt.Errorf("start_date and end_date are required")
		}
		now := time.Now().In(loc)
		endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		startDate := endDate.AddDate(0, 0, -(defaultRangeDays - 1))
		return startDate, endDate.AddDate(0, 0, 1), formatDateRange(startDate, endDate), dateRangeLast30Days, formatDate(startDate), formatDate(endDate), nil
	}

	if startRaw == "" || endRaw == "" {
		return time.Time{}, time.Time{}, "", "", "", "", fmt.Errorf("start_date and end_date are required")
	}

	startDate, err := time.ParseInLocation(dashboardDateLayout, startRaw, loc)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", "", "", fmt.Errorf("start_date must be in YYYY-MM-DD format")
	}

	endDate, err := time.ParseInLocation(dashboardDateLayout, endRaw, loc)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", "", "", fmt.Errorf("end_date must be in YYYY-MM-DD format")
	}

	if endDate.Before(startDate) {
		return time.Time{}, time.Time{}, "", "", "", "", fmt.Errorf("end_date must be after start_date")
	}

	return startDate, endDate.AddDate(0, 0, 1), formatDateRange(startDate, endDate), dateRangeCustom, formatDate(startDate), formatDate(endDate), nil
}

func parseGranularity(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "day", nil
	}

	switch value {
	case "day", "daily":
		return "day", nil
	case "week", "weekly":
		return "week", nil
	case "month", "monthly":
		return "month", nil
	case "annual":
		return "annual", nil
	case "dow_month":
		return "dow_month", nil
	case "dow_year":
		return "dow_year", nil
	default:
		return "", fmt.Errorf("invalid granularity")
	}
}

func resolveFacilityID(r *http.Request, user *authz.AuthUser) (int64, error) {
	rawFacilityID := strings.TrimSpace(r.URL.Query().Get("facility_id"))
	if rawFacilityID != "" {
		if rawFacilityID == "0" {
			return 0, nil
		}
		if facilityID, ok := request.ParseFacilityID(rawFacilityID); ok {
			return facilityID, nil
		}
		return 0, fmt.Errorf("facility_id must be a positive integer")
	}

	if user != nil && user.HomeFacilityID != nil {
		return *user.HomeFacilityID, nil
	}

	if user != nil && user.IsStaff {
		return 0, nil
	}

	return 0, fmt.Errorf("facility_id is required")
}

func requireDashboardAccess(user *authz.AuthUser, facilityID int64) error {
	if user == nil {
		return authz.ErrUnauthenticated
	}
	if !user.IsStaff {
		return authz.ErrForbidden
	}
	if user.HomeFacilityID == nil {
		return nil
	}
	if *user.HomeFacilityID != facilityID {
		return authz.ErrForbidden
	}
	return nil
}

func presetDateRange(preset string, loc *time.Location) (time.Time, time.Time) {
	now := time.Now().In(loc)
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	switch preset {
	case dateRangeToday:
		return endDate, endDate
	case dateRangeLast7Days:
		return endDate.AddDate(0, 0, -6), endDate
	case dateRangeLast30Days:
		return endDate.AddDate(0, 0, -(defaultRangeDays - 1)), endDate
	case dateRangeThisMonth:
		startDate := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		return startDate, endDate
	case dateRangeThisYear:
		startDate := time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, loc)
		return startDate, endDate
	default:
		return time.Time{}, time.Time{}
	}
}

func formatDateRange(startDate time.Time, endDate time.Time) string {
	return fmt.Sprintf("%s to %s", startDate.Format(dashboardDateLayout), endDate.Format(dashboardDateLayout))
}

func formatDate(value time.Time) string {
	return value.Format(dashboardDateLayout)
}

func isKnownDateRangePreset(preset string) bool {
	switch preset {
	case dateRangeToday, dateRangeLast7Days, dateRangeLast30Days, dateRangeThisMonth, dateRangeThisYear, dateRangeCustom:
		return true
	default:
		return false
	}
}

func respondAccessError(w http.ResponseWriter, r *http.Request, facilityID int64, err error) {
	logger := log.Ctx(r.Context())
	user := authz.UserFromContext(r.Context())

	switch {
	case errors.Is(err, authz.ErrUnauthenticated):
		logEvent := logger.Warn().Int64("facility_id", facilityID)
		if user != nil {
			logEvent = logEvent.Int64("user_id", user.ID)
		}
		logEvent.Msg("Dashboard access denied: unauthenticated")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	case errors.Is(err, authz.ErrForbidden):
		logEvent := logger.Warn().Int64("facility_id", facilityID)
		if user != nil {
			logEvent = logEvent.Int64("user_id", user.ID)
		}
		logEvent.Msg("Dashboard access denied: forbidden")
		http.Error(w, "Forbidden", http.StatusForbidden)
	default:
		logEvent := logger.Error().Int64("facility_id", facilityID).Err(err)
		if user != nil {
			logEvent = logEvent.Int64("user_id", user.ID)
		}
		logEvent.Msg("Dashboard access denied: error")
		http.Error(w, "Failed to authorize request", http.StatusInternalServerError)
	}
}

func loadFacilityLocation(logger *zerolog.Logger, timezone string) *time.Location {
	if timezone == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		logger.Error().Err(err).Str("timezone", timezone).Msg("Failed to load facility timezone")
		return time.Local
	}
	return loc
}

func loadQueries() *dbgen.Queries {
	return queries
}
