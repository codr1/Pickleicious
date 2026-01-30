package email

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codr1/Pickleicious/internal/db"
	"github.com/codr1/Pickleicious/internal/testutil"
)

type fakeEmailSender struct {
	sendCalls        int32
	sendFromCalls    int32
	sendStarted      chan struct{}
	sendFromStarted  chan struct{}
	sendCtxErrCh     chan error
	sendFromCtxErrCh chan error
}

func newFakeEmailSender() *fakeEmailSender {
	return &fakeEmailSender{
		sendStarted:      make(chan struct{}, 1),
		sendFromStarted:  make(chan struct{}, 1),
		sendCtxErrCh:     make(chan error, 1),
		sendFromCtxErrCh: make(chan error, 1),
	}
}

func (f *fakeEmailSender) Send(ctx context.Context, recipient, subject, body string) error {
	atomic.AddInt32(&f.sendCalls, 1)
	select {
	case f.sendStarted <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		err := ctx.Err()
		select {
		case f.sendCtxErrCh <- err:
		default:
		}
		return err
	case <-time.After(200 * time.Millisecond):
		return nil
	}
}

func (f *fakeEmailSender) SendFrom(ctx context.Context, recipient, subject, body, sender string) error {
	atomic.AddInt32(&f.sendFromCalls, 1)
	select {
	case f.sendFromStarted <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		err := ctx.Err()
		select {
		case f.sendFromCtxErrCh <- err:
		default:
		}
		return err
	case <-time.After(200 * time.Millisecond):
		return nil
	}
}

func insertTestUser(t *testing.T, database *db.DB, email string) int64 {
	t.Helper()

	result, err := database.Exec(
		`INSERT INTO users (first_name, last_name, email, status) VALUES (?, ?, ?, ?)`,
		"Test",
		"User",
		email,
		"active",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get user id: %v", err)
	}
	return userID
}

func waitForSignal(t *testing.T, ch <-chan struct{}, message string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(200 * time.Millisecond):
		t.Fatal(message)
	}
}

func waitForError(t *testing.T, ch <-chan error, message string) error {
	t.Helper()

	select {
	case err := <-ch:
		return err
	case <-time.After(200 * time.Millisecond):
		t.Fatal(message)
		return nil
	}
}

func TestSendConfirmationEmail_ContextCanceledStopsSend(t *testing.T) {
	database := testutil.NewTestDB(t)
	userID := insertTestUser(t, database, "member@test.com")
	sender := newFakeEmailSender()

	ctx, cancel := context.WithCancel(context.Background())
	SendConfirmationEmail(ctx, database.Queries, sender, userID, ConfirmationEmail{
		Subject: "Subject",
		Body:    "Body",
	}, nil)

	waitForSignal(t, sender.sendStarted, "expected confirmation send to start")
	cancel()

	err := waitForError(t, sender.sendCtxErrCh, "expected confirmation send to observe cancellation")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if atomic.LoadInt32(&sender.sendCalls) != 1 {
		t.Fatalf("expected one send call, got %d", atomic.LoadInt32(&sender.sendCalls))
	}
}

func TestSendReminderEmail_ContextCanceledStopsSend(t *testing.T) {
	database := testutil.NewTestDB(t)
	userID := insertTestUser(t, database, "member@test.com")
	sender := newFakeEmailSender()

	ctx, cancel := context.WithCancel(context.Background())
	SendReminderEmail(ctx, database.Queries, sender, userID, ConfirmationEmail{
		Subject: "Subject",
		Body:    "Body",
	}, "reminders@test.com", nil)

	waitForSignal(t, sender.sendFromStarted, "expected reminder send to start")
	cancel()

	err := waitForError(t, sender.sendFromCtxErrCh, "expected reminder send to observe cancellation")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if atomic.LoadInt32(&sender.sendFromCalls) != 1 {
		t.Fatalf("expected one send call, got %d", atomic.LoadInt32(&sender.sendFromCalls))
	}
}

func TestSendCancellationEmail_ContextCanceledStopsSend(t *testing.T) {
	database := testutil.NewTestDB(t)
	userID := insertTestUser(t, database, "member@test.com")
	sender := newFakeEmailSender()

	ctx, cancel := context.WithCancel(context.Background())
	SendCancellationEmail(ctx, database.Queries, sender, userID, ConfirmationEmail{
		Subject: "Subject",
		Body:    "Body",
	}, "cancellations@test.com", nil)

	waitForSignal(t, sender.sendFromStarted, "expected cancellation send to start")
	cancel()

	err := waitForError(t, sender.sendFromCtxErrCh, "expected cancellation send to observe cancellation")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if atomic.LoadInt32(&sender.sendFromCalls) != 1 {
		t.Fatalf("expected one send call, got %d", atomic.LoadInt32(&sender.sendFromCalls))
	}
}
