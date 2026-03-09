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
	// AuthCookieName is the HttpOnly cookie used for admin authentication
	AuthCookieName = "maxx_admin_auth"
	// AuthCookiePath scopes the admin auth cookie to admin API endpoints
	AuthCookiePath = "/api/admin"
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
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(settingRepo repository.SystemSettingRepository) *AuthMiddleware {
	return &AuthMiddleware{
		password:    os.Getenv(AdminPasswordEnvKey),
		settingRepo: settingRepo,
	}
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

func extractBearerToken(authHeader string) string {
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
}

func isHTTPSRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}

	proto := strings.TrimSpace(strings.ToLower(r.Header.Get("X-Forwarded-Proto")))
	if proto == "" {
		return false
	}
	if idx := strings.Index(proto, ","); idx >= 0 {
		proto = strings.TrimSpace(proto[:idx])
	}
	return proto == "https"
}

func (m *AuthMiddleware) tokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if token := extractBearerToken(r.Header.Get(AuthHeader)); token != "" {
		return token
	}
	cookie, err := r.Cookie(AuthCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func (m *AuthMiddleware) ClaimsFromRequest(r *http.Request) (*MAXXClaims, bool) {
	if m == nil {
		return nil, false
	}
	token := m.tokenFromRequest(r)
	if token == "" {
		return nil, false
	}
	return m.ValidateToken(token)
}

func (m *AuthMiddleware) SetAuthCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    token,
		Path:     AuthCookiePath,
		HttpOnly: true,
		Secure:   isHTTPSRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(TokenExpiry.Seconds()),
		Expires:  time.Now().Add(TokenExpiry),
	})
}

func (m *AuthMiddleware) ClearAuthCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    "",
		Path:     AuthCookiePath,
		HttpOnly: true,
		Secure:   isHTTPSRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

// Wrap wraps a handler with JWT authentication and injects tenant context.
// All requests must provide a valid JWT token.
func (m *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, valid := m.ClaimsFromRequest(r)
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

// NoAuthMiddleware injects default tenant/user context when authentication is disabled.
// Used in single-user / intranet mode where MAXX_ADMIN_PASSWORD is not set.
func NoAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := maxxctx.WithTenantID(r.Context(), domain.DefaultTenantID)
		ctx = maxxctx.WithUserID(ctx, domain.DefaultUserID)
		ctx = maxxctx.WithUserRole(ctx, string(domain.UserRoleAdmin))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
