package models

import "testing"

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
