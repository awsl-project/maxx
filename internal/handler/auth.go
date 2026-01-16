package handler

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// AdminPasswordEnvKey is the environment variable name for admin password
	AdminPasswordEnvKey = "MAXX_ADMIN_PASSWORD"
	// AuthHeader is the header name for JWT authentication
	AuthHeader = "Authorization"
	// TokenExpiry is the JWT token expiry duration
	TokenExpiry = 7 * 24 * time.Hour // 7 days
)

// AuthMiddleware provides JWT authentication for admin API
type AuthMiddleware struct {
	password string
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware() *AuthMiddleware {
	return &AuthMiddleware{
		password: os.Getenv(AdminPasswordEnvKey),
	}
}

// IsEnabled returns true if authentication is enabled
func (m *AuthMiddleware) IsEnabled() bool {
	return m.password != ""
}

// GenerateToken generates a JWT token
func (m *AuthMiddleware) GenerateToken() (string, error) {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenExpiry)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Issuer:    "maxx-admin",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.password))
}

// ValidateToken validates a JWT token
func (m *AuthMiddleware) ValidateToken(tokenString string) bool {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(m.password), nil
	})

	return err == nil && token.Valid
}

// Wrap wraps a handler with JWT authentication
func (m *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.IsEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get(AuthHeader)
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			writeUnauthorized(w)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if !m.ValidateToken(token) {
			writeUnauthorized(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// VerifyPassword checks if the provided password is correct
func (m *AuthMiddleware) VerifyPassword(password string) bool {
	if !m.IsEnabled() {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(m.password), []byte(password)) == 1
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
