// internal/api/openplay/handlers.go
package openplay

import (
	"bytes"
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

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	openplaytempl "github.com/codr1/Pickleicious/internal/templates/components/openplay"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

var (
	queries     *dbgen.Queries
	queriesOnce sync.Once
)

const openPlayQueryTimeout = 5 * time.Second

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(q *dbgen.Queries) {
	if q == nil {
		return
	}
	queriesOnce.Do(func() {
		queries = q
	})
}

// /open-play-rules
func HandleOpenPlayRulesPage(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), openPlayQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	rules, err := q.ListOpenPlayRules(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to fetch open play rules")
		http.Error(w, "Failed to load open play rules", http.StatusInternalServerError)
		return
	}

	page := layouts.Base(openPlayRulesPageComponent(rules))
	if !renderHTMLComponent(r.Context(), w, page, nil, "Failed to render open play rules page", "Failed to render page") {
		return
	}
}

func HandleOpenPlayRulesList(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), openPlayQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	rules, err := q.ListOpenPlayRules(ctx, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to list open play rules")
		http.Error(w, "Failed to fetch open play rules", http.StatusInternalServerError)
		return
	}

	component := openPlayRulesListComponent(rules)
	if !renderHTMLComponent(r.Context(), w, component, nil, "Failed to render open play rules list", "Failed to render list") {
		return
	}
}

func HandleOpenPlayRuleNew(w http.ResponseWriter, r *http.Request) {
	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	rule := dbgen.OpenPlayRule{FacilityID: facilityID}
	component := openplaytempl.OpenPlayRuleForm(openplaytempl.NewOpenPlayRule(rule), facilityID)
	if !renderHTMLComponent(r.Context(), w, component, nil, "Failed to render open play rule form", "Failed to render form") {
		return
	}
}

func HandleOpenPlayRuleEdit(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ruleID, err := openPlayRuleIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid rule ID", http.StatusBadRequest)
		return
	}

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), openPlayQueryTimeout)
	defer cancel()

	rule, err := q.GetOpenPlayRule(ctx, dbgen.GetOpenPlayRuleParams{
		ID:         ruleID,
		FacilityID: facilityID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Open play rule not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("rule_id", ruleID).Msg("Failed to fetch open play rule for edit")
		http.Error(w, "Failed to fetch open play rule", http.StatusInternalServerError)
		return
	}

	component := openplaytempl.OpenPlayRuleForm(openplaytempl.NewOpenPlayRule(rule), facilityID)
	if !renderHTMLComponent(r.Context(), w, component, nil, "Failed to render open play rule form", "Failed to render form") {
		return
	}
}

func HandleOpenPlayRuleCreate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		logger.Error().Err(err).Msg("Failed to parse form")
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	minParticipants, err := parseIntField(r, "min_participants")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	maxParticipantsPerCourt, err := parseIntField(r, "max_participants_per_court")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cancellationCutoffMinutes, err := parseIntField(r, "cancellation_cutoff_minutes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	minCourts, err := parseIntField(r, "min_courts")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	maxCourts, err := parseIntField(r, "max_courts")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	autoScaleEnabled := parseBoolField(r, "auto_scale_enabled")

	if err := validateOpenPlayRuleInput(minParticipants, maxParticipantsPerCourt, cancellationCutoffMinutes, minCourts, maxCourts); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), openPlayQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	rule, err := q.CreateOpenPlayRule(ctx, dbgen.CreateOpenPlayRuleParams{
		FacilityID:                facilityID,
		Name:                      name,
		MinParticipants:           minParticipants,
		MaxParticipantsPerCourt:   maxParticipantsPerCourt,
		CancellationCutoffMinutes: cancellationCutoffMinutes,
		AutoScaleEnabled:          autoScaleEnabled,
		MinCourts:                 minCourts,
		MaxCourts:                 maxCourts,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create open play rule")
		http.Error(w, "Failed to create open play rule", http.StatusInternalServerError)
		return
	}

	component := openPlayRuleDetailComponent(rule)
	headers := map[string]string{
		"HX-Trigger": "refreshOpenPlayRulesList",
	}
	if !renderHTMLComponent(r.Context(), w, component, headers, "Failed to render open play rule detail", "Failed to render response") {
		return
	}
}

func HandleOpenPlayRuleDetail(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ruleID, err := openPlayRuleIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid rule ID", http.StatusBadRequest)
		return
	}

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), openPlayQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	rule, err := q.GetOpenPlayRule(ctx, dbgen.GetOpenPlayRuleParams{
		ID:         ruleID,
		FacilityID: facilityID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Open play rule not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("rule_id", ruleID).Msg("Failed to fetch open play rule")
		http.Error(w, "Failed to fetch open play rule", http.StatusInternalServerError)
		return
	}

	component := openPlayRuleDetailComponent(rule)
	if !renderHTMLComponent(r.Context(), w, component, nil, "Failed to render open play rule detail", "Failed to render detail") {
		return
	}
}

func HandleOpenPlayRuleUpdate(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ruleID, err := openPlayRuleIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid rule ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		logger.Error().Err(err).Msg("Failed to parse form")
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	minParticipants, err := parseIntField(r, "min_participants")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	maxParticipantsPerCourt, err := parseIntField(r, "max_participants_per_court")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cancellationCutoffMinutes, err := parseIntField(r, "cancellation_cutoff_minutes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	minCourts, err := parseIntField(r, "min_courts")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	maxCourts, err := parseIntField(r, "max_courts")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	autoScaleEnabled := parseBoolField(r, "auto_scale_enabled")

	if err := validateOpenPlayRuleInput(minParticipants, maxParticipantsPerCourt, cancellationCutoffMinutes, minCourts, maxCourts); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), openPlayQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	rule, err := q.UpdateOpenPlayRule(ctx, dbgen.UpdateOpenPlayRuleParams{
		ID:                        ruleID,
		FacilityID:                facilityID,
		Name:                      name,
		MinParticipants:           minParticipants,
		MaxParticipantsPerCourt:   maxParticipantsPerCourt,
		CancellationCutoffMinutes: cancellationCutoffMinutes,
		AutoScaleEnabled:          autoScaleEnabled,
		MinCourts:                 minCourts,
		MaxCourts:                 maxCourts,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Warn().Int64("rule_id", ruleID).Msg("Open play rule not found for update; rule may have been deleted during update")
			http.Error(w, "Open play rule not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("rule_id", ruleID).Msg("Failed to update open play rule")
		http.Error(w, "Failed to update open play rule", http.StatusInternalServerError)
		return
	}

	component := openPlayRuleDetailComponent(rule)
	headers := map[string]string{
		"HX-Trigger": "refreshOpenPlayRulesList",
	}
	if !renderHTMLComponent(r.Context(), w, component, headers, "Failed to render open play rule detail", "Failed to render response") {
		return
	}
}

func HandleOpenPlayRuleDelete(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	if q == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ruleID, err := openPlayRuleIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid rule ID", http.StatusBadRequest)
		return
	}

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), openPlayQueryTimeout)
	defer cancel()

	// TODO: enforce facility-level authorization once auth middleware is wired.
	deleted, err := q.DeleteOpenPlayRule(ctx, dbgen.DeleteOpenPlayRuleParams{
		ID:         ruleID,
		FacilityID: facilityID,
	})
	if err != nil {
		logger.Error().Err(err).Int64("rule_id", ruleID).Msg("Failed to delete open play rule")
		http.Error(w, "Failed to delete open play rule", http.StatusInternalServerError)
		return
	}
	if deleted == 0 {
		http.Error(w, "Open play rule not found", http.StatusNotFound)
		return
	}

	headers := map[string]string{
		"HX-Redirect": fmt.Sprintf("/open-play-rules?facility_id=%d", facilityID),
	}
	component := openPlayRuleDeleteComponent()
	if !renderHTMLComponent(r.Context(), w, component, headers, "Failed to render delete response", "Failed to render response") {
		return
	}
}

func facilityIDFromRequest(r *http.Request) (int64, error) {
	queryID := strings.TrimSpace(r.URL.Query().Get("facility_id"))
	formID := ""

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if r.PostForm == nil && r.Form == nil {
			if err := r.ParseForm(); err != nil {
				return 0, fmt.Errorf("failed to parse form")
			}
		}
		if r.PostForm != nil {
			formID = strings.TrimSpace(r.PostForm.Get("facility_id"))
		} else if r.Form != nil {
			formID = strings.TrimSpace(r.Form.Get("facility_id"))
		}
	}

	if queryID == "" && formID == "" {
		return 0, fmt.Errorf("facility_id is required")
	}

	var (
		queryValue int64
		formValue  int64
		queryErr   error
		formErr    error
	)

	if queryID != "" {
		queryValue, queryErr = strconv.ParseInt(queryID, 10, 64)
		if queryErr != nil || queryValue <= 0 {
			return 0, fmt.Errorf("facility_id must be a positive integer")
		}
	}

	if formID != "" {
		formValue, formErr = strconv.ParseInt(formID, 10, 64)
		if formErr != nil || formValue <= 0 {
			return 0, fmt.Errorf("facility_id must be a positive integer")
		}
	}

	if queryID != "" && formID != "" && queryValue != formValue {
		return 0, fmt.Errorf("facility_id mismatch between query and form")
	}

	if queryID != "" {
		return queryValue, nil
	}
	return formValue, nil
}

func loadQueries() *dbgen.Queries {
	return queries
}

func openPlayRuleIDFromRequest(r *http.Request) (int64, error) {
	pathID := strings.TrimSpace(r.PathValue("id"))
	if pathID == "" {
		return 0, fmt.Errorf("invalid rule ID")
	}
	id, err := strconv.ParseInt(pathID, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid rule ID")
	}
	return id, nil
}

func parseIntField(r *http.Request, name string) (int64, error) {
	value := strings.TrimSpace(r.FormValue(name))
	if value == "" {
		return 0, fieldError{Field: name, Reason: "is required"}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fieldError{Field: name, Reason: "must be a number"}
	}
	return parsed, nil
}

func parseBoolField(r *http.Request, name string) bool {
	value := strings.ToLower(strings.TrimSpace(r.FormValue(name)))
	switch value {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

type fieldError struct {
	Field  string
	Reason string
}

func (e fieldError) Error() string {
	return fmt.Sprintf("%s %s", e.Field, e.Reason)
}

func validateOpenPlayRuleInput(minParticipants, maxParticipantsPerCourt, cancellationCutoffMinutes, minCourts, maxCourts int64) error {
	switch {
	case minParticipants <= 0:
		return fieldError{Field: "min_participants", Reason: "must be greater than 0"}
	case maxParticipantsPerCourt <= 0:
		return fieldError{Field: "max_participants_per_court", Reason: "must be greater than 0"}
	case cancellationCutoffMinutes < 0:
		return fieldError{Field: "cancellation_cutoff_minutes", Reason: "must be 0 or greater"}
	case minCourts <= 0:
		return fieldError{Field: "min_courts", Reason: "must be greater than 0"}
	case maxCourts <= 0:
		return fieldError{Field: "max_courts", Reason: "must be greater than 0"}
	case minCourts > maxCourts:
		return fieldError{Field: "min_courts", Reason: "must be less than or equal to max_courts"}
	case minParticipants > maxParticipantsPerCourt*maxCourts:
		return fieldError{Field: "min_participants", Reason: "must be less than or equal to max_participants_per_court * max_courts"}
	default:
		return nil
	}
}

func openPlayRulesPageComponent(rules []dbgen.OpenPlayRule) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<div class="space-y-6">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div class="flex items-center justify-between"><h1 class="text-2xl font-semibold text-gray-900">Open Play Rules</h1></div>`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `<div id="open-play-rules-list">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, buildOpenPlayRulesListHTML(rules)); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `</div></div>`); err != nil {
			return err
		}
		return nil
	})
}

func openPlayRulesListComponent(rules []dbgen.OpenPlayRule) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, buildOpenPlayRulesListHTML(rules))
		return err
	})
}

func openPlayRuleDetailComponent(rule dbgen.OpenPlayRule) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, buildOpenPlayRuleCardHTML(rule))
		return err
	})
}

func openPlayRuleDeleteComponent() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<div class="h-full flex items-center justify-center text-gray-500"><p>Open play rule successfully deleted</p></div>`)
		return err
	})
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

func buildOpenPlayRulesListHTML(rules []dbgen.OpenPlayRule) string {
	if len(rules) == 0 {
		return `<div class="rounded border border-dashed p-6 text-center text-sm text-gray-500">No open play rules found.</div>`
	}

	var builder strings.Builder
	builder.WriteString(`<div class="grid gap-4">`)
	for _, rule := range rules {
		builder.WriteString(buildOpenPlayRuleCardHTML(rule))
	}
	builder.WriteString(`</div>`)
	return builder.String()
}

func buildOpenPlayRuleCardHTML(rule dbgen.OpenPlayRule) string {
	enabledLabel := "No"
	if rule.AutoScaleEnabled {
		enabledLabel = "Yes"
	}

	name := html.EscapeString(rule.Name)

	return fmt.Sprintf(
		`<div class="rounded border bg-white p-4 shadow-sm" data-open-play-rule-id="%d">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<div class="text-lg font-semibold text-gray-900">%s</div>
				<div class="text-xs text-gray-500">ID %d</div>
			</div>
			<dl class="mt-3 grid grid-cols-1 gap-2 text-sm text-gray-700 sm:grid-cols-2">
				<div class="flex items-center justify-between gap-4">
					<dt class="font-medium text-gray-600">Min participants</dt>
					<dd>%d</dd>
				</div>
				<div class="flex items-center justify-between gap-4">
					<dt class="font-medium text-gray-600">Max per court</dt>
					<dd>%d</dd>
				</div>
				<div class="flex items-center justify-between gap-4">
					<dt class="font-medium text-gray-600">Cancellation cutoff</dt>
					<dd>%d min</dd>
				</div>
				<div class="flex items-center justify-between gap-4">
					<dt class="font-medium text-gray-600">Auto scale</dt>
					<dd>%s</dd>
				</div>
				<div class="flex items-center justify-between gap-4">
					<dt class="font-medium text-gray-600">Min courts</dt>
					<dd>%d</dd>
				</div>
				<div class="flex items-center justify-between gap-4">
					<dt class="font-medium text-gray-600">Max courts</dt>
					<dd>%d</dd>
				</div>
			</dl>
		</div>`,
		rule.ID,
		name,
		rule.ID,
		rule.MinParticipants,
		rule.MaxParticipantsPerCourt,
		rule.CancellationCutoffMinutes,
		enabledLabel,
		rule.MinCourts,
		rule.MaxCourts,
	)
}
