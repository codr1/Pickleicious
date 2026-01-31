package email

import "context"

// EmailSender provides a testable abstraction over SES delivery.
type EmailSender interface {
	Send(ctx context.Context, recipient, subject, body string) error
	SendFrom(ctx context.Context, recipient, subject, body, sender string) error
}
