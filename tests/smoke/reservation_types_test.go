//go:build smoke

package smoke

import (
	"testing"

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
