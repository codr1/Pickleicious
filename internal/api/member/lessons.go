package member

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
	"github.com/codr1/Pickleicious/internal/email"
	membertempl "github.com/codr1/Pickleicious/internal/templates/components/member"
)

const lessonReservationTypeName = "PRO_SESSION"

type lessonPro struct {
	ID        int64  `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	HasPhoto  bool   `json:"hasPhoto"`
}

type lessonSlot struct {
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

// HandleLessonBookingFormNew handles GET /member/lessons/new.
func HandleLessonBookingFormNew(w http.ResponseWriter, r *http.Request) {
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

	proRows, err := q.ListProsByFacility(ctx, sql.NullInt64{Int64: *user.HomeFacilityID, Valid: true})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load pros")
		http.Error(w, "Failed to load pros", http.StatusInternalServerError)
		return
	}

	pros := buildLessonProCards(proRows)
	var selectedProID int64
	if len(proRows) > 0 {
		selectedProID = proRows[0].ID
	}

	slots, err := buildLessonSlotOptions(ctx, q, *user.HomeFacilityID, selectedProID, bookingDate)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load lesson availability")
		http.Error(w, "Failed to load availability", http.StatusInternalServerError)
		return
	}

	component := membertempl.LessonBookingForm(membertempl.LessonBookingFormData{
		Pros:                  pros,
		Slots:                 slots,
		DatePicker:            membertempl.DatePickerData{Year: bookingDate.Year(), Month: int(bookingDate.Month()), Day: bookingDate.Day()},
		SelectedProID:         selectedProID,
		MaxAdvanceBookingDays: maxAdvanceDays,
	})
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render lesson booking form", "Failed to render lesson booking form") {
		return
	}
}

// HandleLessonBookingSlots handles GET /member/lessons/slots.
func HandleLessonBookingSlots(w http.ResponseWriter, r *http.Request) {
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

	proID, err := parseOptionalProID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	slots, err := buildLessonSlotOptions(ctx, q, *user.HomeFacilityID, proID, bookingDate)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load lesson availability")
		http.Error(w, "Failed to load availability", http.StatusInternalServerError)
		return
	}

	component := membertempl.ProSlotPicker(slots)
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render lesson slots", "Failed to render lesson slots") {
		return
	}
}

// HandleListPros handles GET /member/lessons/pros.
func HandleListPros(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method != http.MethodGet {
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

	proRows, err := q.ListProsByFacility(ctx, sql.NullInt64{Int64: *user.HomeFacilityID, Valid: true})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load pros")
		http.Error(w, "Failed to load pros", http.StatusInternalServerError)
		return
	}

	userIDs := make([]int64, 0, len(proRows))
	for _, row := range proRows {
		userIDs = append(userIDs, row.UserID)
	}
	photoMap, err := loadUserPhotoMap(ctx, database, userIDs)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load pro photos")
		http.Error(w, "Failed to load pros", http.StatusInternalServerError)
		return
	}

	pros := make([]lessonPro, 0, len(proRows))
	for _, row := range proRows {
		pros = append(pros, lessonPro{
			ID:        row.ID,
			FirstName: row.FirstName,
			LastName:  row.LastName,
			HasPhoto:  photoMap[row.UserID],
		})
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"pros": pros}); err != nil {
		logger.Error().Err(err).Msg("Failed to write pros response")
		return
	}
}

// HandleProAvailability handles GET /member/lessons/pros/{id}/slots.
func HandleProAvailability(w http.ResponseWriter, r *http.Request) {
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

	proID, err := parseProIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid pro ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), portalQueryTimeout)
	defer cancel()

	staffRow, err := q.GetStaffByID(ctx, proID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Pro not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("pro_id", proID).Msg("Failed to load pro")
		http.Error(w, "Failed to load pro", http.StatusInternalServerError)
		return
	}
	if !strings.EqualFold(staffRow.Role, "pro") || !staffRow.HomeFacilityID.Valid || staffRow.HomeFacilityID.Int64 != *user.HomeFacilityID {
		http.Error(w, "Pro not found", http.StatusNotFound)
		return
	}

	targetDate, err := parseLessonDate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	slotMinutes := fmt.Sprintf("%d", int64(memberBookingMinDuration.Minutes()))
	rows, err := q.GetProLessonSlots(ctx, dbgen.GetProLessonSlotsParams{
		TargetDate:  targetDate.Format("2006-01-02"),
		FacilityID:  *user.HomeFacilityID,
		SlotMinutes: sql.NullString{String: slotMinutes, Valid: true},
		ProID:       sql.NullInt64{Int64: proID, Valid: true},
	})
	if err != nil {
		logger.Error().Err(err).Int64("pro_id", proID).Msg("Failed to load pro slots")
		http.Error(w, "Failed to load availability", http.StatusInternalServerError)
		return
	}

	slots := make([]lessonSlot, 0, len(rows))
	for _, row := range rows {
		startTime, err := parseLessonSlotTime(row.StartTime, time.Local)
		if err != nil {
			logger.Error().Err(err).Int64("pro_id", proID).Msg("Failed to parse lesson slot start time")
			http.Error(w, "Failed to load availability", http.StatusInternalServerError)
			return
		}
		endTime, err := parseLessonSlotTime(row.EndTime, time.Local)
		if err != nil {
			logger.Error().Err(err).Int64("pro_id", proID).Msg("Failed to parse lesson slot end time")
			http.Error(w, "Failed to load availability", http.StatusInternalServerError)
			return
		}
		slots = append(slots, lessonSlot{
			StartTime: startTime.Format(memberBookingTimeLayout),
			EndTime:   endTime.Format(memberBookingTimeLayout),
		})
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"slots": slots}); err != nil {
		logger.Error().Err(err).Int64("pro_id", proID).Msg("Failed to write availability response")
		return
	}
}

// HandleLessonBookingCreate handles POST /member/lessons for member lesson booking.
func HandleLessonBookingCreate(w http.ResponseWriter, r *http.Request) {
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

	facilityLoc := time.Local
	facility, err := q.GetFacilityByID(ctx, *user.HomeFacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load facility booking config")
		http.Error(w, "Failed to validate booking rules", http.StatusInternalServerError)
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

	startTime, err := parseMemberBookingTime(r.FormValue("start_time"), "start_time", facilityLoc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		http.Error(w, "Lesson must be at least 1 hour", http.StatusBadRequest)
		return
	}

	maxMemberReservations := facility.MaxMemberReservations

	if startTime.Before(time.Now().In(facilityLoc)) {
		http.Error(w, "start_time must be in the future", http.StatusBadRequest)
		return
	}

	maxAdvanceDays := normalizedMaxAdvanceBookingDays(facility.MaxAdvanceBookingDays)
	now := time.Now().In(facilityLoc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, facilityLoc)
	maxDate := today.AddDate(0, 0, int(maxAdvanceDays))
	startDay := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, facilityLoc)
	if startDay.After(maxDate) {
		http.Error(w, fmt.Sprintf("start_time must be within %d days", maxAdvanceDays), http.StatusBadRequest)
		return
	}

	if facility.LessonMinNoticeHours > 0 {
		earliest := now.Add(time.Duration(facility.LessonMinNoticeHours) * time.Hour)
		if startTime.Before(earliest) {
			http.Error(w, fmt.Sprintf("Lessons must be booked at least %d hours in advance", facility.LessonMinNoticeHours), http.StatusBadRequest)
			return
		}
	}

	staffRow, err := q.GetStaffByID(ctx, proID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Pro not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("pro_id", proID).Msg("Failed to load pro")
		http.Error(w, "Failed to load pro", http.StatusInternalServerError)
		return
	}
	if !strings.EqualFold(staffRow.Role, "pro") || !staffRow.HomeFacilityID.Valid || staffRow.HomeFacilityID.Int64 != *user.HomeFacilityID {
		http.Error(w, "Pro not found", http.StatusNotFound)
		return
	}

	slotMinutes := fmt.Sprintf("%d", int64(memberBookingMinDuration.Minutes()))

	reservationTypeID, err := lookupReservationTypeID(ctx, q, lessonReservationTypeName)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to resolve reservation type")
		http.Error(w, "Reservation type not available", http.StatusInternalServerError)
		return
	}

	var created dbgen.Reservation
	err = database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries
		var eligiblePackageID int64

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

		slots, err := qtx.GetProLessonSlots(ctx, dbgen.GetProLessonSlotsParams{
			TargetDate:  startTime.Format("2006-01-02"),
			FacilityID:  *user.HomeFacilityID,
			SlotMinutes: sql.NullString{String: slotMinutes, Valid: true},
			ProID:       sql.NullInt64{Int64: proID, Valid: true},
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check lesson availability", Err: err}
		}

		available := false
		for _, slot := range slots {
			slotStart, err := parseLessonSlotTime(slot.StartTime, time.Local)
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check lesson availability", Err: err}
			}
			slotEnd, err := parseLessonSlotTime(slot.EndTime, time.Local)
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check lesson availability", Err: err}
			}
			if slotStart.Equal(startTime) && slotEnd.Equal(endTime) {
				available = true
				break
			}
		}
		if !available {
			return apiutil.HandlerError{Status: http.StatusConflict, Message: "Selected lesson time is no longer available", Err: errors.New("lesson slot unavailable")}
		}

		created, err = qtx.CreateReservation(ctx, dbgen.CreateReservationParams{
			FacilityID:        *user.HomeFacilityID,
			ReservationTypeID: reservationTypeID,
			RecurrenceRuleID:  sql.NullInt64{},
			PrimaryUserID:     sql.NullInt64{Int64: user.ID, Valid: true},
			CreatedByUserID:   user.ID,
			ProID:             sql.NullInt64{Int64: proID, Valid: true},
			OpenPlayRuleID:    sql.NullInt64{},
			StartTime:         startTime,
			EndTime:           endTime,
			IsOpenEvent:       false,
			TeamsPerCourt:     sql.NullInt64{},
			PeoplePerTeam:     sql.NullInt64{},
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to create lesson reservation", Err: err}
		}

		if err := qtx.AddParticipant(ctx, dbgen.AddParticipantParams{
			ReservationID: created.ID,
			UserID:        user.ID,
		}); err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to add lesson participant", Err: err}
		}

		eligiblePackage, err := qtx.GetEligibleLessonPackageForUser(ctx, dbgen.GetEligibleLessonPackageForUserParams{
			UserID:         user.ID,
			FacilityID:     *user.HomeFacilityID,
			ComparisonTime: time.Now(),
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check lesson packages", Err: err}
		}
		if err == nil {
			eligiblePackageID = eligiblePackage.ID
		}

		if eligiblePackageID != 0 {
			if _, err := qtx.DecrementLessonPackageLesson(ctx, eligiblePackageID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return apiutil.HandlerError{Status: http.StatusConflict, Message: "Lesson package is no longer available", Err: err}
				}
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to redeem lesson package", Err: err}
			}

			if _, err := qtx.CreateLessonPackageRedemption(ctx, dbgen.CreateLessonPackageRedemptionParams{
				LessonPackageID: eligiblePackageID,
				FacilityID:      *user.HomeFacilityID,
				RedeemedAt:      time.Now(),
				ReservationID:   sql.NullInt64{Int64: created.ID, Valid: true},
			}); err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to redeem lesson package", Err: err}
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
				logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to write lesson reservation limit response")
			}
			return
		}
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			logger.Error().Err(herr.Err).Int64("facility_id", *user.HomeFacilityID).Msg(herr.Message)
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to create lesson reservation")
		http.Error(w, "Failed to create reservation", http.StatusInternalServerError)
		return
	}

	if emailClient != nil {
		emailCtx, emailCancel := context.WithTimeout(context.Background(), portalQueryTimeout)
		defer emailCancel()
		cancellationPolicy, policyErr := cancellationPolicySummary(emailCtx, q, facility.ID, reservationTypeID, startTime, now)
		if policyErr != nil {
			logger.Error().Err(policyErr).Int64("facility_id", facility.ID).Msg("Failed to load cancellation policy for confirmation email")
			cancellationPolicy = "Contact the facility for cancellation policy details."
		}
		date, timeRange := email.FormatDateTimeRange(startTime.In(facilityLoc), endTime.In(facilityLoc))
		confirmation := email.BuildProSessionConfirmation(email.ConfirmationDetails{
			FacilityName:       facility.Name,
			Date:               date,
			TimeRange:          timeRange,
			Courts:             "Assigned at check-in",
			CancellationPolicy: cancellationPolicy,
		})
		email.SendConfirmationEmail(emailCtx, q, emailClient, user.ID, confirmation, logger)
	}

	w.Header().Set("HX-Trigger", "refreshMemberReservations")
	if err := apiutil.WriteJSON(w, http.StatusCreated, created); err != nil {
		logger.Error().Err(err).Int64("reservation_id", created.ID).Msg("Failed to write lesson reservation response")
		return
	}
}

func parseProIDFromRequest(r *http.Request) (int64, error) {
	pathID := strings.TrimSpace(r.PathValue("id"))
	if pathID == "" {
		return 0, fmt.Errorf("invalid pro ID")
	}
	id, err := strconv.ParseInt(pathID, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid pro ID")
	}
	return id, nil
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

func parseOptionalProID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("pro_id"))
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid pro_id")
	}
	return id, nil
}

func buildLessonProCards(rows []dbgen.ListProsByFacilityRow) []membertempl.LessonProCard {
	cards := make([]membertempl.LessonProCard, 0, len(rows))
	for _, row := range rows {
		first := strings.TrimSpace(row.FirstName)
		last := strings.TrimSpace(row.LastName)
		name := strings.TrimSpace(first + " " + last)
		if name == "" {
			name = "TBD"
		}
		initials := ""
		if first != "" {
			initials += strings.ToUpper(first[:1])
		}
		if last != "" {
			initials += strings.ToUpper(last[:1])
		}
		if initials == "" {
			initials = "?"
		}
		cards = append(cards, membertempl.LessonProCard{
			ID:       row.ID,
			Name:     name,
			Initials: initials,
		})
	}
	return cards
}

func buildLessonSlotOptions(
	ctx context.Context,
	q *dbgen.Queries,
	facilityID int64,
	proID int64,
	bookingDate time.Time,
) ([]membertempl.LessonSlotOption, error) {
	if proID <= 0 {
		return nil, nil
	}

	slotMinutes := fmt.Sprintf("%d", int64(memberBookingMinDuration.Minutes()))
	rows, err := q.GetProLessonSlots(ctx, dbgen.GetProLessonSlotsParams{
		TargetDate:  bookingDate.Format("2006-01-02"),
		FacilityID:  facilityID,
		SlotMinutes: sql.NullString{String: slotMinutes, Valid: true},
		ProID:       sql.NullInt64{Int64: proID, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	slots := make([]membertempl.LessonSlotOption, 0, len(rows))
	for _, row := range rows {
		startTime, err := parseLessonSlotTime(row.StartTime, time.Local)
		if err != nil {
			return nil, err
		}
		endTime, err := parseLessonSlotTime(row.EndTime, time.Local)
		if err != nil {
			return nil, err
		}
		slots = append(slots, membertempl.LessonSlotOption{
			StartTime: startTime.Format(memberBookingTimeLayout),
			EndTime:   endTime.Format(memberBookingTimeLayout),
			Label:     fmt.Sprintf("%s - %s", startTime.Format("3:04 PM"), endTime.Format("3:04 PM")),
		})
	}
	return slots, nil
}

func loadUserPhotoMap(ctx context.Context, database *appdb.DB, userIDs []int64) (map[int64]bool, error) {
	photoMap := make(map[int64]bool, len(userIDs))
	if len(userIDs) == 0 {
		return photoMap, nil
	}

	placeholders := make([]string, 0, len(userIDs))
	args := make([]any, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, userID)
	}
	if len(placeholders) == 0 {
		return photoMap, nil
	}

	query := fmt.Sprintf("SELECT user_id FROM user_photos WHERE user_id IN (%s)", strings.Join(placeholders, ","))
	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		photoMap[userID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return photoMap, nil
}
