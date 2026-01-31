package apiutil

import (
	"context"
	"database/sql"
	"errors"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

func NormalizedMaxAdvanceDays(value, defaultValue int64) int64 {
	if value <= 0 {
		return defaultValue
	}
	return value
}

func GetMemberMaxAdvanceDays(
	ctx context.Context,
	q *dbgen.Queries,
	facilityID int64,
	membershipLevel int64,
	defaultValue int64,
) (int64, *dbgen.Facility, error) {
	facility, err := q.GetFacilityByID(ctx, facilityID)
	if err != nil {
		return defaultValue, nil, err
	}

	maxAdvanceDays := NormalizedMaxAdvanceDays(facility.MaxAdvanceBookingDays, defaultValue)
	if !facility.TierBookingEnabled {
		return maxAdvanceDays, &facility, nil
	}

	window, err := q.GetTierBookingWindow(ctx, dbgen.GetTierBookingWindowParams{
		FacilityID:      facilityID,
		MembershipLevel: membershipLevel,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return maxAdvanceDays, &facility, nil
		}
		return maxAdvanceDays, &facility, err
	}

	return NormalizedMaxAdvanceDays(window.MaxAdvanceDays, defaultValue), &facility, nil
}
