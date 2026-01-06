package member

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/auth"
	"github.com/codr1/Pickleicious/internal/api/authz"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	membertempl "github.com/codr1/Pickleicious/internal/templates/components/member"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

var (
	queries     *dbgen.Queries
	queriesOnce sync.Once
)

const portalQueryTimeout = 5 * time.Second

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(q *dbgen.Queries) {
	if q == nil {
		return
	}
	queriesOnce.Do(func() {
		queries = q
	})
}

func loadQueries() *dbgen.Queries {
	return queries
}

// RequireMemberSession ensures member-authenticated sessions reach member routes.
func RequireMemberSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := authz.UserFromContext(r.Context())
		if user == nil || user.SessionType != auth.SessionTypeMember {
			http.Redirect(w, r, "/member/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// HandleMemberPortal renders the member portal for GET /member.
func HandleMemberPortal(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/member/login", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	memberRow, err := q.GetMemberByID(ctx, user.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Redirect(w, r, "/member/login", http.StatusFound)
			return
		}
		logger.Error().Err(err).Msg("Failed to load member profile")
		http.Error(w, "Failed to load profile", http.StatusInternalServerError)
		return
	}

	var activeTheme *models.Theme
	if user.HomeFacilityID != nil {
		theme, err := models.GetActiveTheme(ctx, q, *user.HomeFacilityID)
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load active theme")
		} else {
			activeTheme = theme
		}
	}

	reservations, err := q.ListReservationsByUserID(ctx, sql.NullInt64{Int64: user.ID, Valid: true})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load member reservations")
		reservations = nil
	}

	profile := membertempl.PortalProfile{
		ID:              memberRow.ID,
		FirstName:       memberRow.FirstName,
		LastName:        memberRow.LastName,
		Email:           memberRow.Email.String,
		MembershipLevel: memberRow.MembershipLevel,
		HasPhoto:        memberRow.PhotoID.Valid,
	}
	reservationSummaries := membertempl.NewReservationSummaries(reservations)

	page := layouts.Base(membertempl.MemberPortal(profile, reservationSummaries), activeTheme)
	if err := page.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render member portal")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}
