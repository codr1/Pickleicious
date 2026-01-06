package member

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
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

	reservationData, err := buildReservationListData(ctx, q, user.ID, user.HomeFacilityID, requestedFacilityID(r), logger)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load member reservations")
		reservationData = membertempl.ReservationListData{}
	}

	profile := membertempl.PortalProfile{
		ID:              memberRow.ID,
		FirstName:       memberRow.FirstName,
		LastName:        memberRow.LastName,
		Email:           memberRow.Email.String,
		MembershipLevel: memberRow.MembershipLevel,
		HasPhoto:        memberRow.PhotoID.Valid,
	}

	page := layouts.Base(membertempl.MemberPortal(profile, reservationData), activeTheme)
	if err := page.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render member portal")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

// HandleMemberReservationsPartial renders the reservation list for facility filtering.
func HandleMemberReservationsPartial(w http.ResponseWriter, r *http.Request) {
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

	reservationData, err := buildReservationListData(ctx, q, user.ID, user.HomeFacilityID, requestedFacilityID(r), logger)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load member reservations")
		http.Error(w, "Failed to load reservations", http.StatusInternalServerError)
		return
	}

	if err := membertempl.MemberReservations(reservationData).Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render member reservations")
		http.Error(w, "Failed to render reservations", http.StatusInternalServerError)
		return
	}
}

func requestedFacilityID(r *http.Request) *int64 {
	rawID := r.URL.Query().Get("facility_id")
	if rawID == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func buildReservationListData(
	ctx context.Context,
	q *dbgen.Queries,
	userID int64,
	homeFacilityID *int64,
	requestedFacilityID *int64,
	logger *zerolog.Logger,
) (membertempl.ReservationListData, error) {
	rows, err := q.ListReservationsByUserID(ctx, sql.NullInt64{Int64: userID, Valid: true})
	if err != nil {
		return membertempl.ReservationListData{}, err
	}

	facilitiesByID := make(map[int64]string, len(rows))
	for _, row := range rows {
		facilitiesByID[row.FacilityID] = row.FacilityName
	}

	facilities := make([]membertempl.ReservationFacility, 0, len(facilitiesByID))
	for id, name := range facilitiesByID {
		facilities = append(facilities, membertempl.ReservationFacility{ID: id, Name: name})
	}
	sort.Slice(facilities, func(i, j int) bool {
		return strings.ToLower(facilities[i].Name) < strings.ToLower(facilities[j].Name)
	})

	selectedFacilityID := int64(0)
	if requestedFacilityID != nil {
		if _, ok := facilitiesByID[*requestedFacilityID]; ok {
			selectedFacilityID = *requestedFacilityID
		}
	}
	if selectedFacilityID == 0 && homeFacilityID != nil {
		if _, ok := facilitiesByID[*homeFacilityID]; ok {
			selectedFacilityID = *homeFacilityID
		}
	}
	if selectedFacilityID == 0 && len(facilities) > 0 {
		selectedFacilityID = facilities[0].ID
	}

	var filteredRows []dbgen.ListReservationsByUserIDRow
	if selectedFacilityID != 0 {
		filteredRows = make([]dbgen.ListReservationsByUserIDRow, 0, len(rows))
		for _, row := range rows {
			if row.FacilityID == selectedFacilityID {
				filteredRows = append(filteredRows, row)
			}
		}
	}

	summaries := membertempl.NewReservationSummaries(filteredRows)
	for i := range summaries {
		participants, err := q.ListParticipantsForReservation(ctx, summaries[i].ID)
		if err != nil {
			logger.Error().Err(err).Int64("reservation_id", summaries[i].ID).Msg("Failed to load reservation participants")
			continue
		}
		names := make([]string, 0, len(participants))
		for _, participant := range participants {
			if participant.ID == userID {
				continue
			}
			name := strings.TrimSpace(strings.TrimSpace(participant.FirstName) + " " + strings.TrimSpace(participant.LastName))
			if name == "" && participant.Email.Valid {
				name = participant.Email.String
			}
			if name != "" {
				names = append(names, name)
			}
		}
		summaries[i].OtherParticipants = names
	}

	now := time.Now()
	var upcoming []membertempl.ReservationSummary
	var past []membertempl.ReservationSummary
	for _, summary := range summaries {
		if summary.StartTime.After(now) {
			upcoming = append(upcoming, summary)
		} else {
			past = append(past, summary)
		}
	}

	showFilter := len(facilities) > 1 || homeFacilityID == nil
	if len(facilities) == 0 {
		showFilter = false
	}

	return membertempl.ReservationListData{
		Upcoming:           upcoming,
		Past:               past,
		Facilities:         facilities,
		SelectedFacilityID: selectedFacilityID,
		ShowFacilityFilter: showFilter,
	}, nil
}
