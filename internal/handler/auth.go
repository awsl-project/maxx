package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/golang-jwt/jwt/v5"
)

const (
	// AdminPasswordEnvKey is the environment variable name for admin password
	AdminPasswordEnvKey = "MAXX_ADMIN_PASSWORD"
	// AuthHeader is the header name for JWT authentication
	AuthHeader = "Authorization"
	// TokenExpiry is the JWT token expiry duration
	TokenExpiry = 7 * 24 * time.Hour // 7 days
	// SettingKeyJWTSecret is the system setting key for JWT signing secret
	SettingKeyJWTSecret = "jwt_secret"
)

// MAXXClaims represents the enhanced JWT claims for multi-tenancy
type MAXXClaims struct {
	jwt.RegisteredClaims
	UserID   uint64 `json:"uid"`
	TenantID uint64 `json:"tid"`
	Role     string `json:"role"`
}

// AuthMiddleware provides JWT authentication for admin API
type AuthMiddleware struct {
	password    string
	settingRepo repository.SystemSettingRepository
	userRepo    repository.UserRepository
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(settingRepo repository.SystemSettingRepository, userRepo repository.UserRepository) *AuthMiddleware {
	return &AuthMiddleware{
		password:    os.Getenv(AdminPasswordEnvKey),
		settingRepo: settingRepo,
		userRepo:    userRepo,
	}
}

// IsEnabled returns true if authentication is enabled
func (m *AuthMiddleware) IsEnabled() bool {
	return m.password != ""
}

// IsMultiTenancyEnabled checks if multi-tenancy is active
// Multi-tenancy is active when there are users beyond the default user
func (m *AuthMiddleware) IsMultiTenancyEnabled() bool {
	if m.userRepo == nil {
		return false
	}
	users, err := m.userRepo.List()
	if err != nil {
		return false
	}
	// If more than 1 user exists, multi-tenancy is enabled
	return len(users) > 1
}

// getJWTSecret returns the JWT signing secret
func (m *AuthMiddleware) getJWTSecret() []byte {
	if m.settingRepo != nil {
		secret, err := m.settingRepo.Get(SettingKeyJWTSecret)
		if err == nil && secret != "" {
			return []byte(secret)
		}
	}
	// Fallback to admin password for backward compatibility
	if m.password != "" {
		return []byte(m.password)
	}
	return []byte("maxx-default-secret")
}

// GenerateToken generates a JWT token for a user
func (m *AuthMiddleware) GenerateToken(user *domain.User) (string, error) {
	claims := MAXXClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "maxx-admin",
		},
		UserID:   user.ID,
		TenantID: user.TenantID,
		Role:     string(user.Role),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.getJWTSecret())
}

// GenerateLegacyToken generates a JWT token for single-user mode (backward compatible)
func (m *AuthMiddleware) GenerateLegacyToken() (string, error) {
	user := &domain.User{
		ID:       domain.DefaultUserID,
		TenantID: domain.DefaultTenantID,
		Role:     domain.UserRoleAdmin,
	}
	return m.GenerateToken(user)
}

// ValidateToken validates a JWT token and returns the claims
func (m *AuthMiddleware) ValidateToken(tokenString string) (*MAXXClaims, bool) {
	token, err := jwt.ParseWithClaims(tokenString, &MAXXClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return m.getJWTSecret(), nil
	})
	if err != nil || !token.Valid {
		return nil, false
	}

	claims, ok := token.Claims.(*MAXXClaims)
	if !ok {
		return nil, false
	}

	// For legacy tokens without tenant/user info, set defaults
	if claims.TenantID == 0 {
		claims.TenantID = domain.DefaultTenantID
	}
	if claims.UserID == 0 {
		claims.UserID = domain.DefaultUserID
	}
	if claims.Role == "" {
		claims.Role = string(domain.UserRoleAdmin)
	}

	return claims, true
}

// Wrap wraps a handler with JWT authentication and injects tenant context
func (m *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.IsEnabled() {
			// Auth disabled: inject default tenant context
			ctx := maxxctx.WithTenantID(r.Context(), domain.DefaultTenantID)
			ctx = maxxctx.WithUserID(ctx, domain.DefaultUserID)
			ctx = maxxctx.WithUserRole(ctx, string(domain.UserRoleAdmin))
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		authHeader := r.Header.Get(AuthHeader)
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			writeUnauthorized(w)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, valid := m.ValidateToken(token)
		if !valid {
			writeUnauthorized(w)
			return
		}

		// Inject tenant context from JWT claims
		ctx := maxxctx.WithTenantID(r.Context(), claims.TenantID)
		ctx = maxxctx.WithUserID(ctx, claims.UserID)
		ctx = maxxctx.WithUserRole(ctx, claims.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// VerifyPassword checks if the provided password is correct (legacy single-user mode)
func (m *AuthMiddleware) VerifyPassword(password string) bool {
	if !m.IsEnabled() {
		return true
	}
	// Constant-time comparison
	if len(m.password) != len(password) {
		return false
	}
	result := 0
	for i := 0; i < len(m.password); i++ {
		result |= int(m.password[i]) ^ int(password[i])
	}
	return result == 0
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
