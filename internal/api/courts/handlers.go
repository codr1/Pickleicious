// internal/api/courts/handlers.go
package courts

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
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
	if facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id")); ok {
		ctx, cancel := context.WithTimeout(r.Context(), courtsQueryTimeout)
		defer cancel()

		var err error
		activeTheme, err = models.GetActiveTheme(ctx, q, facilityID)
		if err != nil {
			log.Ctx(r.Context()).Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
			activeTheme = nil
		}
	}

	calendar := courts.Calendar()
	page := layouts.Base(calendar, activeTheme)
	page.Render(r.Context(), w)
}

func HandleCalendarView(w http.ResponseWriter, r *http.Request) {
	component := courts.Calendar()
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

	facilityID, ok := facilityIDFromBookingRequest(r)
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
	if hourErr == nil && hour >= 0 && hour <= 23 {
		now = time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	}
	startTime := now
	endTime := now.Add(time.Hour)

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
	w.Write(buf.Bytes())
}

func loadQueries() *dbgen.Queries {
	return queries
}

func facilityIDFromBookingRequest(r *http.Request) (int64, bool) {
	if facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id")); ok {
		return facilityID, true
	}

	currentURL := strings.TrimSpace(r.Header.Get("HX-Current-URL"))
	if currentURL == "" {
		return 0, false
	}

	parsed, err := url.Parse(currentURL)
	if err != nil {
		log.Ctx(r.Context()).
			Debug().
			Err(err).
			Str("hx_current_url", currentURL).
			Msg("Failed to parse HX-Current-URL")
		return 0, false
	}

	return request.ParseFacilityID(parsed.Query().Get("facility_id"))
}
