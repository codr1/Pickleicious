// internal/api/notifications/handlers.go
package notifications

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
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

	component := notificationtempl.NotificationsList(notifications)
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render notifications list", "Failed to render notifications") {
		return
	}
}
