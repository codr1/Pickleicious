// cmd/server/server.go
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api"
	"github.com/codr1/Pickleicious/internal/api/courts"
	"github.com/codr1/Pickleicious/internal/api/members"
	"github.com/codr1/Pickleicious/internal/api/nav"
	"github.com/codr1/Pickleicious/internal/config"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

func newServer(config *config.Config) *http.Server {
	router := http.NewServeMux()

	// Setup middleware chain
	handler := api.ChainMiddleware(
		router,
		api.WithLogging,
		api.WithRecovery,
		api.WithRequestID,
		api.WithContentType,
	)

	// Register routes
	registerRoutes(router)

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", config.App.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func registerRoutes(mux *http.ServeMux) {
	// Main page handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		component := layouts.Base(nil)
		component.Render(r.Context(), w)
	})

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Navigation routes
	mux.HandleFunc("/api/v1/nav/menu", nav.HandleMenu)
	mux.HandleFunc("/api/v1/nav/menu/close", nav.HandleMenuClose)
	mux.HandleFunc("/api/v1/nav/search", nav.HandleSearch)

	// Member routes
	mux.HandleFunc("/members", members.HandleMembersPage)
	mux.HandleFunc("/api/v1/members", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			members.HandleMembersList(w, r)
		case http.MethodPost:
			members.HandleCreateMember(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/members/search", members.HandleMemberSearch)
	mux.HandleFunc("/api/v1/members/", members.HandleMemberDetail)
	mux.HandleFunc("/api/v1/members/new", members.HandleNewMemberForm)
	mux.HandleFunc("/api/v1/members/billing", members.HandleMemberBilling)

	// Court routes
	mux.HandleFunc("/courts", courts.HandleCourtsPage)
	mux.HandleFunc("/api/v1/courts/calendar", courts.HandleCalendarView)

	// Static file handling with logging and environment awareness
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		// Default to the build directory if not specified
		staticDir = "build/bin/static"
	}
	fs := http.FileServer(http.Dir(staticDir))

	// Add logging middleware for static files
	mux.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info().
			Str("path", r.URL.Path).
			Str("method", r.Method).
			Str("static_dir", staticDir).
			Msg("Static file request")
		http.StripPrefix("/static/", fs).ServeHTTP(w, r)
	}))
}
