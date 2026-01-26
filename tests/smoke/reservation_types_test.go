//go:build smoke

package smoke

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/testutil"
)

func TestSystemReservationTypesSeeded(t *testing.T) {
	db := testutil.NewTestDB(t)

	expected := map[string]struct {
		description string
		color       string
	}{
		"OPEN_PLAY":   {description: "Open play session", color: "#2E7D32"},
		"GAME":        {description: "Standard game reservation", color: "#1976D2"},
		"PRO_SESSION": {description: "Pro-led session", color: "#6A1B9A"},
		"EVENT":       {description: "Special event booking", color: "#F57C00"},
		"MAINTENANCE": {description: "Maintenance block", color: "#546E7A"},
		"LEAGUE":      {description: "League play", color: "#C62828"},
		"LESSON":      {description: "Lesson session", color: "#00897B"},
		"TOURNAMENT":  {description: "Tournament play", color: "#5E35B1"},
		"CLINIC":      {description: "Clinic session", color: "#8D6E63"},
	}

	rows, err := db.Query("SELECT name, description, color FROM reservation_types")
	if err != nil {
		t.Fatalf("query reservation_types: %v", err)
	}
	defer rows.Close()

	found := make(map[string]struct{}, len(expected))
	for rows.Next() {
		var name string
		var description string
		var color string
		if err := rows.Scan(&name, &description, &color); err != nil {
			t.Fatalf("scan reservation_types row: %v", err)
		}

		exp, ok := expected[name]
		if !ok {
			continue
		}

		if description != exp.description {
			t.Fatalf("reservation type %q description mismatch: got %q want %q", name, description, exp.description)
		}
		if color != exp.color {
			t.Fatalf("reservation type %q color mismatch: got %q want %q", name, color, exp.color)
		}
		found[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate reservation_types: %v", err)
	}

	for name := range expected {
		if _, ok := found[name]; !ok {
			t.Fatalf("missing seeded reservation type %q", name)
		}
	}
}

func TestMigrationIdempotency(t *testing.T) {
	db := testutil.NewTestDB(t)

	const seedInsert = `
INSERT INTO reservation_types (name, description, color)
VALUES
    ('OPEN_PLAY', 'Open play session', '#2E7D32'),
    ('GAME', 'Standard game reservation', '#1976D2'),
    ('PRO_SESSION', 'Pro-led session', '#6A1B9A'),
    ('EVENT', 'Special event booking', '#F57C00'),
    ('MAINTENANCE', 'Maintenance block', '#546E7A'),
    ('LEAGUE', 'League play', '#C62828'),
    ('LESSON', 'Lesson session', '#00897B'),
    ('TOURNAMENT', 'Tournament play', '#5E35B1'),
    ('CLINIC', 'Clinic session', '#8D6E63')
ON CONFLICT(name) DO UPDATE
SET description = excluded.description,
    color = excluded.color;
`

	_, err := db.Exec("UPDATE reservation_types SET description = ?, color = ? WHERE name = ?", "Stale description", "#000000", "GAME")
	if err != nil {
		t.Fatalf("corrupt reservation_types row: %v", err)
	}

	// Re-run the migration insert to verify the upsert restores canonical values.
	_, err = db.Exec(seedInsert)
	if err != nil {
		t.Fatalf("reseed reservation_types: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM reservation_types").Scan(&count); err != nil {
		t.Fatalf("count reservation_types: %v", err)
	}
	if count != 9 {
		t.Fatalf("reservation_types count mismatch: got %d want %d", count, 9)
	}

	var description string
	var color string
	if err := db.QueryRow("SELECT description, color FROM reservation_types WHERE name = ?", "GAME").Scan(&description, &color); err != nil {
		t.Fatalf("fetch reservation_types row: %v", err)
	}
	if description != "Standard game reservation" {
		t.Fatalf("reservation_types GAME description mismatch: got %q want %q", description, "Standard game reservation")
	}
	if color != "#1976D2" {
		t.Fatalf("reservation_types GAME color mismatch: got %q want %q", color, "#1976D2")
	}
}

func TestGetReservationTypeByName(t *testing.T) {
	db := testutil.NewTestDB(t)
	queries := dbgen.New(db)
	ctx := context.Background()

	for _, name := range []string{"GAME", "PRO_SESSION"} {
		resType, err := queries.GetReservationTypeByName(ctx, name)
		if err != nil {
			t.Fatalf("get reservation type %q: %v", name, err)
		}
		if resType.ID == 0 {
			t.Fatalf("reservation type %q has zero ID", name)
		}
		if resType.Name != name {
			t.Fatalf("reservation type %q name mismatch: got %q", name, resType.Name)
		}
		if !resType.Description.Valid || resType.Description.String == "" {
			t.Fatalf("reservation type %q missing description", name)
		}
		if !resType.Color.Valid || resType.Color.String == "" {
			t.Fatalf("reservation type %q missing color", name)
		}
		if resType.CreatedAt.IsZero() || resType.UpdatedAt.IsZero() {
			t.Fatalf("reservation type %q missing timestamps", name)
		}
	}

	_, err := queries.GetReservationTypeByName(ctx, "INVALID_TYPE")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for invalid reservation type, got %v", err)
	}
}
