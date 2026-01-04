package apiutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

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
