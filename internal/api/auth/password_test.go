package auth

import "testing"

func TestHashPasswordAndVerify(t *testing.T) {
	password := "p1ckleball!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if hash == password {
		t.Fatal("expected hash to differ from password")
	}

	if !VerifyPassword(hash, password) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("expected password mismatch to fail")
	}
}

func TestVerifyPasswordWithInvalidHash(t *testing.T) {
	if VerifyPassword("not-a-valid-hash", "password") {
		t.Fatal("expected invalid hash to fail verification")
	}
}

func TestValidatePasswordComplexity(t *testing.T) {
	cases := []struct {
		name     string
		password string
		wantErr  string
	}{
		{
			name:     "too_short",
			password: "Ab1!",
			wantErr:  "password must be at least 8 characters",
		},
		{
			name:     "missing_uppercase",
			password: "pickleball!",
			wantErr:  "password must include an uppercase letter",
		},
		{
			name:     "missing_symbol",
			password: "Pickleball1",
			wantErr:  "password must include a symbol",
		},
		{
			name:     "valid",
			password: "Pickleball!",
			wantErr:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePasswordComplexity(tc.password)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tc.wantErr)
				}
				if err.Error() != tc.wantErr {
					t.Fatalf("expected error %q, got %q", tc.wantErr, err.Error())
				}
			}
		})
	}
}
