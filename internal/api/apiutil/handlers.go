package apiutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/authz"
)

type FieldError struct {
	Field  string
	Reason string
}

func (e FieldError) Error() string {
	return fmt.Sprintf("%s %s", e.Field, e.Reason)
}

type HandlerError struct {
	Status  int
	Message string
	Err     error
}

func (e HandlerError) Error() string {
	return e.Message
}

func (e HandlerError) Unwrap() error {
	return e.Err
}

func DecodeJSON(r *http.Request, dst any) error {
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

func WriteJSON(w http.ResponseWriter, status int, payload any) error {
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

func RequireFacilityAccess(w http.ResponseWriter, r *http.Request, facilityID int64) bool {
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

func IsJSONRequest(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json")
}

func ParseOptionalInt64Field(raw string, field string) (*int64, error) {
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

func ReservationIDFromRequest(r *http.Request) (int64, error) {
	pathID := strings.TrimSpace(r.PathValue("id"))
	if pathID == "" {
		return 0, fmt.Errorf("invalid reservation ID")
	}
	id, err := strconv.ParseInt(pathID, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid reservation ID")
	}
	return id, nil
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func ParseRequiredInt64Field(raw string, field string) (int64, error) {
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

func ParseBool(raw string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func IsHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func IsSQLiteForeignKeyViolation(err error) bool {
	var sqliteErr sqlite3.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	return sqliteErr.ExtendedCode == sqlite3.ErrConstraintForeignKey
}

func IsSQLiteUniqueViolation(err error) bool {
	var sqliteErr sqlite3.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	return sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique ||
		sqliteErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey
}

func RenderHTMLComponent(ctx context.Context, w http.ResponseWriter, component templ.Component, headers map[string]string, logMsg string, errMsg string) bool {
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

func WriteHTMLFeedback(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(
		w,
		`<div class="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">%s</div>`,
		html.EscapeString(message),
	)
}
