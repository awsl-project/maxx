package codex

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/event"
)

// CodexTokenValidationResult token validation result
type CodexTokenValidationResult struct {
	Valid             bool   `json:"valid"`
	Error             string `json:"error,omitempty"`
	Email             string `json:"email,omitempty"`
	Name              string `json:"name,omitempty"`
	Picture           string `json:"picture,omitempty"`
	AccountID         string `json:"accountId,omitempty"`
	UserID            string `json:"userId,omitempty"`
	PlanType          string `json:"planType,omitempty"`
	SubscriptionStart string `json:"subscriptionStart,omitempty"`
	SubscriptionEnd   string `json:"subscriptionEnd,omitempty"`
	AccessToken       string `json:"accessToken,omitempty"`
	RefreshToken      string `json:"refreshToken,omitempty"`
	ExpiresAt         string `json:"expiresAt,omitempty"` // RFC3339 format
}

// CodexQuotaResponse represents the quota data for batch API response
// This is the format returned by GET /codex/providers/quotas
type CodexQuotaResponse struct {
	Email            string            `json:"email"`
	AccountID        string            `json:"accountId,omitempty"`
	PlanType         string            `json:"planType,omitempty"`
	IsForbidden      bool              `json:"isForbidden"`
	LastUpdated      int64             `json:"lastUpdated"` // Unix timestamp
	PrimaryWindow    *CodexUsageWindow `json:"primaryWindow,omitempty"`
	SecondaryWindow  *CodexUsageWindow `json:"secondaryWindow,omitempty"`
	CodeReviewWindow *CodexUsageWindow `json:"codeReviewWindow,omitempty"`
}

// ValidateRefreshToken validates a refresh token and retrieves user info
func ValidateRefreshToken(ctx context.Context, refreshToken string) (*CodexTokenValidationResult, error) {
	result := &CodexTokenValidationResult{
		Valid:        false,
		RefreshToken: refreshToken,
	}

	// 1. Refresh the token to get access token and ID token
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

	// 2. Parse ID token to get user info
	if tokenResp.IDToken != "" {
		claims, err := ParseIDToken(tokenResp.IDToken)
		if err == nil {
			result.Email = claims.Email
			result.Name = claims.Name
			result.Picture = claims.Picture
			result.AccountID = claims.GetAccountID()
			result.UserID = claims.GetUserID()
			result.PlanType = claims.GetPlanType()
			result.SubscriptionStart = claims.GetSubscriptionStart()
			result.SubscriptionEnd = claims.GetSubscriptionEnd()
		}
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
	State             string `json:"state"`
	Success           bool   `json:"success"`
	AccessToken       string `json:"accessToken,omitempty"`
	RefreshToken      string `json:"refreshToken,omitempty"`
	ExpiresAt         string `json:"expiresAt,omitempty"` // RFC3339 format
	Email             string `json:"email,omitempty"`
	Name              string `json:"name,omitempty"`
	Picture           string `json:"picture,omitempty"`
	AccountID         string `json:"accountId,omitempty"`
	UserID            string `json:"userId,omitempty"`
	PlanType          string `json:"planType,omitempty"`
	SubscriptionStart string `json:"subscriptionStart,omitempty"`
	SubscriptionEnd   string `json:"subscriptionEnd,omitempty"`
	Error             string `json:"error,omitempty"`
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
	// Generate PKCE challenge
	pkce, err := GeneratePKCEChallenge()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate PKCE challenge: %w", err)
	}

	session := &OAuthSession{
		State:        state,
		CodeVerifier: pkce.CodeVerifier,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(5 * time.Minute), // 5 minute timeout
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

	// Check if expired
	if time.Now().After(session.ExpiresAt) {
		m.sessions.Delete(state)
		return nil, false
	}

	return session, true
}

// CompleteSession completes the OAuth session and broadcasts the result
func (m *OAuthManager) CompleteSession(state string, result *OAuthResult) {
	// Ensure state matches
	result.State = state

	// Delete session
	m.sessions.Delete(state)

	// Broadcast result via WebSocket
	if m.broadcaster != nil {
		m.broadcaster.BroadcastMessage("codex_oauth_result", result)
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
