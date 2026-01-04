// internal/api/courts/handlers.go
package courts

import (
	"bytes"
	"context"
	"fmt"
	"html"
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
	buf.WriteString(`<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/40">`)
	buf.WriteString(`<div class="bg-white rounded-lg shadow-lg w-full max-w-lg p-6">`)
	buf.WriteString(`<div class="flex items-center justify-between mb-4">`)
	buf.WriteString(`<h2 class="text-lg font-semibold text-gray-900">New Reservation</h2>`)
	buf.WriteString(`<button type="button" class="text-gray-400 hover:text-gray-600" onclick="document.getElementById('modal').innerHTML=''">`)
	buf.WriteString(`<span class="sr-only">Close</span>&times;</button></div>`)
	buf.WriteString(`<form hx-post="/api/v1/reservations" hx-target="#modal" hx-swap="innerHTML" class="space-y-4">`)
	buf.WriteString(fmt.Sprintf(`<input type="hidden" name="facility_id" value="%d"/>`, facilityID))

	buf.WriteString(`<div>`)
	buf.WriteString(`<label class="block text-sm font-medium text-gray-700">Reservation type</label>`)
	buf.WriteString(`<select name="reservation_type_id" required class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2">`)
	for _, resType := range reservationTypes {
		buf.WriteString(fmt.Sprintf(`<option value="%d">%s</option>`, resType.ID, html.EscapeString(resType.Name)))
	}
	buf.WriteString(`</select></div>`)

	buf.WriteString(`<div>`)
	buf.WriteString(`<label class="block text-sm font-medium text-gray-700">Court</label>`)
	buf.WriteString(`<select name="court_ids" required class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2">`)
	for _, court := range courtsList {
		selected := ""
		if selectedCourtID != 0 && court.ID == selectedCourtID {
			selected = " selected"
		}
		buf.WriteString(fmt.Sprintf(`<option value="%d"%s>Court %d</option>`, court.ID, selected, court.CourtNumber))
	}
	buf.WriteString(`</select></div>`)

	buf.WriteString(`<div class="grid grid-cols-2 gap-4">`)
	buf.WriteString(`<div><label class="block text-sm font-medium text-gray-700">Start time</label>`)
	buf.WriteString(fmt.Sprintf(`<input type="datetime-local" name="start_time" required value="%s" class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2"/>`, startTime.Format("2006-01-02T15:04")))
	buf.WriteString(`</div>`)
	buf.WriteString(`<div><label class="block text-sm font-medium text-gray-700">End time</label>`)
	buf.WriteString(fmt.Sprintf(`<input type="datetime-local" name="end_time" required value="%s" class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2"/>`, endTime.Format("2006-01-02T15:04")))
	buf.WriteString(`</div></div>`)

	buf.WriteString(`<div class="grid grid-cols-2 gap-4">`)
	buf.WriteString(`<div><label class="block text-sm font-medium text-gray-700">Teams per court</label>`)
	buf.WriteString(`<input type="number" name="teams_per_court" min="1" class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2"/>`)
	buf.WriteString(`</div>`)
	buf.WriteString(`<div><label class="block text-sm font-medium text-gray-700">People per team</label>`)
	buf.WriteString(`<input type="number" name="people_per_team" min="1" class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2"/>`)
	buf.WriteString(`</div></div>`)

	buf.WriteString(`<label class="flex items-center space-x-2 text-sm font-medium text-gray-700">`)
	buf.WriteString(`<input type="checkbox" name="is_open_event" class="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"/>`)
	buf.WriteString(`<span>Open for sign-ups</span></label>`)

	buf.WriteString(`<div class="flex justify-end space-x-3 pt-2">`)
	buf.WriteString(`<button type="button" class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md shadow-sm hover:bg-gray-50" onclick="document.getElementById('modal').innerHTML=''">Cancel</button>`)
	buf.WriteString(`<button type="submit" class="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md shadow-sm hover:bg-blue-700">Create</button>`)
	buf.WriteString(`</div></form></div></div>`)

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
