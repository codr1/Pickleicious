package member

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

func cancellationPolicySummary(ctx context.Context, q *dbgen.Queries, facilityID int64, reservationTypeID *int64, startTime time.Time, now time.Time) (string, error) {
	hoursUntil := hoursUntilReservationStart(startTime, now)
	tier, err := q.GetApplicableCancellationTier(ctx, dbgen.GetApplicableCancellationTierParams{
		FacilityID:            facilityID,
		HoursUntilReservation: hoursUntil,
		ReservationTypeID:     apiutil.ToNullInt64(reservationTypeID),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "Cancellations are fully refundable until start time.", nil
		}
		return "", err
	}

	if tier.MinHoursBefore <= 0 {
		return fmt.Sprintf("Cancel any time before start for %d%% refund.", tier.RefundPercentage), nil
	}
	return fmt.Sprintf("Cancel at least %d hours before start for %d%% refund.", tier.MinHoursBefore, tier.RefundPercentage), nil
}
