package auth

import (
	"errors"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword wraps bcrypt.GenerateFromPassword for local auth storage.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword wraps bcrypt.CompareHashAndPassword for local auth checks.
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// ValidatePasswordComplexity ensures passwords meet basic complexity requirements.
func ValidatePasswordComplexity(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}

	hasUpper := false
	hasSymbol := false
	for _, r := range password {
		if unicode.IsUpper(r) {
			hasUpper = true
		} else if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			hasSymbol = true
		}
	}

	if !hasUpper {
		return errors.New("password must include an uppercase letter")
	}
	if !hasSymbol {
		return errors.New("password must include a symbol")
	}

	return nil
}
