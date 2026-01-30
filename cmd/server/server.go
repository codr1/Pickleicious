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
	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/api/cancellationpolicy"
	"github.com/codr1/Pickleicious/internal/api/checkin"
	"github.com/codr1/Pickleicious/internal/api/clinics"
	"github.com/codr1/Pickleicious/internal/api/courts"
	"github.com/codr1/Pickleicious/internal/api/dashboard"
	"github.com/codr1/Pickleicious/internal/api/leagues"
	"github.com/codr1/Pickleicious/internal/api/lessonpacks"
	"github.com/codr1/Pickleicious/internal/api/member"
	"github.com/codr1/Pickleicious/internal/api/members"
	"github.com/codr1/Pickleicious/internal/api/nav"
	"github.com/codr1/Pickleicious/internal/api/notifications"
	openplayapi "github.com/codr1/Pickleicious/internal/api/openplay"
	"github.com/codr1/Pickleicious/internal/api/operatinghours"
	"github.com/codr1/Pickleicious/internal/api/reservations"
	"github.com/codr1/Pickleicious/internal/api/staff"
	"github.com/codr1/Pickleicious/internal/api/themes"
	"github.com/codr1/Pickleicious/internal/api/visitpacks"
	"github.com/codr1/Pickleicious/internal/api/waitlist"
	"github.com/codr1/Pickleicious/internal/cognito"
	"github.com/codr1/Pickleicious/internal/config"
	"github.com/codr1/Pickleicious/internal/db"
	"github.com/codr1/Pickleicious/internal/email"
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
		api.WithOrganization(database.Queries, config.App.BaseDomain),
		api.WithAuth,
		api.WithContentType,
	)

	// Create Cognito client if configured
	var cognitoClient *cognito.CognitoClient
	if config.AWS.CognitoPoolID != "" && config.AWS.CognitoClientID != "" {
		client, err := cognito.NewClient(config.AWS.CognitoPoolID, config.AWS.CognitoClientID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to init Cognito client")
		} else {
			cognitoClient = client
			log.Info().Msg("Cognito client initialized")
		}
	}

	var emailClient *email.SESClient
	if config.AWS.SESAccessKeyID != "" && config.AWS.SESSecretAccessKey != "" && config.AWS.SESRegion != "" && config.AWS.SESSender != "" {
		client, err := email.NewSESClient(config.AWS.SESAccessKeyID, config.AWS.SESSecretAccessKey, config.AWS.SESRegion, config.AWS.SESSender)
		if err != nil {
			log.Error().Err(err).Msg("Failed to init SES client")
		} else {
			emailClient = client
			log.Info().Msg("SES client initialized")
		}
	}

	auth.InitHandlers(database.Queries, config)
	members.InitHandlers(database.Queries, cognitoClient)
	nav.InitHandlers(database.Queries)
	openplayapi.InitHandlers(database, emailClient)
	themes.InitHandlers(database.Queries)
	courts.InitHandlers(database.Queries)
	dashboard.InitHandlers(database)
	checkin.InitHandlers(database.Queries)
	clinics.InitHandlers(database)
	reservations.InitHandlers(database, emailClient)
	member.InitHandlers(database, emailClient)
	operatinghours.InitHandlers(database.Queries)
	notifications.InitHandlers(database.Queries)
	cancellationpolicy.InitHandlers(database.Queries)
	lessonpacks.InitHandlers(database.Queries)
	visitpacks.InitHandlers(database.Queries)
	waitlist.InitHandlers(database)

	staff.InitHandlers(database)
	leagues.InitHandlers(database)

	if err := scheduler.Init(); err != nil {
		return nil, fmt.Errorf("initialize scheduler: %w", err)
	}

	openplayEngine, err := openplayengine.NewEngine(database, emailClient)
	if err != nil {
		return nil, fmt.Errorf("initialize open play engine: %w", err)
	}
	if err := registerOpenPlayEnforcementJob(config, database, openplayEngine); err != nil {
		return nil, fmt.Errorf("register open play enforcement job: %w", err)
	}
	if err := scheduler.RegisterWaitlistJobs(database); err != nil {
		return nil, fmt.Errorf("register waitlist jobs: %w", err)
	}
	if err := scheduler.RegisterReminderJobs(database, emailClient); err != nil {
		return nil, fmt.Errorf("register reminder jobs: %w", err)
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
		// Redirect to login if not authenticated
		if authz.UserFromContext(r.Context()) == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
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
		sessionType := authz.SessionTypeFromContext(r.Context())
		component := layouts.Base(nil, activeTheme, sessionType)
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
	mux.HandleFunc("/api/v1/notifications/count", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: notifications.HandleNotificationCount,
	}))
	mux.HandleFunc("/api/v1/notifications", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: notifications.HandleNotificationsList,
	}))
	mux.HandleFunc("/api/v1/notifications/close", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: notifications.HandleNotificationsClose,
	}))
	mux.HandleFunc("/api/v1/notifications/{id}/read", methodHandler(map[string]http.HandlerFunc{
		http.MethodPut: notifications.HandleMarkAsRead,
	}))
	mux.HandleFunc("/api/v1/waitlist", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  waitlist.HandleWaitlistList,
		http.MethodPost: waitlist.HandleWaitlistJoin,
	}))
	mux.HandleFunc("/api/v1/waitlist/config", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: waitlist.HandleWaitlistConfigUpdate,
	}))
	mux.HandleFunc("/api/v1/waitlist/{id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodDelete: waitlist.HandleWaitlistLeave,
	}))

	// Auth routes (unified for staff and members)
	mux.HandleFunc("/login", auth.HandleLoginPage)
	mux.HandleFunc("/api/v1/auth/check-staff", auth.HandleCheckStaff)
	mux.HandleFunc("/api/v1/auth/send-code", auth.HandleSendCode)
	mux.HandleFunc("/api/v1/auth/verify-code", auth.HandleVerifyCode)
	mux.HandleFunc("/api/v1/auth/resend-code", auth.HandleResendCode)
	mux.HandleFunc("/api/v1/auth/staff-login", auth.HandleStaffLogin)
	mux.HandleFunc("/api/v1/auth/reset-password", auth.HandleResetPassword)
	mux.HandleFunc("/api/v1/auth/confirm-reset-password", auth.HandleConfirmResetPassword)
	mux.HandleFunc("/api/v1/auth/standard-login", auth.HandleStandardLogin)
	mux.HandleFunc("/api/v1/auth/logout", auth.HandleLogout)

	// Member routes
	mux.Handle("/member", member.RequireMemberSession(http.HandlerFunc(member.HandleMemberPortal)))
	mux.Handle("/member/booking/new", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: member.HandleMemberBookingFormNew,
	}))))
	mux.Handle("/member/booking/slots", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: member.HandleMemberBookingSlots,
	}))))
	mux.Handle("/member/reservations", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  member.HandleMemberReservationsPartial,
		http.MethodPost: member.HandleMemberBookingCreate,
	}))))
	mux.Handle("/member/waitlist", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: member.HandleMemberWaitlistList,
	}))))
	mux.Handle("/member/reservations/{id}", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodDelete: member.HandleMemberReservationCancel,
	}))))
	mux.Handle("/member/openplay", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: member.HandleMemberOpenPlayList,
	}))))
	mux.Handle("/member/openplay/{id}", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodPost:   member.HandleMemberOpenPlaySignup,
		http.MethodDelete: member.HandleMemberOpenPlayCancel,
	}))))
	mux.Handle("/member/lessons/pros", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: member.HandleListPros,
	}))))
	mux.Handle("/member/lessons/pros/{id}/slots", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: member.HandleProAvailability,
	}))))
	mux.Handle("/member/lessons/new", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: member.HandleLessonBookingFormNew,
	}))))
	mux.Handle("/member/lessons/slots", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: member.HandleLessonBookingSlots,
	}))))
	mux.Handle("/member/lessons", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: member.HandleLessonBookingCreate,
	}))))
	mux.Handle("/member/clinics", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: member.HandleListAvailableClinics,
	}))))
	mux.Handle("/member/clinics/{id}/enroll", member.RequireMemberSession(http.HandlerFunc(methodHandler(map[string]http.HandlerFunc{
		http.MethodPost:   member.HandleClinicEnroll,
		http.MethodDelete: member.HandleClinicCancel,
	}))))
	mux.Handle("/api/v1/member/reservations/widget", member.RequireMemberSession(http.HandlerFunc(member.HandleMemberReservationsWidget)))
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

		if strings.HasSuffix(path, "/visits") {
			members.HandleMemberVisits(w, r)
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

	// Check-in routes
	mux.HandleFunc("/checkin", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: checkin.HandleCheckinPage,
	}))
	mux.HandleFunc("/api/v1/checkin/search", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: checkin.HandleCheckinSearch,
	}))
	mux.HandleFunc("/api/v1/checkin", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: checkin.HandleCheckin,
	}))
	mux.HandleFunc("/api/v1/checkin/activity", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: checkin.HandleCheckinActivityUpdate,
	}))

	// Staff routes
	mux.HandleFunc("/staff", staff.HandleStaffPage)
	mux.HandleFunc("/staff/notifications/{id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: staff.HandleNotificationDetail,
	}))
	mux.HandleFunc("/staff/unavailability", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  staff.HandleListProUnavailability,
		http.MethodPost: staff.HandleCreateProUnavailability,
	}))
	mux.HandleFunc("/staff/unavailability/", methodHandler(map[string]http.HandlerFunc{
		http.MethodDelete: staff.HandleDeleteProUnavailability,
	}))
	mux.HandleFunc("/api/v1/staff", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  staff.HandleStaffList,
		http.MethodPost: staff.HandleCreateStaff,
	}))
	mux.HandleFunc("/api/v1/staff/lessons/booking/new", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: staff.HandleStaffLessonBookingFormNew,
	}))
	mux.HandleFunc("/api/v1/staff/lessons/booking/slots", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: staff.HandleStaffLessonBookingSlots,
	}))
	mux.HandleFunc("/api/v1/staff/lessons/schedule", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: staff.HandleStaffProScheduleView,
	}))
	mux.HandleFunc("/api/v1/staff/lessons/booking", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: staff.HandleStaffLessonBookingCreate,
	}))
	mux.HandleFunc("/api/v1/staff/lessons", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: staff.HandleStaffLessonCreate,
	}))
	mux.HandleFunc("/api/v1/staff/waitlist", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: waitlist.HandleStaffWaitlistView,
	}))
	mux.HandleFunc("/api/v1/staff/members/search", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: staff.HandleStaffMemberSearch,
	}))
	mux.HandleFunc("/api/v1/staff/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		staff.HandleNewStaffForm(w, r)
	})
	mux.HandleFunc("/api/v1/staff/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")

		// Handle deactivation routes
		if strings.HasSuffix(path, "/deactivate") {
			if r.Method == http.MethodPost {
				staff.HandleDeactivateStaff(w, r)
				return
			}
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if strings.HasSuffix(path, "/deactivation/confirm") {
			if r.Method == http.MethodPost {
				staff.HandleConfirmDeactivation(w, r)
				return
			}
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		switch r.Method {
		case http.MethodGet:
			if strings.HasSuffix(path, "/edit") {
				staff.HandleEditStaffForm(w, r)
			} else {
				staff.HandleStaffDetail(w, r)
			}
		case http.MethodPut:
			staff.HandleUpdateStaff(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// League routes
	mux.HandleFunc("/leagues", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: leagues.HandleLeaguesPage,
	}))
	mux.HandleFunc("/api/v1/leagues", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  leagues.HandleLeaguesList,
		http.MethodPost: leagues.HandleLeagueCreate,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:    leagues.HandleLeagueDetail,
		http.MethodPut:    leagues.HandleLeagueUpdate,
		http.MethodDelete: leagues.HandleLeagueDelete,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}/teams", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  leagues.HandleListLeagueTeams,
		http.MethodPost: leagues.HandleTeamCreate,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}/teams/{team_id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: leagues.HandleTeamDetail,
		http.MethodPut: leagues.HandleTeamUpdate,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}/teams/{team_id}/members", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: leagues.HandleAddTeamMember,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}/teams/{team_id}/members/{user_id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodDelete: leagues.HandleRemoveTeamMember,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}/free-agents", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: leagues.HandleListFreeAgents,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}/free-agents/{user_id}/assign", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: leagues.HandleAssignFreeAgent,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}/schedule/generate", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: leagues.HandleGenerateSchedule,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}/schedule/regenerate", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: leagues.HandleRegenerateSchedule,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}/matches/{match_id}/result", methodHandler(map[string]http.HandlerFunc{
		http.MethodPut: leagues.HandleRecordMatchResult,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}/standings", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: leagues.HandleLeagueStandings,
	}))
	mux.HandleFunc("/api/v1/leagues/{id}/standings/export", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: leagues.HandleExportStandingsCSV,
	}))

	// Clinic routes
	mux.HandleFunc("/api/v1/clinic-types", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  clinics.HandleClinicTypeList,
		http.MethodPost: clinics.HandleClinicTypeCreate,
	}))
	mux.HandleFunc("/api/v1/clinics", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  clinics.HandleClinicSessionList,
		http.MethodPost: clinics.HandleClinicSessionCreate,
	}))
	mux.HandleFunc("/api/v1/clinics/{id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodPut:    clinics.HandleClinicSessionUpdate,
		http.MethodDelete: clinics.HandleClinicCancel,
	}))
	mux.HandleFunc("/api/v1/clinics/{id}/roster", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: clinics.HandleClinicRoster,
	}))

	// Court routes
	mux.HandleFunc("/courts", courts.HandleCourtsPage)
	mux.HandleFunc("/api/v1/courts/calendar", courts.HandleCalendarView)
	mux.HandleFunc("/api/v1/courts/booking/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		courts.HandleBookingFormNew(w, r)
	})
	mux.HandleFunc("/api/v1/events/booking/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		reservations.HandleEventBookingFormNew(w, r)
	})

	// Reservation routes
	mux.HandleFunc("/api/v1/reservations", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  reservations.HandleReservationsList,
		http.MethodPost: reservations.HandleReservationCreate,
	}))
	mux.HandleFunc("/api/v1/reservations/{id}/edit", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: reservations.HandleReservationEdit,
	}))
	mux.HandleFunc("/api/v1/reservations/{id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodPut:    reservations.HandleReservationUpdate,
		http.MethodDelete: reservations.HandleReservationDelete,
	}))

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

	// Dashboard page
	mux.HandleFunc("/admin/dashboard", dashboard.HandleDashboardPage)
	mux.HandleFunc("/api/v1/dashboard/metrics", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: dashboard.HandleDashboardMetrics,
	}))

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

	// Visit pack admin page
	mux.HandleFunc("/admin/visit-packs", visitpacks.HandleVisitPackTypesPage)

	// Lesson package admin page
	mux.HandleFunc("/admin/lesson-packages", lessonpacks.HandleLessonPackageTypesPage)

	// Visit pack API
	mux.HandleFunc("/api/v1/visit-pack-types", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  visitpacks.HandleVisitPackTypesList,
		http.MethodPost: visitpacks.HandleVisitPackTypeCreate,
	}))
	mux.HandleFunc("/api/v1/visit-pack-types/{id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodPut:    visitpacks.HandleVisitPackTypeUpdate,
		http.MethodDelete: visitpacks.HandleVisitPackTypeDeactivate,
	}))
	mux.HandleFunc("/api/v1/visit-packs", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: visitpacks.HandleVisitPackSale,
	}))
	mux.HandleFunc("/api/v1/users/{id}/visit-packs", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: visitpacks.HandleListUserVisitPacks,
	}))

	// Lesson package API
	mux.HandleFunc("/api/v1/lesson-package-types", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  lessonpacks.HandleLessonPackageTypesList,
		http.MethodPost: lessonpacks.HandleLessonPackageTypeCreate,
	}))
	mux.HandleFunc("/api/v1/lesson-package-types/{id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodPut:    lessonpacks.HandleLessonPackageTypeUpdate,
		http.MethodDelete: lessonpacks.HandleLessonPackageTypeDeactivate,
	}))
	mux.HandleFunc("/api/v1/lesson-packages", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: lessonpacks.HandleLessonPackageSale,
	}))
	mux.HandleFunc("/api/v1/users/{id}/lesson-packages", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: lessonpacks.HandleListUserLessonPackages,
	}))

	// Operating hours admin page
	mux.HandleFunc("/admin/operating-hours", operatinghours.HandleOperatingHoursPage)

	// Operating hours API
	mux.HandleFunc("/api/v1/operating-hours/{day_of_week}", methodHandler(map[string]http.HandlerFunc{
		http.MethodPut: operatinghours.HandleOperatingHoursUpdate,
	}))
	mux.HandleFunc("/api/v1/facility-settings", methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: operatinghours.HandleFacilitySettingsUpdate,
	}))

	// Cancellation policy admin page
	mux.HandleFunc("/admin/cancellation-policy", cancellationpolicy.HandleCancellationPolicyPage)

	// Cancellation policy API
	mux.HandleFunc("/api/v1/cancellation-policy/tiers", methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  cancellationpolicy.HandleCancellationPolicyTierList,
		http.MethodPost: cancellationpolicy.HandleCancellationPolicyTierCreate,
	}))
	mux.HandleFunc("/api/v1/cancellation-policy/tiers/{id}", methodHandler(map[string]http.HandlerFunc{
		http.MethodPut:    cancellationpolicy.HandleCancellationPolicyTierUpdate,
		http.MethodDelete: cancellationpolicy.HandleCancellationPolicyTierDelete,
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
