// internal/api/nav/handlers.go
package nav

import (
	"database/sql"
	"encoding/json"
	"net/http"

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
	q := r.URL.Query().Get("q")
	if q == "" {
		w.Write([]byte("[]"))
		return
	}

	results, err := queries.SearchMembers(r.Context(), dbgen.SearchMembersParams{
		SearchTerm: sql.NullString{String: q, Valid: true},
		Limit:      10,
	})
	if err != nil {
		http.Error(w, "Search failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
