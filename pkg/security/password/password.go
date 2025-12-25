package password

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// PasswordStrengthConfig defines the configuration for password strength validation
type PasswordStrengthConfig struct {
	MinLength        int  // Minimum password length
	RequireUppercase bool // Whether password requires uppercase letters
	RequireLowercase bool // Whether password requires lowercase letters
	RequireDigit     bool // Whether password requires digits
	RequireSpecial   bool // Whether password requires special characters
}

// DefaultPasswordStrengthConfig returns the default password strength configuration
func DefaultPasswordStrengthConfig() PasswordStrengthConfig {
	return PasswordStrengthConfig{
		MinLength:        8,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		RequireSpecial:   false,
	}
}

// ValidatePasswordStrength checks if the password meets the required strength criteria
func ValidatePasswordStrength(password string, config PasswordStrengthConfig) bool {
	if len(password) < config.MinLength {
		return false
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	// Check all required conditions
	if config.RequireUppercase && !hasUpper {
		return false
	}
	if config.RequireLowercase && !hasLower {
		return false
	}
	if config.RequireDigit && !hasDigit {
		return false
	}
	if config.RequireSpecial && !hasSpecial {
		return false
	}

	return true
}

// DefaultCost represents the default number of hash rounds applied during password hashing.
const DefaultCost = 12

// HashPassword generates a salted hash of the password with the default cost.
func HashPassword(password string) (string, error) {
	return HashPasswordWithCost(password, DefaultCost)
}

// HashPasswordWithCost generates a salted hash of the password with the specified cost.
// The returned string has the format: "<cost>$<salt>$<hash>".
func HashPasswordWithCost(password string, cost int) (string, error) {
	if cost < 1 {
		return "", errors.New("cost must be at least 1")
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	derived := deriveKey(password, salt, cost)
	encodedSalt := base64.StdEncoding.EncodeToString(salt)
	encodedHash := base64.StdEncoding.EncodeToString(derived)

	return fmt.Sprintf("%d$%s$%s", cost, encodedSalt, encodedHash), nil
}

// CheckPassword compares a hashed password with its plaintext version.
func CheckPassword(hashedPassword, password string) error {
	parts := strings.Split(hashedPassword, "$")
	if len(parts) != 3 {
		return errors.New("invalid hash format")
	}

	cost, err := strconv.Atoi(parts[0])
	if err != nil || cost < 1 {
		return errors.New("invalid hash format")
	}

	salt, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("decode salt: %w", err)
	}

	expectedHash, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return fmt.Errorf("decode hash: %w", err)
	}

	derived := deriveKey(password, salt, cost)
	if subtle.ConstantTimeCompare(expectedHash, derived) == 1 {
		return nil
	}

	return errors.New("password mismatch")
}

func deriveKey(password string, salt []byte, cost int) []byte {
	combined := append([]byte{}, salt...)
	combined = append(combined, []byte(password)...)

	sum := sha256.Sum256(combined)
	derived := sum[:]

	// Repeat hashing cost-1 additional times to increase work factor.
	for i := 1; i < cost; i++ {
		sum = sha256.Sum256(derived)
		derived = sum[:]
	}

	return derived
}
