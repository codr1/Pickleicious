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
	"time"

	"github.com/rs/zerolog/log"

	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"

	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/cognito"
	"github.com/codr1/Pickleicious/internal/config"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/ratelimit"
	"github.com/codr1/Pickleicious/internal/request"
	authtempl "github.com/codr1/Pickleicious/internal/templates/components/auth"
)

var queries *dbgen.Queries
var appConfig *config.Config
var cognitoClient *cognito.CognitoClient
var otpLimiter *ratelimit.Limiter
var trustProxy bool

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

	// Initialize OTP rate limiter
	if cfg.RateLimit.Enabled {
		rlCfg := cfg.RateLimit.WithDefaults()
		trustProxy = rlCfg.TrustProxy
		otpLimiter = ratelimit.New(&ratelimit.Config{
			SendCooldown:       time.Duration(rlCfg.OTPSend.CooldownSeconds) * time.Second,
			SendMaxPerHour:     rlCfg.OTPSend.MaxPerHour,
			SendMaxIPPerHour:   rlCfg.OTPSend.MaxPerIPPerHour,
			VerifyMaxAttempts:  rlCfg.OTPVerify.MaxAttempts,
			VerifyLockout:      time.Duration(rlCfg.OTPVerify.LockoutSeconds) * time.Second,
			VerifyMaxIPPerHour: rlCfg.OTPVerify.MaxPerIPPerHour,
		})
		log.Info().
			Bool("trust_proxy", trustProxy).
			Msg("OTP rate limiter initialized")
	} else {
		log.Warn().Msg("OTP rate limiting is DISABLED - endpoints are unprotected from abuse")
	}

	// Initialize global Cognito client if configured
	if cfg.AWS.CognitoPoolID != "" && cfg.AWS.CognitoClientID != "" {
		client, err := cognito.NewClient(cfg.AWS.CognitoPoolID, cfg.AWS.CognitoClientID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize Cognito client")
		} else {
			cognitoClient = client
			log.Info().Msg("Cognito client initialized")
		}
	} else {
		log.Warn().Msg("Cognito not configured - OTP auth will only work in dev mode")
	}
}

// CloseHandlers releases resources. Call on server shutdown.
func CloseHandlers() {
	if otpLimiter != nil {
		otpLimiter.Close()
		otpLimiter = nil
		log.Info().Msg("OTP rate limiter closed")
	}
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

	component := authtempl.LoginPage(authz.OrganizationIDString(r.Context()))
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

	component := authtempl.MemberLoginPage(authz.OrganizationIDString(r.Context()))
	if err := component.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render member login page")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleLogout(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, err := parseAuthCookie(r)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to parse auth cookie for logout")
	}

	// Clear both cookies to cover mixed session states.
	ClearAuthCookie(w)
	ClearSession(w, r)

	if session != nil && session.SessionType == SessionTypeMember {
		w.Header().Set("HX-Redirect", "/member/login")
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("HX-Redirect", "/login")
	w.WriteHeader(http.StatusOK)
}

func HandleCheckStaff(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identifier := r.FormValue("identifier")

	if identifier == "" {
		http.Error(w, "Identifier is required", http.StatusBadRequest)
		return
	}

	// Rate limit by IP to prevent user enumeration
	var clientIP string
	if otpLimiter != nil {
		clientIP = ratelimit.GetClientIP(r, trustProxy)
		result := otpLimiter.CheckOTPSend(identifier, clientIP)
		if !result.Allowed {
			// Silent failure - don't reveal rate limiting to attacker
			_, _ = w.Write([]byte(""))
			return
		}
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logger.Error().Err(err).Msg("Database error checking user")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// If user found and local auth is enabled, show staff section
	if err == nil && user.LocalAuthEnabled {
		// Record the check to prevent enumeration attacks
		if otpLimiter != nil {
			otpLimiter.RecordOTPSend(identifier, clientIP)
		}
		component := authtempl.StaffAuthSection()
		if err := component.Render(r.Context(), w); err != nil {
			logger.Error().Err(err).Msg("Failed to render staff auth section")
		}
		return
	}

	// Otherwise render empty response (don't record - user doesn't exist or no local auth)
	_, _ = w.Write([]byte(""))
}

func HandleSendCode(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identifier := r.FormValue("identifier")

	// Get organization from context (set by subdomain middleware)
	org := authz.OrganizationFromContext(r.Context())
	if org == nil {
		http.Error(w, "Organization required", http.StatusBadRequest)
		return
	}
	organizationIDStr := strconv.FormatInt(org.ID, 10)

	if identifier == "" {
		http.Error(w, "Identifier is required", http.StatusBadRequest)
		return
	}

	var clientIP string
	if otpLimiter != nil {
		clientIP = ratelimit.GetClientIP(r, trustProxy)
		// Check rate limit (doesn't consume quota yet)
		result := otpLimiter.CheckOTPSend(identifier, clientIP)
		if !result.Allowed {
			ratelimit.LogRateLimitExceeded("otp_send", identifier, clientIP, result.Reason)
			writeRateLimitError(w, r, result.RetryAfter, false)
			return
		}
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Don't consume rate limit quota for non-existent users
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		logger.Error().Err(err).Msg("Failed to load user for auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !user.IsStaff {
		// Don't consume rate limit quota for non-staff users
		writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// User validated - now record the rate limit (consumes quota)
	if otpLimiter != nil {
		otpLimiter.RecordOTPSend(identifier, clientIP)
	}

	// Dev mode bypass - skip Cognito, return fake session for code entry
	if isDevMode() {
		logger.Warn().Str("identifier", identifier).Msg("Dev mode: skipping Cognito send-code for staff")
		component := authtempl.CodeVerification(identifier, organizationIDStr, devBypassSession)
		if err := component.Render(r.Context(), w); err != nil {
			logger.Error().Err(err).Msg("Failed to render code verification form")
			http.Error(w, "Failed to render page", http.StatusInternalServerError)
		}
		return
	}

	// Check if Cognito is configured
	if cognitoClient == nil {
		logger.Error().Msg("Cognito not configured")
		http.Error(w, "OTP authentication not available", http.StatusServiceUnavailable)
		return
	}

	authResponse, err := cognitoClient.InitiateOTP(r.Context(), identifier)
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
	session := r.FormValue("session")

	// Get organization from context (set by subdomain middleware)
	org := authz.OrganizationFromContext(r.Context())
	if org == nil {
		http.Error(w, "Organization required", http.StatusBadRequest)
		return
	}

	if code == "" || identifier == "" || session == "" {
		http.Error(w, "Code, identifier, and session are required", http.StatusBadRequest)
		return
	}

	var clientIP string
	if otpLimiter != nil {
		clientIP = ratelimit.GetClientIP(r, trustProxy)
		// Check rate limit (doesn't consume quota yet)
		result := otpLimiter.CheckOTPVerify(identifier, clientIP)
		if !result.Allowed {
			ratelimit.LogRateLimitExceeded("otp_verify", identifier, clientIP, result.Reason)
			writeRateLimitError(w, r, result.RetryAfter, true)
			return
		}
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Don't consume rate limit for non-existent users
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		logger.Error().Err(err).Msg("Failed to load user for auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !user.IsStaff {
		// Don't consume rate limit for non-staff users
		writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// User validated - now record the verify attempt (consumes quota)
	if otpLimiter != nil {
		otpLimiter.RecordOTPVerify(identifier, clientIP)
	}

	// Dev mode bypass - accept devBypassCode as valid
	if isDevMode() && code == devBypassCode {
		logger.Warn().Str("identifier", identifier).Msg("Dev mode: bypassing Cognito verify-code for staff")
		if otpLimiter != nil {
			otpLimiter.ResetVerifyAttempts(identifier)
		}
		setStaffSession(w, r, user)
		return
	}

	// Check if Cognito is configured
	if cognitoClient == nil {
		logger.Error().Msg("Cognito not configured")
		http.Error(w, "OTP authentication not available", http.StatusServiceUnavailable)
		return
	}

	authResponse, err := cognitoClient.VerifyOTP(r.Context(), session, identifier, code)
	if err != nil {
		if handleCognitoAuthError(w, r, err, "Invalid verification code", "Verification code expired") {
			return
		}
		logger.Error().Err(err).Msg("Failed to verify Cognito OTP")
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

	// Reset verify attempts on successful verification
	if otpLimiter != nil {
		otpLimiter.ResetVerifyAttempts(identifier)
	}

	setStaffSession(w, r, user)
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

// setStaffSession creates auth session for a staff user and sends redirect response.
// On error, writes error response directly.
func setStaffSession(w http.ResponseWriter, r *http.Request, user dbgen.User) {
	logger := log.Ctx(r.Context())

	var homeFacilityID *int64
	if user.HomeFacilityID.Valid {
		id := user.HomeFacilityID.Int64
		homeFacilityID = &id
	}

	authUser := &authz.AuthUser{
		ID:              user.ID,
		IsStaff:         true,
		SessionType:     SessionTypeStaff,
		HomeFacilityID:  homeFacilityID,
		MembershipLevel: user.MembershipLevel,
	}

	if err := SetAuthCookie(w, r, authUser); err != nil {
		logger.Error().Err(err).Msg("Failed to set staff auth session")
		http.Error(w, "Failed to start session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func HandleMemberSendCode(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identifier := r.FormValue("identifier")

	// Get organization from context (set by subdomain middleware)
	org := authz.OrganizationFromContext(r.Context())
	if org == nil {
		http.Error(w, "Organization required", http.StatusBadRequest)
		return
	}
	organizationIDStr := strconv.FormatInt(org.ID, 10)

	if identifier == "" {
		http.Error(w, "Identifier is required", http.StatusBadRequest)
		return
	}

	var clientIP string
	if otpLimiter != nil {
		clientIP = ratelimit.GetClientIP(r, trustProxy)
		// Check rate limit (doesn't consume quota yet)
		result := otpLimiter.CheckOTPSend(identifier, clientIP)
		if !result.Allowed {
			ratelimit.LogRateLimitExceeded("otp_send", identifier, clientIP, result.Reason)
			writeRateLimitError(w, r, result.RetryAfter, false)
			return
		}
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Don't consume rate limit for non-existent users
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		logger.Error().Err(err).Msg("Failed to load user for auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !user.IsMember {
		// Don't consume rate limit for non-member users
		writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// User validated - now record the rate limit (consumes quota)
	if otpLimiter != nil {
		otpLimiter.RecordOTPSend(identifier, clientIP)
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

	// Check if Cognito is configured
	if cognitoClient == nil {
		logger.Error().Msg("Cognito not configured")
		http.Error(w, "OTP authentication not available", http.StatusServiceUnavailable)
		return
	}

	authResponse, err := cognitoClient.InitiateOTP(r.Context(), identifier)
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
	session := r.FormValue("session")

	// Get organization from context (set by subdomain middleware)
	org := authz.OrganizationFromContext(r.Context())
	if org == nil {
		http.Error(w, "Organization required", http.StatusBadRequest)
		return
	}

	if code == "" || identifier == "" || session == "" {
		http.Error(w, "Code, identifier, and session are required", http.StatusBadRequest)
		return
	}

	var clientIP string
	if otpLimiter != nil {
		clientIP = ratelimit.GetClientIP(r, trustProxy)
		// Check rate limit (doesn't consume quota yet)
		result := otpLimiter.CheckOTPVerify(identifier, clientIP)
		if !result.Allowed {
			ratelimit.LogRateLimitExceeded("otp_verify", identifier, clientIP, result.Reason)
			writeRateLimitError(w, r, result.RetryAfter, true)
			return
		}
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Don't consume rate limit for non-existent users
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		logger.Error().Err(err).Msg("Failed to load user for auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !user.IsMember {
		// Don't consume rate limit for non-member users
		writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// User validated - now record the verify attempt (consumes quota)
	if otpLimiter != nil {
		otpLimiter.RecordOTPVerify(identifier, clientIP)
	}

	// Dev mode bypass - accept devBypassCode as valid
	if isDevMode() && code == devBypassCode {
		logger.Warn().Str("identifier", identifier).Msg("Dev mode: bypassing Cognito verify-code")
		if otpLimiter != nil {
			otpLimiter.ResetVerifyAttempts(identifier)
		}
		setMemberSession(w, r, user)
		return
	}

	// Check if Cognito is configured
	if cognitoClient == nil {
		logger.Error().Msg("Cognito not configured")
		http.Error(w, "OTP authentication not available", http.StatusServiceUnavailable)
		return
	}

	authResponse, err := cognitoClient.VerifyOTP(r.Context(), session, identifier, code)
	if err != nil {
		if handleCognitoAuthError(w, r, err, "Invalid verification code", "Verification code expired") {
			return
		}
		logger.Error().Err(err).Msg("Failed to verify Cognito OTP")
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

	// Reset verify attempts on successful verification
	if otpLimiter != nil {
		otpLimiter.ResetVerifyAttempts(identifier)
	}

	setMemberSession(w, r, user)
}

func HandleMemberResendCode(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identifier := r.FormValue("identifier")

	// Get organization from context (set by subdomain middleware)
	org := authz.OrganizationFromContext(r.Context())
	if org == nil {
		http.Error(w, "Organization required", http.StatusBadRequest)
		return
	}
	organizationIDStr := strconv.FormatInt(org.ID, 10)

	if identifier == "" {
		http.Error(w, "Identifier is required", http.StatusBadRequest)
		return
	}

	var clientIP string
	if otpLimiter != nil {
		clientIP = ratelimit.GetClientIP(r, trustProxy)
		// Check rate limit (doesn't consume quota yet)
		result := otpLimiter.CheckOTPSend(identifier, clientIP)
		if !result.Allowed {
			ratelimit.LogRateLimitExceeded("otp_resend", identifier, clientIP, result.Reason)
			writeRateLimitError(w, r, result.RetryAfter, false)
			return
		}
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Don't consume rate limit for non-existent users
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		logger.Error().Err(err).Msg("Failed to load user for auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !user.IsMember {
		// Don't consume rate limit for non-member users
		writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// User validated - now record the rate limit (consumes quota)
	if otpLimiter != nil {
		otpLimiter.RecordOTPSend(identifier, clientIP)
	}

	// Dev mode bypass
	if isDevMode() {
		logger.Warn().Str("identifier", identifier).Msg("Dev mode: skipping Cognito resend-code for member")
		component := authtempl.MemberCodeVerification(identifier, organizationIDStr, devBypassSession)
		if err := component.Render(r.Context(), w); err != nil {
			logger.Error().Err(err).Msg("Failed to render code verification form")
			http.Error(w, "Failed to render page", http.StatusInternalServerError)
		}
		return
	}

	if cognitoClient == nil {
		logger.Error().Msg("Cognito not configured")
		http.Error(w, "OTP authentication not available", http.StatusServiceUnavailable)
		return
	}

	authResponse, err := cognitoClient.InitiateOTP(r.Context(), identifier)
	if err != nil {
		if handleCognitoAuthError(w, r, err, "Invalid credentials", "Verification code expired") {
			return
		}
		logger.Error().Err(err).Msg("Failed to resend Cognito auth code for member")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	session := ""
	if authResponse != nil && authResponse.Session != nil {
		session = *authResponse.Session
	}

	component := authtempl.MemberCodeVerification(identifier, organizationIDStr, session)
	if err := component.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render member verification screen")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleStaffLogin(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method == http.MethodGet {
		identifier := r.FormValue("identifier")
		component := authtempl.StaffLoginForm(authz.OrganizationIDString(r.Context()), identifier)
		err := component.Render(r.Context(), w)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to render staff login form")
			http.Error(w, "Failed to render page", http.StatusInternalServerError)
		}
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identifier := r.FormValue("identifier")
	password := r.FormValue("password")

	if identifier == "" || password == "" {
		http.Error(w, "Identifier and password are required", http.StatusBadRequest)
		return
	}

	// Password auth uses the same OTP rate limiter for brute-force protection
	var clientIP string
	if otpLimiter != nil {
		clientIP = ratelimit.GetClientIP(r, trustProxy)
		result := otpLimiter.CheckOTPVerify(identifier, clientIP)
		if !result.Allowed {
			ratelimit.LogRateLimitExceeded("password_auth", identifier, clientIP, result.Reason)
			writeRateLimitError(w, r, result.RetryAfter, true)
			return
		}
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

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = VerifyPassword(dummyPasswordHash, password)
		} else {
			logger.Error().Err(err).Msg("Database error during staff login")
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Always run bcrypt to prevent timing attacks that reveal staff status
	hashToCheck := dummyPasswordHash
	if user.PasswordHash.Valid {
		hashToCheck = user.PasswordHash.String
	}
	passwordValid := VerifyPassword(hashToCheck, password)

	if !user.IsStaff || !user.LocalAuthEnabled || !user.PasswordHash.Valid || !passwordValid {
		// Record failed attempt after user validation
		if otpLimiter != nil {
			otpLimiter.RecordOTPVerify(identifier, clientIP)
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Successful login - reset attempts
	if otpLimiter != nil {
		otpLimiter.ResetVerifyAttempts(identifier)
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
		component := authtempl.ResetPasswordForm(identifier, authz.OrganizationIDString(r.Context()))
		err := component.Render(r.Context(), w)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to render reset password form")
			http.Error(w, "Failed to render page", http.StatusInternalServerError)
		}
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Require organization for POST (access control)
	if authz.OrganizationFromContext(r.Context()) == nil {
		http.Error(w, "Organization required", http.StatusBadRequest)
		return
	}

	identifier := r.FormValue("identifier")

	if identifier == "" {
		http.Error(w, "Identifier is required", http.StatusBadRequest)
		return
	}

	var clientIP string
	if otpLimiter != nil {
		clientIP = ratelimit.GetClientIP(r, trustProxy)
		// Check rate limit (doesn't consume quota yet)
		result := otpLimiter.CheckOTPSend(identifier, clientIP)
		if !result.Allowed {
			ratelimit.LogRateLimitExceeded("password_reset", identifier, clientIP, result.Reason)
			writeRateLimitError(w, r, result.RetryAfter, false)
			return
		}
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check if Cognito is configured
	if cognitoClient == nil {
		logger.Error().Msg("Cognito not configured")
		http.Error(w, "Password reset not available", http.StatusServiceUnavailable)
		return
	}

	// Check if user exists in our database before burning quota
	// This prevents attackers from exhausting quota for non-existent users
	_, err := getUserByIdentifier(r.Context(), identifier)
	userExists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logger.Error().Err(err).Msg("Failed to check user for password reset")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Only burn quota and call Cognito if user exists in our system
	if userExists {
		if otpLimiter != nil {
			otpLimiter.RecordOTPSend(identifier, clientIP)
		}

		_, err = cognitoClient.ForgotPassword(r.Context(), identifier)
		if err != nil {
			var userNotFound *types.UserNotFoundException
			if errors.As(err, &userNotFound) {
				// User exists in our DB but not in Cognito - that's OK, don't reveal this
				err = nil
			} else if handleCognitoAuthError(w, r, err, "Unable to send reset code", "Reset request expired") {
				return
			} else {
				logger.Error().Err(err).Msg("Failed to send password reset code")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
	}
	// If user doesn't exist, we silently succeed (don't reveal user existence)

	component := authtempl.ResetPasswordConfirmation(identifier, authz.OrganizationIDString(r.Context()))
	err = component.Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render password reset confirmation form")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleConfirmResetPassword(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get organization from context (set by subdomain middleware)
	org := authz.OrganizationFromContext(r.Context())
	if org == nil {
		http.Error(w, "Organization required", http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")
	newPassword := r.FormValue("new_password")
	identifier := r.FormValue("identifier")

	if code == "" || newPassword == "" || identifier == "" {
		http.Error(w, "Code, new password, and identifier are required", http.StatusBadRequest)
		return
	}

	var clientIP string
	if otpLimiter != nil {
		clientIP = ratelimit.GetClientIP(r, trustProxy)
		// Check rate limit (doesn't consume quota yet)
		result := otpLimiter.CheckOTPVerify(identifier, clientIP)
		if !result.Allowed {
			ratelimit.LogRateLimitExceeded("password_reset_confirm", identifier, clientIP, result.Reason)
			writeRateLimitError(w, r, result.RetryAfter, true)
			return
		}
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check if Cognito is configured
	if cognitoClient == nil {
		logger.Error().Msg("Cognito not configured")
		http.Error(w, "Password reset not available", http.StatusServiceUnavailable)
		return
	}

	// Check if user exists in our database before burning quota
	// This prevents attackers from exhausting quota for non-existent users
	_, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// User doesn't exist - don't burn quota, return generic error
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid verification code")
			return
		}
		logger.Error().Err(err).Msg("Failed to check user for password reset confirm")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// User validated - now record the verify attempt (consumes quota)
	if otpLimiter != nil {
		otpLimiter.RecordOTPVerify(identifier, clientIP)
	}

	_, err = cognitoClient.ConfirmForgotPassword(r.Context(), identifier, code, newPassword)
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

	// Successful password reset - clear attempts
	if otpLimiter != nil {
		otpLimiter.ResetVerifyAttempts(identifier)
	}

	w.Header().Set("HX-Redirect", "/login")
	w.WriteHeader(http.StatusOK)
}

func HandleResendCode(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identifier := r.FormValue("identifier")

	// Get organization from context (set by subdomain middleware)
	org := authz.OrganizationFromContext(r.Context())
	if org == nil {
		http.Error(w, "Organization required", http.StatusBadRequest)
		return
	}
	organizationIDStr := strconv.FormatInt(org.ID, 10)

	if identifier == "" {
		http.Error(w, "Identifier is required", http.StatusBadRequest)
		return
	}

	var clientIP string
	if otpLimiter != nil {
		clientIP = ratelimit.GetClientIP(r, trustProxy)
		// Check rate limit (doesn't consume quota yet)
		result := otpLimiter.CheckOTPSend(identifier, clientIP)
		if !result.Allowed {
			ratelimit.LogRateLimitExceeded("otp_resend", identifier, clientIP, result.Reason)
			writeRateLimitError(w, r, result.RetryAfter, false)
			return
		}
	}

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user, err := getUserByIdentifier(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Don't consume rate limit for non-existent users
			writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		logger.Error().Err(err).Msg("Failed to load user for auth")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// This endpoint is for staff resend - verify user is staff
	if !user.IsStaff {
		// Don't consume rate limit for non-staff users
		writeHTMXError(w, r, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// User validated - now record the rate limit (consumes quota)
	if otpLimiter != nil {
		otpLimiter.RecordOTPSend(identifier, clientIP)
	}

	// Dev mode bypass - return fake session for code entry
	if isDevMode() {
		logger.Warn().Str("identifier", identifier).Msg("Dev mode: skipping Cognito resend-code")
		component := authtempl.CodeVerification(identifier, organizationIDStr, devBypassSession)
		if err := component.Render(r.Context(), w); err != nil {
			logger.Error().Err(err).Msg("Failed to render code verification form")
			http.Error(w, "Failed to render page", http.StatusInternalServerError)
		}
		return
	}

	// Check if Cognito is configured
	if cognitoClient == nil {
		logger.Error().Msg("Cognito not configured")
		http.Error(w, "OTP authentication not available", http.StatusServiceUnavailable)
		return
	}

	authResponse, err := cognitoClient.InitiateOTP(r.Context(), identifier)
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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	component := authtempl.LoginLayout(authz.OrganizationIDString(r.Context()))
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
	// Normalize identifier to match rate limiter behavior and ensure consistent lookups
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if cognito.IsPhoneNumber(identifier) {
		// Normalize phone to E.164 format for consistent lookup
		normalized := cognito.NormalizePhone(identifier)
		if normalized == "" {
			return dbgen.User{}, sql.ErrNoRows // Invalid phone format
		}
		return queries.GetUserByPhone(ctx, sql.NullString{String: normalized, Valid: true})
	}
	return queries.GetUserByEmail(ctx, sql.NullString{String: identifier, Valid: true})
}

func handleCognitoAuthError(w http.ResponseWriter, r *http.Request, err error, unauthorizedMsg string, expiredMsg string) bool {
	switch {
	case errors.Is(err, cognito.ErrInvalidPhone):
		writeHTMXError(w, r, http.StatusBadRequest, "Invalid phone number format")
		return true
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

func writeRateLimitError(w http.ResponseWriter, r *http.Request, retryAfter time.Duration, isVerify bool) {
	w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))

	var msg string
	if isVerify {
		// Brute-force lockout - show minutes
		mins := int(retryAfter.Minutes())
		if mins < 1 {
			mins = 1
		}
		if mins == 1 {
			msg = "Too many attempts. Please try again in 1 minute."
		} else {
			msg = fmt.Sprintf("Too many attempts. Please try again in %d minutes.", mins)
		}
	} else {
		// OTP send cooldown - show seconds for short waits
		secs := int(retryAfter.Seconds())
		if secs <= 60 {
			if secs == 1 {
				msg = "Please wait 1 second before requesting another code."
			} else {
				msg = fmt.Sprintf("Please wait %d seconds before requesting another code.", secs)
			}
		} else {
			mins := int(retryAfter.Minutes())
			if mins < 1 {
				mins = 1
			}
			if mins == 1 {
				msg = "Please wait 1 minute before requesting another code."
			} else {
				msg = fmt.Sprintf("Please wait %d minutes before requesting another code.", mins)
			}
		}
	}
	writeHTMXError(w, r, http.StatusTooManyRequests, msg)
}
