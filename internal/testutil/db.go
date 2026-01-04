package testutil

import (
	"path/filepath"
	"testing"

	"github.com/codr1/Pickleicious/internal/db"
)

// NewTestDB creates a temporary SQLite database with migrations applied.
func NewTestDB(t *testing.T) *db.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	return database
}
