// internal/api/openplay/handlers.go
package openplay

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
	"github.com/rs/zerolog/log"

	appdb "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	openplaytempl "github.com/codr1/Pickleicious/internal/templates/components/openplay"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

var (
	queries     *dbgen.Queries
	store       *appdb.DB
	queriesOnce sync.Once
)

const (
	openPlayQueryTimeout              = 5 * time.Second
	openPlayAuditAutoScaleOverride    = "auto_scale_override"
	openPlayAuditAutoScaleRuleDisable = "auto_scale_rule_disabled"
)

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

	activeTheme, err := models.GetActiveTheme(ctx, q, facilityID)
	if err != nil {
		logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
		activeTheme = nil
	}

	page := layouts.Base(openPlayRulesPageComponent(rules), activeTheme)
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

func HandleOpenPlaySessionAutoScaleToggle(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	q := loadQueries()
	database := loadDB()
	if q == nil || database == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	sessionID, err := openPlaySessionIDFromRequest(r)
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	facilityID, err := facilityIDFromRequest(r)
	if err != nil {
		http.Error(w, "Facility ID is required", http.StatusBadRequest)
		return
	}

	var payload openPlaySessionAutoScaleToggleRequest
	if err := decodeJSON(r, &payload); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), openPlayQueryTimeout)
	defer cancel()

	var updatedSession dbgen.OpenPlaySession
	err = database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		session, err := qtx.GetOpenPlaySession(ctx, dbgen.GetOpenPlaySessionParams{
			ID:         sessionID,
			FacilityID: facilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return handlerError{status: http.StatusNotFound, message: "Open play session not found", err: err}
			}
			return handlerError{status: http.StatusInternalServerError, message: "Failed to fetch open play session", err: err}
		}

		rule, err := qtx.GetOpenPlayRule(ctx, dbgen.GetOpenPlayRuleParams{
			ID:         session.OpenPlayRuleID,
			FacilityID: facilityID,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return handlerError{status: http.StatusNotFound, message: "Open play rule not found", err: err}
			}
			return handlerError{status: http.StatusInternalServerError, message: "Failed to fetch open play rule", err: err}
		}

		nextOverride := !rule.AutoScaleEnabled
		if session.AutoScaleOverride.Valid {
			nextOverride = !session.AutoScaleOverride.Bool
		}

		updatedSession, err = qtx.UpdateSessionAutoScaleOverride(ctx, dbgen.UpdateSessionAutoScaleOverrideParams{
			ID:                sessionID,
			FacilityID:        facilityID,
			AutoScaleOverride: sql.NullBool{Bool: nextOverride, Valid: true},
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return handlerError{status: http.StatusNotFound, message: "Open play session not found", err: err}
			}
			return handlerError{status: http.StatusInternalServerError, message: "Failed to update open play session", err: err}
		}

		if err := createOpenPlayAuditEntry(ctx, qtx, session.ID, openPlayAuditAutoScaleOverride, map[string]any{
			"auto_scale_override": auditBoolValue(session.AutoScaleOverride),
		}, map[string]any{
			"auto_scale_override": auditBoolValue(updatedSession.AutoScaleOverride),
		}, sql.NullString{}); err != nil {
			return handlerError{status: http.StatusInternalServerError, message: "Failed to log open play session auto scale override", err: err}
		}

		if payload.DisableForRule {
			_, err := qtx.UpdateOpenPlayRule(ctx, dbgen.UpdateOpenPlayRuleParams{
				ID:                        rule.ID,
				FacilityID:                rule.FacilityID,
				Name:                      rule.Name,
				MinParticipants:           rule.MinParticipants,
				MaxParticipantsPerCourt:   rule.MaxParticipantsPerCourt,
				CancellationCutoffMinutes: rule.CancellationCutoffMinutes,
				AutoScaleEnabled:          false,
				MinCourts:                 rule.MinCourts,
				MaxCourts:                 rule.MaxCourts,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return handlerError{status: http.StatusNotFound, message: "Open play rule not found", err: err}
				}
				return handlerError{status: http.StatusInternalServerError, message: "Failed to update open play rule", err: err}
			}

			if err := createOpenPlayAuditEntry(ctx, qtx, session.ID, openPlayAuditAutoScaleRuleDisable, map[string]any{
				"auto_scale_enabled": rule.AutoScaleEnabled,
			}, map[string]any{
				"auto_scale_enabled": false,
			}, sql.NullString{String: "disable_for_rule", Valid: true}); err != nil {
				return handlerError{status: http.StatusInternalServerError, message: "Failed to log open play rule auto scale change", err: err}
			}
		}

		return nil
	})
	if err != nil {
		var herr handlerError
		if errors.As(err, &herr) {
			if herr.status == http.StatusInternalServerError {
				logger.Error().Err(herr.err).Int64("session_id", sessionID).Msg(herr.message)
			}
			http.Error(w, herr.message, herr.status)
			return
		}
		logger.Error().Err(err).Int64("session_id", sessionID).Msg("Failed to update open play session auto scale override")
		http.Error(w, "Failed to update open play session", http.StatusInternalServerError)
		return
	}

	if err := writeJSON(w, http.StatusOK, updatedSession); err != nil {
		logger.Error().Err(err).Int64("session_id", sessionID).Msg("Failed to write open play session response")
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
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

func loadDB() *appdb.DB {
	return store
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

func openPlaySessionIDFromRequest(r *http.Request) (int64, error) {
	pathID := strings.TrimSpace(r.PathValue("id"))
	if pathID == "" {
		return 0, fmt.Errorf("invalid session ID")
	}
	id, err := strconv.ParseInt(pathID, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid session ID")
	}
	return id, nil
}

type openPlaySessionAutoScaleToggleRequest struct {
	DisableForRule bool `json:"disable_for_rule"`
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

func writeJSON(w http.ResponseWriter, status int, payload any) error {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	if err := encoder.Encode(payload); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err := w.Write(buf.Bytes())
	return err
}

type handlerError struct {
	status  int
	message string
	err     error
}

func (e handlerError) Error() string {
	return e.message
}

func (e handlerError) Unwrap() error {
	return e.err
}

func auditBoolValue(value sql.NullBool) any {
	if value.Valid {
		return value.Bool
	}
	return nil
}

func createOpenPlayAuditEntry(ctx context.Context, q *dbgen.Queries, sessionID int64, action string, beforeState, afterState map[string]any, reason sql.NullString) error {
	before, err := marshalAuditState(beforeState)
	if err != nil {
		return err
	}
	after, err := marshalAuditState(afterState)
	if err != nil {
		return err
	}
	_, err = q.CreateOpenPlayAuditLog(ctx, dbgen.CreateOpenPlayAuditLogParams{
		SessionID:   sessionID,
		Action:      action,
		BeforeState: before,
		AfterState:  after,
		Reason:      reason,
	})
	if err != nil {
		return fmt.Errorf("create audit log: %w", err)
	}
	return nil
}

func marshalAuditState(state map[string]any) (sql.NullString, error) {
	if len(state) == 0 {
		return sql.NullString{}, nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("marshal audit state: %w", err)
	}
	return sql.NullString{String: string(data), Valid: true}, nil
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
	case minParticipants > maxParticipantsPerCourt*minCourts:
		return fieldError{Field: "min_participants", Reason: "must be less than or equal to max_participants_per_court * min_courts"}
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
