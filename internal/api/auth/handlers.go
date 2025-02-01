package auth

import (
	"database/sql"
	"net/http"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	authtempl "github.com/codr1/Pickleicious/internal/templates/components/auth"
    "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
)

type CognitoConfig struct {
    PoolID       string
    ClientID     string
    ClientSecret string
    Domain       string
    CallbackURL  string
}

var cognitoClient *cognitoidentityprovider.Client

func InitHandlers(q *dbgen.Queries) {
	queries = q
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

	// Check if email or phone
	isEmail := strings.Contains(identifier, "@")
	var staff dbgen.Staff
	var err error

	if isEmail {
		staff, err = queries.GetStaffByEmail(r.Context(), identifier)
	} else {
		staff, err = queries.GetStaffByPhone(r.Context(), identifier)
	}

	if err != nil && err != sql.ErrNoRows {
		logger.Error().Err(err).Msg("Database error checking staff")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// If staff member found and local auth is enabled, show staff section
	if err == nil && staff.LocalAuthEnabled {
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
    organizationID := r.FormValue("organization_id") // Added this

    if identifier == "" || organizationID == "" {
        http.Error(w, "Identifier and organization are required", http.StatusBadRequest)
        return
    }

    // Get Cognito config for this organization
    config, err := queries.GetCognitoConfig(r.Context(), organizationID)
    if err != nil {
        logger.Error().Err(err).Msg("Failed to get Cognito config")
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Initialize Cognito client with organization-specific config
    // TODO: Implement Cognito client initialization with config

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
    organizationID := r.FormValue("organization_id") // Added this

    if code == "" || identifier == "" || organizationID == "" {
        http.Error(w, "Code, identifier, and organization are required", http.StatusBadRequest)
        return
    }

    // Get Cognito config for this organization
    config, err := queries.GetCognitoConfig(r.Context(), organizationID)
    if err != nil {
        logger.Error().Err(err).Msg("Failed to get Cognito config")
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Verify code with Cognito
    // TODO: Implement Cognito verification

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

	// TODO: Implement password verification
	logger.Error().Msg("Staff login not implemented")
	http.Error(w, "Staff login not implemented", http.StatusNotImplemented)
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