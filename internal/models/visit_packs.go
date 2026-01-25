// internal/models/visit_packs.go
package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

var ErrVisitPackUnavailable = errors.New("visit pack unavailable")

type RedeemVisitPackVisitParams struct {
	VisitPackID   int64
	FacilityID    int64
	RedeemedAt    time.Time
	ReservationID *int64
}

type VisitPackRedemptionResult struct {
	VisitPack  dbgen.VisitPack
	Redemption dbgen.VisitPackRedemption
}

// RedeemVisitPackVisit redeems a visit pack visit using the provided querier.
// Callers should pass a transactional querier when the redemption must be atomic with other writes.
func RedeemVisitPackVisit(ctx context.Context, q dbgen.Querier, params RedeemVisitPackVisitParams) (VisitPackRedemptionResult, error) {
	if q == nil {
		return VisitPackRedemptionResult{}, fmt.Errorf("queries are required")
	}
	if params.VisitPackID <= 0 {
		return VisitPackRedemptionResult{}, fmt.Errorf("visit_pack_id must be a positive integer")
	}
	if params.FacilityID <= 0 {
		return VisitPackRedemptionResult{}, fmt.Errorf("facility_id must be a positive integer")
	}
	if params.ReservationID != nil && *params.ReservationID <= 0 {
		return VisitPackRedemptionResult{}, fmt.Errorf("reservation_id must be a positive integer")
	}

	redeemedAt := params.RedeemedAt
	if redeemedAt.IsZero() {
		redeemedAt = time.Now()
	}

	redemptionInfo, err := q.GetVisitPackRedemptionInfo(ctx, params.VisitPackID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return VisitPackRedemptionResult{}, ErrVisitPackUnavailable
		}
		return VisitPackRedemptionResult{}, err
	}

	if redemptionInfo.PackFacilityID != params.FacilityID {
		redemptionFacility, err := q.GetFacilityByID(ctx, params.FacilityID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return VisitPackRedemptionResult{}, ErrVisitPackUnavailable
			}
			return VisitPackRedemptionResult{}, err
		}
		if redemptionFacility.OrganizationID != redemptionInfo.OrganizationID {
			return VisitPackRedemptionResult{}, ErrVisitPackUnavailable
		}

		crossFacility, err := q.GetOrganizationCrossFacilitySetting(ctx, redemptionInfo.OrganizationID)
		if err != nil {
			return VisitPackRedemptionResult{}, err
		}
		if !crossFacility {
			return VisitPackRedemptionResult{}, ErrVisitPackUnavailable
		}
	}

	updated, err := q.DecrementVisitPackVisit(ctx, params.VisitPackID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return VisitPackRedemptionResult{}, ErrVisitPackUnavailable
		}
		return VisitPackRedemptionResult{}, err
	}

	reservationID := sql.NullInt64{}
	if params.ReservationID != nil {
		reservationID = sql.NullInt64{Int64: *params.ReservationID, Valid: true}
	}

	redemption, err := q.CreateVisitPackRedemption(ctx, dbgen.CreateVisitPackRedemptionParams{
		VisitPackID:   params.VisitPackID,
		FacilityID:    params.FacilityID,
		RedeemedAt:    redeemedAt,
		ReservationID: reservationID,
	})
	if err != nil {
		return VisitPackRedemptionResult{}, err
	}

	return VisitPackRedemptionResult{
		VisitPack:  updated,
		Redemption: redemption,
	}, nil
}
