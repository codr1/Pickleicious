// internal/api/courts/handlers.go
package courts

import (
	"context"
	"net/http"
	"sync"
	"time"

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

func loadQueries() *dbgen.Queries {
	return queries
}
