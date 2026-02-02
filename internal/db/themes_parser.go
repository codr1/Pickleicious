package db

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/codr1/Pickleicious/assets"
	"github.com/codr1/Pickleicious/internal/models"
)

const defaultThemeSuffix = " DEFAULT"

var defaultSystemThemeName string

func DefaultSystemThemeName() string {
	return defaultSystemThemeName
}

// ParseThemesFile reads assets/themes and returns system themes in order.
func ParseThemesFile() ([]models.Theme, error) {
	file, err := assets.ThemesFS.Open(assets.ThemesPath)
	if err != nil {
		return nil, fmt.Errorf("open embedded themes file: %w", err)
	}
	defer file.Close()

	lines, err := readNonEmptyLines(file)
	if err != nil {
		return nil, err
	}
	if len(lines)%6 != 0 {
		return nil, fmt.Errorf("themes file has %d non-empty lines, expected multiples of 6", len(lines))
	}

	themes := make([]models.Theme, 0, len(lines)/6)
	defaultName := ""
	for i := 0; i < len(lines); i += 6 {
		name := strings.TrimSpace(lines[i])
		if name == "" {
			return nil, fmt.Errorf("theme name missing at line %d", i+1)
		}

		if strings.HasSuffix(name, defaultThemeSuffix) {
			name = strings.TrimSpace(strings.TrimSuffix(name, defaultThemeSuffix))
			if name == "" {
				return nil, fmt.Errorf("theme name missing before DEFAULT at line %d", i+1)
			}
			if defaultName != "" {
				return nil, fmt.Errorf("multiple DEFAULT themes: %q and %q", defaultName, name)
			}
			defaultName = name
		}

		theme := models.Theme{
			Name:           name,
			IsSystem:       true,
			PrimaryColor:   lines[i+1],
			SecondaryColor: lines[i+2],
			TertiaryColor:  lines[i+3],
			AccentColor:    lines[i+4],
			HighlightColor: lines[i+5],
		}

		if err := theme.Validate(); err != nil {
			return nil, fmt.Errorf("invalid theme %q: %w", name, err)
		}

		themes = append(themes, theme)
	}

	defaultSystemThemeName = defaultName
	return themes, nil
}

func readNonEmptyLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	lines := []string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read themes file: %w", err)
	}
	return lines, nil
}
