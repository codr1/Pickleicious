//go:build integration
// +build integration

package authz_test

// NOTE: Tests cannot use t.Parallel() due to shared package state.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/api/openplay"
	"github.com/codr1/Pickleicious/internal/api/themes"
	"github.com/codr1/Pickleicious/internal/db"
	"github.com/codr1/Pickleicious/internal/testutil"
)

func setupAuthzIntegrationTest(t *testing.T) *db.DB {
	t.Helper()

	database := testutil.NewTestDB(t)

	ctx := context.Background()
	_, err := database.ExecContext(ctx,
		"INSERT INTO organizations (id, name, slug, status) VALUES (?, ?, ?, ?)",
		1,
		"Test Org",
		"test-org",
		"active",
	)
	if err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	_, err = database.ExecContext(ctx,
		"INSERT INTO facilities (id, organization_id, name, slug, timezone) VALUES (?, ?, ?, ?, ?)",
		1,
		1,
		"Facility One",
		"facility-one",
		"UTC",
	)
	if err != nil {
		t.Fatalf("insert facility 1: %v", err)
	}
	_, err = database.ExecContext(ctx,
		"INSERT INTO facilities (id, organization_id, name, slug, timezone) VALUES (?, ?, ?, ?, ?)",
		2,
		1,
		"Facility Two",
		"facility-two",
		"UTC",
	)
	if err != nil {
		t.Fatalf("insert facility 2: %v", err)
	}

	themes.InitHandlers(database.Queries)
	openplay.InitHandlers(database)

	return database
}

func TestFacilityAccessIntegration(t *testing.T) {
	setupAuthzIntegrationTest(t)

	handlers := []struct {
		name       string
		handler    func(http.ResponseWriter, *http.Request)
		newRequest func(int64) *http.Request
	}{
		{
			name:    "themes list",
			handler: themes.HandleThemesList,
			newRequest: func(facilityID int64) *http.Request {
				return httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/themes?facility_id=%d", facilityID), nil)
			},
		},
		{
			name:    "open play rules list",
			handler: openplay.HandleOpenPlayRulesList,
			newRequest: func(facilityID int64) *http.Request {
				return httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/open-play-rules?facility_id=%d", facilityID), nil)
			},
		},
	}

	cases := []struct {
		name       string
		user       *authz.AuthUser
		facilityID int64
		wantStatus int
	}{
		{
			name: "staff home facility access allowed",
			user: &authz.AuthUser{
				ID:             1,
				IsStaff:        true,
				HomeFacilityID: int64Ptr(1),
			},
			facilityID: 1,
			wantStatus: http.StatusOK,
		},
		{
			name: "staff home facility mismatch forbidden",
			user: &authz.AuthUser{
				ID:             2,
				IsStaff:        true,
				HomeFacilityID: int64Ptr(1),
			},
			facilityID: 2,
			wantStatus: http.StatusForbidden,
		},
		{
			name: "non-staff allowed",
			user: &authz.AuthUser{
				ID:      3,
				IsStaff: false,
			},
			facilityID: 2,
			wantStatus: http.StatusOK,
		},
		{
			name:       "unauthenticated rejected",
			user:       nil,
			facilityID: 1,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, handlerCase := range handlers {
				handlerCase := handlerCase
				t.Run(handlerCase.name, func(t *testing.T) {
					req := handlerCase.newRequest(tc.facilityID)
					if tc.user != nil {
						req = req.WithContext(authz.ContextWithUser(req.Context(), tc.user))
					}
					recorder := httptest.NewRecorder()
					handlerCase.handler(recorder, req)
					if recorder.Code != tc.wantStatus {
						t.Fatalf("status: got %d want %d", recorder.Code, tc.wantStatus)
					}
				})
			}
		})
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
