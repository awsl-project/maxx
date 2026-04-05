package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	ErrMissingToken         = errors.New("missing API token")
	ErrInvalidToken         = errors.New("invalid API token")
	ErrTokenDisabled        = errors.New("API token is disabled")
	ErrTokenExpired         = errors.New("API token has expired")
	ErrTokenConcurrentLimit = errors.New("API token concurrent request limit exceeded")
)

// TokenAuthMiddleware handles API token authentication for proxy requests
type TokenAuthMiddleware struct {
	tokenRepo   *cached.APITokenRepository
	settingRepo repository.SystemSettingRepository

	concurrencyMu     sync.Mutex
	activeByTokenID   map[uint64]int
	activeByTokenName map[string]int
}

// NewTokenAuthMiddleware creates a new token authentication middleware
func NewTokenAuthMiddleware(
	tokenRepo *cached.APITokenRepository,
	settingRepo repository.SystemSettingRepository,
) *TokenAuthMiddleware {
	return &TokenAuthMiddleware{
		tokenRepo:         tokenRepo,
		settingRepo:       settingRepo,
		activeByTokenID:   make(map[uint64]int),
		activeByTokenName: make(map[string]int),
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
		if auth := req.Header.Get("Authorization"); auth != "" {
			if parts := strings.Fields(auth); len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				return parts[1]
			}
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

// validateExtractedToken validates a token string that has already been extracted from the request.
func (m *TokenAuthMiddleware) validateExtractedToken(token string) (*domain.APIToken, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrMissingToken
	}
	if !strings.HasPrefix(token, TokenPrefix) {
		return nil, ErrInvalidToken
	}

	// Look up token across all tenants — tenant identity is determined by the token itself
	apiToken, err := m.tokenRepo.GetByToken(domain.TenantIDAll, token)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	if !apiToken.IsEnabled {
		return nil, ErrTokenDisabled
	}
	if apiToken.ExpiresAt != nil && time.Now().After(*apiToken.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	return apiToken, nil
}

// ResolveToken resolves the token attached to a request without assuming a specific client type.
func (m *TokenAuthMiddleware) ResolveToken(req *http.Request) (*domain.APIToken, error) {
	if !m.IsEnabled() {
		return nil, nil
	}
	token := strings.TrimSpace(m.ExtractToken(req, ""))
	return m.validateExtractedToken(token)
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

	apiToken, err := m.validateExtractedToken(token)
	if err != nil {
		return nil, err
	}

	// Update usage (async to not block request)
	lastSeenAt := time.Now()
	clientIP := strings.TrimSpace(getClientIP(req))
	go func(tenantID uint64, tokenID uint64, lastIP string, seenAt time.Time) {
		if err := m.tokenRepo.UpdateLastSeen(tenantID, tokenID, lastIP, seenAt); err != nil {
			log.Printf("[TokenAuth] Failed to update token last seen for ID %d: %v", tokenID, err)
		}
	}(apiToken.TenantID, apiToken.ID, clientIP, lastSeenAt)

	return apiToken, nil
}

func (m *TokenAuthMiddleware) GetConcurrentLimit() int {
	val, err := m.settingRepo.Get(SettingKeyAPITokenConcurrentLimit)
	if err != nil {
		return DefaultAPITokenConcurrentLimit
	}
	limit, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil || limit <= 0 {
		return DefaultAPITokenConcurrentLimit
	}
	return limit
}

func (m *TokenAuthMiddleware) AcquireConcurrency(apiToken *domain.APIToken) error {
	if apiToken == nil {
		return nil
	}

	limit := m.GetConcurrentLimit()
	keyName := strings.TrimSpace(apiToken.Token)
	if apiToken.ID == 0 && keyName == "" {
		return ErrInvalidToken
	}

	m.concurrencyMu.Lock()
	defer m.concurrencyMu.Unlock()

	active := 0
	if apiToken.ID != 0 {
		active = m.activeByTokenID[apiToken.ID]
	} else {
		active = m.activeByTokenName[keyName]
	}
	if active >= limit {
		return ErrTokenConcurrentLimit
	}

	if apiToken.ID != 0 {
		m.activeByTokenID[apiToken.ID] = active + 1
	} else if keyName != "" {
		m.activeByTokenName[keyName] = active + 1
	}
	return nil
}

func (m *TokenAuthMiddleware) ReleaseConcurrency(apiToken *domain.APIToken) {
	if apiToken == nil {
		return
	}

	keyName := strings.TrimSpace(apiToken.Token)

	m.concurrencyMu.Lock()
	defer m.concurrencyMu.Unlock()

	if apiToken.ID != 0 {
		if active := m.activeByTokenID[apiToken.ID]; active > 1 {
			m.activeByTokenID[apiToken.ID] = active - 1
		} else {
			delete(m.activeByTokenID, apiToken.ID)
		}
		return
	}

	if keyName == "" {
		return
	}
	if active := m.activeByTokenName[keyName]; active > 1 {
		m.activeByTokenName[keyName] = active - 1
	} else {
		delete(m.activeByTokenName, keyName)
	}
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

const (
	// Setting key for token auth
	SettingKeyProxyTokenAuthEnabled = "api_token_auth_enabled"
	// Setting key for API token concurrent request limit
	SettingKeyAPITokenConcurrentLimit = "api_token_concurrent_limit"
	// Default concurrent request limit per API token
	DefaultAPITokenConcurrentLimit = 5
)
