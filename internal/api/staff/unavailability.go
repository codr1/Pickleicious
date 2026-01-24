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

	"github.com/codr1/Pickleicious/internal/api/authz"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	stafftempl "github.com/codr1/Pickleicious/internal/templates/components/staff"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

const staffUnavailabilityTimeLayout = "2006-01-02T15:04"

// GET /staff/unavailability
func HandleListProUnavailability(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	staffRow, ok := requireProStaff(w, r, ctx)
	if !ok {
		return
	}

	rows, err := queries.ListProUnavailabilityByProID(ctx, staffRow.ID)
	if err != nil {
		logger.Error().Err(err).Int64("pro_id", staffRow.ID).Msg("Failed to load pro unavailability")
		http.Error(w, "Failed to load unavailability", http.StatusInternalServerError)
		return
	}

	var activeTheme *models.Theme
	if staffRow.HomeFacilityID.Valid {
		theme, err := models.GetActiveTheme(ctx, queries, staffRow.HomeFacilityID.Int64)
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", staffRow.HomeFacilityID.Int64).Msg("Failed to load active theme")
		} else {
			activeTheme = theme
		}
	}

	page := layouts.Base(stafftempl.ProUnavailabilityLayout(rows), activeTheme, authz.SessionTypeFromContext(r.Context()))
	if err := page.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render pro unavailability page")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

// POST /staff/unavailability
func HandleCreateProUnavailability(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	staffRow, ok := requireProStaff(w, r, ctx)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		logger.Error().Err(err).Msg("Failed to parse unavailability form")
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	startTime, endTime, err := parseUnavailabilityTimes(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now()
	if startTime.Before(now) {
		http.Error(w, "Start time must be in the future", http.StatusBadRequest)
		return
	}

	reason := strings.TrimSpace(r.FormValue("reason"))
	params := dbgen.CreateProUnavailabilityParams{
		ProID:     staffRow.ID,
		StartTime: startTime,
		EndTime:   endTime,
		Reason:    sql.NullString{String: reason, Valid: reason != ""},
	}

	blockID, err := queries.CreateProUnavailability(ctx, params)
	if err != nil {
		logger.Error().Err(err).Int64("pro_id", staffRow.ID).Msg("Failed to create pro unavailability")
		http.Error(w, "Failed to create block", http.StatusInternalServerError)
		return
	}

	logger.Info().
		Int64("pro_id", staffRow.ID).
		Int64("block_id", blockID).
		Time("start_time", startTime).
		Time("end_time", endTime).
		Msg("Created pro unavailability block")

	rows, err := queries.ListProUnavailabilityByProID(ctx, staffRow.ID)
	if err != nil {
		logger.Error().Err(err).Int64("pro_id", staffRow.ID).Msg("Failed to reload pro unavailability")
		http.Error(w, "Failed to load unavailability", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := stafftempl.ProUnavailabilityList(rows).Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render pro unavailability list")
		http.Error(w, "Failed to render list", http.StatusInternalServerError)
		return
	}
}

// DELETE /staff/unavailability/{id}
func HandleDeleteProUnavailability(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	blockID, err := parseUnavailabilityIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid unavailability ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	staffRow, ok := requireProStaff(w, r, ctx)
	if !ok {
		return
	}

	block, err := queries.GetProUnavailabilityByID(ctx, blockID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Unavailability block not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("id", blockID).Msg("Failed to fetch unavailability block")
		http.Error(w, "Failed to delete block", http.StatusInternalServerError)
		return
	}

	if block.ProID != staffRow.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := queries.DeleteProUnavailability(ctx, blockID); err != nil {
		logger.Error().Err(err).Int64("id", blockID).Msg("Failed to delete unavailability block")
		http.Error(w, "Failed to delete block", http.StatusInternalServerError)
		return
	}

	logger.Info().
		Int64("pro_id", staffRow.ID).
		Int64("block_id", blockID).
		Time("start_time", block.StartTime).
		Time("end_time", block.EndTime).
		Msg("Deleted pro unavailability block")

	rows, err := queries.ListProUnavailabilityByProID(ctx, staffRow.ID)
	if err != nil {
		logger.Error().Err(err).Int64("pro_id", staffRow.ID).Msg("Failed to reload pro unavailability")
		http.Error(w, "Failed to load unavailability", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := stafftempl.ProUnavailabilityList(rows).Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render pro unavailability list")
		http.Error(w, "Failed to render list", http.StatusInternalServerError)
		return
	}
}

// requireProStaff authorizes the HTTP request as a "pro" staff user and returns the corresponding staff database row on success.
// On failure it writes an appropriate HTTP error to the response (401 if the requester is not staff; 403 if no staff record exists or the staff role is not "pro"; 500 if an internal error occurs) and returns the zero-value staff row and false.
func requireProStaff(w http.ResponseWriter, r *http.Request, ctx context.Context) (dbgen.GetStaffByUserIDRow, bool) {
	logger := log.Ctx(r.Context())
	user := authz.UserFromContext(r.Context())
	if !authz.IsStaff(user) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return dbgen.GetStaffByUserIDRow{}, false
	}

	staffRow, err := queries.GetStaffByUserID(ctx, user.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Staff record not found", http.StatusForbidden)
			return dbgen.GetStaffByUserIDRow{}, false
		}
		logger.Error().Err(err).Int64("user_id", user.ID).Msg("Failed to load staff record")
		http.Error(w, "Failed to authorize request", http.StatusInternalServerError)
		return dbgen.GetStaffByUserIDRow{}, false
	}

	if !strings.EqualFold(staffRow.Role, "pro") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return dbgen.GetStaffByUserIDRow{}, false
	}

	return staffRow, true
}

func parseUnavailabilityTimes(r *http.Request) (time.Time, time.Time, error) {
	startTime, err := parseUnavailabilityTime(r.FormValue("start_time"))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start time")
	}
	endTime, err := parseUnavailabilityTime(r.FormValue("end_time"))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end time")
	}
	if !endTime.After(startTime) {
		return time.Time{}, time.Time{}, fmt.Errorf("end time must be after start time")
	}
	return startTime, endTime, nil
}

func parseUnavailabilityTime(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("time required")
	}
	parsed, err := time.ParseInLocation(staffUnavailabilityTimeLayout, value, time.Local)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func parseUnavailabilityIDFromPath(path string) (int64, error) {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid path")
	}
	idStr := parts[len(parts)-1]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}