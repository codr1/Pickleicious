package request

import (
	"strconv"
	"strings"
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
