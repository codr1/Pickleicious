package staff

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	appdb "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/request"
	reservationstempl "github.com/codr1/Pickleicious/internal/templates/components/reservations"
	stafftempl "github.com/codr1/Pickleicious/internal/templates/components/staff"
)

const (
	staffLessonReservationTypeName = "PRO_SESSION"
	staffLessonTimeLayout          = "2006-01-02 15:04"
	staffLessonMinDuration         = time.Hour
)

type staffLessonCreateRequest struct {
	ProID     int64  `json:"pro_id"`
	MemberID  int64  `json:"member_id"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type staffLessonReservationInput struct {
	FacilityID      int64
	Facility        dbgen.Facility
	FacilityLoc     *time.Location
	ProID           int64
	MemberID        int64
	StartTime       time.Time
	EndTime         time.Time
	CreatedByUserID int64
	ProRow          *dbgen.GetStaffByIDRow
}

func createStaffLessonReservation(ctx context.Context, input staffLessonReservationInput) (dbgen.Reservation, error) {
	if input.FacilityLoc == nil {
		input.FacilityLoc = time.Local
	}

	if !input.EndTime.After(input.StartTime) {
		return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusBadRequest, Message: "end_time must be after start_time"}
	}
	if input.EndTime.Sub(input.StartTime) < staffLessonMinDuration {
		return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusBadRequest, Message: "Lesson must be at least 1 hour"}
	}

	now := time.Now().In(input.FacilityLoc)
	if input.StartTime.Before(now) {
		return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusBadRequest, Message: "start_time must be in the future"}
	}
	if input.Facility.LessonMinNoticeHours > 0 {
		earliest := now.Add(time.Duration(input.Facility.LessonMinNoticeHours) * time.Hour)
		if input.StartTime.Before(earliest) {
			return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Lessons must be booked at least %d hours in advance", input.Facility.LessonMinNoticeHours)}
		}
	}

	memberRow, err := queries.GetUserByID(ctx, input.MemberID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusNotFound, Message: "Member not found"}
		}
		return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load member", Err: err}
	}
	if !memberRow.IsMember || strings.EqualFold(memberRow.Status, "deleted") {
		return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusNotFound, Message: "Member not found"}
	}
	if !memberRow.HomeFacilityID.Valid || memberRow.HomeFacilityID.Int64 != input.FacilityID {
		return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusNotFound, Message: "Member not found"}
	}

	proRow := input.ProRow
	if proRow == nil {
		row, err := queries.GetStaffByID(ctx, input.ProID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusNotFound, Message: "Pro not found"}
			}
			return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to load pro", Err: err}
		}
		proRow = &row
	}
	if !strings.EqualFold(proRow.Role, "pro") || !proRow.HomeFacilityID.Valid || proRow.HomeFacilityID.Int64 != input.FacilityID || strings.EqualFold(proRow.UserStatus, "deleted") {
		return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusNotFound, Message: "Pro not found"}
	}

	reservationTypeID, err := lookupReservationTypeID(ctx, queries, staffLessonReservationTypeName)
	if err != nil {
		return dbgen.Reservation{}, apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Reservation type not available", Err: err}
	}

	var created dbgen.Reservation
	err = store.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		if input.Facility.MaxMemberReservations > 0 {
			activeCount, err := qtx.CountActiveMemberReservations(ctx, dbgen.CountActiveMemberReservationsParams{
				FacilityID:    input.FacilityID,
				PrimaryUserID: sql.NullInt64{Int64: input.MemberID, Valid: true},
			})
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check reservation limits", Err: err}
			}
			if activeCount >= input.Facility.MaxMemberReservations {
				return apiutil.HandlerError{Status: http.StatusConflict, Message: fmt.Sprintf("Member has reached the maximum of %d active reservations", input.Facility.MaxMemberReservations)}
			}
		}

		slotMinutes := fmt.Sprintf("%d", int64(staffLessonMinDuration.Minutes()))
		slots, err := qtx.GetProLessonSlots(ctx, dbgen.GetProLessonSlotsParams{
			TargetDate:  input.StartTime.Format("2006-01-02"),
			FacilityID:  input.FacilityID,
			SlotMinutes: sql.NullString{String: slotMinutes, Valid: true},
			ProID:       sql.NullInt64{Int64: input.ProID, Valid: true},
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check lesson availability", Err: err}
		}

		available := false
		for _, slot := range slots {
			slotStart, err := parseLessonSlotTime(slot.StartTime, input.FacilityLoc)
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check lesson availability", Err: err}
			}
			slotEnd, err := parseLessonSlotTime(slot.EndTime, input.FacilityLoc)
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check lesson availability", Err: err}
			}
			if slotStart.Equal(input.StartTime) && slotEnd.Equal(input.EndTime) {
				available = true
				break
			}
		}
		if !available {
			return apiutil.HandlerError{Status: http.StatusConflict, Message: "Selected lesson time is no longer available", Err: errors.New("lesson slot unavailable")}
		}

		created, err = qtx.CreateReservation(ctx, dbgen.CreateReservationParams{
			FacilityID:        input.FacilityID,
			ReservationTypeID: reservationTypeID,
			RecurrenceRuleID:  sql.NullInt64{},
			PrimaryUserID:     sql.NullInt64{Int64: input.MemberID, Valid: true},
			CreatedByUserID:   input.CreatedByUserID,
			ProID:             sql.NullInt64{Int64: input.ProID, Valid: true},
			OpenPlayRuleID:    sql.NullInt64{},
			StartTime:         input.StartTime,
			EndTime:           input.EndTime,
			IsOpenEvent:       false,
			TeamsPerCourt:     sql.NullInt64{},
			PeoplePerTeam:     sql.NullInt64{},
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to create lesson reservation", Err: err}
		}

		if err := qtx.AddParticipant(ctx, dbgen.AddParticipantParams{
			ReservationID: created.ID,
			UserID:        input.MemberID,
		}); err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to add lesson participant", Err: err}
		}
		return nil
	})
	if err != nil {
		return dbgen.Reservation{}, err
	}

	return created, nil
}

// HandleStaffLessonBookingFormNew handles GET /api/v1/staff/lessons/booking/new.
func HandleStaffLessonBookingFormNew(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	facilityID, hasFacility := request.ParseFacilityID(r.URL.Query().Get("facility_id"))
	showFacilitySelector := user.HomeFacilityID == nil

	var (
		selectedFacilityID int64
		facilities         []dbgen.Facility
	)

	if showFacilitySelector {
		var err error
		facilities, err = loadFacilities(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to load facilities")
			http.Error(w, "Failed to load facilities", http.StatusInternalServerError)
			return
		}
		if hasFacility {
			if !facilityAllowed(facilities, facilityID) {
				http.Error(w, "Invalid facility", http.StatusBadRequest)
				return
			}
			selectedFacilityID = facilityID
		} else if len(facilities) > 0 {
			selectedFacilityID = facilities[0].ID
		}
	} else if user.HomeFacilityID != nil {
		selectedFacilityID = *user.HomeFacilityID
		if hasFacility && facilityID != selectedFacilityID {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	if selectedFacilityID == 0 {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	if !showFacilitySelector && !apiutil.RequireFacilityAccess(w, r, selectedFacilityID) {
		return
	}

	proRows, err := queries.ListProsByFacility(ctx, sql.NullInt64{Int64: selectedFacilityID, Valid: true})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", selectedFacilityID).Msg("Failed to load pros")
		http.Error(w, "Failed to load pros", http.StatusInternalServerError)
		return
	}

	proOptions := reservationstempl.NewProOptions(proRows)
	selectedProID := selectedProIDFromRequest(r, proOptions)

	memberRows, err := queries.ListMembers(ctx, dbgen.ListMembersParams{
		FacilityID: sql.NullInt64{Int64: selectedFacilityID, Valid: true},
		SearchTerm: nil,
		Offset:     0,
		Limit:      50,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load members for lesson booking form")
		memberRows = nil
	}

	component := reservationstempl.StaffLessonBookingForm(reservationstempl.StaffLessonBookingFormData{
		Facilities:           reservationstempl.NewFacilityOptions(facilities),
		Pros:                 proOptions,
		Members:              reservationstempl.NewMemberOptions(memberRows),
		SelectedFacilityID:   selectedFacilityID,
		SelectedProID:        selectedProID,
		DateValue:            dateValueFromRequest(r),
		ShowFacilitySelector: showFacilitySelector,
	})
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render lesson booking form", "Failed to render lesson booking form") {
		return
	}
}

// HandleStaffProScheduleView handles GET /api/v1/staff/lessons/schedule.
func HandleStaffProScheduleView(w http.ResponseWriter, r *http.Request) {
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

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	facilityID, err := resolveStaffLessonFacility(ctx, r, user)
	if err != nil {
		if facilityErr, ok := err.(staffLessonFacilityError); ok {
			http.Error(w, facilityErr.msg, facilityErr.status)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if user.HomeFacilityID != nil {
		if !apiutil.RequireFacilityAccess(w, r, facilityID) {
			return
		}
	}

	proID, err := requiredProID(r)
	if err != nil {
		http.Error(w, "Invalid pro_id", http.StatusBadRequest)
		return
	}

	proRow, err := queries.GetStaffByID(ctx, proID)
	if err != nil {
		logger.Error().Err(err).Int64("pro_id", proID).Msg("Failed to load pro")
		http.Error(w, "Pro not found", http.StatusNotFound)
		return
	}
	if !strings.EqualFold(proRow.Role, "pro") || !proRow.HomeFacilityID.Valid || proRow.HomeFacilityID.Int64 != facilityID || strings.EqualFold(proRow.UserStatus, "deleted") {
		http.Error(w, "Pro not found", http.StatusNotFound)
		return
	}

	facilityLoc := time.Local
	facility, err := queries.GetFacilityByID(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load facility")
		http.Error(w, "Failed to load facility", http.StatusInternalServerError)
		return
	}
	if facility.Timezone != "" {
		loadedLoc, loadErr := time.LoadLocation(facility.Timezone)
		if loadErr != nil {
			logger.Error().Err(loadErr).Str("timezone", facility.Timezone).Msg("Failed to load facility timezone")
		} else {
			facilityLoc = loadedLoc
		}
	}

	sessions, err := queries.GetFutureProSessionsByStaffID(ctx, dbgen.GetFutureProSessionsByStaffIDParams{
		ProID:     sql.NullInt64{Int64: proID, Valid: true},
		StartTime: time.Now().In(facilityLoc),
	})
	if err != nil {
		logger.Error().Err(err).Int64("pro_id", proID).Msg("Failed to load upcoming lessons")
		http.Error(w, "Failed to load upcoming lessons", http.StatusInternalServerError)
		return
	}

	proName := strings.TrimSpace(strings.Join([]string{proRow.FirstName, proRow.LastName}, " "))
	if proName == "" {
		proName = "TBD"
	}

	component := stafftempl.ProScheduleView(stafftempl.ProScheduleViewData{
		ProName:  proName,
		Sessions: sessions,
	})
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render pro schedule", "Failed to render pro schedule") {
		return
	}
}

// HandleStaffLessonBookingSlots handles GET /api/v1/staff/lessons/booking/slots.
func HandleStaffLessonBookingSlots(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	selectedFacilityID, err := resolveStaffLessonFacility(ctx, r, user)
	if err != nil {
		if facilityErr, ok := err.(staffLessonFacilityError); ok {
			http.Error(w, facilityErr.msg, facilityErr.status)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if user.HomeFacilityID != nil {
		if !apiutil.RequireFacilityAccess(w, r, selectedFacilityID) {
			return
		}
	}

	proID, err := requiredProID(r)
	if err != nil {
		http.Error(w, "Invalid pro_id", http.StatusBadRequest)
		return
	}

	proRow, err := queries.GetStaffByID(ctx, proID)
	if err != nil {
		logger.Error().Err(err).Int64("pro_id", proID).Msg("Failed to load pro")
		http.Error(w, "Pro not found", http.StatusNotFound)
		return
	}
	if !strings.EqualFold(proRow.Role, "pro") || !proRow.HomeFacilityID.Valid || proRow.HomeFacilityID.Int64 != selectedFacilityID || strings.EqualFold(proRow.UserStatus, "deleted") {
		http.Error(w, "Pro not found", http.StatusNotFound)
		return
	}

	lessonDate, err := parseLessonDate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	slots, err := buildLessonSlotOptions(ctx, selectedFacilityID, proID, lessonDate)
	if err != nil {
		logger.Error().Err(err).Int64("pro_id", proID).Msg("Failed to load lesson slots")
		http.Error(w, "Failed to load lesson slots", http.StatusInternalServerError)
		return
	}

	proName := strings.TrimSpace(strings.Join([]string{proRow.FirstName, proRow.LastName}, " "))
	if proName == "" {
		proName = "TBD"
	}

	component := reservationstempl.StaffLessonSlotPicker(reservationstempl.StaffLessonSlotsData{
		FacilityID:    selectedFacilityID,
		ProID:         proID,
		ProName:       proName,
		DateValue:     lessonDate.Format("2006-01-02"),
		PrimaryUserID: optionalPrimaryUserID(r),
		Slots:         slots,
	})
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render lesson slots", "Failed to render lesson slots") {
		return
	}
}

// HandleStaffLessonBookingCreate handles POST /api/v1/staff/lessons/booking.
func HandleStaffLessonBookingCreate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if queries == nil || store == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	selectedFacilityID, err := resolveStaffLessonFacilityValue(ctx, user, r.FormValue("facility_id"))
	if err != nil {
		if facilityErr, ok := err.(staffLessonFacilityError); ok {
			http.Error(w, facilityErr.msg, facilityErr.status)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if user.HomeFacilityID != nil {
		if !apiutil.RequireFacilityAccess(w, r, selectedFacilityID) {
			return
		}
	}

	facilityLoc := time.Local
	facility, err := queries.GetFacilityByID(ctx, selectedFacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", selectedFacilityID).Msg("Failed to load facility")
		http.Error(w, "Failed to load facility", http.StatusInternalServerError)
		return
	}
	if facility.Timezone != "" {
		loadedLoc, loadErr := time.LoadLocation(facility.Timezone)
		if loadErr != nil {
			logger.Error().Err(loadErr).Str("timezone", facility.Timezone).Msg("Failed to load facility timezone")
		} else {
			facilityLoc = loadedLoc
		}
	}

	proID, err := apiutil.ParseRequiredInt64Field(r.FormValue("pro_id"), "pro_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	memberID, err := apiutil.ParseRequiredInt64Field(r.FormValue("primary_user_id"), "primary_user_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	startTime, err := parseStaffLessonTime(r.FormValue("start_time"), "start_time", facilityLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	endTime, err := parseStaffLessonTime(r.FormValue("end_time"), "end_time", facilityLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created, err := createStaffLessonReservation(ctx, staffLessonReservationInput{
		FacilityID:      selectedFacilityID,
		Facility:        facility,
		FacilityLoc:     facilityLoc,
		ProID:           proID,
		MemberID:        memberID,
		StartTime:       startTime,
		EndTime:         endTime,
		CreatedByUserID: user.ID,
	})
	if err != nil {
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			if herr.Err != nil {
				logger.Error().Err(herr.Err).Int64("facility_id", selectedFacilityID).Msg(herr.Message)
			}
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("facility_id", selectedFacilityID).Msg("Failed to create lesson reservation")
		http.Error(w, "Failed to create reservation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshCourtsCalendar")
	if err := apiutil.WriteJSON(w, http.StatusCreated, created); err != nil {
		logger.Error().Err(err).Int64("reservation_id", created.ID).Msg("Failed to write lesson reservation response")
		return
	}
}

// HandleStaffLessonCreate handles POST /api/v1/staff/lessons.
func HandleStaffLessonCreate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if queries == nil || store == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var (
		proID     int64
		memberID  int64
		startRaw  string
		endRaw    string
		parseErr  error
	)

	if apiutil.IsJSONRequest(r) {
		var payload staffLessonCreateRequest
		if err := apiutil.DecodeJSON(r, &payload); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		proID, parseErr = parseStaffLessonCreateID(payload.ProID, "pro_id")
		if parseErr != nil {
			http.Error(w, parseErr.Error(), http.StatusBadRequest)
			return
		}
		memberID, parseErr = parseStaffLessonCreateID(payload.MemberID, "member_id")
		if parseErr != nil {
			http.Error(w, parseErr.Error(), http.StatusBadRequest)
			return
		}
		startRaw = payload.StartTime
		endRaw = payload.EndTime
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}
		proID, parseErr = apiutil.ParseRequiredInt64Field(r.FormValue("pro_id"), "pro_id")
		if parseErr != nil {
			http.Error(w, parseErr.Error(), http.StatusBadRequest)
			return
		}
		memberID, parseErr = apiutil.ParseRequiredInt64Field(r.FormValue("member_id"), "member_id")
		if parseErr != nil {
			http.Error(w, parseErr.Error(), http.StatusBadRequest)
			return
		}
		startRaw = r.FormValue("start_time")
		endRaw = r.FormValue("end_time")
	}

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	proRow, err := queries.GetStaffByID(ctx, proID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Pro not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("pro_id", proID).Msg("Failed to load pro")
		http.Error(w, "Failed to load pro", http.StatusInternalServerError)
		return
	}
	if !strings.EqualFold(proRow.Role, "pro") || !proRow.HomeFacilityID.Valid || strings.EqualFold(proRow.UserStatus, "deleted") {
		http.Error(w, "Pro not found", http.StatusNotFound)
		return
	}
	facilityID := proRow.HomeFacilityID.Int64

	if user.HomeFacilityID != nil {
		if !apiutil.RequireFacilityAccess(w, r, facilityID) {
			return
		}
	}

	facilityLoc := time.Local
	facility, err := queries.GetFacilityByID(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load facility")
		http.Error(w, "Failed to load facility", http.StatusInternalServerError)
		return
	}
	if facility.Timezone != "" {
		loadedLoc, loadErr := time.LoadLocation(facility.Timezone)
		if loadErr != nil {
			logger.Error().Err(loadErr).Str("timezone", facility.Timezone).Msg("Failed to load facility timezone")
		} else {
			facilityLoc = loadedLoc
		}
	}

	startTime, err := parseStaffLessonTime(startRaw, "start_time", facilityLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	endTime, err := parseStaffLessonTime(endRaw, "end_time", facilityLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created, err := createStaffLessonReservation(ctx, staffLessonReservationInput{
		FacilityID:      facilityID,
		Facility:        facility,
		FacilityLoc:     facilityLoc,
		ProID:           proID,
		MemberID:        memberID,
		StartTime:       startTime,
		EndTime:         endTime,
		CreatedByUserID: user.ID,
		ProRow:          &proRow,
	})
	if err != nil {
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			if herr.Err != nil {
				logger.Error().Err(herr.Err).Int64("facility_id", facilityID).Msg(herr.Message)
			}
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to create lesson reservation")
		http.Error(w, "Failed to create reservation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshCourtsCalendar")
	if err := apiutil.WriteJSON(w, http.StatusCreated, created); err != nil {
		logger.Error().Err(err).Int64("reservation_id", created.ID).Msg("Failed to write lesson reservation response")
		return
	}
}

// HandleStaffMemberSearch handles GET /api/v1/staff/members/search.
func HandleStaffMemberSearch(w http.ResponseWriter, r *http.Request) {
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

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	facilityID, err := resolveStaffLessonFacility(ctx, r, user)
	if err != nil {
		if facilityErr, ok := err.(staffLessonFacilityError); ok {
			http.Error(w, facilityErr.msg, facilityErr.status)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if user.HomeFacilityID != nil {
		if !apiutil.RequireFacilityAccess(w, r, facilityID) {
			return
		}
	}

	searchTerm := strings.TrimSpace(r.URL.Query().Get("q"))
	if searchTerm == "" {
		if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"members": []reservationstempl.MemberOption{}}); err != nil {
			logger.Error().Err(err).Msg("Failed to write empty member search response")
		}
		return
	}

	limit := parseMemberSearchLimit(r.URL.Query().Get("limit"))
	rows, err := queries.ListMembers(ctx, dbgen.ListMembersParams{
		FacilityID: sql.NullInt64{Int64: facilityID, Valid: true},
		SearchTerm: sql.NullString{String: searchTerm, Valid: true},
		Offset:     0,
		Limit:      limit,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to search members")
		http.Error(w, "Failed to search members", http.StatusInternalServerError)
		return
	}

	members := reservationstempl.NewMemberOptions(rows)
	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"members": members}); err != nil {
		logger.Error().Err(err).Msg("Failed to write member search response")
		return
	}
}

func dateValueFromRequest(r *http.Request) string {
	now := time.Now()
	dateValue := strings.TrimSpace(r.URL.Query().Get("date"))
	if dateValue == "" {
		return now.Format("2006-01-02")
	}
	if parsed, err := time.ParseInLocation("2006-01-02", dateValue, now.Location()); err == nil {
		return parsed.Format("2006-01-02")
	}
	return now.Format("2006-01-02")
}

func selectedProIDFromRequest(r *http.Request, pros []reservationstempl.ProOption) int64 {
	raw := strings.TrimSpace(r.URL.Query().Get("pro_id"))
	if raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			for _, pro := range pros {
				if pro.ID == parsed {
					return parsed
				}
			}
		}
	}
	if len(pros) > 0 {
		return pros[0].ID
	}
	return 0
}

func facilityAllowed(facilities []dbgen.Facility, facilityID int64) bool {
	for _, facility := range facilities {
		if facility.ID == facilityID {
			return true
		}
	}
	return false
}

type staffLessonFacilityError struct {
	status int
	msg    string
}

func (e staffLessonFacilityError) Error() string {
	return e.msg
}

func resolveStaffLessonFacility(ctx context.Context, r *http.Request, user *authz.AuthUser) (int64, error) {
	facilityID, hasFacility := request.ParseFacilityID(r.URL.Query().Get("facility_id"))
	if user.HomeFacilityID == nil {
		facilities, err := loadFacilities(ctx)
		if err != nil {
			return 0, staffLessonFacilityError{status: http.StatusInternalServerError, msg: "Failed to load facilities"}
		}
		if !hasFacility {
			return 0, staffLessonFacilityError{status: http.StatusBadRequest, msg: "facility_id is required"}
		}
		if !facilityAllowed(facilities, facilityID) {
			return 0, staffLessonFacilityError{status: http.StatusBadRequest, msg: "Invalid facility"}
		}
		return facilityID, nil
	}
	if !hasFacility {
		return 0, staffLessonFacilityError{status: http.StatusBadRequest, msg: "facility_id is required"}
	}
	if facilityID != *user.HomeFacilityID {
		return 0, staffLessonFacilityError{status: http.StatusForbidden, msg: "Forbidden"}
	}
	return facilityID, nil
}

func resolveStaffLessonFacilityValue(ctx context.Context, user *authz.AuthUser, raw string) (int64, error) {
	facilityID, hasFacility := request.ParseFacilityID(raw)
	if user.HomeFacilityID == nil {
		facilities, err := loadFacilities(ctx)
		if err != nil {
			return 0, staffLessonFacilityError{status: http.StatusInternalServerError, msg: "Failed to load facilities"}
		}
		if !hasFacility {
			return 0, staffLessonFacilityError{status: http.StatusBadRequest, msg: "facility_id is required"}
		}
		if !facilityAllowed(facilities, facilityID) {
			return 0, staffLessonFacilityError{status: http.StatusBadRequest, msg: "Invalid facility"}
		}
		return facilityID, nil
	}
	if !hasFacility {
		return 0, staffLessonFacilityError{status: http.StatusBadRequest, msg: "facility_id is required"}
	}
	if facilityID != *user.HomeFacilityID {
		return 0, staffLessonFacilityError{status: http.StatusForbidden, msg: "Forbidden"}
	}
	return facilityID, nil
}

func requiredProID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("pro_id"))
	if raw == "" {
		return 0, fmt.Errorf("pro_id is required")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid pro_id")
	}
	return id, nil
}

func optionalPrimaryUserID(r *http.Request) *int64 {
	raw := strings.TrimSpace(r.URL.Query().Get("primary_user_id"))
	if raw == "" {
		return nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	return &id
}

func parseLessonDate(r *http.Request) (time.Time, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("date"))
	if raw == "" {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("date must be in YYYY-MM-DD format")
	}
	return parsed, nil
}

func buildLessonSlotOptions(ctx context.Context, facilityID, proID int64, lessonDate time.Time) ([]reservationstempl.StaffLessonSlotOption, error) {
	slotMinutes := fmt.Sprintf("%d", int64(time.Hour.Minutes()))
	rows, err := queries.GetProLessonSlots(ctx, dbgen.GetProLessonSlotsParams{
		TargetDate:  lessonDate.Format("2006-01-02"),
		FacilityID:  facilityID,
		SlotMinutes: sql.NullString{String: slotMinutes, Valid: true},
		ProID:       sql.NullInt64{Int64: proID, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	slots := make([]reservationstempl.StaffLessonSlotOption, 0, len(rows))
	for _, row := range rows {
		startTime, err := parseLessonSlotTime(row.StartTime, time.Local)
		if err != nil {
			return nil, err
		}
		endTime, err := parseLessonSlotTime(row.EndTime, time.Local)
		if err != nil {
			return nil, err
		}
		slots = append(slots, reservationstempl.StaffLessonSlotOption{
			StartTime: startTime.Format("2006-01-02 15:04"),
			EndTime:   endTime.Format("2006-01-02 15:04"),
			Label:     fmt.Sprintf("%s - %s", startTime.Format("3:04 PM"), endTime.Format("3:04 PM")),
		})
	}
	return slots, nil
}

func parseLessonSlotTime(value interface{}, loc *time.Location) (time.Time, error) {
	switch typed := value.(type) {
	case time.Time:
		return typed.In(loc), nil
	case []byte:
		return parseLessonSlotString(string(typed), loc)
	case string:
		return parseLessonSlotString(typed, loc)
	default:
		if value == nil {
			return time.Time{}, fmt.Errorf("empty slot time")
		}
		return parseLessonSlotString(fmt.Sprint(value), loc)
	}
}

func parseLessonSlotString(raw string, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty slot time")
	}
	layouts := []string{"2006-01-02 15:04:05", "2006-01-02 15:04"}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid slot time")
}

func parseStaffLessonTime(raw string, field string, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("%s is required", field)
	}
	parsed, err := time.ParseInLocation(staffLessonTimeLayout, raw, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be in YYYY-MM-DD HH:MM format", field)
	}
	return parsed, nil
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

func parseStaffLessonCreateID(value int64, field string) (int64, error) {
	if value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", field)
	}
	return value, nil
}

func parseMemberSearchLimit(raw string) int64 {
	value := int64(25)
	if raw == "" {
		return value
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return value
	}
	if parsed > 200 {
		return 200
	}
	return parsed
}
