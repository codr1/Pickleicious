package layouts

import (
	"fmt"
	"strings"

	"github.com/codr1/Pickleicious/internal/models"
)

func getThemeCssVars(theme *models.Theme) string {
	defaultTheme := models.DefaultTheme()
	primary := defaultTheme.PrimaryColor
	secondary := defaultTheme.SecondaryColor
	tertiary := defaultTheme.TertiaryColor
	accent := defaultTheme.AccentColor
	highlight := defaultTheme.HighlightColor

	if theme != nil {
		primary = themeColorOrDefault(theme.PrimaryColor, primary)
		secondary = themeColorOrDefault(theme.SecondaryColor, secondary)
		tertiary = themeColorOrDefault(theme.TertiaryColor, tertiary)
		accent = themeColorOrDefault(theme.AccentColor, accent)
		highlight = themeColorOrDefault(theme.HighlightColor, highlight)
	}

	return fmt.Sprintf(
		":root{--theme-primary:%s;--theme-secondary:%s;--theme-tertiary:%s;--theme-accent:%s;--theme-highlight:%s;}",
		primary,
		secondary,
		tertiary,
		accent,
		highlight,
	)
}

func themeColorOrDefault(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	if !models.IsHexColor(trimmed) {
		return fallback
	}
	return trimmed
}
