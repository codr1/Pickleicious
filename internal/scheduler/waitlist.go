package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

const (
	defaultWaitlistOfferExpiryMinutes int64 = 30
	waitlistStatusExpired                   = "expired"
	waitlistStatusNotified                  = "notified"
)

func ExpireWaitlistOffers(ctx context.Context, database *db.DB, now time.Time) error {
	if database == nil {
		return fmt.Errorf("waitlist offer expiry requires database")
	}

	rows, err := database.Queries.ListExpiredOffers(ctx, now)
	if err != nil {
		return fmt.Errorf("list expired offers: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	logger := log.Ctx(ctx)
	for _, row := range rows {
		var advancedOfferID int64
		var advanced bool

		err := database.RunInTx(ctx, func(txdb *db.DB) error {
			if _, err := txdb.Queries.ExpireOffer(ctx, dbgen.ExpireOfferParams{
				ID:         row.OfferID,
				WaitlistID: row.WaitlistID,
			}); err != nil {
				return fmt.Errorf("expire offer: %w", err)
			}

			if _, err := txdb.Queries.UpdateWaitlistStatus(ctx, dbgen.UpdateWaitlistStatusParams{
				ID:         row.WaitlistID,
				FacilityID: row.FacilityID,
				Status:     waitlistStatusExpired,
			}); err != nil {
				return fmt.Errorf("update waitlist status: %w", err)
			}

			expiryMinutes := row.OfferExpiryMinutes
			if expiryMinutes <= 0 {
				expiryMinutes = defaultWaitlistOfferExpiryMinutes
			}
			expiresAt := now.Add(time.Duration(expiryMinutes) * time.Minute)

			nextOffer, err := txdb.Queries.AdvanceWaitlistOffer(ctx, dbgen.AdvanceWaitlistOfferParams{
				WaitlistID: row.WaitlistID,
				ExpiresAt:  expiresAt,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil
				}
				return fmt.Errorf("advance waitlist offer: %w", err)
			}

			advanced = true
			advancedOfferID = nextOffer.ID

			if _, err := txdb.Queries.UpdateWaitlistStatus(ctx, dbgen.UpdateWaitlistStatusParams{
				ID:         nextOffer.WaitlistID,
				FacilityID: row.FacilityID,
				Status:     waitlistStatusNotified,
			}); err != nil {
				return fmt.Errorf("update next waitlist status: %w", err)
			}

			return nil
		})
		if err != nil {
			logger.Error().Err(err).
				Int64("waitlist_id", row.WaitlistID).
				Int64("offer_id", row.OfferID).
				Msg("Failed to expire waitlist offer")
			continue
		}

		event := logger.Info().
			Int64("waitlist_id", row.WaitlistID).
			Int64("offer_id", row.OfferID)
		if advanced {
			event.Int64("next_offer_id", advancedOfferID)
		}
		event.Msg("Expired waitlist offer")
	}

	return nil
}

func CleanupPastWaitlists(ctx context.Context, database *db.DB, now time.Time) error {
	if database == nil {
		return fmt.Errorf("waitlist cleanup requires database")
	}

	deleted, err := database.Queries.DeletePastWaitlistEntries(ctx, now)
	if err != nil {
		return fmt.Errorf("delete past waitlist entries: %w", err)
	}

	log.Ctx(ctx).Debug().Int64("deleted_waitlists", deleted).Msg("Cleaned up past waitlists")
	return nil
}
