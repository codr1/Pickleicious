// internal/models/visit_packs.go
package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	appdb "github.com/codr1/Pickleicious/internal/db"
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

func RedeemVisitPackVisit(ctx context.Context, database *appdb.DB, params RedeemVisitPackVisitParams) (VisitPackRedemptionResult, error) {
	if database == nil {
		return VisitPackRedemptionResult{}, fmt.Errorf("database is required")
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

	var result VisitPackRedemptionResult
	err := database.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		redemptionInfo, err := qtx.GetVisitPackRedemptionInfo(ctx, params.VisitPackID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrVisitPackUnavailable
			}
			return err
		}

		if redemptionInfo.PackFacilityID != params.FacilityID {
			redemptionFacility, err := qtx.GetFacilityByID(ctx, params.FacilityID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return ErrVisitPackUnavailable
				}
				return err
			}
			if redemptionFacility.OrganizationID != redemptionInfo.OrganizationID {
				return ErrVisitPackUnavailable
			}

			crossFacility, err := qtx.GetOrganizationCrossFacilitySetting(ctx, redemptionInfo.OrganizationID)
			if err != nil {
				return err
			}
			if !crossFacility {
				return ErrVisitPackUnavailable
			}
		}

		updated, err := qtx.DecrementVisitPackVisit(ctx, params.VisitPackID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrVisitPackUnavailable
			}
			return err
		}

		reservationID := sql.NullInt64{}
		if params.ReservationID != nil {
			reservationID = sql.NullInt64{Int64: *params.ReservationID, Valid: true}
		}

		redemption, err := qtx.CreateVisitPackRedemption(ctx, dbgen.CreateVisitPackRedemptionParams{
			VisitPackID:   params.VisitPackID,
			FacilityID:    params.FacilityID,
			RedeemedAt:    redeemedAt,
			ReservationID: reservationID,
		})
		if err != nil {
			return err
		}

		result = VisitPackRedemptionResult{
			VisitPack:  updated,
			Redemption: redemption,
		}
		return nil
	})
	if err != nil {
		return VisitPackRedemptionResult{}, err
	}

	return result, nil
}
