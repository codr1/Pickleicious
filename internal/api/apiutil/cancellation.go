package apiutil

import (
	"context"
	"database/sql"
	"errors"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

func ApplicableRefundPercentage(ctx context.Context, q *dbgen.Queries, facilityID int64, hoursUntilReservation int64, reservationTypeID *int64) (int64, error) {
	reservationTypeFilter := ToNullInt64(reservationTypeID)
	tier, err := q.GetApplicableCancellationTier(ctx, dbgen.GetApplicableCancellationTierParams{
		FacilityID:            facilityID,
		HoursUntilReservation: hoursUntilReservation,
		ReservationTypeID:     reservationTypeFilter,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 100, nil
		}
		return 0, err
	}
	return tier.RefundPercentage, nil
}
