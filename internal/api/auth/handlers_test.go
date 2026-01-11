package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/config"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/testutil"
)

type authTestContext struct {
	orgID      int64
	facilityID int64
}

func setupAuthTest(t *testing.T, env string) authTestContext {
	t.Helper()

	database := testutil.NewTestDB(t)

	// Save and restore global state
	prevConfig := appConfig
	prevQueries := queries
	t.Cleanup(func() {
		appConfig = prevConfig
		queries = prevQueries
	})

	// Set up config with specified environment
	appConfig = &config.Config{}
	appConfig.App.Environment = env
	appConfig.App.SecretKey = "test-secret-key"

	queries = dbgen.New(database.DB)

	ctx := context.Background()

	// Create organization
	orgResult, err := database.ExecContext(ctx,
		"INSERT INTO organizations (name, slug, status) VALUES (?, ?, ?)",
		"Test Org", "test-org", "active",
	)
	if err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	orgID, _ := orgResult.LastInsertId()

	// Create facility
	facilityResult, err := database.ExecContext(ctx,
		"INSERT INTO facilities (organization_id, name, slug, timezone) VALUES (?, ?, ?, ?)",
		orgID, "Test Facility", "test-facility", "UTC",
	)
	if err != nil {
		t.Fatalf("insert facility: %v", err)
	}
	facilityID, _ := facilityResult.LastInsertId()

	// Create a member user
	_, err = database.ExecContext(ctx,
		`INSERT INTO users (email, first_name, last_name, is_member, home_facility_id, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"member@test.com", "Test", "Member", true, facilityID, "active",
	)
	if err != nil {
		t.Fatalf("insert member user: %v", err)
	}

	return authTestContext{orgID: orgID, facilityID: facilityID}
}

func TestDevBypassSendCode(t *testing.T) {
	tc := setupAuthTest(t, devEnvironment)

	form := url.Values{}
	form.Set("identifier", "member@test.com")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/send-code", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Add organization context (simulates subdomain middleware)
	ctx := authz.ContextWithOrganization(req.Context(), &authz.Organization{ID: tc.orgID, Name: "Test Org", Slug: "test-org"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	HandleSendCode(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Should contain the dev-session token
	body := rec.Body.String()
	if !strings.Contains(body, devBypassSession) {
		t.Errorf("expected body to contain dev session token %q, got: %s", devBypassSession, body)
	}
}

func TestDevBypassVerifyCode(t *testing.T) {
	tc := setupAuthTest(t, devEnvironment)

	form := url.Values{}
	form.Set("identifier", "member@test.com")
	form.Set("session", devBypassSession)
	form.Set("code", devBypassCode)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify-code", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Add organization context (simulates subdomain middleware)
	ctx := authz.ContextWithOrganization(req.Context(), &authz.Organization{ID: tc.orgID, Name: "Test Org", Slug: "test-org"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	HandleVerifyCode(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Should have HX-Redirect header set (members go to /member)
	redirect := rec.Header().Get("HX-Redirect")
	if redirect != "/member" {
		t.Errorf("expected HX-Redirect to /member, got %q", redirect)
	}

	// Should have auth cookie set
	cookies := rec.Result().Cookies()
	var hasAuthCookie bool
	for _, c := range cookies {
		if c.Name == authCookieName {
			hasAuthCookie = true
			break
		}
	}
	if !hasAuthCookie {
		t.Error("expected auth cookie to be set")
	}
}

func TestDevBypassWrongCodeFails(t *testing.T) {
	tc := setupAuthTest(t, devEnvironment)

	form := url.Values{}
	form.Set("identifier", "member@test.com")
	form.Set("session", devBypassSession)
	form.Set("code", "999999") // Wrong code

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify-code", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Add organization context (simulates subdomain middleware)
	ctx := authz.ContextWithOrganization(req.Context(), &authz.Organization{ID: tc.orgID, Name: "Test Org", Slug: "test-org"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	HandleVerifyCode(rec, req)

	// Should fail - wrong code should not grant access
	redirect := rec.Header().Get("HX-Redirect")
	if redirect == "/member" {
		t.Error("expected wrong code to NOT redirect to /member")
	}
}

func TestDevBypassDisabledInProduction(t *testing.T) {
	tc := setupAuthTest(t, "production")

	form := url.Values{}
	form.Set("identifier", "member@test.com")
	form.Set("session", devBypassSession)
	form.Set("code", devBypassCode)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify-code", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Add organization context (simulates subdomain middleware)
	ctx := authz.ContextWithOrganization(req.Context(), &authz.Organization{ID: tc.orgID, Name: "Test Org", Slug: "test-org"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	HandleVerifyCode(rec, req)

	// Should fail - bypass not active in production
	redirect := rec.Header().Get("HX-Redirect")
	if redirect == "/member" {
		t.Error("dev bypass should NOT work in production environment")
	}
}

func TestIsDevMode(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		expected bool
	}{
		{"development", devEnvironment, true},
		{"production", "production", false},
		{"staging", "staging", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appConfig = &config.Config{}
			appConfig.App.Environment = tt.env

			if got := isDevMode(); got != tt.expected {
				t.Errorf("isDevMode() with env=%q: got %v, want %v", tt.env, got, tt.expected)
			}
		})
	}

	// Test nil config
	t.Run("nil config", func(t *testing.T) {
		appConfig = nil
		if isDevMode() {
			t.Error("isDevMode() with nil config should return false")
		}
	})
}
