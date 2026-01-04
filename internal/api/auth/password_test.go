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
