package email

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

const reminderEmailTimeout = 5 * time.Second

// SendReminderEmail sends a reminder email asynchronously.
func SendReminderEmail(ctx context.Context, q *dbgen.Queries, client *SESClient, userID int64, message ConfirmationEmail, sender string, logger *zerolog.Logger) {
	if client == nil || q == nil {
		return
	}
	if message.Subject == "" || message.Body == "" {
		return
	}

	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		if logger != nil {
			logger.Error().Err(err).Int64("user_id", userID).Msg("Failed to load user for reminder email")
		}
		return
	}
	if !user.Email.Valid {
		return
	}
	recipient := strings.TrimSpace(user.Email.String)
	if recipient == "" {
		return
	}

	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), reminderEmailTimeout)
		defer cancel()
		if err := client.SendFrom(sendCtx, recipient, message.Subject, message.Body, sender); err != nil {
			if logger != nil {
				logger.Error().Err(err).Int64("user_id", userID).Msg("Failed to send reminder email")
			}
			return
		}
		if logger != nil {
			logger.Info().Int64("user_id", userID).Msg("Reminder email sent")
		}
	}()
}
