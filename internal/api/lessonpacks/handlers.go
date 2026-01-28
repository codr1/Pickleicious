package lessonpacks

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
	lessonPackQueryTimeout = 5 * time.Second
	maxLessonPackTypes     = 1000
	lessonPackTypeIDParam  = "id"
	userIDParam            = "id"
)

var (
	queries     lessonPackQueries
	queriesOnce sync.Once
)

type lessonPackQueries interface {
	models.ThemeQueries
	CountLessonPackageTypesByFacility(ctx context.Context, facilityID int64) (int64, error)
	CreateLessonPackage(ctx context.Context, arg dbgen.CreateLessonPackageParams) (dbgen.LessonPackage, error)
	CreateLessonPackageType(ctx context.Context, arg dbgen.CreateLessonPackageTypeParams) (dbgen.LessonPackageType, error)
	DeactivateLessonPackageType(ctx context.Context, arg dbgen.DeactivateLessonPackageTypeParams) (dbgen.LessonPackageType, error)
	GetLessonPackageType(ctx context.Context, arg dbgen.GetLessonPackageTypeParams) (dbgen.LessonPackageType, error)
	ListActiveLessonPackagesForUser(ctx context.Context, arg dbgen.ListActiveLessonPackagesForUserParams) ([]dbgen.LessonPackage, error)
	ListActiveLessonPackagesForUserByFacility(ctx context.Context, arg dbgen.ListActiveLessonPackagesForUserByFacilityParams) ([]dbgen.LessonPackage, error)
	ListLessonPackageTypes(ctx context.Context, facilityID int64) ([]dbgen.LessonPackageType, error)
	UpdateLessonPackageType(ctx context.Context, arg dbgen.UpdateLessonPackageTypeParams) (dbgen.LessonPackageType, error)
}

type lessonPackageTypeRequest struct {
	FacilityID  *int64 `json:"facilityId"`
	Name        string `json:"name"`
	PriceCents  int64  `json:"priceCents"`
	LessonCount int64  `json:"lessonCount"`
	ValidDays   int64  `json:"validDays"`
}

type lessonPackageSaleRequest struct {
	FacilityID   *int64     `json:"facilityId"`
	UserID       int64      `json:"userId"`
	PackTypeID   int64      `json:"packTypeId"`
	PurchaseDate *time.Time `json:"purchaseDate"`
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

// GET /admin/lesson-packages
func HandleLessonPackageTypesPage(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), lessonPackQueryTimeout)
	defer cancel()

	activeTheme, err := models.GetActiveTheme(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
		activeTheme = nil
	}

	sessionType := authz.SessionTypeFromContext(r.Context())
	page := layouts.Base(lessonPackageTypesPageComponent(facilityID), activeTheme, sessionType)
	if !apiutil.RenderHTMLComponent(r.Context(), w, page, nil, "Failed to render lesson package types page", "Failed to render page") {
		return
	}
}

// GET /api/v1/lesson-package-types
func HandleLessonPackageTypesList(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), lessonPackQueryTimeout)
	defer cancel()

	lessonPackageTypes, err := q.ListLessonPackageTypes(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to list lesson package types")
		http.Error(w, "Failed to load lesson package types", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		component := lessonPackageTypesListComponent(lessonPackageTypes, facilityID)
		if !apiutil.RenderHTMLComponent(r.Context(), w, component, nil, "Failed to render lesson package types list", "Failed to render list") {
			return
		}
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"lessonPackageTypes": lessonPackageTypes}); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to write lesson package types response")
	}
}

// POST /api/v1/lesson-package-types
func HandleLessonPackageTypeCreate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	req, err := decodeLessonPackageTypeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateLessonPackageTypeRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityID, err := apiutil.FacilityIDFromRequest(r, req.FacilityID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), lessonPackQueryTimeout)
	defer cancel()

	count, err := q.CountLessonPackageTypesByFacility(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to count lesson package types")
		http.Error(w, "Failed to validate lesson package type limit", http.StatusInternalServerError)
		return
	}
	if count >= maxLessonPackTypes {
		http.Error(w, "Lesson package type limit reached", http.StatusConflict)
		return
	}

	created, err := q.CreateLessonPackageType(ctx, dbgen.CreateLessonPackageTypeParams{
		FacilityID:  facilityID,
		Name:        strings.TrimSpace(req.Name),
		PriceCents:  req.PriceCents,
		LessonCount: req.LessonCount,
		ValidDays:   req.ValidDays,
		Status:      "active",
	})
	if err != nil {
		if apiutil.IsSQLiteForeignKeyViolation(err) {
			http.Error(w, "Facility not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to create lesson package type")
		http.Error(w, "Failed to create lesson package type", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		w.Header().Set("HX-Trigger", "refreshLessonPackageTypesList")
		apiutil.WriteHTMLFeedback(w, http.StatusCreated, "Lesson package type created.")
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, created); err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to write lesson package type create response")
	}
}

// PUT /api/v1/lesson-package-types/{id}
func HandleLessonPackageTypeUpdate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	lessonPackTypeID, err := lessonPackTypeIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid lesson package type ID", http.StatusBadRequest)
		return
	}

	req, err := decodeLessonPackageTypeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateLessonPackageTypeRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityID, err := apiutil.FacilityIDFromRequest(r, req.FacilityID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), lessonPackQueryTimeout)
	defer cancel()

	updated, err := q.UpdateLessonPackageType(ctx, dbgen.UpdateLessonPackageTypeParams{
		ID:          lessonPackTypeID,
		FacilityID:  facilityID,
		Name:        strings.TrimSpace(req.Name),
		PriceCents:  req.PriceCents,
		LessonCount: req.LessonCount,
		ValidDays:   req.ValidDays,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Lesson package type not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("lesson_package_type_id", lessonPackTypeID).Msg("Failed to update lesson package type")
		http.Error(w, "Failed to update lesson package type", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		w.Header().Set("HX-Trigger", "refreshLessonPackageTypesList")
		apiutil.WriteHTMLFeedback(w, http.StatusOK, "Lesson package type updated.")
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, updated); err != nil {
		logger.Error().Err(err).Int64("lesson_package_type_id", lessonPackTypeID).Msg("Failed to write lesson package type update response")
	}
}

// DELETE /api/v1/lesson-package-types/{id}
func HandleLessonPackageTypeDeactivate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	lessonPackTypeID, err := lessonPackTypeIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid lesson package type ID", http.StatusBadRequest)
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

	ctx, cancel := context.WithTimeout(r.Context(), lessonPackQueryTimeout)
	defer cancel()

	updated, err := q.DeactivateLessonPackageType(ctx, dbgen.DeactivateLessonPackageTypeParams{
		ID:         lessonPackTypeID,
		FacilityID: facilityID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Lesson package type not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("lesson_package_type_id", lessonPackTypeID).Msg("Failed to deactivate lesson package type")
		http.Error(w, "Failed to deactivate lesson package type", http.StatusInternalServerError)
		return
	}

	if htmx.IsRequest(r) {
		w.Header().Set("HX-Trigger", "refreshLessonPackageTypesList")
		apiutil.WriteHTMLFeedback(w, http.StatusOK, "Lesson package type deactivated.")
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, updated); err != nil {
		logger.Error().Err(err).Int64("lesson_package_type_id", lessonPackTypeID).Msg("Failed to write lesson package type deactivate response")
	}
}

// POST /api/v1/lesson-packages
func HandleLessonPackageSale(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := authz.UserFromContext(r.Context())
	if !authz.IsStaff(user) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	req, err := decodeLessonPackageSaleRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateLessonPackageSaleRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	facilityID, err := apiutil.FacilityIDFromRequest(r, req.FacilityID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !apiutil.RequireFacilityAccess(w, r, facilityID) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), lessonPackQueryTimeout)
	defer cancel()

	packType, err := q.GetLessonPackageType(ctx, dbgen.GetLessonPackageTypeParams{
		ID:         req.PackTypeID,
		FacilityID: facilityID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Lesson package type not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("lesson_package_type_id", req.PackTypeID).Msg("Failed to load lesson package type")
		http.Error(w, "Failed to load lesson package type", http.StatusInternalServerError)
		return
	}
	if !strings.EqualFold(packType.Status, "active") {
		http.Error(w, "Lesson package type is inactive", http.StatusConflict)
		return
	}

	purchaseDate := time.Now()
	if req.PurchaseDate != nil {
		purchaseDate = *req.PurchaseDate
	}

	created, err := q.CreateLessonPackage(ctx, dbgen.CreateLessonPackageParams{
		UserID:       req.UserID,
		PurchaseDate: purchaseDate,
		Status:       "active",
		PackTypeID:   req.PackTypeID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Lesson package type is inactive", http.StatusConflict)
			return
		}
		if apiutil.IsSQLiteForeignKeyViolation(err) {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("lesson_package_type_id", req.PackTypeID).Int64("user_id", req.UserID).Msg("Failed to create lesson package")
		http.Error(w, "Failed to create lesson package", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusCreated, created); err != nil {
		logger.Error().Err(err).Int64("lesson_package_id", created.ID).Msg("Failed to write lesson package response")
	}
}

// GET /api/v1/users/{id}/lesson-packages
func HandleListUserLessonPackages(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	userID, err := userIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	requestUser := authz.UserFromContext(r.Context())
	if requestUser == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var facilityID *int64
	if requestUser.IsStaff {
		facilityIDValue, err := apiutil.FacilityIDFromQuery(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !apiutil.RequireFacilityAccess(w, r, facilityIDValue) {
			return
		}
		facilityID = &facilityIDValue
	} else if requestUser.ID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), lessonPackQueryTimeout)
	defer cancel()

	var lessonPackages []dbgen.LessonPackage
	if facilityID != nil {
		lessonPackages, err = q.ListActiveLessonPackagesForUserByFacility(ctx, dbgen.ListActiveLessonPackagesForUserByFacilityParams{
			UserID:         userID,
			FacilityID:     *facilityID,
			ComparisonTime: time.Now(),
		})
	} else {
		lessonPackages, err = q.ListActiveLessonPackagesForUser(ctx, dbgen.ListActiveLessonPackagesForUserParams{
			UserID:         userID,
			ComparisonTime: time.Now(),
		})
	}
	if err != nil {
		logger.Error().Err(err).Int64("user_id", userID).Msg("Failed to list lesson packages")
		http.Error(w, "Failed to load lesson packages", http.StatusInternalServerError)
		return
	}

	if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"lessonPackages": lessonPackages}); err != nil {
		logger.Error().Err(err).Int64("user_id", userID).Msg("Failed to write lesson packages response")
	}
}

func lessonPackageTypesPageComponent(facilityID int64) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<div class="space-y-6">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="flex flex-wrap items-center justify-between gap-4"><div><h2 class="text-2xl font-semibold text-foreground">Lesson Package Types</h2><p class="mt-1 text-sm text-muted-foreground">Manage facility lesson package offerings.</p></div></div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="grid gap-6 lg:grid-cols-[1.2fr_1fr]">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<section class="rounded-lg border border-border bg-background p-4 shadow-sm">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, fmt.Sprintf(`<div class="mb-4 flex items-center justify-between"><h3 class="text-lg font-semibold text-foreground">Lesson package types</h3><span class="text-xs text-muted-foreground">Facility ID %d</span></div>`, facilityID)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, fmt.Sprintf(`<div id="lesson-package-types-list" class="space-y-3" hx-get="/api/v1/lesson-package-types?facility_id=%d" hx-trigger="load, refreshLessonPackageTypesList from:body" hx-swap="innerHTML">`, facilityID)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="rounded border border-dashed border-border p-4 text-sm text-muted-foreground">Loading lesson package types...</div></div></section>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<section class="rounded-lg border border-border bg-background p-4 shadow-sm">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="mb-4"><h3 class="text-lg font-semibold text-foreground">Create lesson package type</h3></div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div id="lesson-package-types-feedback" class="mb-3"></div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, fmt.Sprintf(`<form class="space-y-4" hx-post="/api/v1/lesson-package-types" hx-target="#lesson-package-types-feedback" hx-swap="innerHTML" hx-trigger="submit"><input type="hidden" name="facility_id" value="%d"/>`, facilityID)); err != nil {
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
		if _, err := io.WriteString(w, `<div><label class="block text-sm font-medium text-foreground">Lesson count</label><input type="number" name="lesson_count" min="1" step="1" required class="mt-1 w-full rounded-md border border-border px-3 py-2 text-sm focus:border-blue-500 focus:ring-blue-500"/></div>`); err != nil {
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

func lessonPackageTypesListComponent(lessonPackageTypes []dbgen.LessonPackageType, facilityID int64) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, buildLessonPackageTypesListHTML(lessonPackageTypes, facilityID))
		return err
	})
}

func buildLessonPackageTypesListHTML(lessonPackageTypes []dbgen.LessonPackageType, facilityID int64) string {
	if len(lessonPackageTypes) == 0 {
		return `<div class="rounded border border-dashed border-border p-4 text-sm text-muted-foreground">No lesson package types configured.</div>`
	}

	var builder strings.Builder
	builder.WriteString(`<div class="space-y-3">`)
	for _, packType := range lessonPackageTypes {
		name := html.EscapeString(strings.TrimSpace(packType.Name))
		status := html.EscapeString(strings.TrimSpace(packType.Status))
		builder.WriteString(`<div class="rounded-lg border border-border bg-background p-4 shadow-sm">`)
		builder.WriteString(`<div class="flex flex-wrap items-start justify-between gap-4">`)
		builder.WriteString(`<div>`)
		builder.WriteString(fmt.Sprintf(`<div class="text-sm font-semibold text-foreground">%s</div>`, name))
		builder.WriteString(fmt.Sprintf(`<div class="mt-1 text-xs text-muted-foreground">%d lessons • valid %d days • %s</div>`, packType.LessonCount, packType.ValidDays, apiutil.FormatPriceCents(packType.PriceCents)))
		builder.WriteString(fmt.Sprintf(`<div class="mt-1 text-xs text-muted-foreground">Status: %s</div>`, status))
		builder.WriteString(`</div>`)
		if !strings.EqualFold(packType.Status, "inactive") {
			builder.WriteString(`<div class="flex items-center gap-2">`)
			builder.WriteString(fmt.Sprintf(`<button class="rounded-md border border-border bg-background px-3 py-2 text-xs font-medium text-foreground hover:bg-muted" hx-delete="/api/v1/lesson-package-types/%d?facility_id=%d" hx-confirm="Deactivate this lesson package type?" hx-target="#lesson-package-types-feedback" hx-swap="innerHTML">Deactivate</button>`, packType.ID, facilityID))
			builder.WriteString(`</div>`)
		}
		builder.WriteString(`</div></div>`)
	}
	builder.WriteString(`</div>`)
	return builder.String()
}

func decodeLessonPackageTypeRequest(r *http.Request) (lessonPackageTypeRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req lessonPackageTypeRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return lessonPackageTypeRequest{}, err
	}

	facilityID, err := apiutil.ParseOptionalInt64Field(apiutil.FirstNonEmpty(r.FormValue("facility_id"), r.FormValue("facilityId")), "facility_id")
	if err != nil {
		return lessonPackageTypeRequest{}, err
	}

	priceCents, err := apiutil.ParseNonNegativeInt64Field(apiutil.FirstNonEmpty(r.FormValue("price_cents"), r.FormValue("priceCents")), "price_cents")
	if err != nil {
		return lessonPackageTypeRequest{}, err
	}

	lessonCount, err := apiutil.ParsePositiveInt64Field(apiutil.FirstNonEmpty(r.FormValue("lesson_count"), r.FormValue("lessonCount")), "lesson_count")
	if err != nil {
		return lessonPackageTypeRequest{}, err
	}

	validDays, err := apiutil.ParsePositiveInt64Field(apiutil.FirstNonEmpty(r.FormValue("valid_days"), r.FormValue("validDays")), "valid_days")
	if err != nil {
		return lessonPackageTypeRequest{}, err
	}

	return lessonPackageTypeRequest{
		FacilityID:  facilityID,
		Name:        apiutil.FirstNonEmpty(r.FormValue("name")),
		PriceCents:  priceCents,
		LessonCount: lessonCount,
		ValidDays:   validDays,
	}, nil
}

func validateLessonPackageTypeRequest(req lessonPackageTypeRequest) error {
	switch {
	case strings.TrimSpace(req.Name) == "":
		return fmt.Errorf("name is required")
	case req.PriceCents < 0:
		return fmt.Errorf("price_cents must be 0 or greater")
	case req.LessonCount <= 0:
		return fmt.Errorf("lesson_count must be greater than 0")
	case req.ValidDays <= 0:
		return fmt.Errorf("valid_days must be greater than 0")
	default:
		return nil
	}
}

func decodeLessonPackageSaleRequest(r *http.Request) (lessonPackageSaleRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req lessonPackageSaleRequest
		return req, apiutil.DecodeJSON(r, &req)
	}

	if err := r.ParseForm(); err != nil {
		return lessonPackageSaleRequest{}, err
	}

	facilityID, err := apiutil.ParseOptionalInt64Field(apiutil.FirstNonEmpty(r.FormValue("facility_id"), r.FormValue("facilityId")), "facility_id")
	if err != nil {
		return lessonPackageSaleRequest{}, err
	}

	userID, err := apiutil.ParseRequiredInt64Field(apiutil.FirstNonEmpty(r.FormValue("user_id"), r.FormValue("userId")), "user_id")
	if err != nil {
		return lessonPackageSaleRequest{}, err
	}

	packTypeID, err := apiutil.ParseRequiredInt64Field(apiutil.FirstNonEmpty(r.FormValue("pack_type_id"), r.FormValue("packTypeId")), "pack_type_id")
	if err != nil {
		return lessonPackageSaleRequest{}, err
	}

	rawPurchaseDate := apiutil.FirstNonEmpty(r.FormValue("purchase_date"), r.FormValue("purchaseDate"))
	var purchaseDate *time.Time
	if strings.TrimSpace(rawPurchaseDate) != "" {
		parsed, err := apiutil.ParsePurchaseDate(rawPurchaseDate)
		if err != nil {
			return lessonPackageSaleRequest{}, err
		}
		purchaseDate = &parsed
	}

	return lessonPackageSaleRequest{
		FacilityID:   facilityID,
		UserID:       userID,
		PackTypeID:   packTypeID,
		PurchaseDate: purchaseDate,
	}, nil
}

func validateLessonPackageSaleRequest(req lessonPackageSaleRequest) error {
	if req.UserID <= 0 {
		return fmt.Errorf("user_id must be a positive integer")
	}
	if req.PackTypeID <= 0 {
		return fmt.Errorf("pack_type_id must be a positive integer")
	}
	if req.PurchaseDate != nil && req.PurchaseDate.IsZero() {
		return fmt.Errorf("purchase_date must be a valid date")
	}
	return nil
}

func lessonPackTypeIDFromRequest(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(lessonPackTypeIDParam))
	if raw == "" {
		return 0, fmt.Errorf("invalid lesson package type ID")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid lesson package type ID")
	}
	return value, nil
}

func userIDFromRequest(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(userIDParam))
	if raw == "" {
		return 0, fmt.Errorf("invalid user ID")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid user ID")
	}
	return value, nil
}

func loadQueries() lessonPackQueries {
	return queries
}
