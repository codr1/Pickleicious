//go:build smoke

package smoke

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	dbpkg "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	"github.com/codr1/Pickleicious/internal/testutil"
)

func TestSystemThemesSeeded(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	expectedThemes, err := dbpkg.ParseThemesFile()
	if err != nil {
		t.Fatalf("parse themes file: %v", err)
	}
	if len(expectedThemes) < 30 {
		t.Fatalf("expected at least 30 system themes, got %d", len(expectedThemes))
	}

	expectedByName := make(map[string]models.Theme, len(expectedThemes))
	missing := make(map[string]struct{}, len(expectedThemes))
	for _, theme := range expectedThemes {
		expectedByName[theme.Name] = theme
		missing[theme.Name] = struct{}{}
	}

	queries := dbgen.New(db)
	rows, err := queries.ListSystemThemes(ctx)
	if err != nil {
		t.Fatalf("list system themes: %v", err)
	}
	if len(rows) != len(expectedThemes) {
		t.Fatalf("system themes count mismatch: got %d want %d", len(rows), len(expectedThemes))
	}

	for _, row := range rows {
		expected, ok := expectedByName[row.Name]
		if !ok {
			t.Fatalf("unexpected system theme %q", row.Name)
		}

		if row.PrimaryColor != expected.PrimaryColor {
			t.Fatalf("theme %q primary color mismatch: got %q want %q", row.Name, row.PrimaryColor, expected.PrimaryColor)
		}
		if row.SecondaryColor != expected.SecondaryColor {
			t.Fatalf("theme %q secondary color mismatch: got %q want %q", row.Name, row.SecondaryColor, expected.SecondaryColor)
		}
		if row.TertiaryColor != expected.TertiaryColor {
			t.Fatalf("theme %q tertiary color mismatch: got %q want %q", row.Name, row.TertiaryColor, expected.TertiaryColor)
		}
		if row.AccentColor != expected.AccentColor {
			t.Fatalf("theme %q accent color mismatch: got %q want %q", row.Name, row.AccentColor, expected.AccentColor)
		}
		if row.HighlightColor != expected.HighlightColor {
			t.Fatalf("theme %q highlight color mismatch: got %q want %q", row.Name, row.HighlightColor, expected.HighlightColor)
		}

		if err := models.ThemeFromDB(row).Validate(); err != nil {
			t.Fatalf("seeded theme %q failed validation: %v", row.Name, err)
		}

		delete(missing, row.Name)
	}

	for name := range missing {
		t.Fatalf("missing system theme %q", name)
	}

	defaultName := dbpkg.DefaultSystemThemeName()
	if defaultName == "" {
		t.Fatalf("default system theme name is empty")
	}

	var defaultCount int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM themes WHERE facility_id IS NULL AND name = ?",
		defaultName,
	).Scan(&defaultCount); err != nil {
		t.Fatalf("count default system theme: %v", err)
	}
	if defaultCount != 1 {
		t.Fatalf("default system theme %q count mismatch: got %d want %d", defaultName, defaultCount, 1)
	}
}

func TestSystemThemesSeedIdempotent(t *testing.T) {
	db := testutil.NewTestDB(t)

	expectedThemes, err := dbpkg.ParseThemesFile()
	if err != nil {
		t.Fatalf("parse themes file: %v", err)
	}

	repoRoot := findRepoRoot(t)

	seedSQLPath := filepath.Join(repoRoot, "internal", "db", "migrations", "000045_seed_system_themes.up.sql")
	seedSQL, err := os.ReadFile(seedSQLPath)
	if err != nil {
		t.Fatalf("read system themes migration: %v", err)
	}

	if _, err := db.Exec(string(seedSQL)); err != nil {
		t.Fatalf("reseed system themes: %v", err)
	}

	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM themes WHERE facility_id IS NULL",
	).Scan(&count); err != nil {
		t.Fatalf("count system themes: %v", err)
	}
	if count != len(expectedThemes) {
		t.Fatalf("system themes count mismatch after reseed: got %d want %d", count, len(expectedThemes))
	}

	var distinct int
	if err := db.QueryRow(
		"SELECT COUNT(DISTINCT name) FROM themes WHERE facility_id IS NULL",
	).Scan(&distinct); err != nil {
		t.Fatalf("count distinct system theme names: %v", err)
	}
	if distinct != count {
		t.Fatalf("system themes name duplication detected: %d total, %d distinct", count, distinct)
	}
}
