package email

import (
	"context"
	"time"
)

func newEmailContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	// Detach cancellation so handler-scoped contexts don't abort async sends.
	parent = context.WithoutCancel(parent)
	return context.WithTimeout(parent, timeout)
}
