// internal/api/nav/handlers.go
package nav

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/templates/components/nav"
	"github.com/rs/zerolog/log"
)

var queries *dbgen.Queries

func InitHandlers(q *dbgen.Queries) {
	queries = q
}

func HandleMenu(w http.ResponseWriter, r *http.Request) {
	facilityID, _ := facilityIDFromMenuRequest(r)
	component := nav.Menu(facilityID)
	component.Render(r.Context(), w)
}

func HandleMenuClose(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(""))
}

func HandleSearch(w http.ResponseWriter, r *http.Request) {
	searchTerm := strings.TrimSpace(r.URL.Query().Get("q"))
	var err error
	if searchTerm == "" {
		component := nav.SearchResults("", nil)
		err = component.Render(r.Context(), w)
		if err != nil {
			http.Error(w, "Search failed", http.StatusInternalServerError)
		}
		return
	}

	results, err := queries.SearchMembers(r.Context(), dbgen.SearchMembersParams{
		SearchTerm: sql.NullString{String: searchTerm, Valid: true},
		Limit:      10,
	})
	if err != nil {
		http.Error(w, "Search failed", http.StatusInternalServerError)
		return
	}

	component := nav.SearchResults(searchTerm, results)
	err = component.Render(r.Context(), w)
	if err != nil {
		http.Error(w, "Search failed", http.StatusInternalServerError)
		return
	}
}

func facilityIDFromMenuRequest(r *http.Request) (int64, bool) {
	if facilityID, ok := parseFacilityID(r.URL.Query().Get("facility_id")); ok {
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

	return parseFacilityID(parsed.Query().Get("facility_id"))
}

func parseFacilityID(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	facilityID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || facilityID <= 0 {
		return 0, false
	}

	return facilityID, true
}
