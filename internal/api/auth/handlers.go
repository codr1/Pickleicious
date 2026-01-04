package auth

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"

	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/config"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/request"
	authtempl "github.com/codr1/Pickleicious/internal/templates/components/auth"
)

type CognitoConfig struct {
	PoolID       string
	ClientID     string
	ClientSecret string
	Domain       string
	CallbackURL  string
}

var queries *dbgen.Queries
var limiter *rate.Limiter
var appConfig *config.Config

// Used to mask timing differences when a user record does not exist.
const dummyPasswordHash = "$2a$10$6bhr8BjYp8rXJejsIExR7uOrcalHplR0RnnoSJk5mZXv5fNru2udi"

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(q *dbgen.Queries, cfg *config.Config) {
	if q == nil || cfg == nil {
		log.Error().Msg("Auth handlers init failed: missing database queries or config")
		panic("auth handlers init failed: missing dependencies")
	}
	queries = q
	appConfig = cfg
	limiter = rate.NewLimiter(rate.Limit(100), 10) // More restrictive for auth
}

func HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	component := authtempl.LoginLayout()
	err := component.Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render login page")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleCheckStaff(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	identifier := r.FormValue("identifier")

	if identifier == "" {
		http.Error(w, "Identifier is required", http.StatusBadRequest)
		return
	}

	// Check if email or phone - auth info is on User, not Staff
	isEmail := strings.Contains(identifier, "@")
	var user dbgen.User
	var err error

	if isEmail {
		user, err = queries.GetUserByEmail(r.Context(), sql.NullString{String: identifier, Valid: true})
	} else {
		user, err = queries.GetUserByPhone(r.Context(), sql.NullString{String: identifier, Valid: true})
	}

	if err != nil && err != sql.ErrNoRows {
		logger.Error().Err(err).Msg("Database error checking user")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// If user found and local auth is enabled, show staff section
	if err == nil && user.LocalAuthEnabled {
		component := authtempl.StaffAuthSection()
		component.Render(r.Context(), w)
		return
	}

	// Otherwise render empty response
	w.Write([]byte(""))
}

func HandleSendCode(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	identifier := r.FormValue("identifier")
	organizationIDStr := r.FormValue("organization_id")

	if identifier == "" || organizationIDStr == "" {
		http.Error(w, "Identifier and organization are required", http.StatusBadRequest)
		return
	}

	organizationID, err := strconv.ParseInt(organizationIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid organization ID", http.StatusBadRequest)
		return
	}

	// Get Cognito config for this organization
	cognitoConfig, err := queries.GetCognitoConfig(r.Context(), organizationID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get Cognito config")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Initialize Cognito client with organization-specific config
	// TODO: Implement Cognito client initialization with cognitoConfig
	_ = cognitoConfig // Suppress unused warning until TODO is implemented

	// Send verification code via Cognito
	// TODO: Implement Cognito code sending

	component := authtempl.CodeVerification()
	err = component.Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render verification screen")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleVerifyCode(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	code := r.FormValue("code")
	identifier := r.FormValue("identifier")
	organizationIDStr := r.FormValue("organization_id")

	if code == "" || identifier == "" || organizationIDStr == "" {
		http.Error(w, "Code, identifier, and organization are required", http.StatusBadRequest)
		return
	}

	organizationID, err := strconv.ParseInt(organizationIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid organization ID", http.StatusBadRequest)
		return
	}

	// Get Cognito config for this organization
	cognitoConfig, err := queries.GetCognitoConfig(r.Context(), organizationID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get Cognito config")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Verify code with Cognito
	// TODO: Implement Cognito verification with cognitoConfig
	_ = cognitoConfig // Suppress unused warning until TODO is implemented

	// Update user's Cognito status if needed
	// TODO: Update cognito_status in database

	// TODO: Set up session/JWT
}

func HandleStaffLogin(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method == http.MethodGet {
		component := authtempl.StaffLoginForm()
		err := component.Render(r.Context(), w)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to render staff login form")
			http.Error(w, "Failed to render page", http.StatusInternalServerError)
		}
		return
	}

	identifier := r.FormValue("identifier")
	password := r.FormValue("password")

	if identifier == "" || password == "" {
		http.Error(w, "Identifier and password are required", http.StatusBadRequest)
		return
	}

	if limiter != nil && !limiter.Allow() {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	if devUser, ok := devModeBypass(r, identifier, password); ok {
		if err := SetAuthCookie(w, r, devUser); err != nil {
			logger.Error().Err(err).Msg("Failed to set dev auth session")
			http.Error(w, "Failed to start session", http.StatusInternalServerError)
			return
		}
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
		return
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	isEmail := strings.Contains(identifier, "@")
	var user dbgen.User
	var err error

	if isEmail {
		user, err = queries.GetUserByEmail(r.Context(), sql.NullString{String: identifier, Valid: true})
	} else {
		user, err = queries.GetUserByPhone(r.Context(), sql.NullString{String: identifier, Valid: true})
	}

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = VerifyPassword(dummyPasswordHash, password)
		} else {
			logger.Error().Err(err).Msg("Database error during staff login")
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if !user.IsStaff || !user.LocalAuthEnabled || !user.PasswordHash.Valid || !VerifyPassword(user.PasswordHash.String, password) {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := CreateSession(w, user.ID); err != nil {
		logger.Error().Err(err).Msg("Failed to create auth session")
		http.Error(w, "Failed to start session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	// TODO: Implement password reset flow
	logger.Error().Msg("Password reset not implemented")
	http.Error(w, "Password reset not implemented", http.StatusNotImplemented)
}

func HandleResendCode(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	identifier := r.FormValue("identifier")

	if identifier == "" {
		http.Error(w, "Identifier is required", http.StatusBadRequest)
		return
	}

	// TODO: Implement Cognito code resending
	component := authtempl.CodeVerification()
	err := component.Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render verification screen")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleStandardLogin(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	component := authtempl.LoginLayout()
	err := component.Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render login page")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func devModeBypass(r *http.Request, identifier, password string) (*authz.AuthUser, bool) {
	if appConfig == nil || appConfig.App.Environment != "development" {
		return nil, false
	}
	if !strings.EqualFold(strings.TrimSpace(identifier), "dev@test.local") || password != "devpass" {
		return nil, false
	}

	var homeFacilityID *int64
	if facilityID, ok := request.ParseFacilityID(r.FormValue("facility_id")); ok {
		homeFacilityID = &facilityID
	} else if facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id")); ok {
		homeFacilityID = &facilityID
	}

	log.Ctx(r.Context()).
		Warn().
		Str("identifier", identifier).
		Msg("Dev mode staff login bypass used")

	return &authz.AuthUser{
		ID:             0,
		IsStaff:        true,
		HomeFacilityID: homeFacilityID,
	}, true
}
