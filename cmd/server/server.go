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
	"github.com/codr1/Pickleicious/internal/api/auth"
	"github.com/codr1/Pickleicious/internal/api/courts"
	"github.com/codr1/Pickleicious/internal/api/members"
	"github.com/codr1/Pickleicious/internal/api/nav"
	openplayapi "github.com/codr1/Pickleicious/internal/api/openplay"
	"github.com/codr1/Pickleicious/internal/api/themes"
	"github.com/codr1/Pickleicious/internal/config"
	"github.com/codr1/Pickleicious/internal/db"
	"github.com/codr1/Pickleicious/internal/models"
	openplayengine "github.com/codr1/Pickleicious/internal/openplay"
	"github.com/codr1/Pickleicious/internal/request"
	"github.com/codr1/Pickleicious/internal/scheduler"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

func newServer(config *config.Config, database *db.DB) (*http.Server, error) {
	router := http.NewServeMux()

	// Setup middleware chain
	handler := api.ChainMiddleware(
		router,
		api.WithLogging,
		api.WithRecovery,
		api.WithRequestID,
		api.WithAuth,
		api.WithContentType,
	)

	auth.InitHandlers(database.Queries, config)
	openplayapi.InitHandlers(database)
	themes.InitHandlers(database.Queries)
	courts.InitHandlers(database.Queries)
	if err := scheduler.Init(); err != nil {
		return nil, fmt.Errorf("initialize scheduler: %w", err)
	}

	openplayEngine, err := openplayengine.NewEngine(database)
	if err != nil {
		return nil, fmt.Errorf("initialize open play engine: %w", err)
	}
	if err := registerOpenPlayEnforcementJob(config, database, openplayEngine); err != nil {
		return nil, fmt.Errorf("register open play enforcement job: %w", err)
	}

	// Register routes
	registerRoutes(router, database)

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", config.App.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}, nil
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

	// Auth routes
	mux.HandleFunc("/login", auth.HandleLoginPage)
	mux.HandleFunc("/api/v1/auth/check-staff", auth.HandleCheckStaff)
	mux.HandleFunc("/api/v1/auth/send-code", auth.HandleSendCode)
	mux.HandleFunc("/api/v1/auth/verify-code", auth.HandleVerifyCode)
	mux.HandleFunc("/api/v1/auth/resend-code", auth.HandleResendCode)
	mux.HandleFunc("/api/v1/auth/staff-login", auth.HandleStaffLogin)
	mux.HandleFunc("/api/v1/auth/reset-password", auth.HandleResetPassword)
	mux.HandleFunc("/api/v1/auth/standard-login", auth.HandleStandardLogin)

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
	mux.HandleFunc("/open-play-rules", openplayapi.HandleOpenPlayRulesPage)
	mux.HandleFunc("/api/v1/open-play-rules", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  openplayapi.HandleOpenPlayRulesList,
		http.MethodPost: openplayapi.HandleOpenPlayRuleCreate,
	}))
	mux.HandleFunc("/api/v1/open-play-rules/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		openplayapi.HandleOpenPlayRuleNew(w, r)
	})
	mux.HandleFunc("/api/v1/open-play-rules/{id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:    openplayapi.HandleOpenPlayRuleDetail,
		http.MethodPut:    openplayapi.HandleOpenPlayRuleUpdate,
		http.MethodDelete: openplayapi.HandleOpenPlayRuleDelete,
	}))
	mux.HandleFunc("/api/v1/open-play-rules/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		openplayapi.HandleOpenPlayRuleEdit(w, r)
	})
	mux.HandleFunc("/api/v1/open-play-sessions/{id}/participants", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:    openplayapi.HandleListParticipants,
		http.MethodPost:   openplayapi.HandleAddParticipant,
		http.MethodDelete: openplayapi.HandleRemoveParticipant,
	}))
	mux.HandleFunc("/api/v1/open-play-sessions/{id}/participants/{user_id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodDelete: openplayapi.HandleRemoveParticipant,
	}))
	mux.HandleFunc("/api/v1/open-play-sessions/{id}/auto-scale", methodHandler(map[string]http.HandlerFunc{
		http.MethodPut: openplayapi.HandleOpenPlaySessionAutoScaleToggle,
	}))

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

func registerOpenPlayEnforcementJob(cfg *config.Config, database *db.DB, engine *openplayengine.Engine) error {
	if cfg == nil || database == nil || engine == nil {
		return fmt.Errorf("open play enforcement job requires config, database, and engine")
	}

	jobName := "openplay_enforcement"
	cronExpr := cfg.OpenPlay.EnforcementInterval
	jobLogger := log.With().
		Str("component", "openplay_enforcement_job").
		Str("job_name", jobName).
		Str("cron", cronExpr).
		Logger()

	_, err := scheduler.AddJob(jobName, cronExpr, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		ctx = jobLogger.WithContext(ctx)

		comparisonTime := time.Now()
		facilityIDs, err := listOpenPlayEnforcementFacilities(ctx, database, comparisonTime)
		if err != nil {
			jobLogger.Error().Err(err).Msg("Failed to list facilities for open play enforcement")
			return
		}
		if len(facilityIDs) == 0 {
			jobLogger.Debug().Msg("No facilities with scheduled open play sessions")
			return
		}

		for _, facilityID := range facilityIDs {
			facilityLogger := jobLogger.With().Int64("facility_id", facilityID).Logger()
			facilityCtx := facilityLogger.WithContext(ctx)
			if err := engine.EvaluateSessionsApproachingCutoff(facilityCtx, facilityID, comparisonTime); err != nil {
				facilityLogger.Error().Err(err).Msg("Open play enforcement run failed")
				continue
			}
			facilityLogger.Debug().Msg("Open play enforcement run completed")
		}
	})
	if err != nil {
		return fmt.Errorf("add open play enforcement job: %w", err)
	}

	jobLogger.Info().Msg("Open play enforcement job registered")
	return nil
}

func listOpenPlayEnforcementFacilities(ctx context.Context, database *db.DB, comparisonTime time.Time) ([]int64, error) {
	facilityIDs, err := database.Queries.ListDistinctFacilitiesWithScheduledSessions(ctx, comparisonTime)
	if err != nil {
		return nil, fmt.Errorf("query open play facilities: %w", err)
	}
	return facilityIDs, nil
}
