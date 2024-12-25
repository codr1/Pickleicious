// internal/api/courts/handlers.go
package courts

import (
	"net/http"

	"github.com/codr1/Pickleicious/internal/templates/components/courts"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
	"github.com/rs/zerolog/log"
)

func HandleCourtsPage(w http.ResponseWriter, r *http.Request) {
	log.Info().
		Str("path", r.URL.Path).
		Str("method", r.Method).
		Msg("Handling courts page request")

	calendar := courts.Calendar()
	page := layouts.Base(calendar)
	page.Render(r.Context(), w)
}

func HandleCalendarView(w http.ResponseWriter, r *http.Request) {
	component := courts.Calendar()
	component.Render(r.Context(), w)
}
