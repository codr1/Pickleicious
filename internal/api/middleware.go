// internal/api/middleware.go
package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/auth"
	"github.com/codr1/Pickleicious/internal/api/authz"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

type Middleware func(http.Handler) http.Handler

func ChainMiddleware(h http.Handler, middleware ...Middleware) http.Handler {
	for _, m := range middleware {
		h = m(h)
	}
	return h
}

func WithLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create response wrapper to capture status code
		wrapped := wrapResponseWriter(w)

		next.ServeHTTP(wrapped, r)
		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", wrapped.status).
			Dur("duration", time.Since(start)).
			Str("request_id", r.Context().Value("request_id").(string)).
			Msg("Request completed")
	})
}

func WithRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger := log.Ctx(r.Context())
				// Log the full stack trace
				stack := debug.Stack()
				logger.Error().
					Interface("error", err).
					Str("stack", string(stack)).
					Msg("Panic recovered")

				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.New().String()

		// Create a logger with the request ID
		logger := log.With().Str("request_id", requestID).Logger()

		// Add both the request ID and logger to context
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		ctx = logger.WithContext(ctx)

		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func WithContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set default content type if not set
		if r.Header.Get("Accept") == "" {
			r.Header.Set("Accept", "text/html")
		}
		next.ServeHTTP(w, r)
	})
}

func WithAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := auth.UserFromRequest(w, r)
		if err != nil {
			log.Ctx(r.Context()).Warn().Err(err).Msg("Failed to load auth session")
			next.ServeHTTP(w, r)
			return
		}

		if user != nil {
			ctx := authz.ContextWithUser(r.Context(), user)
			r = r.WithContext(ctx)
		}

		next.ServeHTTP(w, r)
	})
}

func WithStaffAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := log.Ctx(r.Context())
		user := authz.UserFromContext(r.Context())
		if err := authz.RequireRole(r.Context(), "staff"); err != nil {
			switch {
			case errors.Is(err, authz.ErrUnauthenticated):
				logEvent := logger.Warn()
				if user != nil {
					logEvent = logEvent.Int64("user_id", user.ID)
				}
				logEvent.Msg("Staff access denied: unauthenticated")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			case errors.Is(err, authz.ErrForbidden):
				logEvent := logger.Warn()
				if user != nil {
					logEvent = logEvent.Int64("user_id", user.ID)
				}
				logEvent.Msg("Staff access denied: forbidden")
				http.Error(w, "Forbidden", http.StatusForbidden)
			default:
				logEvent := logger.Error().Err(err)
				if user != nil {
					logEvent = logEvent.Int64("user_id", user.ID)
				}
				logEvent.Msg("Staff access denied: error")
				http.Error(w, "Failed to authorize request", http.StatusInternalServerError)
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}

// responseWriter wrapper to capture status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// WithOrganization extracts the organization from the subdomain and adds it to context.
// Subdomain format: {org-slug}.{base_domain} (e.g., pickle.localhost)
func WithOrganization(queries *dbgen.Queries, baseDomain string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip org lookup for static assets and health checks
			path := r.URL.Path
			if strings.HasPrefix(path, "/static/") || path == "/health" || path == "/favicon.ico" {
				next.ServeHTTP(w, r)
				return
			}

			logger := log.Ctx(r.Context())

			host := r.Host
			// Strip port if present
			if idx := strings.LastIndex(host, ":"); idx != -1 {
				host = host[:idx]
			}

			// Check if host ends with base domain
			if !strings.HasSuffix(host, baseDomain) {
				// Not a subdomain request - could be direct IP or different domain
				// Allow through for health checks, static assets, etc.
				next.ServeHTTP(w, r)
				return
			}

			// Extract subdomain: "pickle.localhost" -> "pickle"
			subdomain := strings.TrimSuffix(host, "."+baseDomain)
			if subdomain == "" || subdomain == host {
				// No subdomain - show org picker or error
				logger.Debug().Str("host", r.Host).Msg("No organization subdomain")
				http.Error(w, "Organization not specified. Use {org-slug}."+baseDomain, http.StatusNotFound)
				return
			}

			// Look up organization by slug (timeout only applies to this DB query)
			queryCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()

			org, err := queries.GetOrganizationBySlug(queryCtx, subdomain)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					logger.Warn().Str("slug", subdomain).Msg("Organization not found")
					http.Error(w, "Organization not found", http.StatusNotFound)
					return
				}
				logger.Error().Err(err).Str("slug", subdomain).Msg("Failed to look up organization")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Add organization to context (no timeout for downstream handlers)
			authzOrg := &authz.Organization{
				ID:   org.ID,
				Name: org.Name,
				Slug: org.Slug,
			}
			ctx := authz.ContextWithOrganization(r.Context(), authzOrg)
			r = r.WithContext(ctx)

			logger.Debug().Int64("org_id", org.ID).Str("org_slug", org.Slug).Msg("Organization resolved from subdomain")
			next.ServeHTTP(w, r)
		})
	}
}
