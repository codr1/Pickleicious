// internal/api/reservations/handlers.go
package reservations

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	appdb "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/email"
	"github.com/codr1/Pickleicious/internal/request"
	reservationstempl "github.com/codr1/Pickleicious/internal/templates/components/reservations"
)

var (
	queries     *dbgen.Queries
	store       *appdb.DB
	queriesOnce sync.Once
	emailClient *email.SESClient
)

const (
	reservationQueryTimeout           = 5 * time.Second
	waitlistNotificationTimeout       = 5 * time.Second
	minReservationDuration            = time.Hour
	timeLayoutDatetimeLocal           = "2006-01-02T15:04"
	timeLayoutDatetimeMinute          = "2006-01-02 15:04"
	waitlistTimeLayout                = "15:04:05"
	defaultWaitlistOfferExpiryMinutes = int64(30)
)

const (
	waitlistNotificationBroadcast  = "broadcast"
	waitlistNotificationSequential = "sequential"
	waitlistOfferStatusPending     = "pending"
	waitlistStatusNotified         = "notified"
)

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

// POST /api/v1/reservations
func HandleReservationCreate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	database := loadDB()
	if q == nil || database == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	req, err := decodeReservationRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityID, err := resolveFacilityID(r, req.FacilityID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.FacilityID = facilityID

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), reservationQueryTimeout)
	defer cancel()

	exists, err := facilityExists(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to validate facility")
		http.Error(w, "Failed to validate facility", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Facility not found", http.StatusNotFound)
		return
	}

	startTime, endTime, err := parseReservationTimes(req.StartTime, req.EndTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateReservationInput(req, startTime, endTime); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !user.IsStaff {
		authUserID := user.ID
		if req.PrimaryUserID != nil && *req.PrimaryUserID > 0 && *req.PrimaryUserID != authUserID {
			http.Error(w, "primary_user_id must match authenticated user", http.StatusForbidden)
			return
		}
		req.PrimaryUserID = &authUserID
		if err := enforceMemberTierBookingWindow(ctx, q, facilityID, req.PrimaryUserID, startTime); err != nil {
			var fieldErr apiutil.FieldError
			if errors.As(err, &fieldErr) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to validate member booking window")
			http.Error(w, "Failed to validate booking window", http.StatusInternalServerError)
			return
		}
	}

	req.CourtIDs = normalizeCourtIDs(req.CourtIDs)
	if req.ParticipantIDsSet {
		req.ParticipantIDs = normalizeParticipantIDs(req.ParticipantIDs)
	}
	if err := apiutil.EnsureCourtsAvailable(ctx, q, facilityID, 0, startTime, endTime, req.CourtIDs); err != nil {
		var availErr apiutil.AvailabilityError
		if errors.As(err, &availErr) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to check court availability")
		http.Error(w, "Failed to check court availability", http.StatusInternalServerError)
		return
	}

	var created dbgen.Reservation
	err = database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		created, err = qtx.CreateReservation(ctx, dbgen.CreateReservationParams{
			FacilityID:        facilityID,
			ReservationTypeID: req.ReservationTypeID,
			RecurrenceRuleID:  apiutil.ToNullInt64(req.RecurrenceRuleID),
			PrimaryUserID:     apiutil.ToNullInt64(req.PrimaryUserID),
			CreatedByUserID:   user.ID,
			ProID:             apiutil.ToNullInt64(req.ProID),
			OpenPlayRuleID:    apiutil.ToNullInt64(req.OpenPlayRuleID),
			StartTime:         startTime,
			EndTime:           endTime,
			IsOpenEvent:       req.IsOpenEvent,
			TeamsPerCourt:     apiutil.ToNullInt64(req.TeamsPerCourt),
			PeoplePerTeam:     apiutil.ToNullInt64(req.PeoplePerTeam),
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to create reservation", Err: err}
		}

		for _, courtID := range req.CourtIDs {
			if err := qtx.AddReservationCourt(ctx, dbgen.AddReservationCourtParams{
				ReservationID: created.ID,
				CourtID:       courtID,
			}); err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to assign courts", Err: err}
			}
		}
		for _, participantID := range req.ParticipantIDs {
			if err := qtx.AddParticipant(ctx, dbgen.AddParticipantParams{
				ReservationID: created.ID,
				UserID:        participantID,
			}); err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to add reservation participant", Err: err}
			}
		}
		return nil
	})
	if err != nil {
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			logger.Error().Err(herr.Err).Int64("facility_id", facilityID).Msg(herr.Message)
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to create reservation")
		http.Error(w, "Failed to create reservation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshCourtsCalendar")
	if err := apiutil.WriteJSON(w, http.StatusCreated, created); err != nil {
		logger.Error().Err(err).Int64("reservation_id", created.ID).Msg("Failed to write reservation response")
		return
	}
}

// GET /api/v1/reservations?facility_id=...&start_time=...&end_time=...
func HandleReservationsList(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	startTime, endTime, err := parseReservationTimes(r.URL.Query().Get("start_time"), r.URL.Query().Get("end_time"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !endTime.After(startTime) {
		http.Error(w, "end_time must be after start_time", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), reservationQueryTimeout)
	defer cancel()

	reservations, err := q.ListReservationsByDateRange(ctx, dbgen.ListReservationsByDateRangeParams{
		FacilityID: facilityID,
		StartTime:  startTime,
		EndTime:    endTime,
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to list reservations")
		http.Error(w, "Failed to list reservations", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, reservations); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to write reservation list response")
		return
	}
}

// GET /api/v1/events/booking/new
func HandleEventBookingFormNew(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, ok := request.FacilityIDFromBookingRequest(r)
	if !ok {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}
	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	hourValue := strings.TrimSpace(r.URL.Query().Get("hour"))
	hour, hourErr := strconv.Atoi(hourValue)

	now := time.Now()
	baseDate := now
	dateValue := strings.TrimSpace(r.URL.Query().Get("date"))
	if dateValue != "" {
		parsedDate, err := time.ParseInLocation("2006-01-02", dateValue, now.Location())
		if err == nil {
			baseDate = parsedDate
		}
	}

	startHour := now.Hour()
	if hourErr == nil && hour >= 0 && hour <= 23 {
		startHour = hour
	}
	startTime := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(), startHour, 0, 0, 0, baseDate.Location())
	endTime := startTime.Add(time.Hour)

	ctx, cancel := context.WithTimeout(r.Context(), reservationQueryTimeout)
	defer cancel()

	courtsList, err := q.ListCourts(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load courts")
		http.Error(w, "Failed to load courts", http.StatusInternalServerError)
		return
	}

	reservationTypes, err := q.ListReservationTypes(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load reservation types")
		http.Error(w, "Failed to load reservation types", http.StatusInternalServerError)
		return
	}

	memberRows, err := q.ListMembers(ctx, dbgen.ListMembersParams{
		FacilityID: sql.NullInt64{},
		SearchTerm: nil,
		Offset:     0,
		Limit:      50,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load members for booking form")
		memberRows = nil
	}

	var buf bytes.Buffer
	component := reservationstempl.EventBookingForm(reservationstempl.EventBookingFormData{
		FacilityID:       facilityID,
		StartTime:        startTime,
		EndTime:          endTime,
		Courts:           reservationstempl.NewCourtOptions(courtsList),
		ReservationTypes: reservationstempl.NewReservationTypeOptions(reservationTypes),
		Members:          reservationstempl.NewMemberOptions(memberRows),
	})
	if err := component.Render(r.Context(), &buf); err != nil {
		logger.Error().Err(err).Msg("Failed to render event booking form")
		http.Error(w, "Failed to render booking form", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf.Bytes()); err != nil {
		logger.Error().Err(err).Msg("Failed to write response")
	}
}

// GET /api/v1/reservations/{id}/edit
func HandleReservationEdit(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	reservationID, err := apiutil.ReservationIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid reservation ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), reservationQueryTimeout)
	defer cancel()

	reservation, err := q.GetReservationByID(ctx, reservationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Reservation not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to fetch reservation")
		http.Error(w, "Failed to fetch reservation", http.StatusInternalServerError)
		return
	}
	facilityID := reservation.FacilityID

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	courtsList, err := q.ListCourts(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load courts")
		http.Error(w, "Failed to load courts", http.StatusInternalServerError)
		return
	}

	reservationTypes, err := q.ListReservationTypes(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load reservation types")
		http.Error(w, "Failed to load reservation types", http.StatusInternalServerError)
		return
	}

	reservationCourts, err := q.ListReservationCourts(ctx, reservationID)
	if err != nil {
		logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to load reservation courts")
		http.Error(w, "Failed to load reservation courts", http.StatusInternalServerError)
		return
	}

	memberRows, err := q.ListMembers(ctx, dbgen.ListMembersParams{
		FacilityID: sql.NullInt64{},
		SearchTerm: nil,
		Offset:     0,
		Limit:      50,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load members for booking form")
		memberRows = nil
	}

	var selectedCourtID int64
	if len(reservationCourts) > 0 {
		selectedCourtID = reservationCourts[0].CourtID
	}

	var primaryUserID *int64
	if reservation.PrimaryUserID.Valid {
		value := reservation.PrimaryUserID.Int64
		primaryUserID = &value
	}

	var teamsPerCourt *int64
	if reservation.TeamsPerCourt.Valid {
		value := reservation.TeamsPerCourt.Int64
		teamsPerCourt = &value
	}

	var peoplePerTeam *int64
	if reservation.PeoplePerTeam.Valid {
		value := reservation.PeoplePerTeam.Int64
		peoplePerTeam = &value
	}

	var buf bytes.Buffer
	component := reservationstempl.BookingForm(reservationstempl.BookingFormData{
		FacilityID:                facilityID,
		StartTime:                 reservation.StartTime,
		EndTime:                   reservation.EndTime,
		Courts:                    reservationstempl.NewCourtOptions(courtsList),
		ReservationTypes:          reservationstempl.NewReservationTypeOptions(reservationTypes),
		Members:                   reservationstempl.NewMemberOptions(memberRows),
		SelectedCourtID:           selectedCourtID,
		SelectedReservationTypeID: reservation.ReservationTypeID,
		PrimaryUserID:             primaryUserID,
		IsOpenEvent:               reservation.IsOpenEvent,
		TeamsPerCourt:             teamsPerCourt,
		PeoplePerTeam:             peoplePerTeam,
		IsEdit:                    true,
		ReservationID:             reservationID,
	})
	if err := component.Render(r.Context(), &buf); err != nil {
		logger.Error().Err(err).Msg("Failed to render reservation edit form")
		http.Error(w, "Failed to render reservation edit form", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf.Bytes()); err != nil {
		logger.Error().Err(err).Msg("Failed to write response")
	}
}

// PUT /api/v1/reservations/{id}
func HandleReservationUpdate(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), reservationQueryTimeout)
	defer cancel()

	reservation, err := q.GetReservationByID(ctx, reservationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Reservation not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to fetch reservation")
		http.Error(w, "Failed to fetch reservation", http.StatusInternalServerError)
		return
	}
	facilityID := reservation.FacilityID

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}
	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	req, err := decodeReservationRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.FacilityID != 0 && req.FacilityID != facilityID {
		http.Error(w, "facility_id mismatch between reservation and payload", http.StatusBadRequest)
		return
	}
	req.FacilityID = facilityID

	startTime, endTime, err := parseReservationTimes(req.StartTime, req.EndTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateReservationInput(req, startTime, endTime); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !user.IsStaff {
		authUserID := user.ID
		if req.PrimaryUserID != nil && *req.PrimaryUserID > 0 && *req.PrimaryUserID != authUserID {
			http.Error(w, "primary_user_id must match authenticated user", http.StatusForbidden)
			return
		}
		req.PrimaryUserID = &authUserID
		if err := enforceMemberTierBookingWindow(ctx, q, facilityID, req.PrimaryUserID, startTime); err != nil {
			var fieldErr apiutil.FieldError
			if errors.As(err, &fieldErr) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to validate member booking window")
			http.Error(w, "Failed to validate booking window", http.StatusInternalServerError)
			return
		}
	}

	req.CourtIDs = normalizeCourtIDs(req.CourtIDs)
	if req.ParticipantIDsSet {
		req.ParticipantIDs = normalizeParticipantIDs(req.ParticipantIDs)
	}

	var updated dbgen.Reservation
	err = database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		_, err := qtx.GetReservation(ctx, dbgen.GetReservationParams{
			ID:         reservationID,
			FacilityID: facilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Reservation not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to fetch reservation", Err: err}
		}

		if err := apiutil.EnsureCourtsAvailable(ctx, qtx, facilityID, reservationID, startTime, endTime, req.CourtIDs); err != nil {
			var availErr apiutil.AvailabilityError
			if errors.As(err, &availErr) {
				return apiutil.HandlerError{Status: http.StatusConflict, Message: err.Error(), Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check court availability", Err: err}
		}

		updated, err = qtx.UpdateReservation(ctx, dbgen.UpdateReservationParams{
			ID:                reservationID,
			FacilityID:        facilityID,
			ReservationTypeID: req.ReservationTypeID,
			RecurrenceRuleID:  apiutil.ToNullInt64(req.RecurrenceRuleID),
			PrimaryUserID:     apiutil.ToNullInt64(req.PrimaryUserID),
			ProID:             apiutil.ToNullInt64(req.ProID),
			OpenPlayRuleID:    apiutil.ToNullInt64(req.OpenPlayRuleID),
			StartTime:         startTime,
			EndTime:           endTime,
			IsOpenEvent:       req.IsOpenEvent,
			TeamsPerCourt:     apiutil.ToNullInt64(req.TeamsPerCourt),
			PeoplePerTeam:     apiutil.ToNullInt64(req.PeoplePerTeam),
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to update reservation", Err: err}
		}

		existingCourts, err := qtx.ListReservationCourts(ctx, reservationID)
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load reservation courts", Err: err}
		}

		existing := make(map[int64]struct{}, len(existingCourts))
		for _, court := range existingCourts {
			existing[court.CourtID] = struct{}{}
		}
		next := make(map[int64]struct{}, len(req.CourtIDs))
		for _, courtID := range req.CourtIDs {
			next[courtID] = struct{}{}
		}

		for courtID := range existing {
			if _, ok := next[courtID]; ok {
				continue
			}
			if err := qtx.RemoveReservationCourt(ctx, dbgen.RemoveReservationCourtParams{
				ReservationID: reservationID,
				CourtID:       courtID,
			}); err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to remove reservation court", Err: err}
			}
		}

		for courtID := range next {
			if _, ok := existing[courtID]; ok {
				continue
			}
			if err := qtx.AddReservationCourt(ctx, dbgen.AddReservationCourtParams{
				ReservationID: reservationID,
				CourtID:       courtID,
			}); err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to add reservation court", Err: err}
			}
		}

		if req.ParticipantIDsSet {
			existingParticipants, err := qtx.ListParticipantsForReservation(ctx, reservationID)
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load reservation participants", Err: err}
			}

			existingParticipantIDs := make(map[int64]struct{}, len(existingParticipants))
			for _, participant := range existingParticipants {
				existingParticipantIDs[participant.ID] = struct{}{}
			}
			nextParticipantIDs := make(map[int64]struct{}, len(req.ParticipantIDs))
			for _, participantID := range req.ParticipantIDs {
				nextParticipantIDs[participantID] = struct{}{}
			}

			for participantID := range existingParticipantIDs {
				if _, ok := nextParticipantIDs[participantID]; ok {
					continue
				}
				if err := qtx.RemoveParticipant(ctx, dbgen.RemoveParticipantParams{
					ReservationID: reservationID,
					UserID:        participantID,
				}); err != nil {
					return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to remove reservation participant", Err: err}
				}
			}

			for participantID := range nextParticipantIDs {
				if _, ok := existingParticipantIDs[participantID]; ok {
					continue
				}
				if err := qtx.AddParticipant(ctx, dbgen.AddParticipantParams{
					ReservationID: reservationID,
					UserID:        participantID,
				}); err != nil {
					return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to add reservation participant", Err: err}
				}
			}
		}

		return nil
	})
	if err != nil {
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			if herr.Status == http.StatusInternalServerError {
				logger.Error().Err(herr.Err).Int64("reservation_id", reservationID).Msg(herr.Message)
			}
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to update reservation")
		http.Error(w, "Failed to update reservation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshCourtsCalendar")
	if err := apiutil.WriteJSON(w, http.StatusOK, updated); err != nil {
		logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to write reservation response")
		return
	}
}

// DELETE /api/v1/reservations/{id}
func HandleReservationDelete(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), reservationQueryTimeout)
	defer cancel()

	deleteReq, err := decodeReservationDeleteRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	reservation, err := q.GetReservationByID(ctx, reservationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Reservation not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to fetch reservation")
		http.Error(w, "Failed to fetch reservation", http.StatusInternalServerError)
		return
	}
	facilityID := reservation.FacilityID

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	now := time.Now()
	hoursUntilReservation := hoursUntilReservationStart(reservation.StartTime, now)
	policyRefundPercentage, err := apiutil.ApplicableRefundPercentage(ctx, q, facilityID, hoursUntilReservation, &reservation.ReservationTypeID)
	if err != nil {
		logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to load cancellation policy")
		http.Error(w, "Failed to load cancellation policy", http.StatusInternalServerError)
		return
	}

	if deleteReq.WaiveFee != nil && *deleteReq.WaiveFee && !user.IsStaff {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if user.IsStaff && policyRefundPercentage < 100 && deleteReq.WaiveFee == nil {
		penalty := cancellationPenaltyResponse{
			ReservationID:     reservationID,
			RefundPercentage:  policyRefundPercentage,
			FeePercentage:     100 - policyRefundPercentage,
			HoursBeforeStart:  hoursUntilReservation,
			FacilityID:        facilityID,
			ReservationStarts: reservation.StartTime,
		}
		if apiutil.IsJSONRequest(r) {
			if err := apiutil.WriteJSON(w, http.StatusConflict, penalty); err != nil {
				logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to write cancellation penalty response")
			}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		if _, err := w.Write([]byte(renderCancellationPenaltyPrompt(penalty))); err != nil {
			logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to write cancellation penalty prompt")
		}
		return
	}

	refundPercentage := policyRefundPercentage
	feeWaived := false
	if deleteReq.WaiveFee != nil && *deleteReq.WaiveFee {
		refundPercentage = 100
		feeWaived = true
	}

	var reservationCourts []dbgen.ListReservationCourtsRow
	var reservationParticipants []dbgen.ListParticipantsForReservationRow
	var reservationTypeName string
	err = database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		_, err := qtx.GetReservation(ctx, dbgen.GetReservationParams{
			ID:         reservationID,
			FacilityID: facilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Reservation not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to fetch reservation", Err: err}
		}

		if _, err := qtx.LogCancellation(ctx, dbgen.LogCancellationParams{
			ReservationID:           reservationID,
			CancelledByUserID:       user.ID,
			CancelledAt:             now,
			RefundPercentageApplied: refundPercentage,
			FeeWaived:               feeWaived,
			HoursBeforeStart:        hoursUntilReservation,
		}); err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to log cancellation", Err: err}
		}

		loadedReservationTypeName, err := qtx.GetReservationTypeNameByReservationID(ctx, reservationID)
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load reservation type", Err: err}
		}
		reservationTypeName = loadedReservationTypeName
		if loadedReservationTypeName == "PRO_SESSION" {
			redemptions, err := qtx.ListLessonPackageRedemptionsByReservationID(ctx, sql.NullInt64{Int64: reservationID, Valid: true})
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load lesson package redemption", Err: err}
			}
			if len(redemptions) > 0 {
				for _, redemption := range redemptions {
					if _, err := qtx.RestoreLessonPackageLesson(ctx, redemption.LessonPackageID); err != nil {
						if errors.Is(err, sql.ErrNoRows) {
							logger.Info().Int64("lesson_package_id", redemption.LessonPackageID).Msg("Skipped lesson package restore (expired or already at max)")
							continue
						}
						return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to restore lesson package", Err: err}
					}
				}
				if err := qtx.DeleteLessonPackageRedemptionsByReservationID(ctx, sql.NullInt64{Int64: reservationID, Valid: true}); err != nil {
					return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to clear lesson package redemption", Err: err}
				}
			}
		}
		// Only notify pros for member-initiated lesson cancellations.
		if loadedReservationTypeName == "PRO_SESSION" && !user.IsStaff {
			if !reservation.ProID.Valid {
				logger.Error().Int64("reservation_id", reservationID).Msg("Missing pro for lesson cancellation notification")
			} else {
				memberName := "Member"
				if reservation.PrimaryUserID.Valid {
					member, err := qtx.GetMemberByID(ctx, reservation.PrimaryUserID.Int64)
					if err != nil && !errors.Is(err, sql.ErrNoRows) {
						return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load member details", Err: err}
					}
					if err == nil {
						memberName = strings.TrimSpace(fmt.Sprintf("%s %s", member.FirstName, member.LastName))
					}
					if memberName == "" {
						memberName = "Member"
					}
				}
				message := fmt.Sprintf(
					"Lesson cancelled: %s (%s - %s)",
					memberName,
					reservation.StartTime.Format(timeLayoutDatetimeMinute),
					reservation.EndTime.Format(timeLayoutDatetimeMinute),
				)
				if _, err := qtx.CreateLessonCancelledNotification(ctx, dbgen.CreateLessonCancelledNotificationParams{
					FacilityID: facilityID,
					Message:    message,
					RelatedReservationID: sql.NullInt64{
						Int64: reservationID,
						Valid: true,
					},
					TargetStaffID: reservation.ProID,
				}); err != nil {
					logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to notify pro about lesson cancellation")
				}
			}
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
		return nil
	})
	if err != nil {
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			if herr.Status == http.StatusInternalServerError {
				logger.Error().Err(herr.Err).Int64("reservation_id", reservationID).Msg(herr.Message)
			}
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to delete reservation")
		http.Error(w, "Failed to delete reservation", http.StatusInternalServerError)
		return
	}

	if emailClient != nil && reservation.ID != 0 {
		emailCtx, emailCancel := context.WithTimeout(context.Background(), reservationQueryTimeout)
		defer emailCancel()
		queryCtx, queryCancel := context.WithTimeout(context.Background(), reservationQueryTimeout)
		defer queryCancel()
		facility, err := q.GetFacilityByID(queryCtx, facilityID)
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load facility for cancellation email")
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
				ReservationType:  reservationTypeName,
				Date:             date,
				TimeRange:        timeRange,
				Courts:           courtLabel,
				RefundPercentage: &refund,
				FeeWaived:        feeWaived,
			})
			sender := email.ResolveFromAddress(queryCtx, q, facility, logger)
			recipients := make(map[int64]struct{}, len(reservationParticipants)+1)
			for _, participant := range reservationParticipants {
				recipients[participant.ID] = struct{}{}
			}
			if reservation.PrimaryUserID.Valid {
				recipients[reservation.PrimaryUserID.Int64] = struct{}{}
			}
			for participantID := range recipients {
				email.SendCancellationEmail(emailCtx, q, emailClient, participantID, message, sender, logger)
			}
		}
	}

	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), waitlistNotificationTimeout)
	defer notifyCancel()

	if err := notifyWaitlistedMembers(notifyCtx, database, reservation, reservationCourts); err != nil {
		logger.Error().Err(err).Int64("reservation_id", reservationID).Msg("Failed to notify waitlisted members")
	}

	w.Header().Set("HX-Trigger", "refreshCourtsCalendar")
	w.WriteHeader(http.StatusNoContent)
}

type reservationDeleteRequest struct {
	WaiveFee *bool `json:"waive_fee"`
}

type cancellationPenaltyResponse struct {
	ReservationID     int64     `json:"reservation_id"`
	RefundPercentage  int64     `json:"refund_percentage"`
	FeePercentage     int64     `json:"fee_percentage"`
	HoursBeforeStart  int64     `json:"hours_before_start"`
	FacilityID        int64     `json:"facility_id"`
	ReservationStarts time.Time `json:"reservation_start_time"`
}

func decodeReservationDeleteRequest(r *http.Request) (reservationDeleteRequest, error) {
	if apiutil.IsJSONRequest(r) {
		if r.Body == nil {
			return reservationDeleteRequest{}, nil
		}
		defer r.Body.Close()

		var req reservationDeleteRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return reservationDeleteRequest{}, nil
			}
			return reservationDeleteRequest{}, err
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return reservationDeleteRequest{}, fmt.Errorf("invalid JSON body")
		}
		return req, nil
	}

	if err := r.ParseForm(); err != nil {
		return reservationDeleteRequest{}, fmt.Errorf("invalid form data")
	}

	rawWaiveFee := strings.TrimSpace(apiutil.FirstNonEmpty(r.FormValue("waive_fee"), r.FormValue("waiveFee")))
	if rawWaiveFee == "" {
		return reservationDeleteRequest{}, nil
	}
	value := apiutil.ParseBool(rawWaiveFee)
	return reservationDeleteRequest{WaiveFee: &value}, nil
}

func renderCancellationPenaltyPrompt(penalty cancellationPenaltyResponse) string {
	return fmt.Sprintf(`
<div class="space-y-3">
  <p class="text-sm text-amber-900">This cancellation applies a %d%% fee (%d%% refund). Choose whether to waive the fee.</p>
  <div class="flex flex-wrap gap-2">
    <form hx-delete="/api/v1/reservations/%d" hx-swap="none" hx-on::response-error="document.getElementById('reservation-cancel-feedback').innerHTML = event.detail.xhr.responseText; document.getElementById('reservation-cancel-feedback').classList.remove('hidden');" hx-on::after-request="if(event.detail.xhr.status === 204){document.getElementById('modal').innerHTML='';}">
      <input type="hidden" name="waive_fee" value="false"/>
      <button type="submit" class="px-3 py-2 text-xs font-medium text-amber-900 bg-amber-100 border border-amber-200 rounded-md hover:bg-amber-200">Apply fee</button>
    </form>
    <form hx-delete="/api/v1/reservations/%d" hx-swap="none" hx-on::response-error="document.getElementById('reservation-cancel-feedback').innerHTML = event.detail.xhr.responseText; document.getElementById('reservation-cancel-feedback').classList.remove('hidden');" hx-on::after-request="if(event.detail.xhr.status === 204){document.getElementById('modal').innerHTML='';}">
      <input type="hidden" name="waive_fee" value="true"/>
      <button type="submit" class="px-3 py-2 text-xs font-medium text-white bg-amber-600 rounded-md hover:bg-amber-700">Waive fee</button>
    </form>
  </div>
</div>
`, penalty.FeePercentage, penalty.RefundPercentage, penalty.ReservationID, penalty.ReservationID)
}

type reservationRequest struct {
	FacilityID        int64   `json:"facility_id"`
	ReservationTypeID int64   `json:"reservation_type_id"`
	RecurrenceRuleID  *int64  `json:"recurrence_rule_id,omitempty"`
	PrimaryUserID     *int64  `json:"primary_user_id,omitempty"`
	ProID             *int64  `json:"pro_id,omitempty"`
	OpenPlayRuleID    *int64  `json:"open_play_rule_id,omitempty"`
	StartTime         string  `json:"start_time"`
	EndTime           string  `json:"end_time"`
	IsOpenEvent       bool    `json:"is_open_event"`
	TeamsPerCourt     *int64  `json:"teams_per_court,omitempty"`
	PeoplePerTeam     *int64  `json:"people_per_team,omitempty"`
	CourtIDs          []int64 `json:"court_ids"`
	ParticipantIDs    []int64 `json:"participant_ids"`
	ParticipantIDsSet bool    `json:"-"`
}

func decodeReservationRequest(r *http.Request) (reservationRequest, error) {
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if r.Body == nil {
			return reservationRequest{}, fmt.Errorf("missing request body")
		}
		defer r.Body.Close()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			return reservationRequest{}, err
		}

		var req reservationRequest
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			return reservationRequest{}, err
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return reservationRequest{}, fmt.Errorf("invalid JSON body")
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err == nil {
			if _, ok := raw["participant_ids"]; ok {
				req.ParticipantIDsSet = true
			}
		}
		return req, nil
	}

	if err := r.ParseForm(); err != nil {
		return reservationRequest{}, fmt.Errorf("invalid form data")
	}

	req := reservationRequest{}
	facilityID, err := parseOptionalIntField(r.FormValue("facility_id"))
	if err != nil {
		return reservationRequest{}, err
	}
	req.FacilityID = facilityID

	reservationTypeID, err := parseIntField(r.FormValue("reservation_type_id"), "reservation_type_id")
	if err != nil {
		return reservationRequest{}, err
	}
	req.ReservationTypeID = reservationTypeID

	req.RecurrenceRuleID, err = parseOptionalPointer(r.FormValue("recurrence_rule_id"), "recurrence_rule_id")
	if err != nil {
		return reservationRequest{}, err
	}
	req.PrimaryUserID, err = parseOptionalPointer(r.FormValue("primary_user_id"), "primary_user_id")
	if err != nil {
		return reservationRequest{}, err
	}
	req.ProID, err = parseOptionalPointer(r.FormValue("pro_id"), "pro_id")
	if err != nil {
		return reservationRequest{}, err
	}
	req.OpenPlayRuleID, err = parseOptionalPointer(r.FormValue("open_play_rule_id"), "open_play_rule_id")
	if err != nil {
		return reservationRequest{}, err
	}

	req.StartTime = strings.TrimSpace(r.FormValue("start_time"))
	req.EndTime = strings.TrimSpace(r.FormValue("end_time"))
	req.IsOpenEvent = apiutil.ParseBool(r.FormValue("is_open_event"))

	req.TeamsPerCourt, err = parseOptionalPointer(r.FormValue("teams_per_court"), "teams_per_court")
	if err != nil {
		return reservationRequest{}, err
	}
	req.PeoplePerTeam, err = parseOptionalPointer(r.FormValue("people_per_team"), "people_per_team")
	if err != nil {
		return reservationRequest{}, err
	}

	courtValues := r.Form["court_ids"]
	if len(courtValues) == 0 {
		courtValues = r.Form["court_ids[]"]
	}
	req.CourtIDs, err = parseCourtIDs(courtValues)
	if err != nil {
		return reservationRequest{}, err
	}

	participantValues, participantValuesOK := r.Form["participant_ids"]
	if !participantValuesOK {
		participantValues, participantValuesOK = r.Form["participant_ids[]"]
	}
	if participantValuesOK {
		req.ParticipantIDsSet = true
		req.ParticipantIDs, err = parseParticipantIDs(participantValues)
		if err != nil {
			return reservationRequest{}, err
		}
	}

	return req, nil
}

func validateReservationInput(req reservationRequest, startTime, endTime time.Time) error {
	switch {
	case req.FacilityID <= 0:
		return apiutil.FieldError{Field: "facility_id", Reason: "must be a positive integer"}
	case req.ReservationTypeID <= 0:
		return apiutil.FieldError{Field: "reservation_type_id", Reason: "must be a positive integer"}
	case len(req.CourtIDs) == 0:
		return apiutil.FieldError{Field: "court_ids", Reason: "must include at least one court"}
	case !endTime.After(startTime):
		return apiutil.FieldError{Field: "end_time", Reason: "must be after start_time"}
	case endTime.Sub(startTime) < minReservationDuration:
		return apiutil.FieldError{Field: "end_time", Reason: "must be at least 1 hour after start_time"}
	}

	for _, courtID := range req.CourtIDs {
		if courtID <= 0 {
			return apiutil.FieldError{Field: "court_ids", Reason: "must contain only positive integers"}
		}
	}
	for _, participantID := range req.ParticipantIDs {
		if participantID <= 0 {
			return apiutil.FieldError{Field: "participant_ids", Reason: "must contain only positive integers"}
		}
	}
	for name, value := range map[string]*int64{
		"recurrence_rule_id": req.RecurrenceRuleID,
		"primary_user_id":    req.PrimaryUserID,
		"pro_id":             req.ProID,
		"open_play_rule_id":  req.OpenPlayRuleID,
		"teams_per_court":    req.TeamsPerCourt,
		"people_per_team":    req.PeoplePerTeam,
	} {
		if value != nil && *value <= 0 {
			return apiutil.FieldError{Field: name, Reason: "must be a positive integer"}
		}
	}

	return nil
}

func enforceMemberTierBookingWindow(ctx context.Context, q *dbgen.Queries, facilityID int64, primaryUserID *int64, startTime time.Time) error {
	if primaryUserID == nil || *primaryUserID <= 0 {
		return nil
	}

	member, err := q.GetMemberByID(ctx, *primaryUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apiutil.FieldError{Field: "primary_user_id", Reason: "must be a valid member"}
		}
		return err
	}

	maxAdvanceDays, facility, err := apiutil.GetMemberMaxAdvanceDays(ctx, q, facilityID, member.MembershipLevel, apiutil.DefaultMaxAdvanceDays)
	if err != nil {
		return err
	}

	loc := startTime.Location()
	if facility != nil && facility.Timezone != "" {
		loadedLoc, loadErr := time.LoadLocation(facility.Timezone)
		if loadErr == nil {
			loc = loadedLoc
		}
	}

	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	maxDate := today.AddDate(0, 0, int(maxAdvanceDays))
	startTimeInLoc := startTime.In(loc)
	startDay := time.Date(startTimeInLoc.Year(), startTimeInLoc.Month(), startTimeInLoc.Day(), 0, 0, 0, 0, loc)
	if startDay.After(maxDate) {
		return apiutil.FieldError{Field: "start_time", Reason: fmt.Sprintf("must be within %d days for the member's booking window", maxAdvanceDays)}
	}

	return nil
}

func normalizeCourtIDs(courtIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(courtIDs))
	normalized := make([]int64, 0, len(courtIDs))
	for _, courtID := range courtIDs {
		if _, ok := seen[courtID]; ok {
			continue
		}
		seen[courtID] = struct{}{}
		normalized = append(normalized, courtID)
	}
	return normalized
}

func normalizeParticipantIDs(participantIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(participantIDs))
	normalized := make([]int64, 0, len(participantIDs))
	for _, participantID := range participantIDs {
		if _, ok := seen[participantID]; ok {
			continue
		}
		seen[participantID] = struct{}{}
		normalized = append(normalized, participantID)
	}
	return normalized
}

func parseReservationTimes(startValue, endValue string) (time.Time, time.Time, error) {
	startTime, err := parseReservationTime(startValue, "start_time")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	endTime, err := parseReservationTime(endValue, "end_time")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return startTime, endTime, nil
}

func parseReservationTime(value, field string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, apiutil.FieldError{Field: field, Reason: "is required"}
	}

	layouts := []string{time.RFC3339, timeLayoutDatetimeLocal, timeLayoutDatetimeMinute}
	for _, layout := range layouts {
		if layout == time.RFC3339 {
			parsed, err := time.Parse(layout, value)
			if err == nil {
				return parsed, nil
			}
			continue
		}
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, apiutil.FieldError{Field: field, Reason: "must be a valid datetime"}
}

func notifyWaitlistedMembers(ctx context.Context, database *appdb.DB, reservation dbgen.Reservation, courts []dbgen.ListReservationCourtsRow) error {
	if database == nil {
		return nil
	}

	targetDate, targetStartTime, targetEndTime := reservationWaitlistSlot(reservation)
	now := time.Now()
	return database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		waitlists, err := listMatchingWaitlistsForCancelledSlot(ctx, qtx, reservation.FacilityID, targetDate, targetStartTime, targetEndTime, courts)
		if err != nil {
			return err
		}
		if len(waitlists) == 0 {
			return nil
		}

		config, err := loadWaitlistNotificationConfig(ctx, qtx, reservation.FacilityID)
		if err != nil {
			return err
		}

		waitlists = filterWaitlistsByNotificationWindow(waitlists, reservation.StartTime, now, config.NotificationWindowMinutes)
		if len(waitlists) == 0 {
			return nil
		}

		return createWaitlistNotifications(ctx, qtx, waitlists, config, now)
	})
}

func reservationWaitlistSlot(reservation dbgen.Reservation) (time.Time, string, string) {
	startTime := reservation.StartTime
	endTime := reservation.EndTime
	targetDate := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
	return targetDate, startTime.Format(waitlistTimeLayout), endTime.Format(waitlistTimeLayout)
}

func listMatchingWaitlistsForCancelledSlot(
	ctx context.Context,
	q *dbgen.Queries,
	facilityID int64,
	targetDate time.Time,
	targetStartTime string,
	targetEndTime string,
	courts []dbgen.ListReservationCourtsRow,
) ([]dbgen.Waitlist, error) {
	waitlistsByID := make(map[int64]dbgen.Waitlist)

	addEntries := func(targetCourtParam sql.NullInt64) error {
		entries, err := q.ListMatchingPendingWaitlistsForCancelledSlot(ctx, dbgen.ListMatchingPendingWaitlistsForCancelledSlotParams{
			FacilityID:      facilityID,
			TargetDate:      targetDate,
			TargetStartTime: targetStartTime,
			TargetEndTime:   targetEndTime,
			TargetCourtID:   targetCourtParam,
		})
		if err != nil {
			return err
		}
		for _, entry := range entries {
			waitlistsByID[entry.ID] = entry
		}
		return nil
	}

	if len(courts) == 0 {
		if err := addEntries(sql.NullInt64{}); err != nil {
			return nil, err
		}
	} else {
		for _, court := range courts {
			if err := addEntries(sql.NullInt64{Int64: court.CourtID, Valid: true}); err != nil {
				return nil, err
			}
		}
		if err := addEntries(sql.NullInt64{}); err != nil {
			return nil, err
		}
	}

	waitlists := make([]dbgen.Waitlist, 0, len(waitlistsByID))
	for _, entry := range waitlistsByID {
		waitlists = append(waitlists, entry)
	}
	sort.Slice(waitlists, func(i, j int) bool {
		if waitlists[i].Position != waitlists[j].Position {
			return waitlists[i].Position < waitlists[j].Position
		}
		if !waitlists[i].CreatedAt.Equal(waitlists[j].CreatedAt) {
			return waitlists[i].CreatedAt.Before(waitlists[j].CreatedAt)
		}
		return waitlists[i].ID < waitlists[j].ID
	})

	return waitlists, nil
}

func loadWaitlistNotificationConfig(ctx context.Context, q *dbgen.Queries, facilityID int64) (dbgen.WaitlistConfig, error) {
	config, err := q.GetWaitlistConfig(ctx, facilityID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.WaitlistConfig{
				FacilityID:       facilityID,
				NotificationMode: waitlistNotificationBroadcast,
			}, nil
		}
		return dbgen.WaitlistConfig{}, err
	}
	if strings.TrimSpace(config.NotificationMode) == "" {
		config.NotificationMode = waitlistNotificationBroadcast
	}
	return config, nil
}

func filterWaitlistsByNotificationWindow(waitlists []dbgen.Waitlist, slotStart time.Time, now time.Time, windowMinutes int64) []dbgen.Waitlist {
	if windowMinutes <= 0 {
		return waitlists
	}
	windowStart := now
	windowEnd := now.Add(time.Duration(windowMinutes) * time.Minute)
	if slotStart.Before(windowStart) || slotStart.After(windowEnd) {
		return nil
	}
	return waitlists
}

func createWaitlistNotifications(ctx context.Context, q *dbgen.Queries, waitlists []dbgen.Waitlist, config dbgen.WaitlistConfig, now time.Time) error {
	mode := strings.ToLower(strings.TrimSpace(config.NotificationMode))
	if mode == "" {
		mode = waitlistNotificationBroadcast
	}

	switch mode {
	case waitlistNotificationSequential:
		if len(waitlists) == 0 {
			return nil
		}
		selected := waitlists[0]
		if _, err := q.UpdateWaitlistStatus(ctx, dbgen.UpdateWaitlistStatusParams{
			ID:         selected.ID,
			FacilityID: selected.FacilityID,
			Status:     waitlistStatusNotified,
		}); err != nil {
			return err
		}

		expiryMinutes := config.OfferExpiryMinutes
		if expiryMinutes <= 0 {
			expiryMinutes = defaultWaitlistOfferExpiryMinutes
		}
		expiresAt := now.Add(time.Duration(expiryMinutes) * time.Minute)

		_, err := q.CreateWaitlistOffer(ctx, dbgen.CreateWaitlistOfferParams{
			WaitlistID: selected.ID,
			ExpiresAt:  expiresAt,
			Status:     waitlistOfferStatusPending,
		})
		return err
	default:
		expiryMinutes := config.OfferExpiryMinutes
		if expiryMinutes <= 0 {
			expiryMinutes = defaultWaitlistOfferExpiryMinutes
		}
		expiresAt := now.Add(time.Duration(expiryMinutes) * time.Minute)

		for _, entry := range waitlists {
			if _, err := q.UpdateWaitlistStatus(ctx, dbgen.UpdateWaitlistStatusParams{
				ID:         entry.ID,
				FacilityID: entry.FacilityID,
				Status:     waitlistStatusNotified,
			}); err != nil {
				return err
			}
			if _, err := q.CreateWaitlistOffer(ctx, dbgen.CreateWaitlistOfferParams{
				WaitlistID: entry.ID,
				ExpiresAt:  expiresAt,
				Status:     waitlistOfferStatusPending,
			}); err != nil {
				return err
			}
		}
		return nil
	}
}

func parseCourtIDs(values []string) ([]int64, error) {
	if len(values) == 0 {
		return nil, nil
	}
	var courtIDs []int64
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts := strings.Split(value, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			courtID, err := strconv.ParseInt(part, 10, 64)
			if err != nil || courtID <= 0 {
				return nil, apiutil.FieldError{Field: "court_ids", Reason: "must be a list of positive integers"}
			}
			courtIDs = append(courtIDs, courtID)
		}
	}
	return courtIDs, nil
}

func parseParticipantIDs(values []string) ([]int64, error) {
	if len(values) == 0 {
		return nil, nil
	}
	var participantIDs []int64
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts := strings.Split(value, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			participantID, err := strconv.ParseInt(part, 10, 64)
			if err != nil || participantID <= 0 {
				return nil, apiutil.FieldError{Field: "participant_ids", Reason: "must be a list of positive integers"}
			}
			participantIDs = append(participantIDs, participantID)
		}
	}
	return participantIDs, nil
}

func parseIntField(value, name string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, apiutil.FieldError{Field: name, Reason: "is required"}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, apiutil.FieldError{Field: name, Reason: "must be a number"}
	}
	return parsed, nil
}

func parseOptionalIntField(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, apiutil.FieldError{Field: "facility_id", Reason: "must be a number"}
	}
	return parsed, nil
}

func parseOptionalPointer(value, name string) (*int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, apiutil.FieldError{Field: name, Reason: "must be a number"}
	}
	return &parsed, nil
}

func hoursUntilReservationStart(start time.Time, now time.Time) int64 {
	hours := int64(start.Sub(now).Hours())
	if hours < 0 {
		return 0
	}
	return hours
}

func facilityIDFromRequest(r *http.Request) (int64, error) {
	value := strings.TrimSpace(r.URL.Query().Get("facility_id"))
	if value == "" {
		return 0, fmt.Errorf("facility_id is required")
	}
	facilityID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || facilityID <= 0 {
		return 0, fmt.Errorf("facility_id must be a positive integer")
	}
	return facilityID, nil
}

func resolveFacilityID(r *http.Request, payloadFacilityID int64) (int64, error) {
	queryID := strings.TrimSpace(r.URL.Query().Get("facility_id"))
	if queryID != "" {
		queryValue, err := strconv.ParseInt(queryID, 10, 64)
		if err != nil || queryValue <= 0 {
			return 0, fmt.Errorf("facility_id must be a positive integer")
		}
		if payloadFacilityID != 0 && payloadFacilityID != queryValue {
			return 0, fmt.Errorf("facility_id mismatch between query and payload")
		}
		return queryValue, nil
	}

	if payloadFacilityID <= 0 {
		return 0, fmt.Errorf("facility_id is required")
	}
	return payloadFacilityID, nil
}

func facilityExists(ctx context.Context, q *dbgen.Queries, facilityID int64) (bool, error) {
	count, err := q.FacilityExists(ctx, facilityID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func loadQueries() *dbgen.Queries {
	return queries
}

func loadDB() *appdb.DB {
	return store
}
