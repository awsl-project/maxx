package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
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
		// On error, default to disabled to avoid blocking all requests
		// when the setting hasn't been configured yet
		return false
	}
	return val == "true"
}

// ExtractToken extracts the token from the request based on client type
// First tries the primary header for the client type, then falls back to other headers
func (m *TokenAuthMiddleware) ExtractToken(req *http.Request, clientType domain.ClientType) string {
	// Try primary header based on client type first
	switch clientType {
	case domain.ClientTypeClaude:
		if token := req.Header.Get("x-api-key"); token != "" {
			return token
		}
	case domain.ClientTypeOpenAI, domain.ClientTypeCodex:
		if auth := req.Header.Get("Authorization"); auth != "" {
			if parts := strings.Fields(auth); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				return parts[1]
			}
		}
	case domain.ClientTypeGemini:
		if token := req.Header.Get("x-goog-api-key"); token != "" {
			return token
		}
	}

	// Fallback: try all headers
	// Authorization: Bearer <token>
	if auth := req.Header.Get("Authorization"); auth != "" {
		if parts := strings.Fields(auth); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1]
		}
	}

	// x-api-key (Claude style)
	if token := req.Header.Get("x-api-key"); token != "" {
		return token
	}

	// x-goog-api-key (Gemini style)
	if token := req.Header.Get("x-goog-api-key"); token != "" {
		return token
	}

	return ""
}

// ValidateRequest validates the token from the request
// Returns the token entity if valid, nil if auth is disabled, error if invalid
func (m *TokenAuthMiddleware) ValidateRequest(req *http.Request, clientType domain.ClientType) (*domain.APIToken, error) {
	if !m.IsEnabled() {
		return nil, nil // Auth disabled, allow all
	}

	// Extract token based on client type, with fallback to other headers
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
	go func() {
		if err := m.tokenRepo.IncrementUseCount(apiToken.ID); err != nil {
			log.Printf("[TokenAuth] Failed to increment token use count for ID %d: %v", apiToken.ID, err)
		}
	}()

	return apiToken, nil
}

// GenerateToken creates a new random token
// Returns: plain token, prefix for display, error if generation fails
func GenerateToken() (plain string, prefix string, err error) {
	// Generate 32 random bytes (64 hex chars)
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random token: %w", err)
	}

	plain = TokenPrefix + hex.EncodeToString(bytes)

	// Create display prefix (e.g., "maxx_abc12345...")
	if len(plain) > TokenPrefixDisplayLen {
		prefix = plain[:TokenPrefixDisplayLen] + "..."
	} else {
		prefix = plain
	}

	return plain, prefix, nil
}

// Setting key for token auth
const SettingKeyProxyTokenAuthEnabled = "api_token_auth_enabled"
