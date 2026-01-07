// internal/api/themes/handlers.go
package themes

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
	"github.com/codr1/Pickleicious/internal/api/htmx"
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
	queries     themeQueries
	queriesOnce sync.Once
)

type themeQueries interface {
	models.ThemeQueries
	CountFacilityThemeName(ctx context.Context, arg dbgen.CountFacilityThemeNameParams) (int64, error)
	CountFacilityThemeNameExcludingID(ctx context.Context, arg dbgen.CountFacilityThemeNameExcludingIDParams) (int64, error)
	CountFacilityThemes(ctx context.Context, facilityID sql.NullInt64) (int64, error)
	CountThemeUsage(ctx context.Context, themeID sql.NullInt64) (int64, error)
	CreateTheme(ctx context.Context, arg dbgen.CreateThemeParams) (dbgen.Theme, error)
	DeleteTheme(ctx context.Context, id int64) (int64, error)
	UpdateTheme(ctx context.Context, arg dbgen.UpdateThemeParams) (dbgen.Theme, error)
	UpsertActiveThemeID(ctx context.Context, arg dbgen.UpsertActiveThemeIDParams) (int64, error)
}

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
	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
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
	sessionType := authz.SessionTypeFromContext(r.Context())
	page := layouts.Base(themetempl.ThemeAdminLayout(facilityID, editor), activeTheme, sessionType)
	if !apiutil.RenderHTMLComponent(r.Context(), w, page, nil, "Failed to render themes page", "Failed to render page") {
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
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render new theme form", "Failed to render form") {
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

	theme := models.ThemeFromDB(row)
	facilityID, err := facilityIDFromQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !theme.IsSystem && theme.FacilityID != nil && *theme.FacilityID != facilityID {
		http.Error(w, "Theme does not belong to facility", http.StatusBadRequest)
		return
	}

	if !htmx.IsRequest(r) {
		apiutil.WriteJSON(w, http.StatusOK, theme)
		return
	}

	editor := themeEditorData(theme, facilityID)
	component := themetempl.ThemeEditor(editor)
	if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render theme form", "Failed to render form") {
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

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
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
	if htmx.IsRequest(r) {
		activeThemeID, err := q.GetActiveThemeID(ctx, facilityID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
			activeThemeID = 0
		}

		component := themetempl.ThemeList(themetempl.NewThemes(themes, activeThemeID), facilityID)
		if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render themes list", "Failed to render list") {
			return
		}
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"themes": themes}); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to write themes list response")
	}
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

	if !apiutil.RequireFacilityAccess(w, r, *req.FacilityID) {
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
		if apiutil.IsSQLiteForeignKeyViolation(err) {
			http.Error(w, "Facility not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", *req.FacilityID).Msg("Failed to create theme")
		http.Error(w, "Failed to create theme", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		w.Header().Set("HX-Trigger", "refreshThemesList")
		apiutil.WriteHTMLFeedback(w, http.StatusCreated, "Theme created.")
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, models.ThemeFromDB(created)); err != nil {
		logger.Error().Err(err).Int64("facility_id", *req.FacilityID).Msg("Failed to write theme create response")
	}
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
	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
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

	if htmx.IsRequest(r) {
		w.Header().Set("HX-Trigger", "refreshThemesList")
		apiutil.WriteHTMLFeedback(w, http.StatusOK, "Theme updated.")
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, models.ThemeFromDB(updated)); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Int64("theme_id", themeID).Msg("Failed to write theme update response")
	}
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

	if !apiutil.RequireFacilityAccess(w, r, existing.FacilityID.Int64) {
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
		if apiutil.IsSQLiteForeignKeyViolation(err) {
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

	if htmx.IsRequest(r) {
		w.Header().Set("HX-Trigger", "refreshThemesList")
		apiutil.WriteHTMLFeedback(w, http.StatusOK, "Theme deleted.")
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

	if !apiutil.RequireFacilityAccess(w, r, *req.FacilityID) {
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
		if apiutil.IsSQLiteForeignKeyViolation(err) {
			http.Error(w, "Facility not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", *req.FacilityID).Msg("Failed to clone theme")
		http.Error(w, "Failed to clone theme", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		w.Header().Set("HX-Trigger", "refreshThemesList")
		apiutil.WriteHTMLFeedback(w, http.StatusCreated, "Theme cloned.")
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, models.ThemeFromDB(created)); err != nil {
		logger.Error().Err(err).Int64("facility_id", *req.FacilityID).Msg("Failed to write theme clone response")
	}
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

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
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

	if htmx.IsRequest(r) {
		w.Header().Set("HX-Trigger", "refreshThemesList")
		apiutil.WriteHTMLFeedback(w, http.StatusOK, "Active theme updated.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

func decodeThemeRequest(r *http.Request) (themeRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req themeRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return themeRequest{}, err
	}

	facilityID, err := apiutil.ParseOptionalInt64Field(apiutil.FirstNonEmpty(r.FormValue("facility_id"), r.FormValue("facilityId")), "facility_id")
	if err != nil {
		return themeRequest{}, err
	}

	return themeRequest{
		FacilityID:     facilityID,
		IsSystem:       apiutil.ParseBool(apiutil.FirstNonEmpty(r.FormValue("is_system"), r.FormValue("isSystem"))),
		Name:           apiutil.FirstNonEmpty(r.FormValue("name")),
		PrimaryColor:   apiutil.FirstNonEmpty(r.FormValue("primary_color"), r.FormValue("primaryColor")),
		SecondaryColor: apiutil.FirstNonEmpty(r.FormValue("secondary_color"), r.FormValue("secondaryColor")),
		TertiaryColor:  apiutil.FirstNonEmpty(r.FormValue("tertiary_color"), r.FormValue("tertiaryColor")),
		AccentColor:    apiutil.FirstNonEmpty(r.FormValue("accent_color"), r.FormValue("accentColor")),
		HighlightColor: apiutil.FirstNonEmpty(r.FormValue("highlight_color"), r.FormValue("highlightColor")),
	}, nil
}

func decodeThemeCloneRequest(r *http.Request) (themeCloneRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req themeCloneRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return themeCloneRequest{}, err
	}

	facilityID, err := apiutil.ParseOptionalInt64Field(apiutil.FirstNonEmpty(r.FormValue("facility_id"), r.FormValue("facilityId")), "facility_id")
	if err != nil {
		return themeCloneRequest{}, err
	}

	return themeCloneRequest{
		FacilityID: facilityID,
		Name:       apiutil.FirstNonEmpty(r.FormValue("name")),
	}, nil
}

func decodeFacilityThemeRequest(r *http.Request) (facilityThemeRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req facilityThemeRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return facilityThemeRequest{}, err
	}

	themeID, err := apiutil.ParseRequiredInt64Field(apiutil.FirstNonEmpty(r.FormValue("theme_id"), r.FormValue("themeId")), "theme_id")
	if err != nil {
		return facilityThemeRequest{}, err
	}

	return facilityThemeRequest{ThemeID: themeID}, nil
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

func loadQueries() themeQueries {
	return queries
}
