// internal/api/themes/handlers.go
package themes

import (
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

	"github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
)

const (
	themeQueryTimeout  = 5 * time.Second
	maxFacilityThemes  = 1000
	themeIDParam       = "id"
	facilityIDParam    = "id"
	facilityIDQueryKey = "facility_id"
)

var (
	queries     *dbgen.Queries
	queriesOnce sync.Once
)

type themeRequest struct {
	FacilityID     *int64 `json:"facilityId"`
	IsSystem       bool   `json:"isSystem"`
	Name           string `json:"name"`
	PrimaryColor   string `json:"primaryColor"`
	SecondaryColor string `json:"secondaryColor"`
	TertiaryColor  string `json:"tertiaryColor"`
	AccentColor    string `json:"accentColor"`
	HighlightColor string `json:"highlightColor"`
}

type themeCloneRequest struct {
	FacilityID *int64 `json:"facilityId"`
	Name       string `json:"name"`
}

type facilityThemeRequest struct {
	ThemeID int64 `json:"themeId"`
}

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(q *dbgen.Queries) {
	if q == nil {
		return
	}
	queriesOnce.Do(func() {
		queries = q
	})
}

func HandleThemesList(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, err := facilityIDFromQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), themeQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	systemThemes, err := models.GetSystemThemes(ctx, q)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to list system themes")
		http.Error(w, "Failed to load system themes", http.StatusInternalServerError)
		return
	}

	facilityThemes, err := models.GetFacilityThemes(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to list facility themes")
		http.Error(w, "Failed to load facility themes", http.StatusInternalServerError)
		return
	}

	themes := append(systemThemes, facilityThemes...)
	writeJSON(w, http.StatusOK, map[string]any{"themes": themes})
}

func HandleThemeCreate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var req themeRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.IsSystem {
		http.Error(w, "System themes are read-only", http.StatusForbidden)
		return
	}

	if req.FacilityID == nil || *req.FacilityID <= 0 {
		http.Error(w, "facilityId is required", http.StatusBadRequest)
		return
	}

	theme := models.Theme{
		FacilityID:     req.FacilityID,
		IsSystem:       false,
		Name:           req.Name,
		PrimaryColor:   req.PrimaryColor,
		SecondaryColor: req.SecondaryColor,
		TertiaryColor:  req.TertiaryColor,
		AccentColor:    req.AccentColor,
		HighlightColor: req.HighlightColor,
	}
	if err := theme.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), themeQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	facilityIDParam := sql.NullInt64{Int64: *req.FacilityID, Valid: true}
	count, err := q.CountFacilityThemes(ctx, facilityIDParam)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *req.FacilityID).Msg("Failed to count facility themes")
		http.Error(w, "Failed to validate theme limit", http.StatusInternalServerError)
		return
	}
	if count >= maxFacilityThemes {
		http.Error(w, "Facility theme limit reached", http.StatusConflict)
		return
	}

	nameCount, err := q.CountFacilityThemeName(ctx, dbgen.CountFacilityThemeNameParams{
		FacilityID: facilityIDParam,
		Name:       req.Name,
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *req.FacilityID).Msg("Failed to check theme name uniqueness")
		http.Error(w, "Failed to validate theme name", http.StatusInternalServerError)
		return
	}
	if nameCount > 0 {
		http.Error(w, "Theme name already exists for facility", http.StatusConflict)
		return
	}

	created, err := q.CreateTheme(ctx, dbgen.CreateThemeParams{
		FacilityID:     facilityIDParam,
		Name:           req.Name,
		IsSystem:       false,
		PrimaryColor:   req.PrimaryColor,
		SecondaryColor: req.SecondaryColor,
		TertiaryColor:  req.TertiaryColor,
		AccentColor:    req.AccentColor,
		HighlightColor: req.HighlightColor,
	})
	if err != nil {
		if isSQLiteForeignKeyViolation(err) {
			http.Error(w, "Facility not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", *req.FacilityID).Msg("Failed to create theme")
		http.Error(w, "Failed to create theme", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, themeFromDB(created))
}

func HandleThemeUpdate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	themeID, err := themeIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid theme ID", http.StatusBadRequest)
		return
	}

	var req themeRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.IsSystem {
		http.Error(w, "System themes are read-only", http.StatusForbidden)
		return
	}
	if req.FacilityID != nil {
		http.Error(w, "facilityId cannot be updated", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), themeQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	existing, err := q.GetTheme(ctx, themeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Theme not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("theme_id", themeID).Msg("Failed to fetch theme")
		http.Error(w, "Failed to load theme", http.StatusInternalServerError)
		return
	}
	if existing.IsSystem {
		http.Error(w, "System themes are read-only", http.StatusForbidden)
		return
	}
	if !existing.FacilityID.Valid {
		logger.Error().Int64("theme_id", themeID).Msg("Facility theme missing facility id")
		http.Error(w, "Invalid theme data", http.StatusInternalServerError)
		return
	}

	facilityID := existing.FacilityID.Int64
	theme := models.Theme{
		FacilityID:     &facilityID,
		IsSystem:       false,
		Name:           req.Name,
		PrimaryColor:   req.PrimaryColor,
		SecondaryColor: req.SecondaryColor,
		TertiaryColor:  req.TertiaryColor,
		AccentColor:    req.AccentColor,
		HighlightColor: req.HighlightColor,
	}
	if err := theme.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	nameCount, err := q.CountFacilityThemeNameExcludingID(ctx, dbgen.CountFacilityThemeNameExcludingIDParams{
		FacilityID: sql.NullInt64{Int64: facilityID, Valid: true},
		Name:       req.Name,
		ID:         themeID,
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to check theme name uniqueness")
		http.Error(w, "Failed to validate theme name", http.StatusInternalServerError)
		return
	}
	if nameCount > 0 {
		http.Error(w, "Theme name already exists for facility", http.StatusConflict)
		return
	}

	updated, err := q.UpdateTheme(ctx, dbgen.UpdateThemeParams{
		ID:             themeID,
		Name:           req.Name,
		PrimaryColor:   req.PrimaryColor,
		SecondaryColor: req.SecondaryColor,
		TertiaryColor:  req.TertiaryColor,
		AccentColor:    req.AccentColor,
		HighlightColor: req.HighlightColor,
	})
	if err != nil {
		logger.Error().Err(err).Int64("theme_id", themeID).Msg("Failed to update theme")
		http.Error(w, "Failed to update theme", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, themeFromDB(updated))
}

func HandleThemeDelete(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	themeID, err := themeIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid theme ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), themeQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	existing, err := q.GetTheme(ctx, themeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Theme not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("theme_id", themeID).Msg("Failed to fetch theme")
		http.Error(w, "Failed to load theme", http.StatusInternalServerError)
		return
	}
	if existing.IsSystem {
		http.Error(w, "System themes are read-only", http.StatusForbidden)
		return
	}

	usage, err := q.CountThemeUsage(ctx, themeID)
	if err != nil {
		logger.Error().Err(err).Int64("theme_id", themeID).Msg("Failed to check theme usage")
		http.Error(w, "Failed to validate theme usage", http.StatusInternalServerError)
		return
	}
	if usage > 0 {
		http.Error(w, "Cannot delete active theme", http.StatusConflict)
		return
	}

	deleted, err := q.DeleteTheme(ctx, themeID)
	if err != nil {
		if isSQLiteForeignKeyViolation(err) {
			http.Error(w, "Cannot delete active theme", http.StatusConflict)
			return
		}
		logger.Error().Err(err).Int64("theme_id", themeID).Msg("Failed to delete theme")
		http.Error(w, "Failed to delete theme", http.StatusInternalServerError)
		return
	}
	if deleted == 0 {
		http.Error(w, "Theme not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func HandleThemeClone(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	themeID, err := themeIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid theme ID", http.StatusBadRequest)
		return
	}

	var req themeCloneRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.FacilityID == nil || *req.FacilityID <= 0 {
		http.Error(w, "facilityId is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), themeQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	source, err := q.GetTheme(ctx, themeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Theme not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("theme_id", themeID).Msg("Failed to fetch theme for clone")
		http.Error(w, "Failed to load theme", http.StatusInternalServerError)
		return
	}

	// Allow cloning system themes, but block cross-facility copies for custom themes.
	if source.FacilityID.Valid && source.FacilityID.Int64 != *req.FacilityID {
		http.Error(w, "Theme belongs to a different facility", http.StatusBadRequest)
		return
	}

	cloneName := strings.TrimSpace(req.Name)
	if cloneName == "" {
		cloneName = fmt.Sprintf("Copy of %s", source.Name)
	}

	theme := models.Theme{
		FacilityID:     req.FacilityID,
		IsSystem:       false,
		Name:           cloneName,
		PrimaryColor:   source.PrimaryColor,
		SecondaryColor: source.SecondaryColor,
		TertiaryColor:  source.TertiaryColor,
		AccentColor:    source.AccentColor,
		HighlightColor: source.HighlightColor,
	}
	if err := theme.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityIDParam := sql.NullInt64{Int64: *req.FacilityID, Valid: true}
	count, err := q.CountFacilityThemes(ctx, facilityIDParam)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *req.FacilityID).Msg("Failed to count facility themes")
		http.Error(w, "Failed to validate theme limit", http.StatusInternalServerError)
		return
	}
	if count >= maxFacilityThemes {
		http.Error(w, "Facility theme limit reached", http.StatusConflict)
		return
	}

	nameCount, err := q.CountFacilityThemeName(ctx, dbgen.CountFacilityThemeNameParams{
		FacilityID: facilityIDParam,
		Name:       cloneName,
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", *req.FacilityID).Msg("Failed to check theme name uniqueness")
		http.Error(w, "Failed to validate theme name", http.StatusInternalServerError)
		return
	}
	if nameCount > 0 {
		http.Error(w, "Theme name already exists for facility", http.StatusConflict)
		return
	}

	created, err := q.CreateTheme(ctx, dbgen.CreateThemeParams{
		FacilityID:     facilityIDParam,
		Name:           cloneName,
		IsSystem:       false,
		PrimaryColor:   source.PrimaryColor,
		SecondaryColor: source.SecondaryColor,
		TertiaryColor:  source.TertiaryColor,
		AccentColor:    source.AccentColor,
		HighlightColor: source.HighlightColor,
	})
	if err != nil {
		if isSQLiteForeignKeyViolation(err) {
			http.Error(w, "Facility not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", *req.FacilityID).Msg("Failed to clone theme")
		http.Error(w, "Failed to clone theme", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, themeFromDB(created))
}

func HandleFacilityThemeSet(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, err := facilityIDFromPath(r)
	if err != nil {
		http.Error(w, "Invalid facility ID", http.StatusBadRequest)
		return
	}

	var req facilityThemeRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.ThemeID <= 0 {
		http.Error(w, "themeId is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), themeQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	theme, err := q.GetTheme(ctx, req.ThemeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Theme not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("theme_id", req.ThemeID).Msg("Failed to fetch theme")
		http.Error(w, "Failed to load theme", http.StatusInternalServerError)
		return
	}

	if !theme.IsSystem && theme.FacilityID.Valid && theme.FacilityID.Int64 != facilityID {
		http.Error(w, "Theme does not belong to facility", http.StatusBadRequest)
		return
	}

	err = q.UpsertActiveThemeID(ctx, dbgen.UpsertActiveThemeIDParams{
		FacilityID:    facilityID,
		ActiveThemeID: req.ThemeID,
	})
	if err != nil {
		if isSQLiteForeignKeyViolation(err) {
			_, themeErr := q.GetTheme(ctx, req.ThemeID)
			if errors.Is(themeErr, sql.ErrNoRows) {
				http.Error(w, "Theme not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Facility not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to set active theme")
		http.Error(w, "Failed to update active theme", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func themeFromDB(row dbgen.Theme) models.Theme {
	var facilityID *int64
	if row.FacilityID.Valid {
		id := row.FacilityID.Int64
		facilityID = &id
	}
	return models.Theme{
		ID:             row.ID,
		FacilityID:     facilityID,
		Name:           row.Name,
		IsSystem:       row.IsSystem,
		PrimaryColor:   row.PrimaryColor,
		SecondaryColor: row.SecondaryColor,
		TertiaryColor:  row.TertiaryColor,
		AccentColor:    row.AccentColor,
		HighlightColor: row.HighlightColor,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func facilityIDFromQuery(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(facilityIDQueryKey))
	if raw == "" {
		return 0, fmt.Errorf("%s is required", facilityIDQueryKey)
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", facilityIDQueryKey)
	}
	return id, nil
}

func facilityIDFromPath(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(facilityIDParam))
	if raw == "" {
		return 0, fmt.Errorf("invalid facility ID")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid facility ID")
	}
	return id, nil
}

func themeIDFromRequest(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(themeIDParam))
	if raw == "" {
		return 0, fmt.Errorf("invalid theme ID")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid theme ID")
	}
	return id, nil
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return fmt.Errorf("missing request body")
	}
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("invalid JSON body")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func loadQueries() *dbgen.Queries {
	return queries
}

func isSQLiteForeignKeyViolation(err error) bool {
	var sqliteErr sqlite3.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	return sqliteErr.ExtendedCode == sqlite3.ErrConstraintForeignKey
}
