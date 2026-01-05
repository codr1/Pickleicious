// internal/api/staff/handlers.go
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

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	"github.com/codr1/Pickleicious/internal/request"
	stafftempl "github.com/codr1/Pickleicious/internal/templates/components/staff"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

var queries *dbgen.Queries

const staffQueryTimeout = 5 * time.Second

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(q *dbgen.Queries) {
	queries = q
}

// /staff
func HandleStaffPage(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	staffRows, err := queries.ListStaff(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch initial staff list")
		http.Error(w, "Failed to load staff", http.StatusInternalServerError)
		return
	}

	var activeTheme *models.Theme
	if facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id")); ok {
		theme, err := models.GetActiveTheme(ctx, queries, facilityID)
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
			activeTheme = nil
		} else {
			activeTheme = theme
		}
	}

	templateStaff := stafftempl.NewStaffList(staffRows)
	page := layouts.Base(stafftempl.StaffLayout(templateStaff), activeTheme)
	if err := page.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render staff layout")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

// /api/v1/staff
func HandleStaffList(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, hasFacility := request.ParseFacilityID(r.URL.Query().Get("facility_id"))
	role := strings.TrimSpace(r.URL.Query().Get("role"))

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	var (
		staffRows []dbgen.ListStaffRow
		err       error
	)

	switch {
	case hasFacility:
		rawRows, err := queries.ListStaffByFacility(ctx, sql.NullInt64{Int64: facilityID, Valid: true})
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to fetch staff list by facility")
			http.Error(w, "Failed to fetch staff", http.StatusInternalServerError)
			return
		}
		staffRows = make([]dbgen.ListStaffRow, 0, len(rawRows))
		for _, row := range rawRows {
			staffRows = append(staffRows, toListStaffRow(row))
		}
		if role != "" {
			staffRows = filterStaffRowsByRole(staffRows, role)
		}
	case role != "":
		rawRows, err := queries.ListStaffByRole(ctx, role)
		if err != nil {
			logger.Error().Err(err).Str("role", role).Msg("Failed to fetch staff list by role")
			http.Error(w, "Failed to fetch staff", http.StatusInternalServerError)
			return
		}
		staffRows = make([]dbgen.ListStaffRow, 0, len(rawRows))
		for _, row := range rawRows {
			staffRows = append(staffRows, toListStaffRow(row))
		}
	default:
		staffRows, err = queries.ListStaff(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to fetch staff list")
			http.Error(w, "Failed to fetch staff", http.StatusInternalServerError)
			return
		}
	}

	templateStaff := stafftempl.NewStaffList(staffRows)

	component := stafftempl.StaffList(templateStaff)
	if err := component.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render staff list")
		http.Error(w, "Failed to render staff list", http.StatusInternalServerError)
		return
	}
}

// /api/v1/staff/{id}
func HandleStaffDetail(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	idStr := parts[len(parts)-1]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid staff ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	staffRow, err := queries.GetStaffByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Staff not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("id", id).Msg("Failed to fetch staff")
		http.Error(w, "Failed to fetch staff", http.StatusInternalServerError)
		return
	}

	templStaff := stafftempl.NewStaff(toListStaffRow(staffRow))
	component := stafftempl.StaffDetail(templStaff)
	if err := component.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render staff detail")
		http.Error(w, "Failed to render staff detail", http.StatusInternalServerError)
		return
	}
}

func filterStaffRowsByRole(rows []dbgen.ListStaffRow, role string) []dbgen.ListStaffRow {
	filtered := make([]dbgen.ListStaffRow, 0, len(rows))
	for _, row := range rows {
		if !strings.EqualFold(row.Role, role) {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

// toListStaffRow converts any staff-like struct to ListStaffRow.
func toListStaffRow(staff interface{}) dbgen.ListStaffRow {
	switch s := staff.(type) {
	case dbgen.ListStaffRow:
		return s
	case dbgen.ListStaffByFacilityRow:
		return dbgen.ListStaffRow{
			ID:             s.ID,
			UserID:         s.UserID,
			FirstName:      s.FirstName,
			LastName:       s.LastName,
			HomeFacilityID: s.HomeFacilityID,
			Role:           s.Role,
			CreatedAt:      s.CreatedAt,
			UpdatedAt:      s.UpdatedAt,
			Email:          s.Email,
			Phone:          s.Phone,
		}
	case dbgen.ListStaffByRoleRow:
		return dbgen.ListStaffRow{
			ID:             s.ID,
			UserID:         s.UserID,
			FirstName:      s.FirstName,
			LastName:       s.LastName,
			HomeFacilityID: s.HomeFacilityID,
			Role:           s.Role,
			CreatedAt:      s.CreatedAt,
			UpdatedAt:      s.UpdatedAt,
			Email:          s.Email,
			Phone:          s.Phone,
		}
	case dbgen.GetStaffByIDRow:
		// PasswordHash is intentionally not copied into the list view struct.
		return dbgen.ListStaffRow{
			ID:               s.ID,
			UserID:           s.UserID,
			FirstName:        s.FirstName,
			LastName:         s.LastName,
			HomeFacilityID:   s.HomeFacilityID,
			Role:             s.Role,
			CreatedAt:        s.CreatedAt,
			UpdatedAt:        s.UpdatedAt,
			Email:            s.Email,
			Phone:            s.Phone,
			LocalAuthEnabled: s.LocalAuthEnabled,
			UserStatus:       s.UserStatus,
		}
	default:
		panic(fmt.Sprintf("unsupported staff type: %T", staff))
	}
}
