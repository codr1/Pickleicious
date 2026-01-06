// internal/api/notifications/handlers.go
package notifications

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/api/htmx"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/request"
	notificationtempl "github.com/codr1/Pickleicious/internal/templates/components/notifications"
)

var (
	queries     *dbgen.Queries
	queriesOnce sync.Once
)

const (
	notificationsQueryTimeout = 5 * time.Second
	notificationsListLimit    = 25
)

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

// /api/v1/notifications/count
func HandleNotificationCount(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), notificationsQueryTimeout)
	defer cancel()

	var facilityFilter interface{}
	if user.HomeFacilityID != nil {
		facilityFilter = *user.HomeFacilityID
	}

	count, err := q.CountUnreadStaffNotifications(ctx, facilityFilter)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to count staff notifications")
		http.Error(w, "Failed to load notifications", http.StatusInternalServerError)
		return
	}

	component := notificationtempl.NotificationCountBadge(count)
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render notifications count", "Failed to render notifications count") {
		return
	}
}

// /api/v1/notifications
func HandleNotificationsList(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), notificationsQueryTimeout)
	defer cancel()

	var facilityFilter interface{}
	if user.HomeFacilityID != nil {
		facilityFilter = *user.HomeFacilityID
	}

	notifications, err := q.ListStaffNotificationsForFacilityOrCorporate(ctx, dbgen.ListStaffNotificationsForFacilityOrCorporateParams{
		FacilityID: facilityFilter,
		Offset:     0,
		Limit:      notificationsListLimit,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to list staff notifications")
		http.Error(w, "Failed to load notifications", http.StatusInternalServerError)
		return
	}

	component := notificationtempl.NotificationsPanel(notificationtempl.NewNotifications(notifications))
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render notifications list", "Failed to render notifications") {
		return
	}
}

// /api/v1/notifications/{id}/read
func HandleMarkAsRead(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := strings.TrimSpace(r.PathValue("id"))
	if idStr == "" {
		http.Error(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}

	var facilityID int64
	if user.HomeFacilityID != nil {
		facilityID = *user.HomeFacilityID
	} else if parsedFacilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id")); ok {
		facilityID = parsedFacilityID
	} else {
		http.Error(w, "Facility not set", http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), notificationsQueryTimeout)
	defer cancel()

	_, err = q.MarkStaffNotificationAsRead(ctx, dbgen.MarkStaffNotificationAsReadParams{
		ID:         id,
		FacilityID: facilityID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Notification not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("id", id).Msg("Failed to mark notification as read")
		http.Error(w, "Failed to update notification", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshNotificationCount")
	if !htmx.IsRequest(r) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	notifications, err := q.ListStaffNotificationsForFacilityOrCorporate(ctx, dbgen.ListStaffNotificationsForFacilityOrCorporateParams{
		FacilityID: facilityID,
		Offset:     0,
		Limit:      notificationsListLimit,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to list staff notifications")
		http.Error(w, "Failed to load notifications", http.StatusInternalServerError)
		return
	}

	component := notificationtempl.NotificationsPanel(notificationtempl.NewNotifications(notifications))
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render notifications panel", "Failed to render notifications panel") {
		return
	}
}

// /api/v1/notifications/close
func HandleNotificationsClose(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(""))
}
