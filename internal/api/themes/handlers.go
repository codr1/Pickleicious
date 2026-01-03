// internal/api/themes/handlers.go
package themes

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/authz"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	themetempl "github.com/codr1/Pickleicious/internal/templates/components/themes"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
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

// /admin/themes
func HandleThemesPage(w http.ResponseWriter, r *http.Request) {
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

	activeTheme, err := models.GetActiveTheme(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
		activeTheme = nil
	}

	editor := newThemeEditorData(facilityID)
	page := layouts.Base(themetempl.ThemeAdminLayout(facilityID, editor), activeTheme)
	if !renderHTMLComponent(r.Context(), w, page, nil, "Failed to render themes page", "Failed to render page") {
		return
	}
}

// /api/v1/themes/new
func HandleThemeNew(w http.ResponseWriter, r *http.Request) {
	facilityID, err := facilityIDFromQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	component := themetempl.ThemeEditor(newThemeEditorData(facilityID))
	if !renderHTMLComponent(r.Context(), w, component, nil, "Failed to render new theme form", "Failed to render form") {
		return
	}
}

// /api/v1/themes/{id}
func HandleThemeDetail(w http.ResponseWriter, r *http.Request) {
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

	row, err := q.GetTheme(ctx, themeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Theme not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("theme_id", themeID).Msg("Failed to fetch theme")
		http.Error(w, "Failed to load theme", http.StatusInternalServerError)
		return
	}

	theme := themeFromDB(row)
	facilityID, err := facilityIDFromQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !theme.IsSystem && theme.FacilityID != nil && *theme.FacilityID != facilityID {
		http.Error(w, "Theme does not belong to facility", http.StatusBadRequest)
		return
	}

	if !isHXRequest(r) {
		writeJSON(w, http.StatusOK, theme)
		return
	}

	editor := themeEditorData(theme, facilityID)
	component := themetempl.ThemeEditor(editor)
	if !renderHTMLComponent(r.Context(), w, component, nil, "Failed to render theme form", "Failed to render form") {
		return
	}
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

	if !requireFacilityAccess(w, r, facilityID) {
		return
	}

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
	if isHXRequest(r) {
		activeThemeID, err := q.GetActiveThemeID(ctx, facilityID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
			activeThemeID = 0
		}

		component := themetempl.ThemeList(themetempl.NewThemes(themes, activeThemeID), facilityID)
		if !renderHTMLComponent(r.Context(), w, component, nil, "Failed to render themes list", "Failed to render list") {
			return
		}
		return
	}

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

	req, err := decodeThemeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

	if !requireFacilityAccess(w, r, *req.FacilityID) {
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

	if isHXRequest(r) {
		w.Header().Set("HX-Trigger", "refreshThemesList")
		writeThemeFeedback(w, http.StatusCreated, "Theme created.")
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

	req, err := decodeThemeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	if !requireFacilityAccess(w, r, facilityID) {
		return
	}

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

	if isHXRequest(r) {
		w.Header().Set("HX-Trigger", "refreshThemesList")
		writeThemeFeedback(w, http.StatusOK, "Theme updated.")
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

	if !requireFacilityAccess(w, r, existing.FacilityID.Int64) {
		return
	}

	usage, err := q.CountThemeUsage(ctx, sql.NullInt64{
		Int64: themeID,
		Valid: true,
	})
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

	if isHXRequest(r) {
		w.Header().Set("HX-Trigger", "refreshThemesList")
		writeThemeFeedback(w, http.StatusOK, "Theme deleted.")
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

	req, err := decodeThemeCloneRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.FacilityID == nil || *req.FacilityID <= 0 {
		http.Error(w, "facilityId is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), themeQueryTimeout)
	defer cancel()

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

	if !requireFacilityAccess(w, r, *req.FacilityID) {
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

	if isHXRequest(r) {
		w.Header().Set("HX-Trigger", "refreshThemesList")
		writeThemeFeedback(w, http.StatusCreated, "Theme cloned.")
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

	req, err := decodeFacilityThemeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ThemeID <= 0 {
		http.Error(w, "themeId is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), themeQueryTimeout)
	defer cancel()

	if !requireFacilityAccess(w, r, facilityID) {
		return
	}

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

	updated, err := q.UpsertActiveThemeID(ctx, dbgen.UpsertActiveThemeIDParams{
		FacilityID: facilityID,
		ActiveThemeID: sql.NullInt64{
			Int64: req.ThemeID,
			Valid: true,
		},
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to set active theme")
		http.Error(w, "Failed to update active theme", http.StatusInternalServerError)
		return
	}
	if updated == 0 {
		http.Error(w, "Facility not found", http.StatusNotFound)
		return
	}

	if isHXRequest(r) {
		w.Header().Set("HX-Trigger", "refreshThemesList")
		writeThemeFeedback(w, http.StatusOK, "Active theme updated.")
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

func requireFacilityAccess(w http.ResponseWriter, r *http.Request, facilityID int64) bool {
	logger := log.Ctx(r.Context())
	user := authz.UserFromContext(r.Context())
	if err := authz.RequireFacilityAccess(r.Context(), facilityID); err != nil {
		switch {
		case errors.Is(err, authz.ErrUnauthenticated):
			logEvent := logger.Warn().Int64("facility_id", facilityID)
			if user != nil {
				logEvent = logEvent.Int64("user_id", user.ID)
			}
			logEvent.Msg("Facility access denied: unauthenticated")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		case errors.Is(err, authz.ErrForbidden):
			logEvent := logger.Warn().Int64("facility_id", facilityID)
			if user != nil {
				logEvent = logEvent.Int64("user_id", user.ID)
			}
			logEvent.Msg("Facility access denied: forbidden")
			http.Error(w, "Forbidden", http.StatusForbidden)
		default:
			logEvent := logger.Error().Int64("facility_id", facilityID).Err(err)
			if user != nil {
				logEvent = logEvent.Int64("user_id", user.ID)
			}
			logEvent.Msg("Facility access denied: error")
			http.Error(w, "Failed to authorize request", http.StatusInternalServerError)
		}
		return false
	}
	return true
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

func decodeThemeRequest(r *http.Request) (themeRequest, error) {
	if isJSONRequest(r) {
		var req themeRequest
		return req, decodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return themeRequest{}, err
	}

	facilityID, err := parseOptionalInt64Field(firstNonEmpty(r.FormValue("facility_id"), r.FormValue("facilityId")), "facility_id")
	if err != nil {
		return themeRequest{}, err
	}

	return themeRequest{
		FacilityID:     facilityID,
		IsSystem:       parseBoolField(firstNonEmpty(r.FormValue("is_system"), r.FormValue("isSystem"))),
		Name:           firstNonEmpty(r.FormValue("name")),
		PrimaryColor:   firstNonEmpty(r.FormValue("primary_color"), r.FormValue("primaryColor")),
		SecondaryColor: firstNonEmpty(r.FormValue("secondary_color"), r.FormValue("secondaryColor")),
		TertiaryColor:  firstNonEmpty(r.FormValue("tertiary_color"), r.FormValue("tertiaryColor")),
		AccentColor:    firstNonEmpty(r.FormValue("accent_color"), r.FormValue("accentColor")),
		HighlightColor: firstNonEmpty(r.FormValue("highlight_color"), r.FormValue("highlightColor")),
	}, nil
}

func decodeThemeCloneRequest(r *http.Request) (themeCloneRequest, error) {
	if isJSONRequest(r) {
		var req themeCloneRequest
		return req, decodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return themeCloneRequest{}, err
	}

	facilityID, err := parseOptionalInt64Field(firstNonEmpty(r.FormValue("facility_id"), r.FormValue("facilityId")), "facility_id")
	if err != nil {
		return themeCloneRequest{}, err
	}

	return themeCloneRequest{
		FacilityID: facilityID,
		Name:       firstNonEmpty(r.FormValue("name")),
	}, nil
}

func decodeFacilityThemeRequest(r *http.Request) (facilityThemeRequest, error) {
	if isJSONRequest(r) {
		var req facilityThemeRequest
		return req, decodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return facilityThemeRequest{}, err
	}

	themeID, err := parseRequiredInt64Field(firstNonEmpty(r.FormValue("theme_id"), r.FormValue("themeId")), "theme_id")
	if err != nil {
		return facilityThemeRequest{}, err
	}

	return facilityThemeRequest{ThemeID: themeID}, nil
}

func isJSONRequest(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json")
}

func isHXRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true")
}

func parseOptionalInt64Field(raw string, field string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, fmt.Errorf("%s must be a positive integer", field)
	}
	return &value, nil
}

func parseRequiredInt64Field(raw string, field string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", field)
	}
	return value, nil
}

func parseBoolField(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	return value == "true" || value == "1" || value == "yes" || value == "on"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func renderHTMLComponent(ctx context.Context, w http.ResponseWriter, component templ.Component, headers map[string]string, logMsg string, errMsg string) bool {
	logger := log.Ctx(ctx)
	var buf bytes.Buffer
	if err := component.Render(ctx, &buf); err != nil {
		logger.Error().Err(err).Msg(logMsg)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return false
	}
	w.Header().Set("Content-Type", "text/html")
	for key, value := range headers {
		w.Header().Set(key, value)
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		logger.Error().Err(err).Msg("Failed to write response")
	}
	return true
}

func writeThemeFeedback(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(
		w,
		`<div class="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">%s</div>`,
		html.EscapeString(message),
	)
}

func newThemeEditorData(facilityID int64) themetempl.ThemeEditorData {
	return themeEditorData(models.DefaultTheme(), facilityID)
}

func themeEditorData(theme models.Theme, facilityID int64) themetempl.ThemeEditorData {
	return themetempl.ThemeEditorData{
		Theme:      themetempl.NewTheme(theme, 0),
		FacilityID: facilityID,
		ReadOnly:   theme.IsSystem,
	}
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
