package apiutil

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

func EnsureCourtsAvailable(ctx context.Context, q *dbgen.Queries, facilityID, reservationID int64, startTime, endTime time.Time, courtIDs []int64) error {
	available, err := q.ListAvailableCourts(ctx, dbgen.ListAvailableCourtsParams{
		FacilityID:    facilityID,
		ReservationID: reservationID,
		StartTime:     startTime,
		EndTime:       endTime,
	})
	if err != nil {
		return fmt.Errorf("availability check failed: %w", err)
	}

	availableMap := make(map[int64]struct{}, len(available))
	for _, court := range available {
		availableMap[court.ID] = struct{}{}
	}

	var unavailable []string
	for _, courtID := range courtIDs {
		if _, ok := availableMap[courtID]; ok {
			continue
		}
		unavailable = append(unavailable, strconv.FormatInt(courtID, 10))
	}
	if len(unavailable) > 0 {
		return AvailabilityError{Courts: unavailable}
	}
	return nil
}

type AvailabilityError struct {
	Courts []string
}

func (e AvailabilityError) Error() string {
	return fmt.Sprintf("courts unavailable: %s", strings.Join(e.Courts, ", "))
}
