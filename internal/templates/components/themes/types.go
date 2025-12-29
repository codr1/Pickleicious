package themes

import "github.com/codr1/Pickleicious/internal/models"

type Theme struct {
	models.Theme
	IsActive bool
}

type ThemeEditorData struct {
	Theme      Theme
	FacilityID int64
	ReadOnly   bool
}

func NewTheme(theme models.Theme, activeThemeID int64) Theme {
	return Theme{
		Theme:    theme,
		IsActive: theme.ID != 0 && theme.ID == activeThemeID,
	}
}

func NewThemes(rows []models.Theme, activeThemeID int64) []Theme {
	themes := make([]Theme, len(rows))
	for i, row := range rows {
		themes[i] = NewTheme(row, activeThemeID)
	}
	return themes
}
