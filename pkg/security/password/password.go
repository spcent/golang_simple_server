package password

import (
	"golang.org/x/crypto/bcrypt"
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

// HashPassword generates a bcrypt hash of the password with the default cost.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// HashPasswordWithCost generates a bcrypt hash of the password with the specified cost.
func HashPasswordWithCost(password string, cost int) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	return string(bytes), err
}

// CheckPassword compares a bcrypt hashed password with its plaintext version.
func CheckPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
