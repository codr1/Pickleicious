package email

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/rs/zerolog"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

const cancellationEmailTimeout = 5 * time.Second

// SendCancellationEmail sends a cancellation email asynchronously.
func SendCancellationEmail(ctx context.Context, q *dbgen.Queries, client EmailSender, userID int64, message ConfirmationEmail, sender string, logger *zerolog.Logger) {
	if client == nil || q == nil {
		return
	}
	if userID <= 0 {
		if logger != nil {
			logger.Warn().Int64("user_id", userID).Msg("Skipping cancellation email with invalid user ID")
		}
		return
	}
	if message.Subject == "" || message.Body == "" {
		return
	}

	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		if logger != nil {
			logger.Error().Err(err).Int64("user_id", userID).Msg("Failed to load user for cancellation email")
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
		sendCtx, cancel := newEmailContext(ctx, cancellationEmailTimeout)
		defer cancel()
		if sendCtx.Err() != nil {
			return
		}
		if err := client.SendFrom(sendCtx, recipient, message.Subject, message.Body, sender); err != nil && logger != nil {
			logger.Error().Err(err).Int64("user_id", userID).Msg("Failed to send cancellation email")
		}
	}()
}

// ResolveFromAddress uses the facility email_from_address with an organization fallback.
func ResolveFromAddress(ctx context.Context, q *dbgen.Queries, facility dbgen.Facility, logger *zerolog.Logger) string {
	from := strings.TrimSpace(facility.EmailFromAddress.String)
	if from != "" {
		return from
	}
	if facility.OrganizationID == 0 || q == nil {
		return ""
	}
	org, err := q.GetOrganizationEmailConfig(ctx, facility.OrganizationID)
	if err != nil {
		if logger != nil && !errors.Is(err, sql.ErrNoRows) {
			logger.Error().Err(err).Int64("organization_id", facility.OrganizationID).Msg("Failed to load organization email configuration")
		}
		return ""
	}
	return strings.TrimSpace(org.EmailFromAddress.String)
}
