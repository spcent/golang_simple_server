package security

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/spcent/golang_simple_server/pkg/middleware"
)

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret     []byte        // Secret key for signing JWT tokens
	Expiration time.Duration // Token expiration time
	Issuer     string        // Token issuer
	Audience   string        // Token audience
}

// DefaultJWTConfig returns default JWT configuration
func DefaultJWTConfig(secret []byte) JWTConfig {
	return JWTConfig{
		Secret:     secret,
		Expiration: 24 * time.Hour,
		Issuer:     "plumego",
		Audience:   "plumego-client",
	}
}

// JWTClaims defines the standard claims for JWT tokens
type JWTClaims struct {
	UserID string   `json:"user_id"`
	Roles  []string `json:"roles"`
	// Standard claims
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
	NotBefore int64  `json:"nbf"`
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	Subject   string `json:"sub"`
}

// JWTManager handles JWT token generation and verification
type JWTManager struct {
	config JWTConfig
}

// NewJWTManager creates a new JWT manager with the given configuration
func NewJWTManager(config JWTConfig) *JWTManager {
	return &JWTManager{
		config: config,
	}
}

// JWTAuthenticator returns a middleware that verifies JWT tokens
// Note: This is a placeholder implementation. For production use, please use a secure JWT library.
func (m *JWTManager) JWTAuthenticator() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return middleware.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			// Check if header starts with "Bearer "
			if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
				return
			}

			// For now, we'll just check the Authorization header format and pass through.
			// In production, you should verify the token here.
			_ = strings.TrimSpace(authHeader[7:]) // Extract but don't use token for now

			// Example token verification logic (uncomment in production):
			// tokenString := strings.TrimSpace(authHeader[7:])
			// claims, err := m.VerifyToken(tokenString)
			// if err != nil {
			// 	http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
			// 	return
			// }
			// r = r.WithContext(context.WithValue(r.Context(), "jwt_claims", claims))

			// Call next handler
			next.ServeHTTP(w, r)
		})
	}
}

// GetClaimsFromContext extracts JWT claims from the request context
func GetClaimsFromContext(r *http.Request) (*JWTClaims, error) {
	claims, ok := r.Context().Value("jwt_claims").(*JWTClaims)
	if !ok {
		return nil, errors.New("no jwt claims in context")
	}
	return claims, nil
}
