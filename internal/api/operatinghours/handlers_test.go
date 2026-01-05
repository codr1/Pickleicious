package operatinghours

// NOTE: Tests cannot use t.Parallel() due to shared package state.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/testutil"
)

func setupOperatingHoursTest(t *testing.T) (*db.DB, int64) {
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

	queries = nil
	queriesOnce = sync.Once{}
	InitHandlers(database.Queries)

	t.Cleanup(func() {
		queries = nil
		queriesOnce = sync.Once{}
	})

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

func TestHandleOperatingHoursPage_ContentType(t *testing.T) {
	database, facilityID := setupOperatingHoursTest(t)

	_, err := database.Queries.UpsertOperatingHours(context.Background(), dbgen.UpsertOperatingHoursParams{
		FacilityID: facilityID,
		DayOfWeek:  1,
		OpensAt:    "08:00",
		ClosesAt:   "20:00",
	})
	if err != nil {
		t.Fatalf("seed hours: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/admin/operating-hours?facility_id=%d", facilityID),
		nil,
	)
	req = withAuthUser(req, facilityID)
	recorder := httptest.NewRecorder()

	HandleOperatingHoursPage(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: %d", recorder.Code)
	}
	if recorder.Header().Get("Content-Type") != "text/html" {
		t.Fatalf("content type: %s", recorder.Header().Get("Content-Type"))
	}
	if !strings.Contains(recorder.Body.String(), "Operating Hours") {
		t.Fatalf("missing heading")
	}
}

func TestHandleOperatingHoursPage_Unauthorized(t *testing.T) {
	_, facilityID := setupOperatingHoursTest(t)

	req := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/admin/operating-hours?facility_id=%d", facilityID),
		nil,
	)
	recorder := httptest.NewRecorder()

	HandleOperatingHoursPage(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status: %d", recorder.Code)
	}
}

func TestHandleOperatingHoursUpdate_ValidJSON(t *testing.T) {
	database, facilityID := setupOperatingHoursTest(t)

	payload, err := json.Marshal(map[string]any{
		"facilityId": facilityID,
		"opensAt":    "07:30",
		"closesAt":   "21:00",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/v1/operating-hours/%d", 2),
		strings.NewReader(string(payload)),
	)
	req.SetPathValue(dayOfWeekParam, "2")
	req = withAuthUser(req, facilityID)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	HandleOperatingHoursUpdate(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: %d", recorder.Code)
	}

	hours, err := database.Queries.GetFacilityHours(context.Background(), facilityID)
	if err != nil {
		t.Fatalf("fetch hours: %v", err)
	}
	if len(hours) != 1 {
		t.Fatalf("expected 1 hour row, got %d", len(hours))
	}
	if hours[0].DayOfWeek != 2 {
		t.Fatalf("day: %d", hours[0].DayOfWeek)
	}
	if formatTimeValue(hours[0].OpensAt) != "07:30" {
		t.Fatalf("opens_at: %s", formatTimeValue(hours[0].OpensAt))
	}
	if formatTimeValue(hours[0].ClosesAt) != "21:00" {
		t.Fatalf("closes_at: %s", formatTimeValue(hours[0].ClosesAt))
	}
}

func TestHandleOperatingHoursUpdate_InvalidTimeFormat(t *testing.T) {
	_, facilityID := setupOperatingHoursTest(t)

	payload, err := json.Marshal(map[string]any{
		"facilityId": facilityID,
		"opensAt":    "7am",
		"closesAt":   "21:00",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/operating-hours/3",
		strings.NewReader(string(payload)),
	)
	req.SetPathValue(dayOfWeekParam, "3")
	req = withAuthUser(req, facilityID)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	HandleOperatingHoursUpdate(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", recorder.Code)
	}
}

func TestHandleOperatingHoursUpdate_InvalidOrder(t *testing.T) {
	_, facilityID := setupOperatingHoursTest(t)

	payload, err := json.Marshal(map[string]any{
		"facilityId": facilityID,
		"opensAt":    "22:00",
		"closesAt":   "08:00",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/operating-hours/4",
		strings.NewReader(string(payload)),
	)
	req.SetPathValue(dayOfWeekParam, "4")
	req = withAuthUser(req, facilityID)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	HandleOperatingHoursUpdate(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", recorder.Code)
	}
}

func TestHandleOperatingHoursUpdate_FormEncoded(t *testing.T) {
	database, facilityID := setupOperatingHoursTest(t)

	body := fmt.Sprintf("facility_id=%d&opens_at=8:00+AM&closes_at=9:30+PM", facilityID)
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/operating-hours/5",
		strings.NewReader(body),
	)
	req.SetPathValue(dayOfWeekParam, "5")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAuthUser(req, facilityID)
	recorder := httptest.NewRecorder()

	HandleOperatingHoursUpdate(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: %d", recorder.Code)
	}

	hours, err := database.Queries.GetFacilityHours(context.Background(), facilityID)
	if err != nil {
		t.Fatalf("fetch hours: %v", err)
	}
	if len(hours) != 1 {
		t.Fatalf("expected 1 hour row, got %d", len(hours))
	}
	if hours[0].DayOfWeek != 5 {
		t.Fatalf("day: %d", hours[0].DayOfWeek)
	}
	if formatTimeValue(hours[0].OpensAt) != "08:00" {
		t.Fatalf("opens_at: %s", formatTimeValue(hours[0].OpensAt))
	}
	if formatTimeValue(hours[0].ClosesAt) != "21:30" {
		t.Fatalf("closes_at: %s", formatTimeValue(hours[0].ClosesAt))
	}
}

func TestHandleOperatingHoursUpdate_DeleteClosed(t *testing.T) {
	database, facilityID := setupOperatingHoursTest(t)

	_, err := database.Queries.UpsertOperatingHours(context.Background(), dbgen.UpsertOperatingHoursParams{
		FacilityID: facilityID,
		DayOfWeek:  1,
		OpensAt:    "08:00",
		ClosesAt:   "20:00",
	})
	if err != nil {
		t.Fatalf("seed hours: %v", err)
	}

	body := fmt.Sprintf("facility_id=%d&is_closed=true", facilityID)
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/operating-hours/1",
		strings.NewReader(body),
	)
	req.SetPathValue(dayOfWeekParam, "1")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAuthUser(req, facilityID)
	recorder := httptest.NewRecorder()

	HandleOperatingHoursUpdate(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: %d", recorder.Code)
	}

	hours, err := database.Queries.GetFacilityHours(context.Background(), facilityID)
	if err != nil {
		t.Fatalf("fetch hours: %v", err)
	}
	if len(hours) != 0 {
		t.Fatalf("expected 0 hour rows, got %d", len(hours))
	}
}
