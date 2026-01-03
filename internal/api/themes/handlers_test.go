package themes

// NOTE: Tests cannot use t.Parallel() due to shared package state.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
)

type mockThemeQueries struct {
	mu             sync.Mutex
	nextID         int64
	themes         map[int64]dbgen.Theme
	activeThemeIDs map[int64]int64
}

func newMockThemeQueries() *mockThemeQueries {
	return &mockThemeQueries{
		nextID:         1,
		themes:         make(map[int64]dbgen.Theme),
		activeThemeIDs: make(map[int64]int64),
	}
}

func (m *mockThemeQueries) addTheme(theme dbgen.Theme) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	if theme.ID == 0 {
		theme.ID = m.nextID
		m.nextID++
	}
	if theme.CreatedAt.IsZero() {
		theme.CreatedAt = time.Now().UTC()
	}
	if theme.UpdatedAt.IsZero() {
		theme.UpdatedAt = theme.CreatedAt
	}
	m.themes[theme.ID] = theme
	return theme.ID
}

func (m *mockThemeQueries) CountFacilityThemeName(ctx context.Context, arg dbgen.CountFacilityThemeNameParams) (int64, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	if !arg.FacilityID.Valid {
		return 0, nil
	}
	var count int64
	for _, theme := range m.themes {
		if theme.FacilityID.Valid && theme.FacilityID.Int64 == arg.FacilityID.Int64 && theme.Name == arg.Name {
			count++
		}
	}
	return count, nil
}

func (m *mockThemeQueries) CountFacilityThemeNameExcludingID(ctx context.Context, arg dbgen.CountFacilityThemeNameExcludingIDParams) (int64, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	if !arg.FacilityID.Valid {
		return 0, nil
	}
	var count int64
	for _, theme := range m.themes {
		if theme.ID == arg.ID {
			continue
		}
		if theme.FacilityID.Valid && theme.FacilityID.Int64 == arg.FacilityID.Int64 && theme.Name == arg.Name {
			count++
		}
	}
	return count, nil
}

func (m *mockThemeQueries) CountFacilityThemes(ctx context.Context, facilityID sql.NullInt64) (int64, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	if !facilityID.Valid {
		return 0, nil
	}
	var count int64
	for _, theme := range m.themes {
		if theme.FacilityID.Valid && theme.FacilityID.Int64 == facilityID.Int64 {
			count++
		}
	}
	return count, nil
}

func (m *mockThemeQueries) CountThemeUsage(ctx context.Context, themeID sql.NullInt64) (int64, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	if !themeID.Valid {
		return 0, nil
	}
	for _, activeID := range m.activeThemeIDs {
		if activeID == themeID.Int64 {
			return 1, nil
		}
	}
	return 0, nil
}

func (m *mockThemeQueries) CreateTheme(ctx context.Context, arg dbgen.CreateThemeParams) (dbgen.Theme, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	theme := dbgen.Theme{
		ID:             m.nextID,
		FacilityID:     arg.FacilityID,
		Name:           arg.Name,
		IsSystem:       arg.IsSystem,
		PrimaryColor:   arg.PrimaryColor,
		SecondaryColor: arg.SecondaryColor,
		TertiaryColor:  arg.TertiaryColor,
		AccentColor:    arg.AccentColor,
		HighlightColor: arg.HighlightColor,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	m.nextID++
	m.themes[theme.ID] = theme
	return theme, nil
}

func (m *mockThemeQueries) DeleteTheme(ctx context.Context, id int64) (int64, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.themes[id]; !ok {
		return 0, nil
	}
	delete(m.themes, id)
	return 1, nil
}

func (m *mockThemeQueries) GetActiveThemeID(ctx context.Context, facilityID int64) (int64, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	if themeID, ok := m.activeThemeIDs[facilityID]; ok {
		return themeID, nil
	}
	return 0, sql.ErrNoRows
}

func (m *mockThemeQueries) GetTheme(ctx context.Context, id int64) (dbgen.Theme, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	theme, ok := m.themes[id]
	if !ok {
		return dbgen.Theme{}, sql.ErrNoRows
	}
	return theme, nil
}

func (m *mockThemeQueries) ListFacilityThemes(ctx context.Context, facilityID sql.NullInt64) ([]dbgen.Theme, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	if !facilityID.Valid {
		return nil, nil
	}
	var themes []dbgen.Theme
	for _, theme := range m.themes {
		if theme.FacilityID.Valid && theme.FacilityID.Int64 == facilityID.Int64 {
			themes = append(themes, theme)
		}
	}
	sort.Slice(themes, func(i, j int) bool {
		return themes[i].Name < themes[j].Name
	})
	return themes, nil
}

func (m *mockThemeQueries) ListSystemThemes(ctx context.Context) ([]dbgen.Theme, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	var themes []dbgen.Theme
	for _, theme := range m.themes {
		if !theme.FacilityID.Valid {
			themes = append(themes, theme)
		}
	}
	sort.Slice(themes, func(i, j int) bool {
		return themes[i].Name < themes[j].Name
	})
	return themes, nil
}

func (m *mockThemeQueries) UpdateTheme(ctx context.Context, arg dbgen.UpdateThemeParams) (dbgen.Theme, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	theme, ok := m.themes[arg.ID]
	if !ok {
		return dbgen.Theme{}, sql.ErrNoRows
	}
	theme.Name = arg.Name
	theme.PrimaryColor = arg.PrimaryColor
	theme.SecondaryColor = arg.SecondaryColor
	theme.TertiaryColor = arg.TertiaryColor
	theme.AccentColor = arg.AccentColor
	theme.HighlightColor = arg.HighlightColor
	theme.UpdatedAt = time.Now().UTC()
	m.themes[arg.ID] = theme
	return theme, nil
}

func (m *mockThemeQueries) UpsertActiveThemeID(ctx context.Context, arg dbgen.UpsertActiveThemeIDParams) (int64, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()

	if arg.ActiveThemeID.Valid {
		m.activeThemeIDs[arg.FacilityID] = arg.ActiveThemeID.Int64
	} else {
		delete(m.activeThemeIDs, arg.FacilityID)
	}
	return 1, nil
}

func setupThemeHandlers(t *testing.T) *mockThemeQueries {
	t.Helper()

	mock := newMockThemeQueries()
	queries = mock
	t.Cleanup(func() {
		queries = nil
		queriesOnce = sync.Once{}
	})
	return mock
}

func TestCreateTheme_ValidInput(t *testing.T) {
	setupThemeHandlers(t)

	payload, err := json.Marshal(map[string]any{
		"facilityId":     42,
		"isSystem":       false,
		"name":           "Club Classic",
		"primaryColor":   "#1f2937",
		"secondaryColor": "#e5e7eb",
		"tertiaryColor":  "#f9fafb",
		"accentColor":    "#2563eb",
		"highlightColor": "#16a34a",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/themes", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	HandleThemeCreate(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status: %d", recorder.Code)
	}

	var resp models.Theme
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Name != "Club Classic" {
		t.Fatalf("unexpected name: %s", resp.Name)
	}
	if resp.FacilityID == nil || *resp.FacilityID != 42 {
		t.Fatalf("unexpected facility id: %#v", resp.FacilityID)
	}
}

func TestCreateTheme_InvalidWCAG(t *testing.T) {
	setupThemeHandlers(t)

	payload, err := json.Marshal(map[string]any{
		"facilityId":     42,
		"isSystem":       false,
		"name":           "Low Contrast",
		"primaryColor":   "#12345G",
		"secondaryColor": "#e5e7eb",
		"tertiaryColor":  "#f9fafb",
		"accentColor":    "#2563eb",
		"highlightColor": "#16a34a",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/themes", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	HandleThemeCreate(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status: %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "primary_color must be a 6-digit hex color") {
		t.Fatalf("unexpected error: %s", recorder.Body.String())
	}
}

func TestGetTheme_NotFound(t *testing.T) {
	setupThemeHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/themes/999?facility_id=42", nil)
	req.SetPathValue("id", "999")
	recorder := httptest.NewRecorder()

	HandleThemeDetail(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status: %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Theme not found") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestUpdateTheme_Success(t *testing.T) {
	mock := setupThemeHandlers(t)
	themeID := mock.addTheme(dbgen.Theme{
		FacilityID:     sql.NullInt64{Int64: 77, Valid: true},
		Name:           "Original",
		IsSystem:       false,
		PrimaryColor:   "#1f2937",
		SecondaryColor: "#e5e7eb",
		TertiaryColor:  "#f9fafb",
		AccentColor:    "#2563eb",
		HighlightColor: "#16a34a",
	})

	payload, err := json.Marshal(map[string]any{
		"name":           "Updated Theme",
		"primaryColor":   "#1f2937",
		"secondaryColor": "#e5e7eb",
		"tertiaryColor":  "#f9fafb",
		"accentColor":    "#2563eb",
		"highlightColor": "#16a34a",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	themeIDStr := strconv.FormatInt(themeID, 10)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/themes/"+themeIDStr, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", themeIDStr)
	recorder := httptest.NewRecorder()

	HandleThemeUpdate(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: %d", recorder.Code)
	}

	var resp models.Theme
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != themeID || resp.Name != "Updated Theme" {
		t.Fatalf("unexpected update response: %+v", resp)
	}
}

func TestDeleteTheme_Success(t *testing.T) {
	mock := setupThemeHandlers(t)
	themeID := mock.addTheme(dbgen.Theme{
		FacilityID:     sql.NullInt64{Int64: 77, Valid: true},
		Name:           "Disposable",
		IsSystem:       false,
		PrimaryColor:   "#1f2937",
		SecondaryColor: "#e5e7eb",
		TertiaryColor:  "#f9fafb",
		AccentColor:    "#2563eb",
		HighlightColor: "#16a34a",
	})

	themeIDStr := strconv.FormatInt(themeID, 10)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/themes/"+themeIDStr, nil)
	req.SetPathValue("id", themeIDStr)
	recorder := httptest.NewRecorder()

	HandleThemeDelete(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status: %d", recorder.Code)
	}
	if _, ok := mock.themes[themeID]; ok {
		t.Fatalf("expected theme to be deleted")
	}
}

func TestListThemes_ReturnsAllThemes(t *testing.T) {
	mock := setupThemeHandlers(t)
	mock.addTheme(dbgen.Theme{
		Name:           "Base System",
		IsSystem:       true,
		PrimaryColor:   "#1f2937",
		SecondaryColor: "#e5e7eb",
		TertiaryColor:  "#f9fafb",
		AccentColor:    "#2563eb",
		HighlightColor: "#16a34a",
	})
	mock.addTheme(dbgen.Theme{
		Name:           "Alt System",
		IsSystem:       true,
		PrimaryColor:   "#0f172a",
		SecondaryColor: "#e2e8f0",
		TertiaryColor:  "#f8fafc",
		AccentColor:    "#38bdf8",
		HighlightColor: "#22c55e",
	})
	mock.addTheme(dbgen.Theme{
		FacilityID:     sql.NullInt64{Int64: 5, Valid: true},
		Name:           "Facility One",
		IsSystem:       false,
		PrimaryColor:   "#1f2937",
		SecondaryColor: "#e5e7eb",
		TertiaryColor:  "#f9fafb",
		AccentColor:    "#2563eb",
		HighlightColor: "#16a34a",
	})
	mock.addTheme(dbgen.Theme{
		FacilityID:     sql.NullInt64{Int64: 5, Valid: true},
		Name:           "Facility Two",
		IsSystem:       false,
		PrimaryColor:   "#111827",
		SecondaryColor: "#d1d5db",
		TertiaryColor:  "#f3f4f6",
		AccentColor:    "#0ea5e9",
		HighlightColor: "#15803d",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/themes?facility_id=5", nil)
	recorder := httptest.NewRecorder()

	HandleThemesList(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status: %d", recorder.Code)
	}

	var resp struct {
		Themes []models.Theme `json:"themes"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Themes) != 4 {
		t.Fatalf("unexpected themes count: %d", len(resp.Themes))
	}

	names := map[string]bool{}
	for _, theme := range resp.Themes {
		names[theme.Name] = true
	}
	for _, expected := range []string{"Base System", "Alt System", "Facility One", "Facility Two"} {
		if !names[expected] {
			t.Fatalf("missing theme %q", expected)
		}
	}
}
