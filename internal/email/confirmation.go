package email

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

const confirmationEmailTimeout = 5 * time.Second

// SendConfirmationEmail sends a confirmation email asynchronously.
func SendConfirmationEmail(ctx context.Context, q *dbgen.Queries, client *SESClient, userID int64, confirmation ConfirmationEmail, logger *zerolog.Logger) {
	if client == nil || q == nil {
		return
	}
	if confirmation.Subject == "" || confirmation.Body == "" {
		return
	}

	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		if logger != nil {
			logger.Error().Err(err).Int64("user_id", userID).Msg("Failed to load user for confirmation email")
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
		sendCtx, cancel := context.WithTimeout(context.Background(), confirmationEmailTimeout)
		defer cancel()
		if err := client.Send(sendCtx, recipient, confirmation.Subject, confirmation.Body); err != nil && logger != nil {
			logger.Error().Err(err).Str("recipient", recipient).Msg("Failed to send confirmation email")
		}
	}()
}
