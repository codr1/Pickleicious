package auth

import "golang.org/x/crypto/bcrypt"

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
