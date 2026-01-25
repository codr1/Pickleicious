// internal/api/visitpacks/handlers.go
package visitpacks

import (
	"context"
	"database/sql"
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
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/api/htmx"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

const (
	visitPackQueryTimeout = 5 * time.Second
	maxVisitPackTypes     = 1000
	facilityIDQueryKey    = "facility_id"
	visitPackTypeIDParam  = "id"
)

var (
	queries     visitPackQueries
	queriesOnce sync.Once
)

type visitPackQueries interface {
	models.ThemeQueries
	CountVisitPackTypesByFacility(ctx context.Context, facilityID int64) (int64, error)
	CreateVisitPackType(ctx context.Context, arg dbgen.CreateVisitPackTypeParams) (dbgen.VisitPackType, error)
	DeactivateVisitPackType(ctx context.Context, arg dbgen.DeactivateVisitPackTypeParams) (dbgen.VisitPackType, error)
	ListVisitPackTypes(ctx context.Context, facilityID int64) ([]dbgen.VisitPackType, error)
	UpdateVisitPackType(ctx context.Context, arg dbgen.UpdateVisitPackTypeParams) (dbgen.VisitPackType, error)
}

type visitPackTypeRequest struct {
	FacilityID *int64 `json:"facilityId"`
	Name       string `json:"name"`
	PriceCents int64  `json:"priceCents"`
	VisitCount int64  `json:"visitCount"`
	ValidDays  int64  `json:"validDays"`
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

// GET /admin/visit-packs
func HandleVisitPackTypesPage(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), visitPackQueryTimeout)
	defer cancel()

	activeTheme, err := models.GetActiveTheme(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
		activeTheme = nil
	}

	sessionType := authz.SessionTypeFromContext(r.Context())
	page := layouts.Base(visitPackTypesPageComponent(facilityID), activeTheme, sessionType)
	if !apiutil.RenderHTMLComponent(r.Context(), w, page, nil, "Failed to render visit pack types page", "Failed to render page") {
		return
	}
}

// GET /api/v1/visit-pack-types
func HandleVisitPackTypesList(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), visitPackQueryTimeout)
	defer cancel()

	visitPackTypes, err := q.ListVisitPackTypes(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to list visit pack types")
		http.Error(w, "Failed to load visit pack types", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		component := visitPackTypesListComponent(visitPackTypes, facilityID)
		if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render visit pack types list", "Failed to render list") {
			return
		}
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"visitPackTypes": visitPackTypes}); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to write visit pack types response")
	}
}

// POST /api/v1/visit-pack-types
func HandleVisitPackTypeCreate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	req, err := decodeVisitPackTypeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateVisitPackTypeRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityID, err := facilityIDFromRequest(r, req.FacilityID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), visitPackQueryTimeout)
	defer cancel()

	count, err := q.CountVisitPackTypesByFacility(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to count visit pack types")
		http.Error(w, "Failed to validate visit pack type limit", http.StatusInternalServerError)
		return
	}
	if count >= maxVisitPackTypes {
		http.Error(w, "Visit pack type limit reached", http.StatusConflict)
		return
	}

	created, err := q.CreateVisitPackType(ctx, dbgen.CreateVisitPackTypeParams{
		FacilityID: facilityID,
		Name:       strings.TrimSpace(req.Name),
		PriceCents: req.PriceCents,
		VisitCount: req.VisitCount,
		ValidDays:  req.ValidDays,
		Status:     "active",
	})
	if err != nil {
		if apiutil.IsSQLiteForeignKeyViolation(err) {
			http.Error(w, "Facility not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to create visit pack type")
		http.Error(w, "Failed to create visit pack type", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		w.Header().Set("HX-Trigger", "refreshVisitPackTypesList")
		apiutil.WriteHTMLFeedback(w, http.StatusCreated, "Visit pack type created.")
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, created); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to write visit pack type create response")
	}
}

// PUT /api/v1/visit-pack-types/{id}
func HandleVisitPackTypeUpdate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	visitPackTypeID, err := visitPackTypeIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid visit pack type ID", http.StatusBadRequest)
		return
	}

	req, err := decodeVisitPackTypeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateVisitPackTypeRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityID, err := facilityIDFromRequest(r, req.FacilityID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), visitPackQueryTimeout)
	defer cancel()

	updated, err := q.UpdateVisitPackType(ctx, dbgen.UpdateVisitPackTypeParams{
		ID:         visitPackTypeID,
		FacilityID: facilityID,
		Name:       strings.TrimSpace(req.Name),
		PriceCents: req.PriceCents,
		VisitCount: req.VisitCount,
		ValidDays:  req.ValidDays,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Visit pack type not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("visit_pack_type_id", visitPackTypeID).Msg("Failed to update visit pack type")
		http.Error(w, "Failed to update visit pack type", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		w.Header().Set("HX-Trigger", "refreshVisitPackTypesList")
		apiutil.WriteHTMLFeedback(w, http.StatusOK, "Visit pack type updated.")
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, updated); err != nil {
		logger.Error().Err(err).Int64("visit_pack_type_id", visitPackTypeID).Msg("Failed to write visit pack type update response")
	}
}

// DELETE /api/v1/visit-pack-types/{id}
func HandleVisitPackTypeDeactivate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	visitPackTypeID, err := visitPackTypeIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid visit pack type ID", http.StatusBadRequest)
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

	ctx, cancel := context.WithTimeout(r.Context(), visitPackQueryTimeout)
	defer cancel()

	updated, err := q.DeactivateVisitPackType(ctx, dbgen.DeactivateVisitPackTypeParams{
		ID:         visitPackTypeID,
		FacilityID: facilityID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Visit pack type not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("visit_pack_type_id", visitPackTypeID).Msg("Failed to deactivate visit pack type")
		http.Error(w, "Failed to deactivate visit pack type", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		w.Header().Set("HX-Trigger", "refreshVisitPackTypesList")
		apiutil.WriteHTMLFeedback(w, http.StatusOK, "Visit pack type deactivated.")
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, updated); err != nil {
		logger.Error().Err(err).Int64("visit_pack_type_id", visitPackTypeID).Msg("Failed to write visit pack type deactivate response")
	}
}

func visitPackTypesPageComponent(facilityID int64) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<div class="space-y-6">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="flex flex-wrap items-center justify-between gap-4"><div><h2 class="text-2xl font-semibold text-foreground">Visit Pack Types</h2><p class="mt-1 text-sm text-muted-foreground">Manage facility visit pack offerings.</p></div></div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="grid gap-6 lg:grid-cols-[1.2fr_1fr]">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<section class="rounded-lg border border-border bg-background p-4 shadow-sm">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, fmt.Sprintf(`<div class="mb-4 flex items-center justify-between"><h3 class="text-lg font-semibold text-foreground">Visit pack types</h3><span class="text-xs text-muted-foreground">Facility ID %d</span></div>`, facilityID)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, fmt.Sprintf(`<div id="visit-pack-types-list" class="space-y-3" hx-get="/api/v1/visit-pack-types?facility_id=%d" hx-trigger="load, refreshVisitPackTypesList from:body" hx-swap="innerHTML">`, facilityID)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="rounded border border-dashed border-border p-4 text-sm text-muted-foreground">Loading visit pack types...</div></div></section>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<section class="rounded-lg border border-border bg-background p-4 shadow-sm">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="mb-4"><h3 class="text-lg font-semibold text-foreground">Create visit pack type</h3></div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div id="visit-pack-types-feedback" class="mb-3"></div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, fmt.Sprintf(`<form class="space-y-4" hx-post="/api/v1/visit-pack-types" hx-target="#visit-pack-types-feedback" hx-swap="innerHTML" hx-trigger="submit"><input type="hidden" name="facility_id" value="%d"/>`, facilityID)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div><label class="block text-sm font-medium text-foreground">Name</label><input type="text" name="name" required class="mt-1 w-full rounded-md border border-border px-3 py-2 text-sm focus:border-blue-500 focus:ring-blue-500"/></div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="grid gap-3 sm:grid-cols-2">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div><label class="block text-sm font-medium text-foreground">Price (cents)</label><input type="number" name="price_cents" min="0" step="1" required class="mt-1 w-full rounded-md border border-border px-3 py-2 text-sm focus:border-blue-500 focus:ring-blue-500"/></div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div><label class="block text-sm font-medium text-foreground">Visit count</label><input type="number" name="visit_count" min="1" step="1" required class="mt-1 w-full rounded-md border border-border px-3 py-2 text-sm focus:border-blue-500 focus:ring-blue-500"/></div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div><label class="block text-sm font-medium text-foreground">Valid days</label><input type="number" name="valid_days" min="1" step="1" required class="mt-1 w-full rounded-md border border-border px-3 py-2 text-sm focus:border-blue-500 focus:ring-blue-500"/></div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `</div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<button type="submit" class="rounded-md bg-blue-600 px-4 py-2 text-sm font-semibold text-white hover:bg-blue-700">Create</button></form></section></div></div>`); err != nil {
			return err
		}
		return nil
	})
}

func visitPackTypesListComponent(visitPackTypes []dbgen.VisitPackType, facilityID int64) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, buildVisitPackTypesListHTML(visitPackTypes, facilityID))
		return err
	})
}

func buildVisitPackTypesListHTML(visitPackTypes []dbgen.VisitPackType, facilityID int64) string {
	if len(visitPackTypes) == 0 {
		return `<div class="rounded border border-dashed border-border p-4 text-sm text-muted-foreground">No visit pack types configured.</div>`
	}

	var builder strings.Builder
	builder.WriteString(`<div class="space-y-3">`)
	for _, packType := range visitPackTypes {
		name := html.EscapeString(strings.TrimSpace(packType.Name))
		status := html.EscapeString(strings.TrimSpace(packType.Status))
		builder.WriteString(`<div class="rounded-lg border border-border bg-background p-4 shadow-sm">`)
		builder.WriteString(`<div class="flex flex-wrap items-start justify-between gap-4">`)
		builder.WriteString(`<div>`)
		builder.WriteString(fmt.Sprintf(`<div class="text-sm font-semibold text-foreground">%s</div>`, name))
		builder.WriteString(fmt.Sprintf(`<div class="mt-1 text-xs text-muted-foreground">%d visits • valid %d days • %s</div>`, packType.VisitCount, packType.ValidDays, formatPriceCents(packType.PriceCents)))
		builder.WriteString(fmt.Sprintf(`<div class="mt-1 text-xs text-muted-foreground">Status: %s</div>`, status))
		builder.WriteString(`</div>`)
		if !strings.EqualFold(packType.Status, "inactive") {
			builder.WriteString(`<div class="flex items-center gap-2">`)
			builder.WriteString(fmt.Sprintf(`<button class="rounded-md border border-border bg-background px-3 py-2 text-xs font-medium text-foreground hover:bg-muted" hx-delete="/api/v1/visit-pack-types/%d?facility_id=%d" hx-confirm="Deactivate this visit pack type?" hx-target="#visit-pack-types-feedback" hx-swap="innerHTML">Deactivate</button>`, packType.ID, facilityID))
			builder.WriteString(`</div>`)
		}
		builder.WriteString(`</div></div>`)
	}
	builder.WriteString(`</div>`)
	return builder.String()
}

func formatPriceCents(cents int64) string {
	return fmt.Sprintf("$%.2f", float64(cents)/100)
}

func decodeVisitPackTypeRequest(r *http.Request) (visitPackTypeRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req visitPackTypeRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return visitPackTypeRequest{}, err
	}

	facilityID, err := apiutil.ParseOptionalInt64Field(apiutil.FirstNonEmpty(r.FormValue("facility_id"), r.FormValue("facilityId")), "facility_id")
	if err != nil {
		return visitPackTypeRequest{}, err
	}

	priceCents, err := parseNonNegativeInt64Field(apiutil.FirstNonEmpty(r.FormValue("price_cents"), r.FormValue("priceCents")), "price_cents")
	if err != nil {
		return visitPackTypeRequest{}, err
	}

	visitCount, err := parsePositiveInt64Field(apiutil.FirstNonEmpty(r.FormValue("visit_count"), r.FormValue("visitCount")), "visit_count")
	if err != nil {
		return visitPackTypeRequest{}, err
	}

	validDays, err := parsePositiveInt64Field(apiutil.FirstNonEmpty(r.FormValue("valid_days"), r.FormValue("validDays")), "valid_days")
	if err != nil {
		return visitPackTypeRequest{}, err
	}

	return visitPackTypeRequest{
		FacilityID: facilityID,
		Name:       apiutil.FirstNonEmpty(r.FormValue("name")),
		PriceCents: priceCents,
		VisitCount: visitCount,
		ValidDays:  validDays,
	}, nil
}

func validateVisitPackTypeRequest(req visitPackTypeRequest) error {
	switch {
	case strings.TrimSpace(req.Name) == "":
		return fmt.Errorf("name is required")
	case req.PriceCents < 0:
		return fmt.Errorf("price_cents must be 0 or greater")
	case req.VisitCount <= 0:
		return fmt.Errorf("visit_count must be greater than 0")
	case req.ValidDays <= 0:
		return fmt.Errorf("valid_days must be greater than 0")
	default:
		return nil
	}
}

func parseNonNegativeInt64Field(raw string, field string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be 0 or greater", field)
	}
	return value, nil
}

func parsePositiveInt64Field(raw string, field string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be greater than 0", field)
	}
	return value, nil
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

func facilityIDFromRequest(r *http.Request, fromBody *int64) (int64, error) {
	if fromBody != nil {
		if *fromBody <= 0 {
			return 0, fmt.Errorf("facility_id must be a positive integer")
		}
		return *fromBody, nil
	}
	return facilityIDFromQuery(r)
}

func visitPackTypeIDFromRequest(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(visitPackTypeIDParam))
	if raw == "" {
		return 0, fmt.Errorf("invalid visit pack type ID")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid visit pack type ID")
	}
	return value, nil
}

func loadQueries() visitPackQueries {
	return queries
}
