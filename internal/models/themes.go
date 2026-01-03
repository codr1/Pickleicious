// internal/models/themes.go
package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

// Theme colors often back larger UI elements, not body text, so we use the AA large-text threshold.
const wcagAAMinContrastRatio = 3.0
const wcagAAContrastNote = "WCAG AA for large text/UI components"
const maxThemeNameLength = 100
const darkTextColor = "#000000"
const lightTextColor = "#FFFFFF"
const defaultThemePrimary = "#1f2937"
const defaultThemeSecondary = "#e5e7eb"
const defaultThemeTertiary = "#f9fafb"
const defaultThemeAccent = "#2563eb"
const defaultThemeHighlight = "#16a34a"

var hexColorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var themeNameRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ()-]*$`)

func IsHexColor(value string) bool {
	return hexColorRegex.MatchString(strings.TrimSpace(value))
}

type Theme struct {
	ID             int64     `json:"id"`
	FacilityID     *int64    `json:"facilityId,omitempty"`
	Name           string    `json:"name"`
	IsSystem       bool      `json:"isSystem"`
	PrimaryColor   string    `json:"primaryColor"`
	SecondaryColor string    `json:"secondaryColor"`
	TertiaryColor  string    `json:"tertiaryColor"`
	AccentColor    string    `json:"accentColor"`
	HighlightColor string    `json:"highlightColor"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type ThemeQueries interface {
	ListSystemThemes(ctx context.Context) ([]dbgen.Theme, error)
	ListFacilityThemes(ctx context.Context, facilityID sql.NullInt64) ([]dbgen.Theme, error)
	GetActiveThemeID(ctx context.Context, facilityID int64) (int64, error)
	GetTheme(ctx context.Context, id int64) (dbgen.Theme, error)
}

func DefaultTheme() Theme {
	return Theme{
		Name:           "",
		IsSystem:       false,
		PrimaryColor:   defaultThemePrimary,
		SecondaryColor: defaultThemeSecondary,
		TertiaryColor:  defaultThemeTertiary,
		AccentColor:    defaultThemeAccent,
		HighlightColor: defaultThemeHighlight,
	}
}

func (t Theme) Validate() error {
	trimmedName := strings.TrimSpace(t.Name)
	if trimmedName == "" {
		return fmt.Errorf("name is required")
	}
	if trimmedName != t.Name {
		return fmt.Errorf("name must not have leading or trailing whitespace")
	}
	if len(trimmedName) > maxThemeNameLength {
		return fmt.Errorf("name must be %d characters or fewer", maxThemeNameLength)
	}
	if !themeNameRegex.MatchString(trimmedName) {
		return fmt.Errorf("name may only contain letters, numbers, spaces, hyphens, and parentheses")
	}

	if t.IsSystem && t.FacilityID != nil {
		return fmt.Errorf("system themes must have facility_id set to NULL")
	}
	if !t.IsSystem && t.FacilityID == nil {
		return fmt.Errorf("facility themes must have facility_id set")
	}

	colorFields := map[string]string{
		"primary_color":   t.PrimaryColor,
		"secondary_color": t.SecondaryColor,
		"tertiary_color":  t.TertiaryColor,
		"accent_color":    t.AccentColor,
		"highlight_color": t.HighlightColor,
	}

	for name, value := range colorFields {
		if !hexColorRegex.MatchString(value) {
			return fmt.Errorf("%s must be a 6-digit hex color like #AABBCC", name)
		}
		if err := validateTextContrast(name, value); err != nil {
			return err
		}
	}

	return nil
}

func GetSystemThemes(ctx context.Context, queries ThemeQueries) ([]Theme, error) {
	rows, err := queries.ListSystemThemes(ctx)
	if err != nil {
		return nil, err
	}
	return themesFromDB(rows), nil
}

func GetFacilityThemes(ctx context.Context, queries ThemeQueries, facilityID int64) ([]Theme, error) {
	rows, err := queries.ListFacilityThemes(ctx, sql.NullInt64{
		Int64: facilityID,
		Valid: true,
	})
	if err != nil {
		return nil, err
	}
	return themesFromDB(rows), nil
}

func GetActiveTheme(ctx context.Context, queries ThemeQueries, facilityID int64) (*Theme, error) {
	activeThemeID, err := queries.GetActiveThemeID(ctx, facilityID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if activeThemeID <= 0 {
		return nil, nil
	}
	row, err := queries.GetTheme(ctx, activeThemeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	theme := ThemeFromDB(row)
	return &theme, nil
}

func themesFromDB(rows []dbgen.Theme) []Theme {
	results := make([]Theme, 0, len(rows))
	for _, row := range rows {
		results = append(results, ThemeFromDB(row))
	}
	return results
}

func ThemeFromDB(row dbgen.Theme) Theme {
	var facilityID *int64
	if row.FacilityID.Valid {
		id := row.FacilityID.Int64
		facilityID = &id
	}
	return Theme{
		ID:             row.ID,
		FacilityID:     facilityID,
		Name:           row.Name,
		IsSystem:       row.IsSystem,
		PrimaryColor:   row.PrimaryColor,
		SecondaryColor: row.SecondaryColor,
		TertiaryColor:  row.TertiaryColor,
		AccentColor:    row.AccentColor,
		HighlightColor: row.HighlightColor,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func validateTextContrast(colorName, backgroundColor string) error {
	textColors := []string{darkTextColor, lightTextColor}
	bestRatio := 0.0
	bestText := ""
	for _, textColor := range textColors {
		ratio, err := contrastRatio(textColor, backgroundColor)
		if err != nil {
			return err
		}
		if ratio > bestRatio {
			bestRatio = ratio
			bestText = textColor
		}
	}
	if bestRatio < wcagAAMinContrastRatio {
		return fmt.Errorf(
			"%s must have contrast ratio >= %.1f with #000000 or #FFFFFF text (%s); best is %s at %.2f",
			colorName,
			wcagAAMinContrastRatio,
			wcagAAContrastNote,
			bestText,
			bestRatio,
		)
	}
	return nil
}

func contrastRatio(textColor, backgroundColor string) (float64, error) {
	textL, err := relativeLuminance(textColor)
	if err != nil {
		return 0, err
	}
	backgroundL, err := relativeLuminance(backgroundColor)
	if err != nil {
		return 0, err
	}
	lightest := math.Max(textL, backgroundL)
	darkest := math.Min(textL, backgroundL)
	return (lightest + 0.05) / (darkest + 0.05), nil
}

func relativeLuminance(hexColor string) (float64, error) {
	r, g, b, err := parseHexColor(hexColor)
	if err != nil {
		return 0, err
	}

	rl := srgbToLinear(r)
	gl := srgbToLinear(g)
	bl := srgbToLinear(b)

	return 0.2126*rl + 0.7152*gl + 0.0722*bl, nil
}

func parseHexColor(hexColor string) (float64, float64, float64, error) {
	if !hexColorRegex.MatchString(hexColor) {
		return 0, 0, 0, fmt.Errorf("invalid hex color: %s", hexColor)
	}

	hex := strings.TrimPrefix(hexColor, "#")
	value, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid hex color: %s", hexColor)
	}

	r := float64((value >> 16) & 0xFF)
	g := float64((value >> 8) & 0xFF)
	b := float64(value & 0xFF)

	return r / 255, g / 255, b / 255, nil
}

func srgbToLinear(value float64) float64 {
	if value <= 0.03928 {
		return value / 12.92
	}
	return math.Pow((value+0.055)/1.055, 2.4)
}
