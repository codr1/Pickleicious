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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	appdb "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/request"
	reservationstempl "github.com/codr1/Pickleicious/internal/templates/components/reservations"
)

var (
	queries     *dbgen.Queries
	store       *appdb.DB
	queriesOnce sync.Once
)

const (
	reservationQueryTimeout  = 5 * time.Second
	minReservationDuration   = time.Hour
	timeLayoutDatetimeLocal  = "2006-01-02T15:04"
	timeLayoutDatetimeMinute = "2006-01-02 15:04"
)

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(database *appdb.DB) {
	if database == nil {
		return
	}
	queriesOnce.Do(func() {
		queries = database.Queries
		store = database
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

	req.CourtIDs = normalizeCourtIDs(req.CourtIDs)
	if req.ParticipantIDsSet {
		req.ParticipantIDs = normalizeParticipantIDs(req.ParticipantIDs)
	}
	if err := ensureCourtsAvailable(ctx, q, facilityID, 0, startTime, endTime, req.CourtIDs); err != nil {
		var availErr availabilityError
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
			RecurrenceRuleID:  toNullInt64(req.RecurrenceRuleID),
			PrimaryUserID:     toNullInt64(req.PrimaryUserID),
			ProID:             toNullInt64(req.ProID),
			OpenPlayRuleID:    toNullInt64(req.OpenPlayRuleID),
			StartTime:         startTime,
			EndTime:           endTime,
			IsOpenEvent:       req.IsOpenEvent,
			TeamsPerCourt:     toNullInt64(req.TeamsPerCourt),
			PeoplePerTeam:     toNullInt64(req.PeoplePerTeam),
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

	reservationID, err := reservationIDFromRequest(r)
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

	reservationID, err := reservationIDFromRequest(r)
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

		if err := ensureCourtsAvailable(ctx, qtx, facilityID, reservationID, startTime, endTime, req.CourtIDs); err != nil {
			var availErr availabilityError
			if errors.As(err, &availErr) {
				return apiutil.HandlerError{Status: http.StatusConflict, Message: err.Error(), Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check court availability", Err: err}
		}

		updated, err = qtx.UpdateReservation(ctx, dbgen.UpdateReservationParams{
			ID:                reservationID,
			FacilityID:        facilityID,
			ReservationTypeID: req.ReservationTypeID,
			RecurrenceRuleID:  toNullInt64(req.RecurrenceRuleID),
			PrimaryUserID:     toNullInt64(req.PrimaryUserID),
			ProID:             toNullInt64(req.ProID),
			OpenPlayRuleID:    toNullInt64(req.OpenPlayRuleID),
			StartTime:         startTime,
			EndTime:           endTime,
			IsOpenEvent:       req.IsOpenEvent,
			TeamsPerCourt:     toNullInt64(req.TeamsPerCourt),
			PeoplePerTeam:     toNullInt64(req.PeoplePerTeam),
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

	reservationID, err := reservationIDFromRequest(r)
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

	var deleted int64
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

		courts, err := qtx.ListReservationCourts(ctx, reservationID)
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load reservation courts", Err: err}
		}
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
		for _, participant := range participants {
			if err := qtx.RemoveParticipant(ctx, dbgen.RemoveParticipantParams{
				ReservationID: reservationID,
				UserID:        participant.ID,
			}); err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to remove reservation participant", Err: err}
			}
		}

		deleted, err = qtx.DeleteReservation(ctx, dbgen.DeleteReservationParams{
			ID:         reservationID,
			FacilityID: facilityID,
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to delete reservation", Err: err}
		}
		if deleted == 0 {
			return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Reservation not found", Err: sql.ErrNoRows}
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

	w.Header().Set("HX-Trigger", "refreshCourtsCalendar")
	w.WriteHeader(http.StatusNoContent)
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
	req.IsOpenEvent = parseBoolField(r.FormValue("is_open_event"))

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

func ensureCourtsAvailable(ctx context.Context, q *dbgen.Queries, facilityID, reservationID int64, startTime, endTime time.Time, courtIDs []int64) error {
	available, err := q.ListAvailableCourtsForOpenPlay(ctx, dbgen.ListAvailableCourtsForOpenPlayParams{
		FacilityID:    facilityID,
		ReservationID: reservationID,
		StartTime:     startTime,
		EndTime:       endTime,
	})
	if err != nil {
		return fmt.Errorf("availability check failed: %w", err)
	}

	availableMap := make(map[int64]struct{}, len(available))
	for _, court := range available {
		availableMap[court.ID] = struct{}{}
	}

	var unavailable []string
	for _, courtID := range courtIDs {
		if _, ok := availableMap[courtID]; ok {
			continue
		}
		unavailable = append(unavailable, strconv.FormatInt(courtID, 10))
	}
	if len(unavailable) > 0 {
		return availabilityError{Courts: unavailable}
	}
	return nil
}

type availabilityError struct {
	Courts []string
}

func (e availabilityError) Error() string {
	return fmt.Sprintf("courts unavailable: %s", strings.Join(e.Courts, ", "))
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

func parseBoolField(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
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

func reservationIDFromRequest(r *http.Request) (int64, error) {
	pathID := strings.TrimSpace(r.PathValue("id"))
	if pathID == "" {
		return 0, fmt.Errorf("invalid reservation ID")
	}
	id, err := strconv.ParseInt(pathID, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid reservation ID")
	}
	return id, nil
}

func facilityExists(ctx context.Context, q *dbgen.Queries, facilityID int64) (bool, error) {
	count, err := q.FacilityExists(ctx, facilityID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func toNullInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func loadQueries() *dbgen.Queries {
	return queries
}

func loadDB() *appdb.DB {
	return store
}
