package request

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

// ParseFacilityID parses a positive int64 facility ID from a query value.
func ParseFacilityID(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	facilityID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || facilityID <= 0 {
		return 0, false
	}

	return facilityID, true
}

// FacilityIDFromBookingRequest parses facility_id from the query or HX-Current-URL header.
func FacilityIDFromBookingRequest(r *http.Request) (int64, bool) {
	if facilityID, ok := ParseFacilityID(r.URL.Query().Get("facility_id")); ok {
		return facilityID, true
	}

	currentURL := strings.TrimSpace(r.Header.Get("HX-Current-URL"))
	if currentURL == "" {
		return 0, false
	}

	parsed, err := url.Parse(currentURL)
	if err != nil {
		log.Ctx(r.Context()).
			Debug().
			Err(err).
			Str("hx_current_url", currentURL).
			Msg("Failed to parse HX-Current-URL")
		return 0, false
	}

	return ParseFacilityID(parsed.Query().Get("facility_id"))
}
