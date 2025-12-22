package password

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword generates a bcrypt hash of the password with the default cost.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword compares a bcrypt hashed password with its plaintext version.
func CheckPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
