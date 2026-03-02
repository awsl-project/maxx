package claude

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/event"
)

// ClaudeTokenValidationResult token validation result
type ClaudeTokenValidationResult struct {
	Valid          bool   `json:"valid"`
	Error          string `json:"error,omitempty"`
	Email          string `json:"email,omitempty"`
	OrganizationID string `json:"organizationId,omitempty"`
	AccessToken    string `json:"accessToken,omitempty"`
	RefreshToken   string `json:"refreshToken,omitempty"`
	ExpiresAt      string `json:"expiresAt,omitempty"` // RFC3339 format
}

// ValidateRefreshToken validates a refresh token and retrieves user info
func ValidateRefreshToken(ctx context.Context, refreshToken string) (*ClaudeTokenValidationResult, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return &ClaudeTokenValidationResult{
			Valid: false,
			Error: "refresh token is empty",
		}, nil
	}

	result := &ClaudeTokenValidationResult{
		Valid:        false,
		RefreshToken: refreshToken,
	}

	// Refresh the token to get access token
	tokenResp, err := RefreshAccessToken(ctx, refreshToken)
	if err != nil {
		result.Error = fmt.Sprintf("Token refresh failed: %v", err)
		return result, nil
	}

	result.AccessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		result.RefreshToken = tokenResp.RefreshToken
	}

	// Calculate expiration time
	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	result.ExpiresAt = expiresAt.Format(time.RFC3339)

	// Extract account info from token response
	if tokenResp.Account != nil {
		result.Email = tokenResp.Account.EmailAddress
	}
	if tokenResp.Organization != nil {
		result.OrganizationID = tokenResp.Organization.UUID
	}

	result.Valid = true
	return result, nil
}

// OAuthSession represents an OAuth authorization session
type OAuthSession struct {
	State        string
	CodeVerifier string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

// OAuthResult represents the OAuth authorization result
type OAuthResult struct {
	State          string `json:"state"`
	Success        bool   `json:"success"`
	AccessToken    string `json:"accessToken,omitempty"`
	RefreshToken   string `json:"refreshToken,omitempty"`
	ExpiresAt      string `json:"expiresAt,omitempty"` // RFC3339 format
	Email          string `json:"email,omitempty"`
	OrganizationID string `json:"organizationId,omitempty"`
	Error          string `json:"error,omitempty"`
}

// OAuthManager manages OAuth authorization sessions
type OAuthManager struct {
	sessions    sync.Map          // state -> *OAuthSession
	broadcaster event.Broadcaster // for pushing OAuth results
}

// NewOAuthManager creates a new OAuth manager
func NewOAuthManager(broadcaster event.Broadcaster) *OAuthManager {
	manager := &OAuthManager{
		broadcaster: broadcaster,
	}

	// Start cleanup goroutine
	go manager.cleanupExpired()

	return manager
}

// GenerateState generates a random state token
func (m *OAuthManager) GenerateState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// CreateSession creates a new OAuth session with PKCE
func (m *OAuthManager) CreateSession(state string) (*OAuthSession, *PKCEChallenge, error) {
	pkce, err := GeneratePKCEChallenge()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate PKCE challenge: %w", err)
	}

	session := &OAuthSession{
		State:        state,
		CodeVerifier: pkce.CodeVerifier,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(5 * time.Minute),
	}

	m.sessions.Store(state, session)
	return session, pkce, nil
}

// GetSession retrieves a session by state
func (m *OAuthManager) GetSession(state string) (*OAuthSession, bool) {
	val, ok := m.sessions.Load(state)
	if !ok {
		return nil, false
	}

	session, ok := val.(*OAuthSession)
	if !ok {
		return nil, false
	}

	if time.Now().After(session.ExpiresAt) {
		m.sessions.Delete(state)
		return nil, false
	}

	return session, true
}

// CompleteSession completes the OAuth session and broadcasts the result
func (m *OAuthManager) CompleteSession(state string, result *OAuthResult) {
	result.State = state
	m.sessions.Delete(state)

	if m.broadcaster != nil {
		m.broadcaster.BroadcastMessage("claude_oauth_result", result)
	}
}

// cleanupExpired periodically cleans up expired sessions
func (m *OAuthManager) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		m.sessions.Range(func(key, value interface{}) bool {
			session, ok := value.(*OAuthSession)
			if ok && now.After(session.ExpiresAt) {
				m.sessions.Delete(key)
			}
			return true
		})
	}
}
