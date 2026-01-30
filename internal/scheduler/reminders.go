package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/apiutil"
	"github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/email"
)

const (
	defaultReminderHoursBefore int64 = 24
	reminderJobWindow                = 15 * time.Minute
)

// RegisterReminderJobs registers scheduled reservation reminder tasks.
func RegisterReminderJobs(database *db.DB, emailClient *email.SESClient) error {
	if database == nil {
		return fmt.Errorf("reminder jobs require database")
	}

	jobName := "reservation_reminders"
	cronExpr := "*/15 * * * *"
	jobLogger := log.With().
		Str("component", "reservation_reminders_job").
		Str("job_name", jobName).
		Str("cron", cronExpr).
		Logger()

	_, err := AddJob(jobName, cronExpr, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		ctx = jobLogger.WithContext(ctx)

		if emailClient == nil {
			jobLogger.Debug().Msg("Reminder job skipped: email client not configured")
			return
		}

		now := time.Now().UTC()
		facilities, err := database.Queries.ListFacilities(ctx)
		if err != nil {
			jobLogger.Error().Err(err).Msg("Failed to load facilities for reminder job")
			return
		}

		for _, facility := range facilities {
			facilityLogger := jobLogger.With().Int64("facility_id", facility.ID).Logger()
			facilityCtx := facilityLogger.WithContext(ctx)

			reminderHours := resolveReminderHours(facilityCtx, database.Queries, facility, &facilityLogger)

			windowStart := now.Add(time.Duration(reminderHours) * time.Hour)
			windowEnd := windowStart.Add(reminderJobWindow)

			reservations, err := database.Queries.ListReservationsStartingBetween(facilityCtx, dbgen.ListReservationsStartingBetweenParams{
				FacilityID: facility.ID,
				StartTime:  windowStart,
				EndTime:    windowEnd,
			})
			if err != nil {
				facilityLogger.Error().Err(err).Msg("Failed to load reservations for reminder job")
				continue
			}
			if len(reservations) == 0 {
				continue
			}

			facilityLoc := time.Local
			if facility.Timezone != "" {
				loadedLoc, loadErr := time.LoadLocation(facility.Timezone)
				if loadErr != nil {
					facilityLogger.Error().Err(loadErr).Str("timezone", facility.Timezone).Msg("Failed to load facility timezone for reminders")
				} else {
					facilityLoc = loadedLoc
				}
			}

			for _, reservation := range reservations {
				if err := sendReservationReminder(facilityCtx, database, emailClient, facility, reservation, facilityLoc, &facilityLogger); err != nil {
					facilityLogger.Error().Err(err).Int64("reservation_id", reservation.ID).Msg("Failed to send reminder emails")
					continue
				}
			}
		}
	}, gocron.WithSingletonMode(gocron.LimitModeWait))
	if err != nil {
		return fmt.Errorf("add reservation reminder job: %w", err)
	}

	jobLogger.Info().Msg("Reservation reminder job registered")
	return nil
}

func sendReservationReminder(ctx context.Context, database *db.DB, emailClient *email.SESClient, facility dbgen.Facility, reservation dbgen.Reservation, facilityLoc *time.Location, logger *zerolog.Logger) error {
	if database == nil || emailClient == nil {
		return nil
	}

	reservationTypeName, err := database.Queries.GetReservationTypeNameByReservationID(ctx, reservation.ID)
	if err != nil {
		return fmt.Errorf("load reservation type: %w", err)
	}
	courtRows, err := database.Queries.ListReservationCourts(ctx, reservation.ID)
	if err != nil {
		return fmt.Errorf("load reservation courts: %w", err)
	}
	participants, err := database.Queries.ListParticipantsForReservation(ctx, reservation.ID)
	if err != nil {
		return fmt.Errorf("load reservation participants: %w", err)
	}

	recipientIDs := make(map[int64]struct{})
	for _, participant := range participants {
		if participant.ID == 0 {
			continue
		}
		recipientIDs[participant.ID] = struct{}{}
	}
	if reservation.PrimaryUserID.Valid {
		recipientIDs[reservation.PrimaryUserID.Int64] = struct{}{}
	}
	if len(recipientIDs) == 0 {
		return nil
	}

	date, timeRange := email.FormatDateTimeRange(reservation.StartTime.In(facilityLoc), reservation.EndTime.In(facilityLoc))
	reminder := email.BuildReminderEmail(email.ReminderDetails{
		FacilityName:    facility.Name,
		ReservationType: email.ReservationTypeLabel(reservationTypeName),
		Date:            date,
		TimeRange:       timeRange,
		Courts:          apiutil.ReservationCourtLabel(courtRows),
	})
	sender := email.ResolveFromAddress(ctx, database.Queries, facility, logger)

	for userID := range recipientIDs {
		email.SendReminderEmail(ctx, database.Queries, emailClient, userID, reminder, sender, logger)
	}

	return nil
}

func resolveReminderHours(ctx context.Context, q *dbgen.Queries, facility dbgen.Facility, logger *zerolog.Logger) int64 {
	reminderHours := facility.ReminderHoursBefore
	useOrgFallback := reminderHours <= 0 || reminderHours == defaultReminderHoursBefore
	if useOrgFallback && q != nil && facility.OrganizationID != 0 {
		orgConfig, err := q.GetOrganizationReminderConfig(ctx, facility.OrganizationID)
		if err != nil {
			if logger != nil && !errors.Is(err, sql.ErrNoRows) {
				logger.Error().Err(err).Int64("organization_id", facility.OrganizationID).Msg("Failed to load organization reminder configuration")
			}
		} else if orgConfig.ReminderHoursBefore > 0 {
			return orgConfig.ReminderHoursBefore
		}
	}

	if reminderHours > 0 {
		return reminderHours
	}

	return defaultReminderHoursBefore
}
