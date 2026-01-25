// internal/api/staff/notifications.go
package staff

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/authz"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	stafftempl "github.com/codr1/Pickleicious/internal/templates/components/staff"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

// HandleNotificationDetail handles GET /staff/notifications/{id}.
func HandleNotificationDetail(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if !authz.IsStaff(user) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := strings.TrimSpace(r.PathValue("id"))
	if idStr == "" {
		http.Error(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}
	notificationID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	staffRow, err := queries.GetStaffByUserID(ctx, user.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		logger.Error().Err(err).Int64("user_id", user.ID).Msg("Failed to load staff record")
		http.Error(w, "Failed to load staff", http.StatusInternalServerError)
		return
	}

	notification, err := queries.GetStaffNotificationByID(ctx, notificationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Notification not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("notification_id", notificationID).Msg("Failed to load staff notification")
		http.Error(w, "Failed to load notification", http.StatusInternalServerError)
		return
	}

	if !notification.TargetStaffID.Valid || notification.TargetStaffID.Int64 != staffRow.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if !notification.RelatedReservationID.Valid {
		http.Error(w, "Reservation not found", http.StatusNotFound)
		return
	}

	reservation, err := queries.GetReservationByID(ctx, notification.RelatedReservationID.Int64)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Reservation not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("reservation_id", notification.RelatedReservationID.Int64).Msg("Failed to load reservation")
		http.Error(w, "Failed to load reservation", http.StatusInternalServerError)
		return
	}

	if !reservation.PrimaryUserID.Valid {
		http.Error(w, "Member not found", http.StatusNotFound)
		return
	}

	member, err := queries.GetUserByID(ctx, reservation.PrimaryUserID.Int64)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Member not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("member_id", reservation.PrimaryUserID.Int64).Msg("Failed to load member")
		http.Error(w, "Failed to load member", http.StatusInternalServerError)
		return
	}

	courtRows, err := queries.ListReservationCourtsByDateRange(ctx, dbgen.ListReservationCourtsByDateRangeParams{
		FacilityID: reservation.FacilityID,
		StartTime:  reservation.StartTime,
		EndTime:    reservation.EndTime,
	})
	if err != nil {
		logger.Error().Err(err).Int64("reservation_id", reservation.ID).Msg("Failed to load reservation courts")
		http.Error(w, "Failed to load courts", http.StatusInternalServerError)
		return
	}

	courtLabels := make([]string, 0, len(courtRows))
	for _, row := range courtRows {
		if row.ReservationID != reservation.ID {
			continue
		}
		courtLabels = append(courtLabels, fmt.Sprintf("Court %d", row.CourtNumber))
	}

	courtLabel := "Court TBD"
	if len(courtLabels) > 0 {
		courtLabel = strings.Join(courtLabels, ", ")
	}

	memberName := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(member.FirstName), strings.TrimSpace(member.LastName)}, " "))
	if memberName == "" {
		memberName = "Member"
	}

	memberEmail := ""
	if member.Email.Valid {
		memberEmail = member.Email.String
	}
	memberPhone := ""
	if member.Phone.Valid {
		memberPhone = member.Phone.String
	}

	if !notification.Read {
		updated, err := queries.MarkStaffNotificationAsRead(ctx, dbgen.MarkStaffNotificationAsReadParams{
			ID:         notification.ID,
			FacilityID: notification.FacilityID,
		})
		if err != nil {
			logger.Error().Err(err).Int64("notification_id", notification.ID).Msg("Failed to mark notification as read")
		} else {
			notification = updated
			w.Header().Set("HX-Trigger", "refreshNotificationCount")
		}
	}

	detailData := stafftempl.NotificationDetailData{
		Notification: notification,
		MemberName:   memberName,
		MemberEmail:  memberEmail,
		MemberPhone:  memberPhone,
		LessonDate:   reservation.StartTime.Format("Jan 2, 2006"),
		LessonTime:   fmt.Sprintf("%s - %s", reservation.StartTime.Format("3:04 PM"), reservation.EndTime.Format("3:04 PM")),
		CourtLabel:   courtLabel,
	}

	var activeTheme *models.Theme
	activeTheme, err = models.GetActiveTheme(ctx, queries, notification.FacilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", notification.FacilityID).Msg("Failed to load active theme")
		activeTheme = nil
	}

	sessionType := authz.SessionTypeFromContext(r.Context())
	page := layouts.Base(stafftempl.NotificationDetail(detailData), activeTheme, sessionType)
	if err := page.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render notification detail page")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}
