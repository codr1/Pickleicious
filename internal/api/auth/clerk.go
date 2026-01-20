package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/cognito"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

// clerkInitialized indicates whether the Clerk SDK has been initialized
var clerkInitialized bool

// InitClerk initializes Clerk SDK with the secret key
func InitClerk(secretKey string) {
	if secretKey == "" {
		log.Warn().Msg("Clerk secret key not configured")
		return
	}
	clerk.SetKey(secretKey)
	clerkInitialized = true
	log.Info().Msg("Clerk SDK initialized")
}

// HandleClerkCallback handles the redirect after Clerk authentication
// It validates the Clerk session, looks up the local user, and creates a local session
func HandleClerkCallback(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !clerkInitialized {
		logger.Error().Msg("Clerk not configured")
		http.Error(w, "Authentication service not available", http.StatusServiceUnavailable)
		return
	}

	// Get session claims from Clerk middleware (set by WithClerkSession)
	claims, ok := clerk.SessionClaimsFromContext(r.Context())
	if !ok {
		logger.Warn().Msg("No Clerk session claims in context")
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// Get the Clerk user details
	clerkUser, err := user.Get(r.Context(), claims.Subject)
	if err != nil {
		logger.Error().Err(err).Str("clerk_user_id", claims.Subject).Msg("Failed to get Clerk user")
		http.Error(w, "Failed to verify user", http.StatusInternalServerError)
		return
	}

	// Find local user by email or phone from Clerk user
	localUser, err := findLocalUserFromClerk(r.Context(), clerkUser)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Warn().
				Str("clerk_user_id", claims.Subject).
				Msg("Clerk user has no matching local account")
			// User authenticated with Clerk but doesn't exist locally
			// This shouldn't happen if members are pre-registered
			http.Error(w, "Account not found. Please contact support.", http.StatusForbidden)
			return
		}
		logger.Error().Err(err).Msg("Failed to look up local user")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Create local session based on user type
	setSessionByUserType(w, r, localUser)
}

// findLocalUserFromClerk looks up the local user by email or phone from Clerk user data
func findLocalUserFromClerk(ctx context.Context, clerkUser *clerk.User) (dbgen.User, error) {
	if queries == nil {
		return dbgen.User{}, errors.New("database not initialized")
	}

	// Try primary email first
	if clerkUser.PrimaryEmailAddressID != nil {
		for _, email := range clerkUser.EmailAddresses {
			if email.ID == *clerkUser.PrimaryEmailAddressID {
				user, err := queries.GetUserByEmail(ctx, sql.NullString{String: email.EmailAddress, Valid: true})
				if err == nil {
					return user, nil
				}
				if !errors.Is(err, sql.ErrNoRows) {
					return dbgen.User{}, err
				}
				break
			}
		}
	}

	// Try primary phone
	if clerkUser.PrimaryPhoneNumberID != nil {
		for _, phone := range clerkUser.PhoneNumbers {
			if phone.ID == *clerkUser.PrimaryPhoneNumberID {
				normalized := cognito.NormalizePhone(phone.PhoneNumber)
				if normalized == "" {
					break // Invalid phone format, try other identifiers
				}
				user, err := queries.GetUserByPhone(ctx, sql.NullString{String: normalized, Valid: true})
				if err == nil {
					return user, nil
				}
				if !errors.Is(err, sql.ErrNoRows) {
					return dbgen.User{}, err
				}
				break
			}
		}
	}

	// Try all emails
	for _, email := range clerkUser.EmailAddresses {
		user, err := queries.GetUserByEmail(ctx, sql.NullString{String: email.EmailAddress, Valid: true})
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return dbgen.User{}, err
		}
	}

	// Try all phones
	for _, phone := range clerkUser.PhoneNumbers {
		normalized := cognito.NormalizePhone(phone.PhoneNumber)
		if normalized == "" {
			continue // Invalid phone format, try next
		}
		user, err := queries.GetUserByPhone(ctx, sql.NullString{String: normalized, Valid: true})
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return dbgen.User{}, err
		}
	}

	return dbgen.User{}, sql.ErrNoRows
}

// WithClerkSession is middleware that validates Clerk session tokens
// and adds session claims to the request context
func WithClerkSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !clerkInitialized {
			next.ServeHTTP(w, r)
			return
		}

		// Check for Clerk session cookie
		sessionToken, err := r.Cookie("__session")
		if err != nil {
			// No session cookie, continue without claims
			next.ServeHTTP(w, r)
			return
		}

		// Verify the session token
		claims, err := jwt.Verify(r.Context(), &jwt.VerifyParams{
			Token: sessionToken.Value,
		})
		if err != nil {
			log.Ctx(r.Context()).Debug().Err(err).Msg("Invalid Clerk session token")
			next.ServeHTTP(w, r)
			return
		}

		// Add claims to context
		ctx := clerk.ContextWithSessionClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireClerkSession is middleware that requires a valid Clerk session
func RequireClerkSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := clerk.SessionClaimsFromContext(r.Context())
		if !ok || claims == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
