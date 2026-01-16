package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/repository/cached"
)

const (
	// TokenPrefix is the prefix for all API tokens
	TokenPrefix = "maxx_"
	// TokenPrefixDisplayLen is the length of token prefix to display (including "maxx_")
	TokenPrefixDisplayLen = 12
)

var (
	ErrMissingToken  = errors.New("missing API token")
	ErrInvalidToken  = errors.New("invalid API token")
	ErrTokenDisabled = errors.New("API token is disabled")
	ErrTokenExpired  = errors.New("API token has expired")
)

// TokenAuthMiddleware handles API token authentication for proxy requests
type TokenAuthMiddleware struct {
	tokenRepo   *cached.APITokenRepository
	settingRepo repository.SystemSettingRepository
}

// NewTokenAuthMiddleware creates a new token authentication middleware
func NewTokenAuthMiddleware(
	tokenRepo *cached.APITokenRepository,
	settingRepo repository.SystemSettingRepository,
) *TokenAuthMiddleware {
	return &TokenAuthMiddleware{
		tokenRepo:   tokenRepo,
		settingRepo: settingRepo,
	}
}

// IsEnabled checks if token authentication is required
func (m *TokenAuthMiddleware) IsEnabled() bool {
	val, err := m.settingRepo.Get(SettingKeyProxyTokenAuthEnabled)
	if err != nil {
		return false
	}
	return val == "true"
}

// ExtractToken extracts the token from the request based on client type
func (m *TokenAuthMiddleware) ExtractToken(req *http.Request, clientType domain.ClientType) string {
	switch clientType {
	case domain.ClientTypeClaude:
		// Claude uses x-api-key header
		return req.Header.Get("x-api-key")
	case domain.ClientTypeOpenAI, domain.ClientTypeCodex:
		// OpenAI/Codex uses Authorization: Bearer <token>
		auth := req.Header.Get("Authorization")
		return strings.TrimPrefix(auth, "Bearer ")
	case domain.ClientTypeGemini:
		// Gemini uses x-goog-api-key header
		return req.Header.Get("x-goog-api-key")
	}
	return ""
}

// ValidateRequest validates the token from the request
// Returns the token entity if valid, nil if auth is disabled, error if invalid
func (m *TokenAuthMiddleware) ValidateRequest(req *http.Request, clientType domain.ClientType) (*domain.APIToken, error) {
	if !m.IsEnabled() {
		return nil, nil // Auth disabled, allow all
	}

	// Extract token based on client type
	token := m.ExtractToken(req, clientType)
	token = strings.TrimSpace(token)

	if token == "" {
		return nil, ErrMissingToken
	}

	// Check if it's a maxx token
	if !strings.HasPrefix(token, TokenPrefix) {
		return nil, ErrInvalidToken
	}

	// Look up token directly (plaintext comparison)
	apiToken, err := m.tokenRepo.GetByToken(token)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}

	// Check if enabled
	if !apiToken.IsEnabled {
		return nil, ErrTokenDisabled
	}

	// Check expiration
	if apiToken.ExpiresAt != nil && time.Now().After(*apiToken.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	// Update usage (async to not block request)
	go m.tokenRepo.IncrementUseCount(apiToken.ID)

	return apiToken, nil
}

// GenerateToken creates a new random token
// Returns: plain token, prefix for display
func GenerateToken() (plain string, prefix string) {
	// Generate 32 random bytes (64 hex chars)
	bytes := make([]byte, 32)
	rand.Read(bytes)

	plain = TokenPrefix + hex.EncodeToString(bytes)

	// Create display prefix (e.g., "maxx_abc12345...")
	if len(plain) > TokenPrefixDisplayLen {
		prefix = plain[:TokenPrefixDisplayLen] + "..."
	} else {
		prefix = plain
	}

	return plain, prefix
}

// Setting key for token auth
const SettingKeyProxyTokenAuthEnabled = "api_token_auth_enabled"
