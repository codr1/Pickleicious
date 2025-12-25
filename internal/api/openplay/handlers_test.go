package openplay

// NOTE: Tests cannot use t.Parallel() due to shared package state.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codr1/Pickleicious/internal/db"
)

func setupOpenPlayTest(t *testing.T) (*db.DB, int64) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "open_play.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	ctx := context.Background()
	orgResult, err := database.ExecContext(ctx,
		"INSERT INTO organizations (name, slug, status) VALUES (?, ?, ?)",
		"Test Org",
		"test-org",
		"active",
	)
	if err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	orgID, err := orgResult.LastInsertId()
	if err != nil {
		t.Fatalf("organization id: %v", err)
	}

	facilityResult, err := database.ExecContext(ctx,
		"INSERT INTO facilities (organization_id, name, slug, timezone) VALUES (?, ?, ?, ?)",
		orgID,
		"Main Facility",
		"main-facility",
		"UTC",
	)
	if err != nil {
		t.Fatalf("insert facility: %v", err)
	}
	facilityID, err := facilityResult.LastInsertId()
	if err != nil {
		t.Fatalf("facility id: %v", err)
	}

	InitHandlers(database.Queries)
	return database, facilityID
}

func TestOpenPlayRuleHandlersCRUD(t *testing.T) {
	database, facilityID := setupOpenPlayTest(t)
	ctx := context.Background()

	createForm := url.Values{
		"name":                        []string{"Morning Open Play"},
		"min_participants":            []string{"4"},
		"max_participants_per_court":  []string{"8"},
		"cancellation_cutoff_minutes": []string{"60"},
		"min_courts":                  []string{"1"},
		"max_courts":                  []string{"2"},
		"auto_scale_enabled":          []string{"on"},
	}
	createReq := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/api/v1/open-play/rules?facility_id=%d", facilityID),
		strings.NewReader(createForm.Encode()),
	)
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createRecorder := httptest.NewRecorder()
	HandleOpenPlayRuleCreate(createRecorder, createReq)

	if createRecorder.Code != http.StatusOK {
		t.Fatalf("create status: %d", createRecorder.Code)
	}
	if !strings.Contains(createRecorder.Body.String(), "Morning Open Play") {
		t.Fatalf("create body missing name: %s", createRecorder.Body.String())
	}

	rules, err := database.Queries.ListOpenPlayRules(ctx, facilityID)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("expected open play rule to be created")
	}
	ruleID := rules[0].ID

	pageReq := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/open-play?facility_id=%d", facilityID),
		nil,
	)
	pageRecorder := httptest.NewRecorder()
	HandleOpenPlayRulesPage(pageRecorder, pageReq)
	if pageRecorder.Code != http.StatusOK {
		t.Fatalf("page status: %d", pageRecorder.Code)
	}
	if !strings.Contains(pageRecorder.Body.String(), "Open Play Rules") {
		t.Fatalf("page body missing header: %s", pageRecorder.Body.String())
	}

	listReq := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/api/v1/open-play/rules?facility_id=%d", facilityID),
		nil,
	)
	listRecorder := httptest.NewRecorder()
	HandleOpenPlayRulesList(listRecorder, listReq)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status: %d", listRecorder.Code)
	}
	if !strings.Contains(listRecorder.Body.String(), "Morning Open Play") {
		t.Fatalf("list body missing name: %s", listRecorder.Body.String())
	}

	detailReq := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/api/v1/open-play/rules/%d?facility_id=%d", ruleID, facilityID),
		nil,
	)
	detailRecorder := httptest.NewRecorder()
	HandleOpenPlayRuleDetail(detailRecorder, detailReq)
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("detail status: %d", detailRecorder.Code)
	}
	if !strings.Contains(detailRecorder.Body.String(), "Morning Open Play") {
		t.Fatalf("detail body missing name: %s", detailRecorder.Body.String())
	}

	updateForm := url.Values{
		"name":                        []string{"Updated Open Play"},
		"min_participants":            []string{"6"},
		"max_participants_per_court":  []string{"8"},
		"cancellation_cutoff_minutes": []string{"30"},
		"min_courts":                  []string{"1"},
		"max_courts":                  []string{"3"},
		"auto_scale_enabled":          []string{"true"},
	}
	updateReq := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/v1/open-play/rules/%d?facility_id=%d", ruleID, facilityID),
		strings.NewReader(updateForm.Encode()),
	)
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateRecorder := httptest.NewRecorder()
	HandleOpenPlayRuleUpdate(updateRecorder, updateReq)
	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("update status: %d", updateRecorder.Code)
	}
	if !strings.Contains(updateRecorder.Body.String(), "Updated Open Play") {
		t.Fatalf("update body missing name: %s", updateRecorder.Body.String())
	}

	deleteReq := httptest.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("/api/v1/open-play/rules/%d?facility_id=%d", ruleID, facilityID),
		nil,
	)
	deleteRecorder := httptest.NewRecorder()
	HandleOpenPlayRuleDelete(deleteRecorder, deleteReq)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete status: %d", deleteRecorder.Code)
	}
	expectedRedirect := fmt.Sprintf("/open-play?facility_id=%d", facilityID)
	if deleteRecorder.Header().Get("HX-Redirect") != expectedRedirect {
		t.Fatalf("delete HX-Redirect: %s", deleteRecorder.Header().Get("HX-Redirect"))
	}

	redirectReq := httptest.NewRequest(http.MethodGet, expectedRedirect, nil)
	redirectRecorder := httptest.NewRecorder()
	HandleOpenPlayRulesPage(redirectRecorder, redirectReq)
	if redirectRecorder.Code != http.StatusOK {
		t.Fatalf("redirect page status: %d", redirectRecorder.Code)
	}
}

func TestOpenPlayRuleCreateValidation(t *testing.T) {
	_, facilityID := setupOpenPlayTest(t)

	cases := []struct {
		name        string
		form        url.Values
		wantStatus  int
		wantMessage string
	}{
		{
			name: "missing min_participants",
			form: url.Values{
				"name":                        []string{"Missing Min"},
				"max_participants_per_court":  []string{"8"},
				"cancellation_cutoff_minutes": []string{"60"},
				"min_courts":                  []string{"1"},
				"max_courts":                  []string{"2"},
			},
			wantStatus:  http.StatusBadRequest,
			wantMessage: "min_participants is required",
		},
		{
			name: "negative cancellation cutoff",
			form: url.Values{
				"name":                        []string{"Negative Cutoff"},
				"min_participants":            []string{"4"},
				"max_participants_per_court":  []string{"8"},
				"cancellation_cutoff_minutes": []string{"-1"},
				"min_courts":                  []string{"1"},
				"max_courts":                  []string{"2"},
			},
			wantStatus:  http.StatusBadRequest,
			wantMessage: "cancellation_cutoff_minutes must be 0 or greater",
		},
		{
			name: "min courts greater than max courts",
			form: url.Values{
				"name":                        []string{"Court Range"},
				"min_participants":            []string{"4"},
				"max_participants_per_court":  []string{"8"},
				"cancellation_cutoff_minutes": []string{"60"},
				"min_courts":                  []string{"3"},
				"max_courts":                  []string{"1"},
			},
			wantStatus:  http.StatusBadRequest,
			wantMessage: "min_courts must be less than or equal to max_courts",
		},
		{
			name: "min participants exceeds capacity",
			form: url.Values{
				"name":                        []string{"Capacity Limit"},
				"min_participants":            []string{"9"},
				"max_participants_per_court":  []string{"4"},
				"cancellation_cutoff_minutes": []string{"15"},
				"min_courts":                  []string{"1"},
				"max_courts":                  []string{"2"},
			},
			wantStatus:  http.StatusBadRequest,
			wantMessage: "min_participants must be less than or equal to max_participants_per_court * max_courts",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodPost,
				fmt.Sprintf("/api/v1/open-play/rules?facility_id=%d", facilityID),
				strings.NewReader(tc.form.Encode()),
			)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			recorder := httptest.NewRecorder()
			HandleOpenPlayRuleCreate(recorder, req)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status: %d", recorder.Code)
			}
			if !strings.Contains(recorder.Body.String(), tc.wantMessage) {
				t.Fatalf("message %q missing in: %s", tc.wantMessage, recorder.Body.String())
			}
		})
	}
}

func TestOpenPlayRuleIDFromPathValidation(t *testing.T) {
	id, err := openPlayRuleIDFromPath("/api/v1/open-play/rules/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 123 {
		t.Fatalf("expected id 123, got %d", id)
	}
	if _, err := openPlayRuleIDFromPath("/api/v1/open-play/rules/123/edit"); err == nil {
		t.Fatal("expected error for edit suffix")
	}
	if _, err := openPlayRuleIDFromPath("/api/v1/open-play/rules/"); err == nil {
		t.Fatal("expected error for missing rule id")
	}
}
