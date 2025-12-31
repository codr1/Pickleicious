// cmd/server/server.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api"
	"github.com/codr1/Pickleicious/internal/api/courts"
	"github.com/codr1/Pickleicious/internal/api/members"
	"github.com/codr1/Pickleicious/internal/api/nav"
	"github.com/codr1/Pickleicious/internal/api/openplay"
	"github.com/codr1/Pickleicious/internal/api/themes"
	"github.com/codr1/Pickleicious/internal/config"
	"github.com/codr1/Pickleicious/internal/db"
	"github.com/codr1/Pickleicious/internal/models"
	"github.com/codr1/Pickleicious/internal/request"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

func newServer(config *config.Config, database *db.DB) *http.Server {
	router := http.NewServeMux()

	// Setup middleware chain
	handler := api.ChainMiddleware(
		router,
		api.WithLogging,
		api.WithRecovery,
		api.WithRequestID,
		api.WithContentType,
	)

	openplay.InitHandlers(database.Queries)
	themes.InitHandlers(database.Queries)
	courts.InitHandlers(database.Queries)

	// Register routes
	registerRoutes(router, database)

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", config.App.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func methodHandler(handlers map[string]http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handler, ok := handlers[r.Method]
		if !ok {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handler(w, r)
	}
}

func registerRoutes(mux *http.ServeMux, database *db.DB) {
	// Main page handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		var activeTheme *models.Theme
		if facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id")); ok {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()

			var err error
			activeTheme, err = models.GetActiveTheme(ctx, database.Queries, facilityID)
			if err != nil {
				log.Ctx(r.Context()).
					Error().
					Err(err).
					Int64("facility_id", facilityID).
					Msg("Failed to load active theme")
				activeTheme = nil
			}
		}
		component := layouts.Base(nil, activeTheme)
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
	mux.HandleFunc("/api/v1/members", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  members.HandleMembersList,
		http.MethodPost: members.HandleCreateMember,
	}))
	mux.HandleFunc("/api/v1/members/search", members.HandleMemberSearch)
	mux.HandleFunc("/api/v1/members/new", members.HandleNewMemberForm)
	mux.HandleFunc("/api/v1/members/billing", members.HandleMemberBilling)

	// Photo endpoint
	mux.HandleFunc("/api/v1/members/photo/", members.HandleMemberPhoto)

	// Member detail routes
	mux.HandleFunc("/api/v1/members/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")

		// Check for billing path first
		if strings.HasSuffix(path, "/billing") {
			members.HandleMemberBilling(w, r)
			return
		}

		// Handle other member routes
		switch r.Method {
		case http.MethodGet:
			if strings.HasSuffix(path, "/edit") {
				members.HandleEditMemberForm(w, r)
			} else {
				members.HandleMemberDetail(w, r)
			}
		case http.MethodPut:
			members.HandleUpdateMember(w, r)
		case http.MethodDelete:
			members.HandleDeleteMember(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Court routes
	mux.HandleFunc("/courts", courts.HandleCourtsPage)
	mux.HandleFunc("/api/v1/courts/calendar", courts.HandleCalendarView)

	// Open play rules
	mux.HandleFunc("/open-play-rules", openplay.HandleOpenPlayRulesPage)
	mux.HandleFunc("/api/v1/open-play-rules", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  openplay.HandleOpenPlayRulesList,
		http.MethodPost: openplay.HandleOpenPlayRuleCreate,
	}))
	mux.HandleFunc("/api/v1/open-play-rules/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		openplay.HandleOpenPlayRuleNew(w, r)
	})
	mux.HandleFunc("/api/v1/open-play-rules/{id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:    openplay.HandleOpenPlayRuleDetail,
		http.MethodPut:    openplay.HandleOpenPlayRuleUpdate,
		http.MethodDelete: openplay.HandleOpenPlayRuleDelete,
	}))
	mux.HandleFunc("/api/v1/open-play-rules/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		openplay.HandleOpenPlayRuleEdit(w, r)
	})

	// Theme admin page
	mux.HandleFunc("/admin/themes", themes.HandleThemesPage)

	// Theme API
	mux.HandleFunc("/api/v1/themes", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  themes.HandleThemesList,
		http.MethodPost: themes.HandleThemeCreate,
	}))
	mux.HandleFunc("/api/v1/themes/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		themes.HandleThemeNew(w, r)
	})
	mux.HandleFunc("/api/v1/themes/{id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:    themes.HandleThemeDetail,
		http.MethodPut:    themes.HandleThemeUpdate,
		http.MethodDelete: themes.HandleThemeDelete,
	}))
	mux.HandleFunc("/api/v1/themes/{id}/clone", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: themes.HandleThemeClone,
	}))
	mux.HandleFunc("/api/v1/facilities/{id}/theme", methodHandler(map[string]http.HandlerFunc{
		http.MethodPut: themes.HandleFacilityThemeSet,
	}))

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

	// Add this new route for member restoration
	mux.HandleFunc("/api/v1/members/restore", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: members.HandleRestoreDecision,
	}))
}
