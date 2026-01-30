package member

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/auth"
	"github.com/codr1/Pickleicious/internal/api/authz"
	appdb "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/email"
	"github.com/codr1/Pickleicious/internal/models"
	membertempl "github.com/codr1/Pickleicious/internal/templates/components/member"
	reservationstempl "github.com/codr1/Pickleicious/internal/templates/components/reservations"
	waitlisttempl "github.com/codr1/Pickleicious/internal/templates/components/waitlist"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

var (
	queries     *dbgen.Queries
	store       *appdb.DB
	queriesOnce sync.Once
	emailClient *email.SESClient
)

const portalQueryTimeout = 5 * time.Second
const memberReservationTypeName = "GAME"
const memberBookingTimeLayout = "2006-01-02T15:04"
const memberBookingMinDuration = time.Hour
const memberBookingDefaultOpensAt = "08:00"
const memberBookingDefaultClosesAt = "21:00"
const memberBookingDefaultMaxAdvanceDays int64 = 7
const cancellationPenaltyWindow = 10 * time.Minute

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(database *appdb.DB, client *email.SESClient) {
	if database == nil {
		return
	}
	queriesOnce.Do(func() {
		queries = database.Queries
		store = database
		emailClient = client
	})
}

func loadQueries() *dbgen.Queries {
	return queries
}

func loadDB() *appdb.DB {
	return store
}

func ensureOpenPlayReservation(ctx context.Context, qtx *dbgen.Queries, session dbgen.GetOpenPlaySessionRow, facilityID int64) error {
	reservationCount, err := qtx.CountOpenPlayReservationsForSession(ctx, dbgen.CountOpenPlayReservationsForSessionParams{
		FacilityID:     facilityID,
		OpenPlayRuleID: sql.NullInt64{Int64: session.OpenPlayRuleID, Valid: true},
		StartTime:      session.StartTime,
		EndTime:        session.EndTime,
	})
	if err != nil {
		return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to verify open play reservation", Err: err}
	}
	if reservationCount == 0 {
		return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Open play reservation not found"}
	}
	if reservationCount > 1 {
		return apiutil.HandlerError{
			Status:  http.StatusInternalServerError,
			Message: "Open play reservation is misconfigured",
			Err: fmt.Errorf(
				"expected 1 open play reservation, found %d for session %d (rule %d, facility %d, start %s, end %s)",
				reservationCount,
				session.ID,
				session.OpenPlayRuleID,
				facilityID,
				session.StartTime.Format(time.RFC3339),
				session.EndTime.Format(time.RFC3339),
			),
		}
	}

	return nil
}

// RequireMemberSession ensures member-authenticated sessions reach member routes.
func RequireMemberSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := log.Ctx(r.Context())

		user := authz.UserFromContext(r.Context())
		if user == nil || user.SessionType != auth.SessionTypeMember {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		q := loadQueries()
		if q == nil {
			logger.Error().Msg("Database queries not initialized")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
		defer cancel()

		memberRow, err := q.GetMemberByID(ctx, user.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			logger.Error().Err(err).Int64("member_id", user.ID).Msg("Failed to load member profile")
			http.Error(w, "Failed to load member profile", http.StatusInternalServerError)
			return
		}

		if memberRow.MembershipLevel < 1 {
			http.Error(w, "Active membership required", http.StatusForbidden)
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
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	memberRow, err := q.GetMemberByID(ctx, user.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Redirect(w, r, "/login", http.StatusFound)
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

	page := layouts.Base(membertempl.MemberPortal(profile, reservationData), activeTheme, user.SessionType)
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
		http.Redirect(w, r, "/login", http.StatusFound)
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

// HandleMemberReservationsWidget renders the upcoming reservations widget for the nav.
func HandleMemberReservationsWidget(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	widgetData, err := buildReservationWidgetData(ctx, q, user.ID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load upcoming reservations")
		widgetData = membertempl.NewReservationWidgetData(nil)
	}

	if err := membertempl.MemberReservationsWidget(widgetData).Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render reservations widget")
		http.Error(w, "Failed to render reservations widget", http.StatusInternalServerError)
		return
	}
}

// HandleMemberWaitlistList renders waitlist entries for members.
func HandleMemberWaitlistList(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if user.HomeFacilityID == nil {
		http.Error(w, "Home facility is required", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	rows, err := q.ListWaitlistsByUserAndFacility(ctx, dbgen.ListWaitlistsByUserAndFacilityParams{
		UserID:     user.ID,
		FacilityID: *user.HomeFacilityID,
	})
	if err != nil {
		logger.Error().Err(err).Int64("member_id", user.ID).Msg("Failed to load waitlist entries")
		http.Error(w, "Failed to load waitlist entries", http.StatusInternalServerError)
		return
	}

	entries := buildWaitlistEntrySummaries(ctx, q, rows, logger)
	component := waitlisttempl.WaitlistEntryList(waitlisttempl.WaitlistEntryListData{Entries: entries})
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render waitlist list", "Failed to render waitlist entries") {
		return
	}
}

// HandleMemberBookingFormNew handles GET /member/booking/new.
func HandleMemberBookingFormNew(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if user.HomeFacilityID == nil {
		http.Error(w, "Home facility is required", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	maxAdvanceDays := memberBookingDefaultMaxAdvanceDays
	facility, err := q.GetFacilityByID(ctx, *user.HomeFacilityID)
	facilityLoaded := err == nil
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load facility booking config")
	} else {
		maxAdvanceDays = normalizedMaxAdvanceBookingDays(facility.MaxAdvanceBookingDays)
	}

	courtsList, err := q.ListCourts(ctx, *user.HomeFacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load courts")
		http.Error(w, "Failed to load courts", http.StatusInternalServerError)
		return
	}
	activeCourts := courtsList[:0]
	for _, court := range courtsList {
		if court.Status == "active" {
			activeCourts = append(activeCourts, court)
		}
	}

	bookingDate := bookingDateFromRequest(r, maxAdvanceDays)
	availableSlots, err := buildMemberBookingSlots(ctx, q, *user.HomeFacilityID, bookingDate, logger)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load available slots")
		http.Error(w, "Failed to load available slots", http.StatusInternalServerError)
		return
	}
	waitlistStartTime, waitlistEndTime := waitlistFallbackTimes(bookingDate, availableSlots)
	var visitPackOptions []membertempl.MemberVisitPackOption
	if user.MembershipLevel <= 1 && facilityLoaded {
		crossFacility, err := q.GetOrganizationCrossFacilitySetting(ctx, facility.OrganizationID)
		if err != nil {
			logger.Error().Err(err).Int64("organization_id", facility.OrganizationID).Msg("Failed to load visit pack settings")
		} else {
			visitPacks, err := listActiveVisitPacksForMemberBooking(ctx, q, user.ID, facility.ID, facility.OrganizationID, crossFacility, time.Now())
			if err != nil {
				logger.Error().Err(err).Int64("user_id", user.ID).Msg("Failed to load visit packs")
			} else {
				visitPackOptions = buildMemberVisitPackOptions(visitPacks)
			}
		}
	}

	component := membertempl.MemberBookingForm(membertempl.MemberBookingFormData{
		FacilityID:            *user.HomeFacilityID,
		Courts:                reservationstempl.NewCourtOptions(activeCourts),
		AvailableSlots:        availableSlots,
		DatePicker:            membertempl.DatePickerData{Year: bookingDate.Year(), Month: int(bookingDate.Month()), Day: bookingDate.Day()},
		MaxAdvanceBookingDays: maxAdvanceDays,
		WaitlistStartTime:     waitlistStartTime,
		WaitlistEndTime:       waitlistEndTime,
		VisitPacks:            visitPackOptions,
	})
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render member booking form", "Failed to render booking form") {
		return
	}
}

// HandleMemberBookingSlots handles GET /member/booking/slots.
func HandleMemberBookingSlots(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if user.HomeFacilityID == nil {
		http.Error(w, "Home facility is required", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	maxAdvanceDays := memberBookingDefaultMaxAdvanceDays
	facility, err := q.GetFacilityByID(ctx, *user.HomeFacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load facility booking config")
	} else {
		maxAdvanceDays = normalizedMaxAdvanceBookingDays(facility.MaxAdvanceBookingDays)
	}

	bookingDate := bookingDateFromRequest(r, maxAdvanceDays)
	availableSlots, err := buildMemberBookingSlots(ctx, q, *user.HomeFacilityID, bookingDate, logger)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load available slots")
		http.Error(w, "Failed to load available slots", http.StatusInternalServerError)
		return
	}
	waitlistStartTime, waitlistEndTime := waitlistFallbackTimes(bookingDate, availableSlots)

	component := membertempl.MemberBookingDateTime(membertempl.MemberBookingFormData{
		FacilityID:            *user.HomeFacilityID,
		AvailableSlots:        availableSlots,
		DatePicker:            membertempl.DatePickerData{Year: bookingDate.Year(), Month: int(bookingDate.Month()), Day: bookingDate.Day()},
		MaxAdvanceBookingDays: maxAdvanceDays,
		WaitlistStartTime:     waitlistStartTime,
		WaitlistEndTime:       waitlistEndTime,
	})
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render booking slots", "Failed to render booking slots") {
		return
	}
}

type reservationLimitError struct {
	currentCount int64
	limit        int64
}

func (e reservationLimitError) Error() string {
	return fmt.Sprintf("reservation limit reached (%d/%d)", e.currentCount, e.limit)
}

func listActiveVisitPacksForMemberBooking(ctx context.Context, q *dbgen.Queries, userID, facilityID, organizationID int64, crossFacility bool, comparisonTime time.Time) ([]dbgen.VisitPack, error) {
	if crossFacility {
		return q.ListActiveVisitPacksForUserByOrganization(ctx, dbgen.ListActiveVisitPacksForUserByOrganizationParams{
			UserID:         userID,
			OrganizationID: organizationID,
			ComparisonTime: comparisonTime,
		})
	}
	return q.ListActiveVisitPacksForUserByFacility(ctx, dbgen.ListActiveVisitPacksForUserByFacilityParams{
		UserID:         userID,
		FacilityID:     facilityID,
		ComparisonTime: comparisonTime,
	})
}

func buildMemberVisitPackOptions(visitPacks []dbgen.VisitPack) []membertempl.MemberVisitPackOption {
	if len(visitPacks) == 0 {
		return nil
	}
	options := make([]membertempl.MemberVisitPackOption, len(visitPacks))
	for i, pack := range visitPacks {
		options[i] = membertempl.MemberVisitPackOption{
			ID:              pack.ID,
			VisitsRemaining: pack.VisitsRemaining,
			ExpiresAt:       pack.ExpiresAt,
		}
	}
	return options
}

// HandleMemberBookingCreate handles POST /member/reservations for member booking.
func HandleMemberBookingCreate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := loadQueries()
	database := loadDB()
	if q == nil || database == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if user.HomeFacilityID == nil {
		http.Error(w, "Home facility is required", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	maxAdvanceDays := memberBookingDefaultMaxAdvanceDays
	var maxMemberReservations int64
	facilityLoc := time.Local
	facilityLoaded := false
	facility, err := q.GetFacilityByID(ctx, *user.HomeFacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load facility booking config")
	} else {
		facilityLoaded = true
		maxAdvanceDays = normalizedMaxAdvanceBookingDays(facility.MaxAdvanceBookingDays)
		maxMemberReservations = facility.MaxMemberReservations
		if facility.Timezone != "" {
			loadedLoc, loadErr := time.LoadLocation(facility.Timezone)
			if loadErr != nil {
				logger.Error().Err(loadErr).Str("timezone", facility.Timezone).Msg("Failed to load facility timezone")
			} else {
				facilityLoc = loadedLoc
			}
		}
	}

	visitPackID, visitPackSelected, err := parseOptionalPositiveInt64(r.FormValue("visit_pack_id"), "visit_pack_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var availableVisitPackIDs map[int64]struct{}
	if user.MembershipLevel <= 1 {
		if facilityLoaded {
			crossFacility, err := q.GetOrganizationCrossFacilitySetting(ctx, facility.OrganizationID)
			if err != nil {
				logger.Error().Err(err).Int64("organization_id", facility.OrganizationID).Msg("Failed to load visit pack settings")
				http.Error(w, "Failed to load visit packs", http.StatusInternalServerError)
				return
			}
			visitPacks, err := listActiveVisitPacksForMemberBooking(ctx, q, user.ID, facility.ID, facility.OrganizationID, crossFacility, time.Now())
			if err != nil {
				logger.Error().Err(err).Int64("user_id", user.ID).Msg("Failed to load visit packs")
				http.Error(w, "Failed to load visit packs", http.StatusInternalServerError)
				return
			}
			availableVisitPackIDs = make(map[int64]struct{}, len(visitPacks))
			for _, pack := range visitPacks {
				availableVisitPackIDs[pack.ID] = struct{}{}
			}
		}
		if visitPackSelected {
			if !facilityLoaded || len(availableVisitPackIDs) == 0 {
				http.Error(w, "Selected visit pack is not available", http.StatusBadRequest)
				return
			}
			if _, ok := availableVisitPackIDs[visitPackID]; !ok {
				http.Error(w, "Selected visit pack is not available", http.StatusBadRequest)
				return
			}
		}
	} else if visitPackSelected {
		http.Error(w, "Visit packs are not available for your membership level", http.StatusBadRequest)
		return
	}

	startTime, err := parseMemberBookingTime(r.FormValue("start_time"), "start_time", facilityLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if startTime.Before(time.Now().In(facilityLoc)) {
		http.Error(w, "start_time must be in the future", http.StatusBadRequest)
		return
	}

	now := time.Now().In(facilityLoc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, facilityLoc)
	maxDate := today.AddDate(0, 0, int(maxAdvanceDays))
	startDay := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, facilityLoc)
	if startDay.After(maxDate) {
		http.Error(w, fmt.Sprintf("start_time must be within %d days", maxAdvanceDays), http.StatusBadRequest)
		return
	}

	endTime, err := parseMemberBookingTime(r.FormValue("end_time"), "end_time", facilityLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !endTime.After(startTime) {
		http.Error(w, "end_time must be after start_time", http.StatusBadRequest)
		return
	}
	if endTime.Sub(startTime) < memberBookingMinDuration {
		http.Error(w, "Reservation must be at least 1 hour", http.StatusBadRequest)
		return
	}

	courtID, err := parseMemberCourtID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	court, err := q.GetCourt(ctx, courtID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Court not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("court_id", courtID).Msg("Failed to load court")
		http.Error(w, "Failed to validate court", http.StatusInternalServerError)
		return
	}
	if court.FacilityID != *user.HomeFacilityID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	reservationTypeID, err := lookupReservationTypeID(ctx, q, memberReservationTypeName)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to resolve reservation type")
		http.Error(w, "Reservation type not available", http.StatusInternalServerError)
		return
	}

	var created dbgen.Reservation
	err = database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		if maxMemberReservations > 0 {
			activeCount, err := qtx.CountActiveMemberReservations(ctx, dbgen.CountActiveMemberReservationsParams{
				FacilityID:    *user.HomeFacilityID,
				PrimaryUserID: sql.NullInt64{Int64: user.ID, Valid: true},
			})
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check reservation limits", Err: err}
			}
			if activeCount >= maxMemberReservations {
				return reservationLimitError{currentCount: activeCount, limit: maxMemberReservations}
			}
		}

		if err := apiutil.EnsureCourtsAvailable(ctx, qtx, *user.HomeFacilityID, 0, startTime, endTime, []int64{courtID}); err != nil {
			var availErr apiutil.AvailabilityError
			if errors.As(err, &availErr) {
				return apiutil.HandlerError{Status: http.StatusConflict, Message: err.Error(), Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check court availability", Err: err}
		}

		var err error
		created, err = qtx.CreateReservation(ctx, dbgen.CreateReservationParams{
			FacilityID:        *user.HomeFacilityID,
			ReservationTypeID: reservationTypeID,
			RecurrenceRuleID:  sql.NullInt64{},
			PrimaryUserID:     sql.NullInt64{Int64: user.ID, Valid: true},
			CreatedByUserID:   user.ID,
			ProID:             sql.NullInt64{},
			OpenPlayRuleID:    sql.NullInt64{},
			StartTime:         startTime,
			EndTime:           endTime,
			IsOpenEvent:       false,
			TeamsPerCourt:     sql.NullInt64{},
			PeoplePerTeam:     sql.NullInt64{},
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to create reservation", Err: err}
		}

		if err := qtx.AddReservationCourt(ctx, dbgen.AddReservationCourtParams{
			ReservationID: created.ID,
			CourtID:       courtID,
		}); err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to assign reservation court", Err: err}
		}

		if err := qtx.AddParticipant(ctx, dbgen.AddParticipantParams{
			ReservationID: created.ID,
			UserID:        user.ID,
		}); err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to add reservation participant", Err: err}
		}

		if visitPackSelected {
			_, err := models.RedeemVisitPackVisit(ctx, qtx, models.RedeemVisitPackVisitParams{
				VisitPackID:   visitPackID,
				FacilityID:    *user.HomeFacilityID,
				ReservationID: &created.ID,
			})
			if err != nil {
				if errors.Is(err, models.ErrVisitPackUnavailable) {
					return apiutil.HandlerError{Status: http.StatusBadRequest, Message: "Selected visit pack is not available", Err: err}
				}
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to redeem visit pack", Err: err}
			}
		}

		return nil
	})
	if err != nil {
		var limitErr reservationLimitError
		if errors.As(err, &limitErr) {
			message := fmt.Sprintf("You have reached the maximum of %d active reservations", limitErr.limit)
			if err := apiutil.WriteJSON(w, http.StatusConflict, map[string]any{
				"error":         message,
				"current_count": limitErr.currentCount,
				"limit":         limitErr.limit,
			}); err != nil {
				logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to write reservation limit response")
			}
			return
		}
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			logger.Error().Err(herr.Err).Int64("facility_id", *user.HomeFacilityID).Msg(herr.Message)
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to create reservation")
		http.Error(w, "Failed to create reservation", http.StatusInternalServerError)
		return
	}

	if emailClient != nil && facilityLoaded {
		cancellationPolicy, policyErr := cancellationPolicySummary(ctx, q, facility.ID, reservationTypeID, startTime, now)
		if policyErr != nil {
			logger.Error().Err(policyErr).Int64("facility_id", facility.ID).Msg("Failed to load cancellation policy for confirmation email")
			cancellationPolicy = "Contact the facility for cancellation policy details."
		}
		date, timeRange := email.FormatDateTimeRange(startTime.In(facilityLoc), endTime.In(facilityLoc))
		confirmation := email.BuildGameConfirmation(email.ConfirmationDetails{
			FacilityName:       facility.Name,
			Date:               date,
			TimeRange:          timeRange,
			Courts:             court.Name,
			CancellationPolicy: cancellationPolicy,
		})
		email.SendConfirmationEmail(ctx, q, emailClient, user.ID, confirmation, logger)
	}

	w.Header().Set("HX-Trigger", "refreshMemberReservations")
	if err := apiutil.WriteJSON(w, http.StatusCreated, created); err != nil {
		logger.Error().Err(err).Int64("reservation_id", created.ID).Msg("Failed to write reservation response")
		return
	}
}

// HandleMemberReservationCancel handles DELETE /member/reservations/{id}.
func HandleMemberReservationCancel(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	database := loadDB()
	if q == nil || database == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	reservationID, err := apiutil.ReservationIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid reservation ID", http.StatusBadRequest)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	confirmCancellation := requestCancellationConfirm(r)

	var refundPercentage int64
	var reservation dbgen.Reservation
	var reservationCourts []dbgen.ListReservationCourtsRow
	var reservationParticipants []dbgen.ListParticipantsForReservationRow
	var reservationTypeName string
	err = database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		loadedReservation, err := qtx.GetReservationByID(ctx, reservationID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Reservation not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to fetch reservation", Err: err}
		}
		reservation = loadedReservation
		if !reservation.PrimaryUserID.Valid || reservation.PrimaryUserID.Int64 != user.ID {
			return apiutil.HandlerError{Status: http.StatusForbidden, Message: "Forbidden"}
		}
		now := time.Now()
		if !reservation.StartTime.After(now) {
			return apiutil.HandlerError{Status: http.StatusBadRequest, Message: "Reservation must be in the future"}
		}
		facilityID := reservation.FacilityID
		hoursUntilReservation := hoursUntilReservationStart(reservation.StartTime, now)
		refundPercentage, err = apiutil.ApplicableRefundPercentage(ctx, qtx, facilityID, hoursUntilReservation, &reservation.ReservationTypeID)
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load cancellation policy", Err: err}
		}
		buildPenalty := func(calculatedAt time.Time, refundPercentage int64, hoursBeforeStart int64) (membertempl.CancellationPenaltyData, error) {
			courts, err := qtx.ListReservationCourts(ctx, reservationID)
			if err != nil {
				return membertempl.CancellationPenaltyData{}, apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load reservation courts", Err: err}
			}
			facility, err := qtx.GetFacilityByID(ctx, facilityID)
			if err != nil {
				return membertempl.CancellationPenaltyData{}, apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load facility", Err: err}
			}
			return membertempl.CancellationPenaltyData{
				ReservationID:    reservationID,
				RefundPercentage: refundPercentage,
				FeePercentage:    100 - refundPercentage,
				HoursBeforeStart: hoursBeforeStart,
				StartTime:        reservation.StartTime,
				EndTime:          reservation.EndTime,
				CourtName:        apiutil.ReservationCourtLabel(courts),
				FacilityName:     facility.Name,
				ExpiresAt:        calculatedAt.Add(cancellationPenaltyWindow),
				CalculatedAt:     calculatedAt,
			}, nil
		}

		if refundPercentage < 100 {
			if confirmCancellation {
				previousCalculatedAt, previousHours, ok := requestCancellationPenaltyDetails(r)
				if !ok {
					penalty, err := buildPenalty(now, refundPercentage, hoursUntilReservation)
					if err != nil {
						return err
					}
					return cancellationPenaltyError{Penalty: penalty}
				}
				penaltyExpired := now.Sub(previousCalculatedAt) > cancellationPenaltyWindow
				if penaltyExpired {
					penalty, err := buildPenalty(now, refundPercentage, hoursUntilReservation)
					if err != nil {
						return err
					}
					return cancellationPenaltyError{Penalty: penalty}
				}
				previousRefundPercentage, err := apiutil.ApplicableRefundPercentage(ctx, qtx, facilityID, previousHours, &reservation.ReservationTypeID)
				if err != nil {
					return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load cancellation policy", Err: err}
				}
				if previousRefundPercentage != refundPercentage {
					penalty, err := buildPenalty(now, refundPercentage, hoursUntilReservation)
					if err != nil {
						return err
					}
					return cancellationPenaltyError{Penalty: penalty}
				}
			} else {
				penalty, err := buildPenalty(now, refundPercentage, hoursUntilReservation)
				if err != nil {
					return err
				}
				return cancellationPenaltyError{Penalty: penalty}
			}
		}
		if _, err := qtx.LogCancellation(ctx, dbgen.LogCancellationParams{
			ReservationID:           reservationID,
			CancelledByUserID:       user.ID,
			CancelledAt:             now,
			RefundPercentageApplied: refundPercentage,
			FeeWaived:               false,
			HoursBeforeStart:        hoursUntilReservation,
		}); err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to log cancellation", Err: err}
		}

		courts, err := qtx.ListReservationCourts(ctx, reservationID)
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load reservation courts", Err: err}
		}
		reservationCourts = courts
		for _, court := range courts {
			if err := qtx.RemoveReservationCourt(ctx, dbgen.RemoveReservationCourtParams{
				ReservationID: reservationID,
				CourtID:       court.CourtID,
			}); err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to remove reservation court", Err: err}
			}
		}

		participants, err := qtx.ListParticipantsForReservation(ctx, reservationID)
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load reservation participants", Err: err}
		}
		reservationParticipants = participants
		for _, participant := range participants {
			if err := qtx.RemoveParticipant(ctx, dbgen.RemoveParticipantParams{
				ReservationID: reservationID,
				UserID:        participant.ID,
			}); err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to remove reservation participant", Err: err}
			}
		}

		name, err := qtx.GetReservationTypeNameByReservationID(ctx, reservationID)
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load reservation type", Err: err}
		}
		reservationTypeName = name

		return nil
	})
	if err != nil {
		var penaltyErr cancellationPenaltyError
		if errors.As(err, &penaltyErr) {
			if apiutil.IsJSONRequest(r) {
				if err := apiutil.WriteJSON(w, http.StatusConflict, penaltyErr.Penalty); err != nil {
					logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to write cancellation penalty response")
				}
				return
			}
			var buf bytes.Buffer
			component := membertempl.CancellationConfirmModal(penaltyErr.Penalty)
			if err := component.Render(r.Context(), &buf); err != nil {
				logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to render cancellation confirmation modal")
				http.Error(w, "Failed to render modal", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("HX-Retarget", "#modal")
			w.Header().Set("HX-Reswap", "innerHTML")
			w.WriteHeader(http.StatusConflict)
			if _, err := w.Write(buf.Bytes()); err != nil {
				logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to write cancellation confirmation modal")
			}
			return
		}
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			if herr.Status == http.StatusInternalServerError {
				logger.Error().Err(herr.Err).Int64("reservation_id", reservationID).Msg(herr.Message)
			}
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to cancel reservation")
		http.Error(w, "Failed to cancel reservation", http.StatusInternalServerError)
		return
	}

	if emailClient != nil && reservation.ID != 0 {
		facility, err := q.GetFacilityByID(ctx, reservation.FacilityID)
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", reservation.FacilityID).Msg("Failed to load facility for cancellation email")
		} else {
			facilityLoc := time.Local
			if facility.Timezone != "" {
				if loadedLoc, loadErr := time.LoadLocation(facility.Timezone); loadErr == nil {
					facilityLoc = loadedLoc
				} else {
					logger.Error().Err(loadErr).Str("timezone", facility.Timezone).Msg("Failed to load facility timezone for cancellation email")
				}
			}
			date, timeRange := email.FormatDateTimeRange(reservation.StartTime.In(facilityLoc), reservation.EndTime.In(facilityLoc))
			courtLabel := apiutil.ReservationCourtLabel(reservationCourts)
			refund := refundPercentage
			message := email.BuildCancellationEmail(email.CancellationDetails{
				FacilityName:     facility.Name,
				ReservationType:  email.ReservationTypeLabel(reservationTypeName),
				Date:             date,
				TimeRange:        timeRange,
				Courts:           courtLabel,
				RefundPercentage: &refund,
			})
			sender := email.ResolveFromAddress(ctx, q, facility, logger)
			recipients := make(map[int64]struct{}, len(reservationParticipants)+1)
			for _, participant := range reservationParticipants {
				recipients[participant.ID] = struct{}{}
			}
			if reservation.PrimaryUserID.Valid {
				recipients[reservation.PrimaryUserID.Int64] = struct{}{}
			}
			for participantID := range recipients {
				email.SendCancellationEmail(ctx, q, emailClient, participantID, message, sender, logger)
			}
		}
	}

	w.Header().Set("HX-Trigger", "refreshMemberReservations")
	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{
		"refund_percentage": refundPercentage,
	}); err != nil {
		logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to write cancellation response")
		return
	}
}

// HandleMemberOpenPlayList renders upcoming open play sessions for members.
func HandleMemberOpenPlayList(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if user.HomeFacilityID == nil {
		http.Error(w, "Home facility is required", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	rows, err := q.ListMemberUpcomingOpenPlaySessions(ctx, dbgen.ListMemberUpcomingOpenPlaySessionsParams{
		FacilityIds:    []int64{*user.HomeFacilityID},
		ComparisonTime: time.Now(),
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load open play sessions")
		http.Error(w, "Failed to load open play sessions", http.StatusInternalServerError)
		return
	}

	summaries := membertempl.NewOpenPlaySessionSummaries(rows)
	for i := range summaries {
		isParticipant, err := q.IsMemberOpenPlayParticipant(ctx, dbgen.IsMemberOpenPlayParticipantParams{
			SessionID:  summaries[i].ID,
			FacilityID: *user.HomeFacilityID,
			UserID:     user.ID,
		})
		if err != nil {
			logger.Error().Err(err).Int64("session_id", summaries[i].ID).Msg("Failed to check open play participation")
			continue
		}
		summaries[i].IsSignedUp = isParticipant > 0
	}

	component := membertempl.MemberOpenPlaySessions(membertempl.OpenPlayListData{Upcoming: summaries})
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render open play list", "Failed to render open play sessions") {
		return
	}
}

// HandleMemberOpenPlaySignup handles POST /member/openplay/{id}.
func HandleMemberOpenPlaySignup(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := loadQueries()
	database := loadDB()
	if q == nil || database == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sessionID, err := memberOpenPlaySessionIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if user.HomeFacilityID == nil {
		http.Error(w, "Home facility is required", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	maxMemberReservations := int64(0)
	facility, err := q.GetFacilityByID(ctx, *user.HomeFacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load facility booking config")
	} else {
		maxMemberReservations = facility.MaxMemberReservations
	}

	var participant dbgen.ReservationParticipant
	var session dbgen.GetOpenPlaySessionRow
	var rule dbgen.OpenPlayRule
	err = database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		if maxMemberReservations > 0 {
			activeCount, err := qtx.CountActiveMemberReservations(ctx, dbgen.CountActiveMemberReservationsParams{
				FacilityID:    *user.HomeFacilityID,
				PrimaryUserID: sql.NullInt64{Int64: user.ID, Valid: true},
			})
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check reservation limits", Err: err}
			}
			if activeCount >= maxMemberReservations {
				return reservationLimitError{currentCount: activeCount, limit: maxMemberReservations}
			}
		}

		session, err = qtx.GetOpenPlaySession(ctx, dbgen.GetOpenPlaySessionParams{
			ID:         sessionID,
			FacilityID: *user.HomeFacilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Open play session not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to fetch open play session", Err: err}
		}
		if session.Status != "scheduled" {
			return apiutil.HandlerError{Status: http.StatusBadRequest, Message: "Open play session is not scheduled"}
		}
		if !session.StartTime.After(time.Now()) {
			return apiutil.HandlerError{Status: http.StatusBadRequest, Message: "Open play session must be in the future"}
		}

		rule, err = qtx.GetOpenPlayRule(ctx, dbgen.GetOpenPlayRuleParams{
			ID:         session.OpenPlayRuleID,
			FacilityID: *user.HomeFacilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Open play rule not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to fetch open play rule", Err: err}
		}
		maxParticipants := rule.MaxParticipantsPerCourt * session.CurrentCourtCount
		if session.ParticipantCount >= maxParticipants {
			return apiutil.HandlerError{Status: http.StatusConflict, Message: "Session is full"}
		}

		isParticipant, err := qtx.IsMemberOpenPlayParticipant(ctx, dbgen.IsMemberOpenPlayParticipantParams{
			SessionID:  sessionID,
			FacilityID: *user.HomeFacilityID,
			UserID:     user.ID,
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check open play participation", Err: err}
		}
		if isParticipant > 0 {
			return apiutil.HandlerError{Status: http.StatusConflict, Message: "Already signed up"}
		}

		if err := ensureOpenPlayReservation(ctx, qtx, session, *user.HomeFacilityID); err != nil {
			return err
		}

		participant, err = qtx.AddOpenPlayParticipant(ctx, dbgen.AddOpenPlayParticipantParams{
			UserID:         user.ID,
			FacilityID:     *user.HomeFacilityID,
			OpenPlayRuleID: sql.NullInt64{Int64: session.OpenPlayRuleID, Valid: true},
			StartTime:      session.StartTime,
			EndTime:        session.EndTime,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Open play reservation not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to add open play participant", Err: err}
		}

		updatedSession, err := qtx.GetOpenPlaySession(ctx, dbgen.GetOpenPlaySessionParams{
			ID:         sessionID,
			FacilityID: *user.HomeFacilityID,
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to verify open play capacity", Err: err}
		}
		if updatedSession.ParticipantCount > maxParticipants {
			return apiutil.HandlerError{Status: http.StatusConflict, Message: "Session is full"}
		}

		return nil
	})
	if err != nil {
		var limitErr reservationLimitError
		if errors.As(err, &limitErr) {
			message := fmt.Sprintf("You have reached the maximum of %d active reservations", limitErr.limit)
			http.Error(w, message, http.StatusConflict)
			return
		}
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			if herr.Status == http.StatusInternalServerError {
				logger.Error().Err(herr.Err).Int64("session_id", sessionID).Msg(herr.Message)
			}
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("session_id", sessionID).Msg("Failed to sign up for open play")
		http.Error(w, "Failed to sign up for open play", http.StatusInternalServerError)
		return
	}

	if emailClient != nil && facility.ID != 0 {
		facilityLoc := time.Local
		if facility.Timezone != "" {
			if loadedLoc, loadErr := time.LoadLocation(facility.Timezone); loadErr == nil {
				facilityLoc = loadedLoc
			} else {
				logger.Error().Err(loadErr).Str("timezone", facility.Timezone).Msg("Failed to load facility timezone for confirmation email")
			}
		}
		date, timeRange := email.FormatDateTimeRange(session.StartTime.In(facilityLoc), session.EndTime.In(facilityLoc))
		courtsLabel := fmt.Sprintf("%d courts", session.CurrentCourtCount)
		if session.CurrentCourtCount == 1 {
			courtsLabel = "1 court"
		}
		cancellationPolicy := fmt.Sprintf("Cancel at least %d minutes before start time to avoid penalties.", rule.CancellationCutoffMinutes)
		confirmation := email.BuildOpenPlayConfirmation(email.ConfirmationDetails{
			FacilityName:       facility.Name,
			Date:               date,
			TimeRange:          timeRange,
			Courts:             courtsLabel,
			CancellationPolicy: cancellationPolicy,
		})
		email.SendConfirmationEmail(ctx, q, emailClient, user.ID, confirmation, logger)
	}

	w.Header().Set("HX-Trigger", "refreshMemberReservations,refreshMemberOpenPlay")
	if err := apiutil.WriteJSON(w, http.StatusCreated, participant); err != nil {
		logger.Error().Err(err).Int64("session_id", sessionID).Msg("Failed to write open play signup response")
		return
	}
}

// HandleMemberOpenPlayCancel handles DELETE /member/openplay/{id}.
func HandleMemberOpenPlayCancel(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := loadQueries()
	database := loadDB()
	if q == nil || database == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sessionID, err := memberOpenPlaySessionIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if user.HomeFacilityID == nil {
		http.Error(w, "Home facility is required", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	err = database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		session, err := qtx.GetOpenPlaySession(ctx, dbgen.GetOpenPlaySessionParams{
			ID:         sessionID,
			FacilityID: *user.HomeFacilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Open play session not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to fetch open play session", Err: err}
		}
		if session.Status != "scheduled" {
			return apiutil.HandlerError{Status: http.StatusBadRequest, Message: "Open play session is not scheduled"}
		}
		now := time.Now()
		if !session.StartTime.After(now) {
			return apiutil.HandlerError{Status: http.StatusBadRequest, Message: "Open play session must be in the future"}
		}

		isParticipant, err := qtx.IsMemberOpenPlayParticipant(ctx, dbgen.IsMemberOpenPlayParticipantParams{
			SessionID:  sessionID,
			FacilityID: *user.HomeFacilityID,
			UserID:     user.ID,
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check open play participation", Err: err}
		}
		if isParticipant == 0 {
			return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Open play participant not found"}
		}

		rule, err := qtx.GetOpenPlayRule(ctx, dbgen.GetOpenPlayRuleParams{
			ID:         session.OpenPlayRuleID,
			FacilityID: *user.HomeFacilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Open play rule not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to fetch open play rule", Err: err}
		}
		if rule.CancellationCutoffMinutes > 0 {
			cutoff := session.StartTime.Add(-time.Duration(rule.CancellationCutoffMinutes) * time.Minute)
			if !now.Before(cutoff) {
				return apiutil.HandlerError{Status: http.StatusConflict, Message: "Cancellation cutoff has passed"}
			}
		}

		if err := ensureOpenPlayReservation(ctx, qtx, session, *user.HomeFacilityID); err != nil {
			return err
		}

		removed, err := qtx.RemoveOpenPlayParticipant(ctx, dbgen.RemoveOpenPlayParticipantParams{
			UserID:         user.ID,
			FacilityID:     *user.HomeFacilityID,
			OpenPlayRuleID: sql.NullInt64{Int64: session.OpenPlayRuleID, Valid: true},
			StartTime:      session.StartTime,
			EndTime:        session.EndTime,
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to cancel open play signup", Err: err}
		}
		if removed == 0 {
			return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Open play participant not found"}
		}

		return nil
	})
	if err != nil {
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			if herr.Status == http.StatusInternalServerError {
				logger.Error().Err(herr.Err).Int64("session_id", sessionID).Msg(herr.Message)
			}
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("session_id", sessionID).Msg("Failed to cancel open play signup")
		http.Error(w, "Failed to cancel open play signup", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshMemberReservations,refreshMemberOpenPlay")
	w.WriteHeader(http.StatusNoContent)
}

func lookupReservationTypeID(ctx context.Context, q *dbgen.Queries, name string) (int64, error) {
	resType, err := q.GetReservationTypeByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("reservation type %q not found", name)
		}
		return 0, err
	}
	return resType.ID, nil
}

func hoursUntilReservationStart(start time.Time, now time.Time) int64 {
	hours := int64(start.Sub(now).Hours())
	if hours < 0 {
		return 0
	}
	return hours
}

func requestCancellationConfirm(r *http.Request) bool {
	rawConfirm := strings.TrimSpace(r.URL.Query().Get("confirm"))
	if rawConfirm == "" {
		if err := r.ParseForm(); err == nil {
			rawConfirm = strings.TrimSpace(r.FormValue("confirm"))
		}
	}
	return apiutil.ParseBool(rawConfirm)
}

func requestCancellationPenaltyDetails(r *http.Request) (time.Time, int64, bool) {
	rawCalculatedAt := strings.TrimSpace(r.URL.Query().Get("penalty_calculated_at"))
	rawHours := strings.TrimSpace(r.URL.Query().Get("hours_before_start"))
	if rawCalculatedAt == "" || rawHours == "" {
		if err := r.ParseForm(); err == nil {
			if rawCalculatedAt == "" {
				rawCalculatedAt = strings.TrimSpace(r.FormValue("penalty_calculated_at"))
			}
			if rawHours == "" {
				rawHours = strings.TrimSpace(r.FormValue("hours_before_start"))
			}
		}
	}
	if rawCalculatedAt == "" || rawHours == "" {
		return time.Time{}, 0, false
	}
	hoursBeforeStart, err := strconv.ParseInt(rawHours, 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	calculatedAt, err := time.Parse(time.RFC3339Nano, rawCalculatedAt)
	if err != nil {
		calculatedAt, err = time.Parse(time.RFC3339, rawCalculatedAt)
		if err != nil {
			return time.Time{}, 0, false
		}
	}
	return calculatedAt, hoursBeforeStart, true
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
	for i := range summaries {
		if summaries[i].StartTime.After(now) {
			hoursUntilReservation := hoursUntilReservationStart(summaries[i].StartTime, now)
			refundPercentage, err := apiutil.ApplicableRefundPercentage(ctx, q, summaries[i].FacilityID, hoursUntilReservation, &summaries[i].ReservationTypeID)
			if err != nil {
				logger.Error().Err(err).Int64("facility_id", summaries[i].FacilityID).Msg("Failed to load cancellation policy refund percentage")
				refundPercentage = 100
			}
			summaries[i].RefundPercentage = refundPercentage
			upcoming = append(upcoming, summaries[i])
		} else {
			past = append(past, summaries[i])
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

func buildReservationWidgetData(
	ctx context.Context,
	q *dbgen.Queries,
	userID int64,
) (membertempl.ReservationWidgetData, error) {
	rows, err := q.ListReservationsByUserID(ctx, sql.NullInt64{Int64: userID, Valid: true})
	if err != nil {
		return membertempl.ReservationWidgetData{}, err
	}

	summaries := membertempl.NewReservationSummaries(rows)
	now := time.Now()
	upcoming := make([]membertempl.ReservationSummary, 0, len(summaries))
	for _, summary := range summaries {
		if summary.StartTime.After(now) {
			upcoming = append(upcoming, summary)
		}
	}

	sort.Slice(upcoming, func(i, j int) bool {
		return upcoming[i].StartTime.Before(upcoming[j].StartTime)
	})

	return membertempl.NewReservationWidgetData(upcoming), nil
}

func bookingDateFromRequest(r *http.Request, maxAdvanceDays int64) time.Time {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if maxAdvanceDays <= 0 {
		maxAdvanceDays = memberBookingDefaultMaxAdvanceDays
	}
	maxDate := today.AddDate(0, 0, int(maxAdvanceDays))

	dateParam := strings.TrimSpace(r.URL.Query().Get("date"))
	if dateParam != "" {
		parsed, err := time.ParseInLocation("2006-01-02", dateParam, now.Location())
		if err == nil {
			return clampBookingDate(parsed, today, maxDate)
		}
	}

	yearParam := strings.TrimSpace(r.URL.Query().Get("booking_year"))
	monthParam := strings.TrimSpace(r.URL.Query().Get("booking_month"))
	dayParam := strings.TrimSpace(r.URL.Query().Get("booking_day"))
	if yearParam == "" || monthParam == "" || dayParam == "" {
		return today
	}

	year, err := strconv.Atoi(yearParam)
	if err != nil {
		return today
	}
	month, err := strconv.Atoi(monthParam)
	if err != nil || month < 1 || month > 12 {
		return today
	}
	day, err := strconv.Atoi(dayParam)
	if err != nil || day < 1 {
		return today
	}

	daysInMonth := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, now.Location()).Day()
	if day > daysInMonth {
		day = daysInMonth
	}
	selected := time.Date(year, time.Month(month), day, 0, 0, 0, 0, now.Location())
	return clampBookingDate(selected, today, maxDate)
}

func clampBookingDate(selected time.Time, min time.Time, max time.Time) time.Time {
	if selected.Before(min) {
		return min
	}
	if selected.After(max) {
		return max
	}
	return selected
}

func normalizedMaxAdvanceBookingDays(value int64) int64 {
	if value <= 0 {
		return memberBookingDefaultMaxAdvanceDays
	}
	return value
}

func buildMemberBookingSlots(
	ctx context.Context,
	q *dbgen.Queries,
	facilityID int64,
	baseDate time.Time,
	logger *zerolog.Logger,
) ([]membertempl.MemberBookingSlot, error) {
	opensAt := memberBookingDefaultOpensAt
	closesAt := memberBookingDefaultClosesAt

	hours, err := q.GetFacilityHours(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load operating hours")
		hours = nil
	}
	weekday := int64(baseDate.Weekday())
	for _, hour := range hours {
		if hour.DayOfWeek == weekday {
			opensAtRaw := apiutil.FormatOperatingHourValue(hour.OpensAt)
			if strings.TrimSpace(opensAtRaw) != "" {
				opensAt = opensAtRaw
			}
			closesAtRaw := apiutil.FormatOperatingHourValue(hour.ClosesAt)
			if strings.TrimSpace(closesAtRaw) != "" {
				closesAt = closesAtRaw
			}
			break
		}
	}

	openTime, err := parseBookingTimeOfDay(opensAt, "opens_at")
	if err != nil {
		openTime, _ = parseBookingTimeOfDay(memberBookingDefaultOpensAt, "opens_at")
	}
	closeTime, err := parseBookingTimeOfDay(closesAt, "closes_at")
	if err != nil {
		closeTime, _ = parseBookingTimeOfDay(memberBookingDefaultClosesAt, "closes_at")
	}

	dayOpen := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(), openTime.Hour(), openTime.Minute(), 0, 0, baseDate.Location())
	dayClose := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(), closeTime.Hour(), closeTime.Minute(), 0, 0, baseDate.Location())
	if !dayClose.After(dayOpen) {
		return nil, nil
	}

	slotStart := dayOpen
	now := time.Now().In(baseDate.Location())
	if sameDay(now, baseDate) && now.After(dayOpen) {
		slotStart = roundUpToHour(now)
		if slotStart.Before(dayOpen) {
			slotStart = dayOpen
		}
	}

	var slots []membertempl.MemberBookingSlot
	for start := slotStart; start.Add(memberBookingMinDuration).Before(dayClose) || start.Add(memberBookingMinDuration).Equal(dayClose); start = start.Add(memberBookingMinDuration) {
		available, err := q.ListAvailableCourts(ctx, dbgen.ListAvailableCourtsParams{
			FacilityID:    facilityID,
			ReservationID: 0,
			StartTime:     start,
			EndTime:       start.Add(memberBookingMinDuration),
		})
		if err != nil {
			return nil, err
		}
		if len(available) == 0 {
			continue
		}
		slots = append(slots, membertempl.MemberBookingSlot{
			StartTime: start,
			EndTime:   start.Add(memberBookingMinDuration),
		})
	}
	return slots, nil
}

func waitlistFallbackTimes(baseDate time.Time, slots []membertempl.MemberBookingSlot) (time.Time, time.Time) {
	if len(slots) > 0 {
		return slots[0].StartTime, slots[0].EndTime
	}
	openTime, err := parseBookingTimeOfDay(memberBookingDefaultOpensAt, "opens_at")
	if err != nil {
		start := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(), 9, 0, 0, 0, baseDate.Location())
		return start, start.Add(memberBookingMinDuration)
	}
	start := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(), openTime.Hour(), openTime.Minute(), 0, 0, baseDate.Location())
	return start, start.Add(memberBookingMinDuration)
}

func parseBookingTimeOfDay(raw string, field string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("%s is required", field)
	}
	parsed, err := time.Parse("15:04", raw)
	if err != nil {
		parsed, err = time.Parse("3:04 PM", strings.ToUpper(raw))
		if err != nil {
			return time.Time{}, fmt.Errorf("%s must be in HH:MM or H:MM AM/PM format", field)
		}
	}
	return parsed, nil
}

func parseMemberBookingTime(raw string, field string, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("%s is required", field)
	}
	parsed, err := time.ParseInLocation(memberBookingTimeLayout, raw, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be in YYYY-MM-DDTHH:MM format", field)
	}
	return parsed, nil
}

func buildWaitlistEntrySummaries(ctx context.Context, q *dbgen.Queries, rows []dbgen.Waitlist, logger *zerolog.Logger) []waitlisttempl.WaitlistEntry {
	entries := make([]waitlisttempl.WaitlistEntry, 0, len(rows))
	facilityNames := make(map[int64]string)
	courtNames := make(map[int64]string)

	for _, row := range rows {
		facilityName, ok := facilityNames[row.FacilityID]
		if !ok {
			facility, err := q.GetFacilityByID(ctx, row.FacilityID)
			if err != nil {
				logger.Error().Err(err).Int64("facility_id", row.FacilityID).Msg("Failed to load facility for waitlist entry")
			} else {
				facilityName = facility.Name
			}
			if strings.TrimSpace(facilityName) == "" {
				facilityName = fmt.Sprintf("Facility %d", row.FacilityID)
			}
			facilityNames[row.FacilityID] = facilityName
		}

		var courtName string
		if row.TargetCourtID.Valid {
			if cached, ok := courtNames[row.TargetCourtID.Int64]; ok {
				courtName = cached
			} else {
				court, err := q.GetCourt(ctx, row.TargetCourtID.Int64)
				if err != nil {
					logger.Error().Err(err).Int64("court_id", row.TargetCourtID.Int64).Msg("Failed to load court for waitlist entry")
				} else {
					courtName = court.Name
				}
				courtNames[row.TargetCourtID.Int64] = courtName
			}
		}

		startTime, err := waitlistDateTime(row.TargetDate, row.TargetStartTime)
		if err != nil {
			logger.Error().Err(err).Int64("waitlist_id", row.ID).Msg("Failed to parse waitlist start time")
			startTime = row.TargetDate
		}
		endTime, err := waitlistDateTime(row.TargetDate, row.TargetEndTime)
		if err != nil {
			logger.Error().Err(err).Int64("waitlist_id", row.ID).Msg("Failed to parse waitlist end time")
			endTime = row.TargetDate
		}

		entries = append(entries, waitlisttempl.WaitlistEntry{
			ID:           row.ID,
			FacilityID:   row.FacilityID,
			FacilityName: facilityName,
			CourtName:    courtName,
			StartTime:    startTime,
			EndTime:      endTime,
			Position:     row.Position,
			Status:       row.Status,
		})
	}

	return entries
}

func waitlistDateTime(targetDate time.Time, value interface{}) (time.Time, error) {
	raw := strings.TrimSpace(waitlistTimeValue(value))
	if raw == "" {
		return time.Time{}, fmt.Errorf("waitlist time is required")
	}
	layouts := []string{"15:04:05", "15:04"}
	for _, layout := range layouts {
		parsed, err := time.ParseInLocation(layout, raw, targetDate.Location())
		if err == nil {
			return time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), 0, targetDate.Location()), nil
		}
	}
	return time.Time{}, fmt.Errorf("waitlist time must be in HH:MM or HH:MM:SS format")
}

func waitlistTimeValue(value interface{}) string {
	switch typed := value.(type) {
	case time.Time:
		return typed.Format("15:04:05")
	case []byte:
		return string(typed)
	case string:
		return typed
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}

func parseMemberCourtID(r *http.Request) (int64, error) {
	values := r.Form["court_ids"]
	if len(values) == 0 {
		values = r.Form["court_ids[]"]
	}
	if len(values) == 0 {
		return 0, fmt.Errorf("court_ids is required")
	}
	value := strings.TrimSpace(values[0])
	if value == "" {
		return 0, fmt.Errorf("court_ids is required")
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("court_ids must be a positive integer")
	}
	return id, nil
}

func parseOptionalPositiveInt64(value string, field string) (int64, bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false, nil
	}
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || id <= 0 {
		return 0, false, fmt.Errorf("%s must be a positive integer", field)
	}
	return id, true, nil
}

func memberOpenPlaySessionIDFromRequest(r *http.Request) (int64, error) {
	pathID := strings.TrimSpace(r.PathValue("id"))
	if pathID == "" {
		return 0, fmt.Errorf("invalid session ID")
	}
	id, err := strconv.ParseInt(pathID, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid session ID")
	}
	return id, nil
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func roundUpToHour(value time.Time) time.Time {
	truncated := time.Date(value.Year(), value.Month(), value.Day(), value.Hour(), 0, 0, 0, value.Location())
	if value.After(truncated) {
		return truncated.Add(time.Hour)
	}
	return truncated
}
