package assets

import "embed"

//go:embed themes
var ThemesFS embed.FS

const ThemesPath = "themes"
