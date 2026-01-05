package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/codr1/Pickleicious/internal/config"
)

func TestParseAuthCookieSessionType(t *testing.T) {
	prevConfig := appConfig
	appConfig = &config.Config{}
	appConfig.App.SecretKey = "test-secret"
	t.Cleanup(func() {
		appConfig = prevConfig
	})

	sessionPayload := authSession{
		UserID:      42,
		SessionType: sessionTypeStaff,
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
	}

	payloadBytes, err := json.Marshal(sessionPayload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := makeAuthRequest(t, payloadBytes)

	session, err := parseAuthCookie(req)
	if err != nil {
		t.Fatalf("parse auth cookie: %v", err)
	}
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	if session.SessionType != sessionTypeStaff {
		t.Fatalf("expected session type %q, got %q", sessionTypeStaff, session.SessionType)
	}
}

func TestParseAuthCookieSessionTypeMember(t *testing.T) {
	prevConfig := appConfig
	appConfig = &config.Config{}
	appConfig.App.SecretKey = "test-secret"
	t.Cleanup(func() {
		appConfig = prevConfig
	})

	sessionPayload := authSession{
		UserID:      42,
		SessionType: sessionTypeMember,
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
	}

	payloadBytes, err := json.Marshal(sessionPayload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := makeAuthRequest(t, payloadBytes)

	session, err := parseAuthCookie(req)
	if err != nil {
		t.Fatalf("parse auth cookie: %v", err)
	}
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	if session.SessionType != sessionTypeMember {
		t.Fatalf("expected session type %q, got %q", sessionTypeMember, session.SessionType)
	}
}

func TestNormalizeSessionTypeUnknownDefaultsToMember(t *testing.T) {
	normalized := normalizeSessionType("unknown")
	if normalized != sessionTypeMember {
		t.Fatalf("expected session type %q, got %q", sessionTypeMember, normalized)
	}
}

func makeAuthRequest(t *testing.T, payload []byte) *http.Request {
	t.Helper()

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature, err := signPayload(encodedPayload)
	if err != nil {
		t.Fatalf("sign payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  authCookieName,
		Value: encodedPayload + "." + signature,
	})

	return req
}
