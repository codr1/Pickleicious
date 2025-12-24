// internal/api/nav/handlers.go
package nav

import (
	"database/sql"
	"net/http"
	"strings"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/templates/components/nav"
)

var queries *dbgen.Queries

func InitHandlers(q *dbgen.Queries) {
	queries = q
}

func HandleMenu(w http.ResponseWriter, r *http.Request) {
	component := nav.Menu()
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
