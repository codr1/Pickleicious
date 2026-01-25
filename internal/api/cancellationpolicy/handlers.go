// internal/api/cancellationpolicy/handlers.go
package cancellationpolicy

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

	"github.com/a-h/templ"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/api/authz"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	cancellationpolicytempl "github.com/codr1/Pickleicious/internal/templates/components/cancellationpolicy"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

const (
	cancellationPolicyQueryTimeout = 5 * time.Second
	facilityIDQueryKey             = "facility_id"
	tierIDParam                    = "id"
)

var (
	queries     *dbgen.Queries
	queriesOnce sync.Once
)

type cancellationPolicyTierRequest struct {
	FacilityID        *int64 `json:"facilityId"`
	ReservationTypeID *int64 `json:"reservationTypeId"`
	MinHoursBefore    int64  `json:"minHoursBefore"`
	RefundPercentage  int64  `json:"refundPercentage"`
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

// GET /admin/cancellation-policy?facility_id=X
func HandleCancellationPolicyPage(w http.ResponseWriter, r *http.Request) {
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

	filterReservationTypeID, err := reservationTypeFilterIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), cancellationPolicyQueryTimeout)
	defer cancel()

	tiers, err := q.ListCancellationPolicyTiers(ctx, dbgen.ListCancellationPolicyTiersParams{
		FacilityID:        facilityID,
		ReservationTypeID: apiutil.ToNullInt64(filterReservationTypeID),
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to fetch cancellation policy tiers")
		http.Error(w, "Failed to load cancellation policy tiers", http.StatusInternalServerError)
		return
	}
	reservationTypes, err := q.ListReservationTypes(ctx)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load reservation types")
		reservationTypes = nil
	}

	activeTheme, err := models.GetActiveTheme(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
		activeTheme = nil
	}

	sessionType := authz.SessionTypeFromContext(r.Context())
	filterOptions := reservationTypeFilterOptions(reservationTypes)
	page := layouts.Base(cancellationpolicytempl.CancellationPolicyLayout(
		facilityID,
		cancellationPolicyTierData(tiers, reservationTypeNameMap(reservationTypes)),
		reservationTypeOptions(reservationTypes, filterOptions),
		filterOptions,
		filterReservationTypeID,
	), activeTheme, sessionType)
	if !apiutil.RenderHTMLComponent(r.Context(), w, page, nil, "Failed to render cancellation policy page", "Failed to render page") {
		return
	}
}

// GET /api/v1/cancellation-policy/tiers?facility_id=X
func HandleCancellationPolicyTierList(w http.ResponseWriter, r *http.Request) {
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

	filterReservationTypeID, err := reservationTypeFilterIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), cancellationPolicyQueryTimeout)
	defer cancel()

	tiers, err := q.ListCancellationPolicyTiers(ctx, dbgen.ListCancellationPolicyTiersParams{
		FacilityID:        facilityID,
		ReservationTypeID: apiutil.ToNullInt64(filterReservationTypeID),
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to fetch cancellation policy tiers")
		http.Error(w, "Failed to load cancellation policy tiers", http.StatusInternalServerError)
		return
	}

	reservationTypes, err := q.ListReservationTypes(ctx)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load reservation types")
		reservationTypes = nil
	}

	renderCancellationPolicyList(r.Context(), w, facilityID, tiers, reservationTypeNameMap(reservationTypes), filterReservationTypeID)
}

// POST /api/v1/cancellation-policy/tiers
func HandleCancellationPolicyTierCreate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	req, err := decodeCancellationPolicyTierRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCancellationPolicyTierRequest(req); err != nil {
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

	ctx, cancel := context.WithTimeout(r.Context(), cancellationPolicyQueryTimeout)
	defer cancel()

	tier, err := q.CreateCancellationPolicyTier(ctx, dbgen.CreateCancellationPolicyTierParams{
		FacilityID:        facilityID,
		ReservationTypeID: apiutil.ToNullInt64(req.ReservationTypeID),
		MinHoursBefore:    req.MinHoursBefore,
		RefundPercentage:  req.RefundPercentage,
	})
	if err != nil {
		if apiutil.IsSQLiteUniqueViolation(err) {
			http.Error(w, "A tier with this configuration already exists", http.StatusConflict)
			return
		}
		if apiutil.IsSQLiteForeignKeyViolation(err) {
			http.Error(w, "Facility or reservation type not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to create cancellation policy tier")
		http.Error(w, "Failed to create cancellation policy tier", http.StatusInternalServerError)
		return
	}

	if apiutil.IsJSONRequest(r) {
		if err := apiutil.WriteJSON(w, http.StatusCreated, tier); err != nil {
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to write cancellation policy response")
		}
		return
	}

	filterReservationTypeID, err := reservationTypeFilterIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tiers, err := q.ListCancellationPolicyTiers(ctx, dbgen.ListCancellationPolicyTiersParams{
		FacilityID:        facilityID,
		ReservationTypeID: apiutil.ToNullInt64(filterReservationTypeID),
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to fetch cancellation policy tiers")
		http.Error(w, "Failed to load cancellation policy tiers", http.StatusInternalServerError)
		return
	}
	reservationTypes, err := q.ListReservationTypes(ctx)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load reservation types")
		reservationTypes = nil
	}
	renderCancellationPolicyList(r.Context(), w, facilityID, tiers, reservationTypeNameMap(reservationTypes), filterReservationTypeID)
}

// PUT /api/v1/cancellation-policy/tiers/{id}
func HandleCancellationPolicyTierUpdate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tierID, err := tierIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid tier ID", http.StatusBadRequest)
		return
	}

	req, err := decodeCancellationPolicyTierRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCancellationPolicyTierRequest(req); err != nil {
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

	ctx, cancel := context.WithTimeout(r.Context(), cancellationPolicyQueryTimeout)
	defer cancel()

	tier, err := q.UpdateCancellationPolicyTier(ctx, dbgen.UpdateCancellationPolicyTierParams{
		ID:                tierID,
		FacilityID:        facilityID,
		ReservationTypeID: apiutil.ToNullInt64(req.ReservationTypeID),
		MinHoursBefore:    req.MinHoursBefore,
		RefundPercentage:  req.RefundPercentage,
	})
	if err != nil {
		if apiutil.IsSQLiteUniqueViolation(err) {
			http.Error(w, "A tier with this configuration already exists", http.StatusConflict)
			return
		}
		if apiutil.IsSQLiteForeignKeyViolation(err) {
			http.Error(w, "Facility or reservation type not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Cancellation policy tier not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("tier_id", tierID).Msg("Failed to update cancellation policy tier")
		http.Error(w, "Failed to update cancellation policy tier", http.StatusInternalServerError)
		return
	}

	if apiutil.IsJSONRequest(r) {
		if err := apiutil.WriteJSON(w, http.StatusOK, tier); err != nil {
			logger.Error().Err(err).Int64("tier_id", tierID).Msg("Failed to write cancellation policy response")
		}
		return
	}

	filterReservationTypeID, err := reservationTypeFilterIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tiers, err := q.ListCancellationPolicyTiers(ctx, dbgen.ListCancellationPolicyTiersParams{
		FacilityID:        facilityID,
		ReservationTypeID: apiutil.ToNullInt64(filterReservationTypeID),
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to fetch cancellation policy tiers")
		http.Error(w, "Failed to load cancellation policy tiers", http.StatusInternalServerError)
		return
	}
	reservationTypes, err := q.ListReservationTypes(ctx)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load reservation types")
		reservationTypes = nil
	}
	renderCancellationPolicyList(r.Context(), w, facilityID, tiers, reservationTypeNameMap(reservationTypes), filterReservationTypeID)
}

// DELETE /api/v1/cancellation-policy/tiers/{id}
func HandleCancellationPolicyTierDelete(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tierID, err := tierIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid tier ID", http.StatusBadRequest)
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

	ctx, cancel := context.WithTimeout(r.Context(), cancellationPolicyQueryTimeout)
	defer cancel()

	deleted, err := q.DeleteCancellationPolicyTier(ctx, dbgen.DeleteCancellationPolicyTierParams{
		ID:         tierID,
		FacilityID: facilityID,
	})
	if err != nil {
		logger.Error().Err(err).Int64("tier_id", tierID).Msg("Failed to delete cancellation policy tier")
		http.Error(w, "Failed to delete cancellation policy tier", http.StatusInternalServerError)
		return
	}
	if deleted == 0 {
		http.Error(w, "Cancellation policy tier not found", http.StatusNotFound)
		return
	}

	if apiutil.IsJSONRequest(r) {
		if err := apiutil.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true}); err != nil {
			logger.Error().Err(err).Int64("tier_id", tierID).Msg("Failed to write cancellation policy response")
		}
		return
	}

	filterReservationTypeID, err := reservationTypeFilterIDFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tiers, err := q.ListCancellationPolicyTiers(ctx, dbgen.ListCancellationPolicyTiersParams{
		FacilityID:        facilityID,
		ReservationTypeID: apiutil.ToNullInt64(filterReservationTypeID),
	})
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to fetch cancellation policy tiers")
		http.Error(w, "Failed to load cancellation policy tiers", http.StatusInternalServerError)
		return
	}
	reservationTypes, err := q.ListReservationTypes(ctx)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load reservation types")
		reservationTypes = nil
	}
	renderCancellationPolicyList(r.Context(), w, facilityID, tiers, reservationTypeNameMap(reservationTypes), filterReservationTypeID)
}

func renderCancellationPolicyList(ctx context.Context, w http.ResponseWriter, facilityID int64, tiers []dbgen.CancellationPolicyTier, reservationTypeNames map[int64]string, filterReservationTypeID *int64) {
	component := cancellationPolicyListComponent(facilityID, tiers, reservationTypeNames, filterReservationTypeID)
	if !apiutil.RenderHTMLComponent(ctx, w, component, nil, "Failed to render cancellation policy list", "Failed to render cancellation policy list") {
		return
	}
}

func cancellationPolicyListComponent(facilityID int64, tiers []dbgen.CancellationPolicyTier, reservationTypeNames map[int64]string, filterReservationTypeID *int64) templ.Component {
	return cancellationpolicytempl.CancellationPolicyList(facilityID, cancellationPolicyTierData(tiers, reservationTypeNames), filterReservationTypeID)
}

func cancellationPolicyTierData(tiers []dbgen.CancellationPolicyTier, reservationTypeNames map[int64]string) []cancellationpolicytempl.PolicyTierData {
	data := make([]cancellationpolicytempl.PolicyTierData, 0, len(tiers))
	for _, tier := range tiers {
		var reservationTypeID *int64
		var reservationTypeName *string
		if tier.ReservationTypeID.Valid {
			value := tier.ReservationTypeID.Int64
			reservationTypeID = &value
			if reservationTypeNames != nil {
				if name, ok := reservationTypeNames[value]; ok {
					valueName := name
					reservationTypeName = &valueName
				}
			}
		}
		data = append(data, cancellationpolicytempl.PolicyTierData{
			ID:                  tier.ID,
			MinHoursBefore:      tier.MinHoursBefore,
			RefundPercentage:    tier.RefundPercentage,
			ReservationTypeID:   reservationTypeID,
			ReservationTypeName: reservationTypeName,
		})
	}
	return data
}

func reservationTypeNameMap(reservationTypes []dbgen.ReservationType) map[int64]string {
	if len(reservationTypes) == 0 {
		return nil
	}
	names := make(map[int64]string, len(reservationTypes))
	for _, reservationType := range reservationTypes {
		names[reservationType.ID] = reservationType.Name
	}
	return names
}

func reservationTypeOptions(reservationTypes []dbgen.ReservationType, filterOptions []cancellationpolicytempl.ReservationTypeFilterOption) []cancellationpolicytempl.ReservationTypeOption {
	if len(reservationTypes) == 0 || len(filterOptions) == 0 {
		return nil
	}
	allowed := make(map[int64]struct{}, len(filterOptions))
	for _, option := range filterOptions {
		allowed[option.ID] = struct{}{}
	}
	options := make([]cancellationpolicytempl.ReservationTypeOption, 0, len(filterOptions))
	for _, reservationType := range reservationTypes {
		if _, ok := allowed[reservationType.ID]; !ok {
			continue
		}
		options = append(options, cancellationpolicytempl.ReservationTypeOption{
			ID:   reservationType.ID,
			Name: reservationType.Name,
		})
	}
	return options
}

func reservationTypeFilterOptions(reservationTypes []dbgen.ReservationType) []cancellationpolicytempl.ReservationTypeFilterOption {
	if len(reservationTypes) == 0 {
		return nil
	}
	options := make([]cancellationpolicytempl.ReservationTypeFilterOption, 0, 2)
	for _, reservationType := range reservationTypes {
		switch reservationType.Name {
		case "GAME":
			options = append(options, cancellationpolicytempl.ReservationTypeFilterOption{
				ID:    reservationType.ID,
				Label: "Court Reservations/GAME",
			})
		case "PRO_SESSION":
			options = append(options, cancellationpolicytempl.ReservationTypeFilterOption{
				ID:    reservationType.ID,
				Label: "Lessons/PRO_SESSION",
			})
		}
	}
	return options
}

func validateCancellationPolicyTierRequest(req cancellationPolicyTierRequest) error {
	if req.MinHoursBefore < 0 {
		return fmt.Errorf("min_hours_before must be 0 or greater")
	}
	if req.RefundPercentage < 0 || req.RefundPercentage > 100 {
		return fmt.Errorf("refund_percentage must be between 0 and 100")
	}
	if req.ReservationTypeID != nil && *req.ReservationTypeID <= 0 {
		return fmt.Errorf("reservation_type_id must be a positive integer")
	}
	return nil
}

func decodeCancellationPolicyTierRequest(r *http.Request) (cancellationPolicyTierRequest, error) {
	if apiutil.IsJSONRequest(r) {
		var req cancellationPolicyTierRequest
		if err := apiutil.DecodeJSON(r, &req); err != nil {
			return req, err
		}
		return req, nil
	}

	if err := r.ParseForm(); err != nil {
		return cancellationPolicyTierRequest{}, err
	}

	facilityID, err := apiutil.ParseOptionalInt64Field(apiutil.FirstNonEmpty(r.FormValue("facility_id"), r.FormValue("facilityId")), "facility_id")
	if err != nil {
		return cancellationPolicyTierRequest{}, err
	}

	reservationTypeID, err := apiutil.ParseOptionalInt64Field(apiutil.FirstNonEmpty(r.FormValue("reservation_type_id"), r.FormValue("reservationTypeId")), "reservation_type_id")
	if err != nil {
		return cancellationPolicyTierRequest{}, err
	}

	minHoursBefore, err := parseNonNegativeInt64Field(apiutil.FirstNonEmpty(r.FormValue("min_hours_before"), r.FormValue("minHoursBefore")), "min_hours_before")
	if err != nil {
		return cancellationPolicyTierRequest{}, err
	}

	refundPercentage, err := parseInt64InRange(apiutil.FirstNonEmpty(r.FormValue("refund_percentage"), r.FormValue("refundPercentage")), "refund_percentage", 0, 100)
	if err != nil {
		return cancellationPolicyTierRequest{}, err
	}

	return cancellationPolicyTierRequest{
		FacilityID:        facilityID,
		ReservationTypeID: reservationTypeID,
		MinHoursBefore:    minHoursBefore,
		RefundPercentage:  refundPercentage,
	}, nil
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

func parseInt64InRange(raw string, field string, min int64, max int64) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("%s must be between %d and %d", field, min, max)
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

func tierIDFromRequest(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.PathValue(tierIDParam))
	if raw == "" {
		return 0, fmt.Errorf("invalid tier ID")
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid tier ID")
	}
	return value, nil
}

func loadQueries() *dbgen.Queries {
	return queries
}

func reservationTypeFilterIDFromRequest(r *http.Request) (*int64, error) {
	// Query param takes precedence for GET filtering; form value supports HTMX posts.
	raw := apiutil.FirstNonEmpty(
		r.URL.Query().Get("reservation_type_id"),
		r.FormValue("filter_reservation_type_id"),
	)
	return apiutil.ParseOptionalInt64Field(raw, "reservation_type_id")
}
