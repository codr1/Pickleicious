package apiutil

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const facilityIDQueryKey = "facility_id"

func ParseNonNegativeInt64Field(raw string, field string) (int64, error) {
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

func ParsePositiveInt64Field(raw string, field string) (int64, error) {
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

func FacilityIDFromQuery(r *http.Request) (int64, error) {
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

func FacilityIDFromRequest(r *http.Request, fromBody *int64) (int64, error) {
	if fromBody != nil {
		if *fromBody <= 0 {
			return 0, fmt.Errorf("facility_id must be a positive integer")
		}
		return *fromBody, nil
	}
	return FacilityIDFromQuery(r)
}

func FormatPriceCents(cents int64) string {
	return fmt.Sprintf("$%.2f", float64(cents)/100)
}

func ParsePurchaseDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("purchase_date is required")
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if layout == time.RFC3339 {
			parsed, err := time.Parse(layout, raw)
			if err == nil {
				return parsed.UTC(), nil
			}
			continue
		}
		parsed, err := time.ParseInLocation(layout, raw, time.Local)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("purchase_date must be a valid date")
}
