package db

import (
	"strings"
	"testing"
)

func TestParseThemesFile(t *testing.T) {
	themes, err := ParseThemesFile()
	if err != nil {
		t.Fatalf("ParseThemesFile() error = %v", err)
	}

	// assets/themes currently defines 34 themes.
	if len(themes) != 34 {
		t.Fatalf("ParseThemesFile() theme count = %d, want 34", len(themes))
	}

	foundDefault := false
	for _, theme := range themes {
		if err := theme.Validate(); err != nil {
			t.Fatalf("theme %q failed validation: %v", theme.Name, err)
		}
		if strings.HasSuffix(theme.Name, " DEFAULT") {
			t.Fatalf("theme name still contains DEFAULT suffix: %q", theme.Name)
		}
		if theme.Name == "Simple" {
			foundDefault = true
		}
	}

	if !foundDefault {
		t.Fatalf("expected default theme named Simple")
	}

	if DefaultSystemThemeName() != "Simple" {
		t.Fatalf("default system theme name = %q, want Simple", DefaultSystemThemeName())
	}
}
