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
	clinictempl "github.com/codr1/Pickleicious/internal/templates/components/clinics"
	membertempl "github.com/codr1/Pickleicious/internal/templates/components/member"
)

const clinicEnrollmentBelowMinimumNotification = "clinic_enrollment_below_minimum"

type memberClinicSummary struct {
	ID               int64   `json:"id"`
	ClinicTypeID     int64   `json:"clinicTypeId"`
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	PriceCents       int64   `json:"priceCents"`
	MinParticipants  int64   `json:"minParticipants"`
	MaxParticipants  int64   `json:"maxParticipants"`
	ProFirstName     string  `json:"proFirstName"`
	ProLastName      string  `json:"proLastName"`
	StartTime        string  `json:"startTime"`
	EndTime          string  `json:"endTime"`
	EnrollmentStatus string  `json:"enrollmentStatus"`
	EnrolledCount    int64   `json:"enrolledCount"`
	WaitlistCount    int64   `json:"waitlistCount"`
	IsFull           bool    `json:"isFull"`
	UserStatus       string  `json:"userStatus,omitempty"`
}

// HandleListAvailableClinics handles GET /member/clinics.
func HandleListAvailableClinics(w http.ResponseWriter, r *http.Request) {
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

	facilityLoc := time.Local
	facility, err := q.GetFacilityByID(ctx, *user.HomeFacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load facility booking config")
	} else if facility.Timezone != "" {
		loadedLoc, loadErr := time.LoadLocation(facility.Timezone)
		if loadErr != nil {
			logger.Error().Err(loadErr).Str("timezone", facility.Timezone).Msg("Failed to load facility timezone")
		} else {
			facilityLoc = loadedLoc
		}
	}

	sessions, err := q.ListClinicSessionsByFacility(ctx, *user.HomeFacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to list clinic sessions")
		http.Error(w, "Failed to load clinics", http.StatusInternalServerError)
		return
	}

	now := time.Now().In(facilityLoc)
	clinics := make([]memberClinicSummary, 0, len(sessions))
	clinicCards := make([]clinictempl.ClinicSessionCard, 0, len(sessions))
	for _, session := range sessions {
		if session.EnrollmentStatus != "open" {
			continue
		}
		startLocal := session.StartTime.In(facilityLoc)
		if !startLocal.After(now) {
			continue
		}

		clinicType, err := q.GetClinicType(ctx, dbgen.GetClinicTypeParams{
			ID:         session.ClinicTypeID,
			FacilityID: session.FacilityID,
		})
		if err != nil {
			logger.Error().Err(err).Int64("clinic_session_id", session.ID).Msg("Failed to load clinic type")
			continue
		}
		if clinicType.Status != "active" {
			continue
		}

		proFirstName := ""
		proLastName := ""
		proRow, err := q.GetStaffByID(ctx, session.ProID)
		if err != nil {
			logger.Error().Err(err).Int64("pro_id", session.ProID).Msg("Failed to load clinic pro")
		} else {
			proFirstName = strings.TrimSpace(proRow.FirstName)
			proLastName = strings.TrimSpace(proRow.LastName)
		}

		enrollments, err := q.ListEnrollmentsForClinic(ctx, dbgen.ListEnrollmentsForClinicParams{
			ClinicSessionID: session.ID,
			FacilityID:      session.FacilityID,
		})
		if err != nil {
			logger.Error().Err(err).Int64("clinic_session_id", session.ID).Msg("Failed to load clinic enrollments")
			http.Error(w, "Failed to load clinics", http.StatusInternalServerError)
			return
		}

		enrolledCount := int64(0)
		waitlistCount := int64(0)
		userStatus := ""
		for _, enrollment := range enrollments {
			switch enrollment.Status {
			case "enrolled":
				enrolledCount++
			case "waitlisted":
				waitlistCount++
			}
			if enrollment.UserID == user.ID && enrollment.Status != "cancelled" {
				userStatus = enrollment.Status
			}
		}

		var description *string
		descriptionText := ""
		if clinicType.Description.Valid {
			desc := strings.TrimSpace(clinicType.Description.String)
			description = &desc
			descriptionText = desc
		}

		endLocal := session.EndTime.In(facilityLoc)
		clinics = append(clinics, memberClinicSummary{
			ID:               session.ID,
			ClinicTypeID:     clinicType.ID,
			Name:             clinicType.Name,
			Description:      description,
			PriceCents:       clinicType.PriceCents,
			MinParticipants:  clinicType.MinParticipants,
			MaxParticipants:  clinicType.MaxParticipants,
			ProFirstName:     proFirstName,
			ProLastName:      proLastName,
			StartTime:        startLocal.Format(memberBookingTimeLayout),
			EndTime:          endLocal.Format(memberBookingTimeLayout),
			EnrollmentStatus: session.EnrollmentStatus,
			EnrolledCount:    enrolledCount,
			WaitlistCount:    waitlistCount,
			IsFull:           enrolledCount >= clinicType.MaxParticipants,
			UserStatus:       userStatus,
		})

		clinicCards = append(clinicCards, clinictempl.ClinicSessionCard{
			ID:              session.ID,
			Name:            clinicType.Name,
			Description:     descriptionText,
			ProFirstName:    proFirstName,
			ProLastName:     proLastName,
			StartTime:       startLocal,
			EndTime:         endLocal,
			MinParticipants: clinicType.MinParticipants,
			MaxParticipants: clinicType.MaxParticipants,
			EnrolledCount:   enrolledCount,
			WaitlistCount:   waitlistCount,
			PriceCents:      clinicType.PriceCents,
			IsFull:          enrolledCount >= clinicType.MaxParticipants,
			UserStatus:      toEnrollmentStatus(userStatus),
		})
	}

	acceptHeader := strings.ToLower(r.Header.Get("Accept"))
	wantsHTML := apiutil.IsHTMXRequest(r) || strings.Contains(acceptHeader, "text/html")
	wantsJSON := strings.Contains(acceptHeader, "application/json")
	if wantsHTML && (!wantsJSON || apiutil.IsHTMXRequest(r)) {
		component := membertempl.MemberClinics(clinictempl.ClinicListData{Upcoming: clinicCards})
		if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render member clinics", "Failed to render clinics") {
			return
		}
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"clinics": clinics}); err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to write clinic list response")
		return
	}
}

// HandleClinicEnroll handles POST /member/clinics/{id}/enroll.
func HandleClinicEnroll(w http.ResponseWriter, r *http.Request) {
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

	clinicID, err := parseClinicSessionID(r)
	if err != nil {
		http.Error(w, "Invalid clinic ID", http.StatusBadRequest)
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
	maxMemberReservations := int64(0)
	lessonMinNoticeHours := int64(0)
	facility, err := q.GetFacilityByID(ctx, *user.HomeFacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to load facility booking config")
	} else {
		maxMemberReservations = facility.MaxMemberReservations
		lessonMinNoticeHours = facility.LessonMinNoticeHours
		if facility.Timezone != "" {
			loadedLoc, loadErr := time.LoadLocation(facility.Timezone)
			if loadErr != nil {
				logger.Error().Err(loadErr).Str("timezone", facility.Timezone).Msg("Failed to load facility timezone")
			} else {
				facilityLoc = loadedLoc
			}
		}
	}

	var enrollment dbgen.ClinicEnrollment
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

		session, err := qtx.GetClinicSession(ctx, dbgen.GetClinicSessionParams{
			ID:         clinicID,
			FacilityID: *user.HomeFacilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Clinic session not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to fetch clinic session", Err: err}
		}
		if session.EnrollmentStatus != "open" {
			return apiutil.HandlerError{Status: http.StatusConflict, Message: "Clinic enrollment is closed"}
		}

		startLocal := session.StartTime.In(facilityLoc)
		now := time.Now().In(facilityLoc)
		if !startLocal.After(now) {
			return apiutil.HandlerError{Status: http.StatusBadRequest, Message: "Clinic session must be in the future"}
		}
		if lessonMinNoticeHours > 0 {
			earliest := now.Add(time.Duration(lessonMinNoticeHours) * time.Hour)
			if startLocal.Before(earliest) {
				return apiutil.HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Clinics must be booked at least %d hours in advance", lessonMinNoticeHours)}
			}
		}

		clinicType, err := qtx.GetClinicType(ctx, dbgen.GetClinicTypeParams{
			ID:         session.ClinicTypeID,
			FacilityID: session.FacilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Clinic type not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to fetch clinic type", Err: err}
		}
		if clinicType.Status != "active" {
			return apiutil.HandlerError{Status: http.StatusBadRequest, Message: "Clinic is not available"}
		}

		enrollments, err := qtx.ListEnrollmentsForClinic(ctx, dbgen.ListEnrollmentsForClinicParams{
			ClinicSessionID: session.ID,
			FacilityID:      session.FacilityID,
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check clinic enrollment", Err: err}
		}

		var existing *dbgen.ClinicEnrollment
		enrolledCount := int64(0)
		for i := range enrollments {
			if enrollments[i].Status == "enrolled" {
				enrolledCount++
			}
			if enrollments[i].UserID == user.ID {
				existing = &enrollments[i]
			}
		}

		if existing != nil && existing.Status != "cancelled" {
			return apiutil.HandlerError{Status: http.StatusConflict, Message: "Already enrolled"}
		}

		targetStatus := "enrolled"
		if enrolledCount >= clinicType.MaxParticipants {
			targetStatus = "waitlisted"
		}

		if existing != nil {
			enrollment, err = qtx.UpdateEnrollmentStatus(ctx, dbgen.UpdateEnrollmentStatusParams{
				ID:         existing.ID,
				FacilityID: *user.HomeFacilityID,
				Status:     targetStatus,
			})
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to update clinic enrollment", Err: err}
			}
		} else {
			enrollment, err = qtx.CreateClinicEnrollment(ctx, dbgen.CreateClinicEnrollmentParams{
				ClinicSessionID: session.ID,
				UserID:          user.ID,
				Status:          targetStatus,
				FacilityID:      session.FacilityID,
			})
			if err != nil {
				if apiutil.IsSQLiteUniqueViolation(err) {
					return apiutil.HandlerError{Status: http.StatusConflict, Message: "Already enrolled", Err: err}
				}
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to enroll in clinic", Err: err}
			}
		}

		if targetStatus == "enrolled" {
			updatedEnrollments, err := qtx.ListEnrollmentsForClinic(ctx, dbgen.ListEnrollmentsForClinicParams{
				ClinicSessionID: session.ID,
				FacilityID:      session.FacilityID,
			})
			if err != nil {
				return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to verify clinic capacity", Err: err}
			}
			enrolledCount = 0
			for _, updated := range updatedEnrollments {
				if updated.Status == "enrolled" {
					enrolledCount++
				}
			}
			if enrolledCount > clinicType.MaxParticipants {
				enrollment, err = qtx.UpdateEnrollmentStatus(ctx, dbgen.UpdateEnrollmentStatusParams{
					ID:         enrollment.ID,
					FacilityID: *user.HomeFacilityID,
					Status:     "waitlisted",
				})
				if err != nil {
					return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to apply clinic waitlist", Err: err}
				}
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
				logger.Error().Err(err).Int64("facility_id", *user.HomeFacilityID).Msg("Failed to write clinic reservation limit response")
			}
			return
		}
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			if herr.Status == http.StatusInternalServerError {
				logger.Error().Err(herr.Err).Int64("clinic_session_id", clinicID).Msg(herr.Message)
			}
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("clinic_session_id", clinicID).Msg("Failed to enroll in clinic")
		http.Error(w, "Failed to enroll in clinic", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshMemberReservations,refreshMemberClinics")
	if err := apiutil.WriteJSON(w, http.StatusCreated, enrollment); err != nil {
		logger.Error().Err(err).Int64("clinic_session_id", clinicID).Msg("Failed to write clinic enrollment response")
		return
	}
}

// HandleClinicCancel handles DELETE /member/clinics/{id}/enroll.
func HandleClinicCancel(w http.ResponseWriter, r *http.Request) {
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

	clinicID, err := parseClinicSessionID(r)
	if err != nil {
		http.Error(w, "Invalid clinic ID", http.StatusBadRequest)
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
	} else if facility.Timezone != "" {
		loadedLoc, loadErr := time.LoadLocation(facility.Timezone)
		if loadErr != nil {
			logger.Error().Err(loadErr).Str("timezone", facility.Timezone).Msg("Failed to load facility timezone")
		} else {
			facilityLoc = loadedLoc
		}
	}

	err = database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		session, err := qtx.GetClinicSession(ctx, dbgen.GetClinicSessionParams{
			ID:         clinicID,
			FacilityID: *user.HomeFacilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Clinic session not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to fetch clinic session", Err: err}
		}

		startLocal := session.StartTime.In(facilityLoc)
		if !startLocal.After(time.Now().In(facilityLoc)) {
			return apiutil.HandlerError{Status: http.StatusBadRequest, Message: "Clinic session must be in the future"}
		}

		enrollments, err := qtx.ListEnrollmentsForClinic(ctx, dbgen.ListEnrollmentsForClinicParams{
			ClinicSessionID: session.ID,
			FacilityID:      session.FacilityID,
		})
		if err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to check clinic enrollment", Err: err}
		}

		var existing *dbgen.ClinicEnrollment
		enrolledCount := int64(0)
		for i := range enrollments {
			if enrollments[i].Status == "enrolled" {
				enrolledCount++
			}
			if enrollments[i].UserID == user.ID {
				existing = &enrollments[i]
			}
		}
		if existing == nil {
			return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Clinic enrollment not found"}
		}
		if existing.Status == "cancelled" {
			return apiutil.HandlerError{Status: http.StatusConflict, Message: "Clinic enrollment already cancelled"}
		}

		clinicType, err := qtx.GetClinicType(ctx, dbgen.GetClinicTypeParams{
			ID:         session.ClinicTypeID,
			FacilityID: session.FacilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return apiutil.HandlerError{Status: http.StatusNotFound, Message: "Clinic type not found", Err: err}
			}
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to fetch clinic type", Err: err}
		}

		if _, err := qtx.UpdateEnrollmentStatus(ctx, dbgen.UpdateEnrollmentStatusParams{
			ID:         existing.ID,
			FacilityID: *user.HomeFacilityID,
			Status:     "cancelled",
		}); err != nil {
			return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to cancel clinic enrollment", Err: err}
		}

		remaining := enrolledCount
		var promoted *dbgen.ClinicEnrollment
		if existing.Status == "enrolled" {
			remaining--
			for i := range enrollments {
				if enrollments[i].Status == "waitlisted" {
					promoted = &enrollments[i]
					break
				}
			}
			if promoted != nil {
				if _, err := qtx.UpdateEnrollmentStatus(ctx, dbgen.UpdateEnrollmentStatusParams{
					ID:         promoted.ID,
					FacilityID: *user.HomeFacilityID,
					Status:     "enrolled",
				}); err != nil {
					return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to promote clinic waitlist", Err: err}
				}
				remaining++
			}
		}

		if existing.Status == "enrolled" && enrolledCount >= clinicType.MinParticipants {
			if remaining < clinicType.MinParticipants {
				message := fmt.Sprintf("%s on %s dropped below minimum enrollment (%d/%d)",
					clinicType.Name,
					startLocal.Format("Jan 2 3:04 PM"),
					remaining,
					clinicType.MinParticipants,
				)
				if _, err := qtx.CreateStaffNotification(ctx, dbgen.CreateStaffNotificationParams{
					FacilityID:             session.FacilityID,
					NotificationType:       clinicEnrollmentBelowMinimumNotification,
					Message:                message,
					RelatedClinicSessionID: sql.NullInt64{Int64: session.ID, Valid: true},
				}); err != nil {
					return apiutil.HandlerError{Status: http.StatusInternalServerError, Message: "Failed to notify staff", Err: err}
				}
			}
		}

		return nil
	})
	if err != nil {
		var herr apiutil.HandlerError
		if errors.As(err, &herr) {
			if herr.Status == http.StatusInternalServerError {
				logger.Error().Err(herr.Err).Int64("clinic_session_id", clinicID).Msg(herr.Message)
			}
			http.Error(w, herr.Message, herr.Status)
			return
		}
		logger.Error().Err(err).Int64("clinic_session_id", clinicID).Msg("Failed to cancel clinic enrollment")
		http.Error(w, "Failed to cancel clinic enrollment", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshMemberReservations,refreshMemberClinics")
	w.WriteHeader(http.StatusNoContent)
}

func parseClinicSessionID(r *http.Request) (int64, error) {
	pathID := strings.TrimSpace(r.PathValue("id"))
	if pathID == "" {
		return 0, fmt.Errorf("invalid clinic ID")
	}
	id, err := strconv.ParseInt(pathID, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid clinic ID")
	}
	return id, nil
}

func toEnrollmentStatus(status string) clinictempl.EnrollmentStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "enrolled":
		return clinictempl.EnrollmentStatusEnrolled
	case "waitlisted":
		return clinictempl.EnrollmentStatusWaitlisted
	case "cancelled":
		return clinictempl.EnrollmentStatusCancelled
	default:
		return clinictempl.EnrollmentStatusNone
	}
}
