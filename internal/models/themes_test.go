package models

import (
	"math"
	"strings"
	"testing"
)

func TestIsHexColor(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty", value: "", want: false},
		{name: "whitespace", value: "   ", want: false},
		{name: "missing_hash", value: "AABBCC", want: false},
		{name: "short_hex", value: "#ABC", want: false},
		{name: "long_hex", value: "#AABBCCDD", want: false},
		{name: "invalid_char", value: "#AABBCG", want: false},
		{name: "lowercase_hex", value: "#aabbcc", want: true},
		{name: "uppercase_hex", value: "#AABBCC", want: true},
		{name: "trimmed_hex", value: "  #AABBCC  ", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsHexColor(test.value); got != test.want {
				t.Fatalf("IsHexColor(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestCalculateContrastRatio(t *testing.T) {
	tests := []struct {
		name            string
		textColor       string
		backgroundColor string
		want            float64
		wantErr         bool
		errContains     string
	}{
		{
			name:            "black_on_white",
			textColor:       "#000000",
			backgroundColor: "#FFFFFF",
			want:            21.0,
		},
		{
			name:            "same_colors",
			textColor:       "#336699",
			backgroundColor: "#336699",
			want:            1.0,
		},
		{
			name:            "invalid_text_color",
			textColor:       "#GGGGGG",
			backgroundColor: "#FFFFFF",
			wantErr:         true,
			errContains:     "invalid hex color",
		},
		{
			name:            "invalid_background_color",
			textColor:       "#000000",
			backgroundColor: "#GGGGGG",
			wantErr:         true,
			errContains:     "invalid hex color",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := contrastRatio(test.textColor, test.backgroundColor)
			if test.wantErr {
				if err == nil {
					t.Fatalf("contrastRatio(%q, %q) error = nil", test.textColor, test.backgroundColor)
				}
				if test.errContains != "" && !strings.Contains(err.Error(), test.errContains) {
					t.Fatalf("contrastRatio(%q, %q) error = %v, want %q", test.textColor, test.backgroundColor, err, test.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("contrastRatio(%q, %q) error = %v", test.textColor, test.backgroundColor, err)
			}
			if math.Abs(got-test.want) > 0.0001 {
				t.Fatalf("contrastRatio(%q, %q) = %.4f, want %.4f", test.textColor, test.backgroundColor, got, test.want)
			}
		})
	}
}

func TestValidateTextContrast(t *testing.T) {
	tests := []struct {
		name            string
		backgroundColor string
		wantErr         bool
		errContains     string
	}{
		{name: "white_background", backgroundColor: "#FFFFFF"},
		{name: "black_background", backgroundColor: "#000000"},
		{name: "red_background", backgroundColor: "#FF0000"},
		{
			name:            "invalid_hex_color",
			backgroundColor: "#GGGGGG",
			wantErr:         true,
			errContains:     "invalid hex color",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTextContrast("background", test.backgroundColor)
			if test.wantErr {
				if err == nil {
					t.Fatalf("validateTextContrast(%q) error = nil", test.backgroundColor)
				}
				if test.errContains != "" && !strings.Contains(err.Error(), test.errContains) {
					t.Fatalf("validateTextContrast(%q) error = %v, want %q", test.backgroundColor, err, test.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateTextContrast(%q) error = %v", test.backgroundColor, err)
			}
		})
	}
}
