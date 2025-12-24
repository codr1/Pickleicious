// internal/models/theme.go
package models

import "time"

type ThemeColors struct {
	Background        string `json:"background"`
	Foreground        string `json:"foreground"`
	Primary           string `json:"primary"`
	PrimaryForeground string `json:"primaryForeground"`
	Secondary         string `json:"secondary"`
	Border            string `json:"border"`
	// ... other color definitions
}

type Theme struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Colors      ThemeColors `json:"colors"`
	IsBuiltIn   bool        `json:"isBuiltIn"`
	CreatedBy   string      `json:"createdBy,omitempty"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
}
