package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"

	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/cognito"
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
var newCognitoClient = func(cfg dbgen.CognitoConfig) (CognitoAuthClient, error) {
	return cognito.NewClient(cfg)
}

type CognitoAuthClient interface {
	InitiateCustomAuth(ctx context.Context, username string, authMethod string) (*cognitoidentityprovider.InitiateAuthOutput, error)
	RespondToAuthChallenge(ctx context.Context, session string, username string, code string) (*cognitoidentityprovider.RespondToAuthChallengeOutput, error)
	ForgotPassword(ctx context.Context, username string, authMethod string) (*cognitoidentityprovider.ForgotPasswordOutput, error)
	ConfirmForgotPassword(ctx context.Context, username string, code string, newPassword string) (*cognitoidentityprovider.ConfirmForgotPasswordOutput, error)
}

// Used to mask timing differences when a user record does not exist.
const dummyPasswordHash = "$2a$10$6bhr8BjYp8rXJejsIExR7uOrcalHplR0RnnoSJk5mZXv5fNru2udi"

// Dev mode bypass constants - only active when environment == "development"
const (
	devBypassCode    = "123456"      // OTP code that bypasses Cognito in dev mode
	devBypassSession = "dev-session" // Fake session token for dev mode
	devEnvironment   = "development"
)

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

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	organizationID := r.FormValue("organization_id")
	component := authtempl.LoginPage(organizationID)
	if err := component.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render login page")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleMemberLoginPage(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	organizationID := r.FormValue("organization_id")
	component := authtempl.MemberLoginPage(organizationID)
	if err := component.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render member login page")
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
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identifier := r.FormValue("identifier")
	organizationIDStr := r.FormValue("organization_id")

	if identifier == "" || organizationIDStr == "" {
		http.Error(w, "Identifier and organization are required", http.StatusBadRequest)
		return
	}

	if limiter != nil && !limiter.Allow() {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	organizationID, err := strconv.ParseInt(organizationIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid organization ID", http.StatusBadRequest)
		return
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		logger.Error().Err(err).Msg("Failed to load user for auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !user.IsStaff {
		writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
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
	client, err := newCognitoClient(cognitoConfig)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to initialize Cognito client")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	authMethod := ""
	if user.PreferredAuthMethod.Valid {
		authMethod = user.PreferredAuthMethod.String
	}

	authResponse, err := client.InitiateCustomAuth(r.Context(), identifier, authMethod)
	if err != nil {
		if handleCognitoAuthError(w, r, err, "Invalid credentials", "Verification code expired") {
			return
		}
		logger.Error().Err(err).Msg("Failed to initiate Cognito auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	session := ""
	if authResponse != nil && authResponse.Session != nil {
		session = *authResponse.Session
	}

	component := authtempl.CodeVerification(identifier, organizationIDStr, session)
	err = component.Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render verification screen")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleVerifyCode(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	code := r.FormValue("code")
	identifier := r.FormValue("identifier")
	organizationIDStr := r.FormValue("organization_id")
	session := r.FormValue("session")

	if code == "" || identifier == "" || organizationIDStr == "" || session == "" {
		http.Error(w, "Code, identifier, organization, and session are required", http.StatusBadRequest)
		return
	}

	if limiter != nil && !limiter.Allow() {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	organizationID, err := strconv.ParseInt(organizationIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid organization ID", http.StatusBadRequest)
		return
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		logger.Error().Err(err).Msg("Failed to load user for auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !user.IsStaff {
		writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
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
	client, err := newCognitoClient(cognitoConfig)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to initialize Cognito client")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	authResponse, err := client.RespondToAuthChallenge(r.Context(), session, identifier, code)
	if err != nil {
		if handleCognitoAuthError(w, r, err, "Invalid verification code", "Verification code expired") {
			return
		}
		logger.Error().Err(err).Msg("Failed to respond to Cognito challenge")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if authResponse == nil || authResponse.AuthenticationResult == nil || authResponse.ChallengeName != "" {
		writeHTMXError(w, r, http.StatusUnauthorized, "Additional verification required")
		return
	}

	// Update user's Cognito status if needed
	err = queries.UpdateUserCognitoStatus(r.Context(), dbgen.UpdateUserCognitoStatusParams{
		ID:            user.ID,
		CognitoStatus: sql.NullString{String: "CONFIRMED", Valid: true},
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to update Cognito status")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var homeFacilityID *int64
	if user.HomeFacilityID.Valid {
		id := user.HomeFacilityID.Int64
		homeFacilityID = &id
	}

	authUser := &authz.AuthUser{
		ID:              user.ID,
		IsStaff:         user.IsStaff,
		SessionType:     sessionTypeFromStaff(user.IsStaff),
		HomeFacilityID:  homeFacilityID,
		MembershipLevel: user.MembershipLevel,
	}

	if err := SetAuthCookie(w, r, authUser); err != nil {
		logger.Error().Err(err).Msg("Failed to set auth session")
		http.Error(w, "Failed to start session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

// isDevMode returns true if running in development environment
func isDevMode() bool {
	return appConfig != nil && appConfig.App.Environment == devEnvironment
}

// setMemberSession creates auth session for a member and sends redirect response.
// On error, writes error response directly.
func setMemberSession(w http.ResponseWriter, r *http.Request, user dbgen.User) {
	logger := log.Ctx(r.Context())

	var homeFacilityID *int64
	if user.HomeFacilityID.Valid {
		id := user.HomeFacilityID.Int64
		homeFacilityID = &id
	}

	authUser := &authz.AuthUser{
		ID:              user.ID,
		IsStaff:         false,
		SessionType:     SessionTypeMember,
		HomeFacilityID:  homeFacilityID,
		MembershipLevel: user.MembershipLevel,
	}

	if err := SetAuthCookie(w, r, authUser); err != nil {
		logger.Error().Err(err).Msg("Failed to set member auth session")
		http.Error(w, "Failed to start session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/member")
	w.WriteHeader(http.StatusOK)
}

func HandleMemberSendCode(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identifier := r.FormValue("identifier")
	organizationIDStr := r.FormValue("organization_id")

	if identifier == "" || organizationIDStr == "" {
		http.Error(w, "Identifier and organization are required", http.StatusBadRequest)
		return
	}

	if limiter != nil && !limiter.Allow() {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	organizationID, err := strconv.ParseInt(organizationIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid organization ID", http.StatusBadRequest)
		return
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		logger.Error().Err(err).Msg("Failed to load user for auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !user.IsMember {
		writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Dev mode bypass - skip Cognito, return fake session for code entry
	if isDevMode() {
		logger.Warn().Str("identifier", identifier).Msg("Dev mode: skipping Cognito send-code")
		component := authtempl.MemberCodeVerification(identifier, organizationIDStr, devBypassSession)
		if err := component.Render(r.Context(), w); err != nil {
			logger.Error().Err(err).Msg("Failed to render code verification form")
			http.Error(w, "Failed to render page", http.StatusInternalServerError)
		}
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
	client, err := newCognitoClient(cognitoConfig)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to initialize Cognito client")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	authMethod := ""
	if user.PreferredAuthMethod.Valid {
		authMethod = user.PreferredAuthMethod.String
	}

	authResponse, err := client.InitiateCustomAuth(r.Context(), identifier, authMethod)
	if err != nil {
		if handleCognitoAuthError(w, r, err, "Invalid credentials", "Verification code expired") {
			return
		}
		logger.Error().Err(err).Msg("Failed to initiate Cognito auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	session := ""
	if authResponse != nil && authResponse.Session != nil {
		session = *authResponse.Session
	}

	component := authtempl.MemberCodeVerification(identifier, organizationIDStr, session)
	err = component.Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render member verification screen")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleMemberVerifyCode(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	code := r.FormValue("code")
	identifier := r.FormValue("identifier")
	organizationIDStr := r.FormValue("organization_id")
	session := r.FormValue("session")

	if code == "" || identifier == "" || organizationIDStr == "" || session == "" {
		http.Error(w, "Code, identifier, organization, and session are required", http.StatusBadRequest)
		return
	}

	if limiter != nil && !limiter.Allow() {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	organizationID, err := strconv.ParseInt(organizationIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid organization ID", http.StatusBadRequest)
		return
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		logger.Error().Err(err).Msg("Failed to load user for auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !user.IsMember {
		writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Dev mode bypass - accept devBypassCode as valid
	if isDevMode() && code == devBypassCode {
		logger.Warn().Str("identifier", identifier).Msg("Dev mode: bypassing Cognito verify-code")
		setMemberSession(w, r, user)
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
	client, err := newCognitoClient(cognitoConfig)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to initialize Cognito client")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	authResponse, err := client.RespondToAuthChallenge(r.Context(), session, identifier, code)
	if err != nil {
		if handleCognitoAuthError(w, r, err, "Invalid verification code", "Verification code expired") {
			return
		}
		logger.Error().Err(err).Msg("Failed to respond to Cognito challenge")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if authResponse == nil || authResponse.AuthenticationResult == nil || authResponse.ChallengeName != "" {
		writeHTMXError(w, r, http.StatusUnauthorized, "Additional verification required")
		return
	}

	// Update user's Cognito status if needed
	err = queries.UpdateUserCognitoStatus(r.Context(), dbgen.UpdateUserCognitoStatusParams{
		ID:            user.ID,
		CognitoStatus: sql.NullString{String: "CONFIRMED", Valid: true},
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to update Cognito status")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	setMemberSession(w, r, user)
}

func HandleStaffLogin(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method == http.MethodGet {
		organizationID := r.FormValue("organization_id")
		component := authtempl.StaffLoginForm(organizationID)
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
	if r.Method == http.MethodGet {
		identifier := r.FormValue("identifier")
		organizationID := r.FormValue("organization_id")
		component := authtempl.ResetPasswordForm(identifier, organizationID)
		err := component.Render(r.Context(), w)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to render reset password form")
			http.Error(w, "Failed to render page", http.StatusInternalServerError)
		}
		return
	}

	if limiter != nil && !limiter.Allow() {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

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

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	cognitoConfig, err := queries.GetCognitoConfig(r.Context(), organizationID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get Cognito config")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	client, err := newCognitoClient(cognitoConfig)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to initialize Cognito client")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	authMethod := ""
	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logger.Error().Err(err).Msg("Failed to load user for password reset")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err == nil && user.PreferredAuthMethod.Valid {
		authMethod = user.PreferredAuthMethod.String
	}

	_, err = client.ForgotPassword(r.Context(), identifier, authMethod)
	if err != nil {
		var userNotFound *types.UserNotFoundException
		if errors.As(err, &userNotFound) {
			err = nil
		} else if handleCognitoAuthError(w, r, err, "Unable to send reset code", "Reset request expired") {
			return
		} else {
			logger.Error().Err(err).Msg("Failed to send password reset code")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	component := authtempl.ResetPasswordConfirmation(identifier, organizationIDStr)
	err = component.Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render password reset confirmation form")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleConfirmResetPassword(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	code := r.FormValue("code")
	newPassword := r.FormValue("new_password")
	identifier := r.FormValue("identifier")
	organizationIDStr := r.FormValue("organization_id")

	if code == "" || newPassword == "" || identifier == "" || organizationIDStr == "" {
		http.Error(w, "Code, new password, identifier, and organization are required", http.StatusBadRequest)
		return
	}

	if limiter != nil && !limiter.Allow() {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	organizationID, err := strconv.ParseInt(organizationIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid organization ID", http.StatusBadRequest)
		return
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	cognitoConfig, err := queries.GetCognitoConfig(r.Context(), organizationID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get Cognito config")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	client, err := newCognitoClient(cognitoConfig)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to initialize Cognito client")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	_, err = client.ConfirmForgotPassword(r.Context(), identifier, code, newPassword)
	if err != nil {
		var userNotFound *types.UserNotFoundException
		if errors.As(err, &userNotFound) {
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid verification code")
			return
		}
		if handleCognitoAuthError(w, r, err, "Invalid verification code", "Verification code expired") {
			return
		}
		var invalidPassword *types.InvalidPasswordException
		if errors.As(err, &invalidPassword) {
			writeHTMXError(w, r, http.StatusBadRequest, "Password does not meet requirements")
			return
		}
		logger.Error().Err(err).Msg("Failed to confirm password reset")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", fmt.Sprintf("/login?organization_id=%s", organizationIDStr))
	w.WriteHeader(http.StatusOK)
}

func HandleResendCode(w http.ResponseWriter, r *http.Request) {
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

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		logger.Error().Err(err).Msg("Failed to load user for auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	cognitoConfig, err := queries.GetCognitoConfig(r.Context(), organizationID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get Cognito config")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	client, err := newCognitoClient(cognitoConfig)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to initialize Cognito client")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	authMethod := ""
	if user.PreferredAuthMethod.Valid {
		authMethod = user.PreferredAuthMethod.String
	}

	authResponse, err := client.InitiateCustomAuth(r.Context(), identifier, authMethod)
	if err != nil {
		if handleCognitoAuthError(w, r, err, "Invalid credentials", "Verification code expired") {
			return
		}
		logger.Error().Err(err).Msg("Failed to resend Cognito auth code")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	session := ""
	if authResponse != nil && authResponse.Session != nil {
		session = *authResponse.Session
	}

	component := authtempl.CodeVerification(identifier, organizationIDStr, session)
	err = component.Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render verification screen")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleStandardLogin(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	organizationID := r.FormValue("organization_id")
	component := authtempl.LoginLayout(organizationID)
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
		ID:              0,
		IsStaff:         true,
		SessionType:     SessionTypeStaff,
		HomeFacilityID:  homeFacilityID,
		MembershipLevel: 0,
	}, true
}

func getUserByIdentifier(ctx context.Context, identifier string) (dbgen.User, error) {
	if strings.Contains(identifier, "@") {
		return queries.GetUserByEmail(ctx, sql.NullString{String: identifier, Valid: true})
	}
	return queries.GetUserByPhone(ctx, sql.NullString{String: identifier, Valid: true})
}

func handleCognitoAuthError(w http.ResponseWriter, r *http.Request, err error, unauthorizedMsg string, expiredMsg string) bool {
	switch {
	case errors.Is(err, cognito.ErrCognitoThrottled):
		writeHTMXError(w, r, http.StatusTooManyRequests, "Too many requests. Please try again in a few minutes.")
		return true
	case errors.Is(err, cognito.ErrCognitoNotAuthorized), errors.Is(err, cognito.ErrCognitoCodeMismatch):
		writeHTMXError(w, r, http.StatusUnauthorized, unauthorizedMsg)
		return true
	case errors.Is(err, cognito.ErrCognitoExpiredCode):
		writeHTMXError(w, r, http.StatusForbidden, expiredMsg)
		return true
	default:
		return false
	}
}

func writeHTMXError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if strings.EqualFold(r.Header.Get("HX-Request"), "true") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("HX-Retarget", "#auth-error")
		w.Header().Set("HX-Reswap", "innerHTML")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(fmt.Sprintf(
			`<div class="mt-4 rounded-md bg-red-50 p-3 text-sm text-red-700">%s</div>`,
			html.EscapeString(message),
		)))
		return
	}
	http.Error(w, message, status)
}
