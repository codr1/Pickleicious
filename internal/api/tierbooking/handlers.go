// internal/api/tierbooking/handlers.go
package tierbooking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	appdb "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	tierbookingtempl "github.com/codr1/Pickleicious/internal/templates/components/tierbooking"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

const (
	tierBookingQueryTimeout      = 5 * time.Second
	defaultMemberMaxAdvanceDays  = int64(7)
	defaultMemberPlusAdvanceDays = int64(14)
	maxAdvanceDaysLimit          = int64(364)
)

var (
	queries     *dbgen.Queries
	store       *appdb.DB
	queriesOnce sync.Once
)

var tierMembershipLevels = []int64{0, 1, 2, 3}

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

// GET /admin/booking-windows?facility_id=X
func HandleTierBookingPage(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, err := apiutil.FacilityIDFromQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), tierBookingQueryTimeout)
	defer cancel()

	facility, err := q.GetFacilityByID(ctx, facilityID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Facility not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to fetch facility")
		http.Error(w, "Failed to load booking windows", http.StatusInternalServerError)
		return
	}

	windows, err := q.ListTierBookingWindowsForFacility(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to list tier booking windows")
		http.Error(w, "Failed to load booking windows", http.StatusInternalServerError)
		return
	}

	activeTheme, err := models.GetActiveTheme(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
		activeTheme = nil
	}

	pageData := newTierBookingPageData(facility, windows)
	sessionType := authz.SessionTypeFromContext(r.Context())
	page := layouts.Base(tierbookingtempl.TierBookingLayout(pageData), activeTheme, sessionType)
	if !apiutil.RenderHTMLComponent(r.Context(), w, page, nil, "Failed to render tier booking page", "Failed to render page") {
		return
	}
}

// POST /api/v1/tier-booking/toggle
func HandleTierBookingToggle(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := loadQueries()
	if q == nil || store == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		logger.Error().Err(err).Msg("Failed to parse form")
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	facilityID, err := apiutil.ParseRequiredInt64Field(r.FormValue("facility_id"), "facility_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), tierBookingQueryTimeout)
	defer cancel()

	enabled := false
	if values, ok := r.Form["tier_booking_enabled"]; ok {
		for _, value := range values {
			if apiutil.ParseBool(value) {
				enabled = true
				break
			}
		}
	}

	var facility dbgen.Facility
	if enabled {
		facility, err = q.GetFacilityByID(ctx, facilityID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Facility not found", http.StatusNotFound)
				return
			}
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to fetch facility")
			http.Error(w, "Failed to update tier booking", http.StatusInternalServerError)
			return
		}
	}

	tx, err := store.BeginTx(ctx)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to start transaction")
		http.Error(w, "Failed to update tier booking", http.StatusInternalServerError)
		return
	}

	commit := false
	defer func() {
		if !commit {
			if rbErr := tx.Rollback(); rbErr != nil {
				logger.Error().Err(rbErr).Int64("facility_id", facilityID).Msg("Failed to rollback transaction")
			}
		}
	}()

	result, err := tx.ExecContext(ctx, `
		UPDATE facilities
		SET tier_booking_enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		enabled,
		facilityID,
	)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Bool("enabled", enabled).Msg("Failed to toggle tier booking")
		http.Error(w, "Failed to update tier booking", http.StatusInternalServerError)
		return
	}

	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to read tier booking update result")
		http.Error(w, "Failed to update tier booking", http.StatusInternalServerError)
		return
	}
	if affected == 0 {
		http.Error(w, "Facility not found", http.StatusNotFound)
		return
	}

	if enabled {
		defaultAdvanceDays := apiutil.NormalizedMaxAdvanceDays(facility.MaxAdvanceBookingDays, defaultMemberMaxAdvanceDays)
		qtx := dbgen.New(tx)
		defaults := tierBookingDefaultMap(defaultAdvanceDays)
		for _, level := range tierMembershipLevels {
			if _, err := qtx.UpsertTierBookingWindow(ctx, dbgen.UpsertTierBookingWindowParams{
				FacilityID:      facilityID,
				MembershipLevel: level,
				MaxAdvanceDays:  defaults[level],
			}); err != nil {
				logger.Error().Err(err).Int64("facility_id", facilityID).Int64("membership_level", level).Msg("Failed to upsert tier booking window defaults")
				http.Error(w, "Failed to update tier booking windows", http.StatusInternalServerError)
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to commit tier booking update")
		http.Error(w, "Failed to update tier booking", http.StatusInternalServerError)
		return
	}
	commit = true

	w.Header().Set("HX-Redirect", fmt.Sprintf("/admin/booking-windows?facility_id=%d", facilityID))
	if enabled {
		apiutil.WriteHTMLFeedback(w, http.StatusOK, "Tier booking enabled with default windows.")
		return
	}
	apiutil.WriteHTMLFeedback(w, http.StatusOK, "Tier booking disabled.")
}

// POST/PUT /api/v1/tier-booking/windows
func HandleTierBookingSave(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := loadQueries()
	if q == nil || store == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		logger.Error().Err(err).Msg("Failed to parse form")
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	facilityID, err := apiutil.ParseRequiredInt64Field(r.FormValue("facility_id"), "facility_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), tierBookingQueryTimeout)
	defer cancel()

	tx, err := store.BeginTx(ctx)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to start transaction")
		http.Error(w, "Failed to update tier booking windows", http.StatusInternalServerError)
		return
	}

	commit := false
	defer func() {
		if !commit {
			if rbErr := tx.Rollback(); rbErr != nil {
				logger.Error().Err(rbErr).Int64("facility_id", facilityID).Msg("Failed to rollback transaction")
			}
		}
	}()

	qtx := dbgen.New(tx)
	for _, level := range tierMembershipLevels {
		field := fmt.Sprintf("max_advance_days_%d", level)
		value, err := parseMaxAdvanceDays(r.FormValue(field), field)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if _, err := qtx.UpsertTierBookingWindow(ctx, dbgen.UpsertTierBookingWindowParams{
			FacilityID:      facilityID,
			MembershipLevel: level,
			MaxAdvanceDays:  value,
		}); err != nil {
			if apiutil.IsSQLiteForeignKeyViolation(err) {
				http.Error(w, "Facility not found", http.StatusNotFound)
				return
			}
			logger.Error().Err(err).Int64("facility_id", facilityID).Int64("membership_level", level).Msg("Failed to upsert tier booking window")
			http.Error(w, "Failed to update tier booking windows", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to commit tier booking window updates")
		http.Error(w, "Failed to update tier booking windows", http.StatusInternalServerError)
		return
	}
	commit = true

	apiutil.WriteHTMLFeedback(w, http.StatusOK, "Tier booking windows saved.")
}

func loadQueries() *dbgen.Queries {
	return queries
}

func parseMaxAdvanceDays(value string, field string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", field)
	}
	if parsed > maxAdvanceDaysLimit {
		return 0, fmt.Errorf("%s must be %d or less", field, maxAdvanceDaysLimit)
	}
	return parsed, nil
}

func newTierBookingPageData(facility dbgen.Facility, windows []dbgen.MemberTierBookingWindow) tierbookingtempl.TierBookingPageData {
	defaultAdvanceDays := apiutil.NormalizedMaxAdvanceDays(facility.MaxAdvanceBookingDays, defaultMemberMaxAdvanceDays)
	windowMap := make(map[int64]dbgen.MemberTierBookingWindow, len(windows))
	for _, window := range windows {
		windowMap[window.MembershipLevel] = window
	}

	defaults := tierBookingDefaultMap(defaultAdvanceDays)
	items := make([]tierbookingtempl.TierBookingWindowData, 0, len(tierMembershipLevels))
	for _, level := range tierMembershipLevels {
		maxAdvanceDays := defaults[level]
		if window, ok := windowMap[level]; ok {
			maxAdvanceDays = apiutil.NormalizedMaxAdvanceDays(window.MaxAdvanceDays, maxAdvanceDays)
		}
		items = append(items, tierbookingtempl.TierBookingWindowData{
			MembershipLevel: level,
			Label:           tierLabel(level),
			MaxAdvanceDays:  maxAdvanceDays,
			Note:            tierNote(level),
		})
	}

	return tierbookingtempl.TierBookingPageData{
		FacilityID:                    facility.ID,
		TierBookingEnabled:            facility.TierBookingEnabled,
		FacilityDefaultMaxAdvanceDays: defaultAdvanceDays,
		Windows:                       items,
	}
}

func tierBookingDefaultMap(facilityDefault int64) map[int64]int64 {
	return map[int64]int64{
		0: facilityDefault,
		1: facilityDefault,
		2: defaultMemberMaxAdvanceDays,
		3: defaultMemberPlusAdvanceDays,
	}
}

func tierLabel(level int64) string {
	switch level {
	case 0:
		return "Level 0 - Unverified Guest"
	case 1:
		return "Level 1 - Verified Guest"
	case 2:
		return "Level 2 - Member"
	default:
		return "Level 3+ - Member+"
	}
}

func tierNote(level int64) string {
	switch level {
	case 0:
		return "Applies to unverified guest bookings."
	case 1:
		return "Applies to verified guest bookings."
	case 2:
		return "Applies to member bookings."
	default:
		return "Applies to level 3 and higher memberships."
	}
}
