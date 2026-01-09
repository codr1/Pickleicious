package cognito

import "testing"

func TestIsPhoneNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid phone numbers
		{"10 digits", "5551234567", true},
		{"10 digits with dashes", "555-123-4567", true},
		{"10 digits with parens", "(555) 123-4567", true},
		{"10 digits with dots", "555.123.4567", true},
		{"11 digits with leading 1", "15551234567", true},
		{"E.164 format", "+15551234567", true},
		{"E.164 with spaces", "+1 555 123 4567", true},

		// Invalid - emails
		{"simple email", "user@example.com", false},
		{"email with plus", "+user@example.com", false},
		{"email with plus tag", "user+tag@example.com", false},
		{"numeric local part email", "5551234567@carrier.com", false},
		{"email with digits", "user123@example.com", false},

		// Invalid - too short
		{"empty string", "", false},
		{"single digit", "5", false},
		{"9 digits", "555123456", false},

		// Invalid - not enough digits
		{"letters only", "abcdefghij", false},
		{"mixed but few digits", "abc123def", false},

		// Invalid - contains letters (not valid phone formatting)
		{"garbage with 10 digits", "abc1234567890xyz", false},
		{"letters mixed in", "555abc1234567", false},
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
		{"10 digits plain", "5551234567", "+15551234567"},
		{"10 digits with dashes", "555-123-4567", "+15551234567"},
		{"10 digits with parens", "(555) 123-4567", "+15551234567"},
		{"10 digits with dots", "555.123.4567", "+15551234567"},
		{"10 digits with spaces", "555 123 4567", "+15551234567"},

		// 11 digit US numbers starting with 1
		{"11 digits with 1", "15551234567", "+15551234567"},
		{"11 digits formatted", "1-555-123-4567", "+15551234567"},

		// Already E.164
		{"E.164 format", "+15551234567", "+15551234567"},
		{"E.164 with spaces", "+1 555 123 4567", "+15551234567"},

		// International (12+ digits) - already E.164 preserved
		{"UK number", "+447911123456", "+447911123456"},

		// Invalid - too short
		{"empty string", "", ""},
		{"9 digits", "555123456", ""},
		{"few digits", "123", ""},
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
