// internal/api/nav/handlers.go
package nav

import (
	"net/http"

	"github.com/codr1/Pickleicious/internal/templates/components/nav"
)

func HandleMenu(w http.ResponseWriter, r *http.Request) {
	component := nav.Menu()
	component.Render(r.Context(), w)
}

func HandleMenuClose(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(""))
}

func HandleSearch(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement search functionality
	w.Write([]byte("Search results will go here"))
}
