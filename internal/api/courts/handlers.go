// internal/api/courts/handlers.go
package courts

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	"github.com/codr1/Pickleicious/internal/request"
	"github.com/codr1/Pickleicious/internal/templates/components/courts"
	reservationstempl "github.com/codr1/Pickleicious/internal/templates/components/reservations"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
	"github.com/rs/zerolog/log"
)

var (
	queries     *dbgen.Queries
	queriesOnce sync.Once
)

const courtsQueryTimeout = 5 * time.Second

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(q *dbgen.Queries) {
	if q == nil {
		return
	}
	queriesOnce.Do(func() {
		queries = q
	})
}

func HandleCourtsPage(w http.ResponseWriter, r *http.Request) {
	log.Info().
		Str("path", r.URL.Path).
		Str("method", r.Method).
		Msg("Handling courts page request")

	q := loadQueries()
	if q == nil {
		log.Ctx(r.Context()).Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var activeTheme *models.Theme
	displayDate := calendarDateFromRequest(r)
	calendarData := courts.CalendarData{DisplayDate: displayDate}
	if facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id")); ok {
		if !apiutil.RequireFacilityAccess(w, r, facilityID) {
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), courtsQueryTimeout)
		defer cancel()

		var err error
		activeTheme, err = models.GetActiveTheme(ctx, q, facilityID)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
			activeTheme = nil
		}

		calendarData, err = buildCalendarData(ctx, q, facilityID, displayDate)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load calendar reservations")
			calendarData = courts.CalendarData{DisplayDate: displayDate, FacilityID: facilityID}
		}
	}

	calendar := courts.Calendar(calendarData)
	page := layouts.Base(calendar, activeTheme)
	page.Render(r.Context(), w)
}

func HandleCalendarView(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, ok := request.FacilityIDFromBookingRequest(r)
	if !ok {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	displayDate := calendarDateFromRequest(r)
	ctx, cancel := context.WithTimeout(r.Context(), courtsQueryTimeout)
	defer cancel()

	calendarData, err := buildCalendarData(ctx, q, facilityID, displayDate)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load calendar reservations")
		http.Error(w, "Failed to load calendar reservations", http.StatusInternalServerError)
		return
	}

	component := courts.Calendar(calendarData)
	component.Render(r.Context(), w)
}

// GET /api/v1/courts/booking/new
func HandleBookingFormNew(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, ok := request.FacilityIDFromBookingRequest(r)
	if !ok {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}
	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	courtNumber, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("court")), 10, 64)
	hourValue := strings.TrimSpace(r.URL.Query().Get("hour"))
	hour, hourErr := strconv.Atoi(hourValue)

	now := time.Now()
	baseDate := now
	dateValue := strings.TrimSpace(r.URL.Query().Get("date"))
	if dateValue != "" {
		parsedDate, err := time.ParseInLocation("2006-01-02", dateValue, now.Location())
		if err == nil {
			baseDate = parsedDate
		}
	}

	startHour := now.Hour()
	if hourErr == nil && hour >= 0 && hour <= 23 {
		startHour = hour
	}
	startTime := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(), startHour, 0, 0, 0, baseDate.Location())
	endTime := startTime.Add(time.Hour)

	ctx, cancel := context.WithTimeout(r.Context(), courtsQueryTimeout)
	defer cancel()

	courtsList, err := q.ListCourts(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load courts")
		http.Error(w, "Failed to load courts", http.StatusInternalServerError)
		return
	}

	reservationTypes, err := q.ListReservationTypes(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load reservation types")
		http.Error(w, "Failed to load reservation types", http.StatusInternalServerError)
		return
	}

	memberRows, err := q.ListMembers(ctx, dbgen.ListMembersParams{
		SearchTerm: nil,
		Offset:     0,
		Limit:      50,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load members for booking form")
		memberRows = nil
	}

	var selectedCourtID int64
	if courtNumber > 0 {
		for _, court := range courtsList {
			if court.CourtNumber == courtNumber {
				selectedCourtID = court.ID
				break
			}
		}
	}

	var buf bytes.Buffer
	component := reservationstempl.BookingForm(reservationstempl.BookingFormData{
		FacilityID:        facilityID,
		StartTime:         startTime,
		EndTime:           endTime,
		Courts:            reservationstempl.NewCourtOptions(courtsList),
		ReservationTypes:  reservationstempl.NewReservationTypeOptions(reservationTypes),
		Members:           reservationstempl.NewMemberOptions(memberRows),
		SelectedCourtID:   selectedCourtID,
	})
	if err := component.Render(r.Context(), &buf); err != nil {
		logger.Error().Err(err).Msg("Failed to render booking form")
		http.Error(w, "Failed to render booking form", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf.Bytes()); err != nil {
		logger.Error().Err(err).Msg("Failed to write response")
	}
}

func loadQueries() *dbgen.Queries {
	return queries
}

func calendarDateFromRequest(r *http.Request) time.Time {
	now := time.Now()
	dateParam := strings.TrimSpace(r.URL.Query().Get("date"))
	if dateParam == "" {
		return now
	}

	parsed, err := time.ParseInLocation("2006-01-02", dateParam, now.Location())
	if err != nil {
		return now
	}

	return parsed
}

func buildCalendarData(ctx context.Context, q *dbgen.Queries, facilityID int64, displayDate time.Time) (courts.CalendarData, error) {
	calendarData := courts.CalendarData{DisplayDate: displayDate, FacilityID: facilityID}

	courtsList, err := q.ListCourts(ctx, facilityID)
	if err != nil {
		return calendarData, err
	}
	calendarData.Courts = make([]courts.CalendarCourt, 0, len(courtsList))
	for _, court := range courtsList {
		calendarData.Courts = append(calendarData.Courts, courts.CalendarCourt{
			ID:          court.ID,
			CourtNumber: court.CourtNumber,
			Name:        court.Name,
		})
	}

	dayStart := time.Date(displayDate.Year(), displayDate.Month(), displayDate.Day(), 0, 0, 0, 0, displayDate.Location())
	dayEnd := dayStart.AddDate(0, 0, 1)

	reservations, err := q.ListReservationsByDateRange(ctx, dbgen.ListReservationsByDateRangeParams{
		FacilityID: facilityID,
		StartTime:  dayStart,
		EndTime:    dayEnd,
	})
	if err != nil {
		return calendarData, err
	}

	reservationCourts, err := q.ListReservationCourtsByDateRange(ctx, dbgen.ListReservationCourtsByDateRangeParams{
		FacilityID: facilityID,
		StartTime:  dayStart,
		EndTime:    dayEnd,
	})
	if err != nil {
		return calendarData, err
	}

	reservationTypes, err := q.ListReservationTypes(ctx)
	if err != nil {
		return calendarData, err
	}

	typeByID := make(map[int64]dbgen.ReservationType, len(reservationTypes))
	for _, resType := range reservationTypes {
		typeByID[resType.ID] = resType
	}

	courtsByReservation := make(map[int64][]int64, len(reservationCourts))
	for _, row := range reservationCourts {
		courtsByReservation[row.ReservationID] = append(courtsByReservation[row.ReservationID], row.CourtNumber)
	}

	for _, reservation := range reservations {
		resType := typeByID[reservation.ReservationTypeID]
		typeName := strings.TrimSpace(resType.Name)
		if typeName == "" {
			typeName = "Reservation"
		}

		typeColor := ""
		if resType.Color.Valid {
			typeColor = strings.TrimSpace(resType.Color.String)
		}

		for _, courtNumber := range courtsByReservation[reservation.ID] {
			calendarData.Reservations = append(calendarData.Reservations, courts.CalendarReservation{
				ID:          reservation.ID,
				CourtNumber: courtNumber,
				StartTime:   reservation.StartTime,
				EndTime:     reservation.EndTime,
				TypeName:    typeName,
				TypeColor:   typeColor,
			})
		}
	}

	return calendarData, nil
}
