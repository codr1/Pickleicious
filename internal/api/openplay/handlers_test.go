package openplay

// NOTE: Tests cannot use t.Parallel() due to shared package state.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/testutil"
)

func setupOpenPlayTest(t *testing.T) (*db.DB, int64) {
	t.Helper()

	database := testutil.NewTestDB(t)

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

	queriesOnce = sync.Once{}
	queries = nil
	store = nil
	InitHandlers(database, nil)

	return database, facilityID
}

func withAuthUser(req *http.Request, facilityID int64) *http.Request {
	homeFacilityID := facilityID
	user := &authz.AuthUser{
		ID:             1,
		IsStaff:        true,
		HomeFacilityID: &homeFacilityID,
	}
	return req.WithContext(authz.ContextWithUser(req.Context(), user))
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
		fmt.Sprintf("/api/v1/open-play-rules?facility_id=%d", facilityID),
		strings.NewReader(createForm.Encode()),
	)
	createReq = withAuthUser(createReq, facilityID)
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
		fmt.Sprintf("/open-play-rules?facility_id=%d", facilityID),
		nil,
	)
	pageReq = withAuthUser(pageReq, facilityID)
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
		fmt.Sprintf("/api/v1/open-play-rules?facility_id=%d", facilityID),
		nil,
	)
	listReq = withAuthUser(listReq, facilityID)
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
		fmt.Sprintf("/api/v1/open-play-rules/%d?facility_id=%d", ruleID, facilityID),
		nil,
	)
	detailReq = withAuthUser(detailReq, facilityID)
	detailReq.SetPathValue("id", fmt.Sprintf("%d", ruleID))
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
		fmt.Sprintf("/api/v1/open-play-rules/%d?facility_id=%d", ruleID, facilityID),
		strings.NewReader(updateForm.Encode()),
	)
	updateReq = withAuthUser(updateReq, facilityID)
	updateReq.SetPathValue("id", fmt.Sprintf("%d", ruleID))
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
		fmt.Sprintf("/api/v1/open-play-rules/%d?facility_id=%d", ruleID, facilityID),
		nil,
	)
	deleteReq = withAuthUser(deleteReq, facilityID)
	deleteReq.SetPathValue("id", fmt.Sprintf("%d", ruleID))
	deleteRecorder := httptest.NewRecorder()
	HandleOpenPlayRuleDelete(deleteRecorder, deleteReq)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete status: %d", deleteRecorder.Code)
	}
	expectedRedirect := fmt.Sprintf("/open-play-rules?facility_id=%d", facilityID)
	if deleteRecorder.Header().Get("HX-Redirect") != expectedRedirect {
		t.Fatalf("delete HX-Redirect: %s", deleteRecorder.Header().Get("HX-Redirect"))
	}

	redirectReq := httptest.NewRequest(http.MethodGet, expectedRedirect, nil)
	redirectReq = withAuthUser(redirectReq, facilityID)
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
				"min_courts":                  []string{"2"},
				"max_courts":                  []string{"4"},
			},
			wantStatus:  http.StatusBadRequest,
			wantMessage: "min_participants must be less than or equal to max_participants_per_court * min_courts",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodPost,
				fmt.Sprintf("/api/v1/open-play-rules?facility_id=%d", facilityID),
				strings.NewReader(tc.form.Encode()),
			)
			req = withAuthUser(req, facilityID)
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

func TestOpenPlaySessionAutoScaleToggle(t *testing.T) {
	database, facilityID := setupOpenPlayTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	rule, err := database.Queries.CreateOpenPlayRule(ctx, dbgen.CreateOpenPlayRuleParams{
		FacilityID:                facilityID,
		Name:                      "Auto Scale Rule",
		MinParticipants:           4,
		MaxParticipantsPerCourt:   8,
		CancellationCutoffMinutes: 30,
		AutoScaleEnabled:          true,
		MinCourts:                 1,
		MaxCourts:                 2,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	session, err := database.Queries.CreateOpenPlaySession(ctx, dbgen.CreateOpenPlaySessionParams{
		FacilityID:         facilityID,
		OpenPlayRuleID:     rule.ID,
		StartTime:          now.Add(2 * time.Hour),
		EndTime:            now.Add(3 * time.Hour),
		Status:             "scheduled",
		CurrentCourtCount:  1,
		AutoScaleOverride:  sql.NullBool{},
		CancelledAt:        sql.NullTime{},
		CancellationReason: sql.NullString{},
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	toggleReq := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/v1/open-play-sessions/%d/auto-scale?facility_id=%d", session.ID, facilityID),
		strings.NewReader(`{"disable_for_rule":false}`),
	)
	toggleReq.Header.Set("Content-Type", "application/json")
	toggleReq.SetPathValue("id", fmt.Sprintf("%d", session.ID))
	toggleRecorder := httptest.NewRecorder()
	HandleOpenPlaySessionAutoScaleToggle(toggleRecorder, toggleReq)

	if toggleRecorder.Code != http.StatusOK {
		t.Fatalf("toggle status: %d", toggleRecorder.Code)
	}

	var updated dbgen.GetOpenPlaySessionRow
	if err := json.NewDecoder(toggleRecorder.Body).Decode(&updated); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !updated.AutoScaleOverride.Valid || updated.AutoScaleOverride.Bool {
		t.Fatalf("expected override to be set to false, got %+v", updated.AutoScaleOverride)
	}

	logs, err := database.Queries.ListOpenPlayAuditLog(ctx, dbgen.ListOpenPlayAuditLogParams{
		SessionID:  session.ID,
		FacilityID: facilityID,
	})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if logs[0].Action != openPlayAuditAutoScaleOverride {
		t.Fatalf("unexpected audit action: %s", logs[0].Action)
	}

	ruleTwo, err := database.Queries.CreateOpenPlayRule(ctx, dbgen.CreateOpenPlayRuleParams{
		FacilityID:                facilityID,
		Name:                      "Disable Rule",
		MinParticipants:           4,
		MaxParticipantsPerCourt:   8,
		CancellationCutoffMinutes: 30,
		AutoScaleEnabled:          true,
		MinCourts:                 1,
		MaxCourts:                 2,
	})
	if err != nil {
		t.Fatalf("create second rule: %v", err)
	}

	sessionTwo, err := database.Queries.CreateOpenPlaySession(ctx, dbgen.CreateOpenPlaySessionParams{
		FacilityID:         facilityID,
		OpenPlayRuleID:     ruleTwo.ID,
		StartTime:          now.Add(4 * time.Hour),
		EndTime:            now.Add(5 * time.Hour),
		Status:             "scheduled",
		CurrentCourtCount:  1,
		AutoScaleOverride:  sql.NullBool{},
		CancelledAt:        sql.NullTime{},
		CancellationReason: sql.NullString{},
	})
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}

	disableReq := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/v1/open-play-sessions/%d/auto-scale?facility_id=%d", sessionTwo.ID, facilityID),
		strings.NewReader(`{"disable_for_rule":true}`),
	)
	disableReq.Header.Set("Content-Type", "application/json")
	disableReq.SetPathValue("id", fmt.Sprintf("%d", sessionTwo.ID))
	disableRecorder := httptest.NewRecorder()
	HandleOpenPlaySessionAutoScaleToggle(disableRecorder, disableReq)

	if disableRecorder.Code != http.StatusOK {
		t.Fatalf("disable status: %d", disableRecorder.Code)
	}

	updatedRule, err := database.Queries.GetOpenPlayRule(ctx, dbgen.GetOpenPlayRuleParams{
		ID:         ruleTwo.ID,
		FacilityID: facilityID,
	})
	if err != nil {
		t.Fatalf("get updated rule: %v", err)
	}
	if updatedRule.AutoScaleEnabled {
		t.Fatal("expected auto scale to be disabled for rule")
	}

	logs, err = database.Queries.ListOpenPlayAuditLog(ctx, dbgen.ListOpenPlayAuditLogParams{
		SessionID:  sessionTwo.ID,
		FacilityID: facilityID,
	})
	if err != nil {
		t.Fatalf("list audit logs for disable: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 audit logs, got %d", len(logs))
	}
	actions := map[string]bool{}
	for _, entry := range logs {
		actions[entry.Action] = true
	}
	if !actions[openPlayAuditAutoScaleOverride] || !actions[openPlayAuditAutoScaleRuleDisable] {
		t.Fatalf("expected audit actions for override and rule disable, got %v", actions)
	}
}
