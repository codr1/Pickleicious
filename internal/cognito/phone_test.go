package cognito

import "testing"

func TestIsPhoneNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid phone numbers (using real area codes, not 555)
		{"10 digits", "3024422842", true},
		{"10 digits with dashes", "302-442-2842", true},
		{"10 digits with parens", "(302) 442-2842", true},
		{"10 digits with dots", "302.442.2842", true},
		{"11 digits with leading 1", "13024422842", true},
		{"E.164 format", "+13024422842", true},
		{"E.164 with spaces", "+1 302 442 2842", true},

		// Invalid - emails
		{"simple email", "user@example.com", false},
		{"email with plus", "+user@example.com", false},
		{"email with plus tag", "user+tag@example.com", false},
		{"numeric local part email", "3024422842@carrier.com", false},
		{"email with digits", "user123@example.com", false},

		// Invalid - too short or invalid
		{"empty string", "", false},
		{"single digit", "5", false},
		{"9 digits", "302442284", false},

		// Invalid - not phone number format
		{"letters only", "abcdefghij", false},
		{"mixed but few digits", "abc123def", false},
		{"garbage with 10 digits", "abc1234567890xyz", false},
		{"letters mixed in", "302abc4422842", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPhoneNumber(tt.input)
			if got != tt.expected {
				t.Errorf("IsPhoneNumber(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// 10 digit US numbers -> +1 prefix
		{"10 digits plain", "3024422842", "+13024422842"},
		{"10 digits with dashes", "302-442-2842", "+13024422842"},
		{"10 digits with parens", "(302) 442-2842", "+13024422842"},
		{"10 digits with dots", "302.442.2842", "+13024422842"},
		{"10 digits with spaces", "302 442 2842", "+13024422842"},

		// 11 digit US numbers starting with 1
		{"11 digits with 1", "13024422842", "+13024422842"},
		{"11 digits formatted", "1-302-442-2842", "+13024422842"},

		// Already E.164
		{"E.164 format", "+13024422842", "+13024422842"},
		{"E.164 with spaces", "+1 302 442 2842", "+13024422842"},
		{"E.164 with dashes", "+1-302-442-2842", "+13024422842"},

		// International numbers
		{"UK mobile", "+447911123456", "+447911123456"},
		{"German number", "+4915123456789", "+4915123456789"},

		// Invalid - too short or malformed
		{"empty string", "", ""},
		{"9 digits", "302442284", ""},
		{"few digits", "123", ""},
		{"invalid format", "not-a-phone", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizePhone(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizePhone(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
