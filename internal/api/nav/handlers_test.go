package nav

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

func TestHandleSearch(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "search.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	InitHandlers(database.Queries)

	ctx := context.Background()
	_, err = database.Queries.CreateMember(ctx, dbgen.CreateMemberParams{
		FirstName:       "Alice",
		LastName:        "Smith",
		Email:           sql.NullString{String: "alice@example.com", Valid: true},
		Phone:           sql.NullString{},
		StreetAddress:   sql.NullString{},
		City:            sql.NullString{},
		State:           sql.NullString{},
		PostalCode:      sql.NullString{},
		Status:          "active",
		DateOfBirth:     time.Date(1990, time.January, 2, 0, 0, 0, 0, time.UTC),
		WaiverSigned:    true,
		MembershipLevel: 0,
	})
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}

	_, err = database.Queries.CreateMember(ctx, dbgen.CreateMemberParams{
		FirstName:       "Bob",
		LastName:        "Jones",
		Email:           sql.NullString{String: "bob@example.com", Valid: true},
		Phone:           sql.NullString{},
		StreetAddress:   sql.NullString{},
		City:            sql.NullString{},
		State:           sql.NullString{},
		PostalCode:      sql.NullString{},
		Status:          "active",
		DateOfBirth:     time.Date(1987, time.March, 5, 0, 0, 0, 0, time.UTC),
		WaiverSigned:    true,
		MembershipLevel: 0,
	})
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nav/search?q=Ali", nil)
	recorder := httptest.NewRecorder()

	HandleSearch(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "Alice") {
		t.Fatalf("expected search results to include Alice, got: %s", body)
	}
	if strings.Contains(body, "Bob") {
		t.Fatalf("expected search results to exclude Bob, got: %s", body)
	}
}
