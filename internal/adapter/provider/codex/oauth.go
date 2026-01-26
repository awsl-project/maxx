package codex

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PKCEChallenge holds PKCE verifier and challenge
type PKCEChallenge struct {
	CodeVerifier  string `json:"codeVerifier"`
	CodeChallenge string `json:"codeChallenge"`
}

// TokenResponse represents the OAuth token response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token,omitempty"`
}

// CodexAuthInfo contains authentication-related details specific to Codex
type CodexAuthInfo struct {
	ChatgptAccountID               string `json:"chatgpt_account_id"`
	ChatgptPlanType                string `json:"chatgpt_plan_type"`
	ChatgptUserID                  string `json:"chatgpt_user_id"`
	UserID                         string `json:"user_id"`
	ChatgptSubscriptionActiveStart any    `json:"chatgpt_subscription_active_start"`
	ChatgptSubscriptionActiveUntil any    `json:"chatgpt_subscription_active_until"`
}

// IDTokenClaims represents the decoded ID token claims
type IDTokenClaims struct {
	Sub           string        `json:"sub"`
	Email         string        `json:"email"`
	EmailVerified bool          `json:"email_verified"`
	Name          string        `json:"name"`
	Picture       string        `json:"picture,omitempty"`
	Aud           any           `json:"aud"` // Can be string or []string
	Iss           string        `json:"iss"`
	Iat           int64         `json:"iat"`
	Exp           int64         `json:"exp"`
	AuthInfo      CodexAuthInfo `json:"https://api.openai.com/auth"`
}

// GetAccountID returns the ChatGPT account ID
func (c *IDTokenClaims) GetAccountID() string {
	if c.AuthInfo.ChatgptAccountID != "" {
		return c.AuthInfo.ChatgptAccountID
	}
	return c.Sub // fallback to sub
}

// GetUserID returns the ChatGPT user ID
func (c *IDTokenClaims) GetUserID() string {
	if c.AuthInfo.ChatgptUserID != "" {
		return c.AuthInfo.ChatgptUserID
	}
	return c.AuthInfo.UserID
}

// GetPlanType returns the ChatGPT plan type
func (c *IDTokenClaims) GetPlanType() string {
	return c.AuthInfo.ChatgptPlanType
}

// GetSubscriptionStart returns the subscription start time as string
func (c *IDTokenClaims) GetSubscriptionStart() string {
	return formatSubscriptionTime(c.AuthInfo.ChatgptSubscriptionActiveStart)
}

// GetSubscriptionEnd returns the subscription end time as string
func (c *IDTokenClaims) GetSubscriptionEnd() string {
	return formatSubscriptionTime(c.AuthInfo.ChatgptSubscriptionActiveUntil)
}

// formatSubscriptionTime converts subscription time to RFC3339 string
func formatSubscriptionTime(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// Unix timestamp
		return time.Unix(int64(t), 0).Format(time.RFC3339)
	case int64:
		return time.Unix(t, 0).Format(time.RFC3339)
	default:
		return ""
	}
}

// GeneratePKCEChallenge generates a PKCE code_verifier and code_challenge
func GeneratePKCEChallenge() (*PKCEChallenge, error) {
	// Generate 32 random bytes for code_verifier
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode as base64url (no padding)
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// Generate code_challenge = base64url(sha256(code_verifier))
	hash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(hash[:])

	return &PKCEChallenge{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
	}, nil
}

// GetAuthURL builds the OpenAI OAuth authorization URL
// Uses fixed localhost redirect URI as required by OpenAI
func GetAuthURL(state string, pkce *PKCEChallenge) string {
	params := url.Values{}
	params.Set("client_id", OAuthClientID)
	params.Set("redirect_uri", OAuthRedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", OAuthScopes)
	params.Set("state", state)
	params.Set("code_challenge", pkce.CodeChallenge)
	params.Set("code_challenge_method", "S256")
	// Additional params from CLIProxyAPI
	params.Set("prompt", "login")
	params.Set("id_token_add_organizations", "true")
	params.Set("codex_cli_simplified_flow", "true")

	return OpenAIAuthURL + "?" + params.Encode()
}

// ExchangeCodeForTokens exchanges the authorization code for tokens
func ExchangeCodeForTokens(ctx context.Context, code, redirectURI, codeVerifier string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", OAuthClientID)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, "POST", OpenAITokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

// RefreshAccessToken refreshes the access token using a refresh token
func RefreshAccessToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", OAuthClientID)
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", OpenAITokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

// ParseIDToken decodes the ID token (JWT) without verifying signature
// Note: In a production environment, you should verify the signature
func ParseIDToken(idToken string) (*IDTokenClaims, error) {
	// Split the JWT
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid ID token format")
	}

	// Decode the payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Try with padding
		payload, err = base64.StdEncoding.DecodeString(parts[1] + strings.Repeat("=", (4-len(parts[1])%4)%4))
		if err != nil {
			return nil, fmt.Errorf("failed to decode ID token payload: %w", err)
		}
	}

	var claims IDTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse ID token claims: %w", err)
	}

	return &claims, nil
}

// ============================================================================
// Usage/Quota types and functions
// ============================================================================

// CodexUsageWindow represents a rate limit window (5h, weekly, etc.)
type CodexUsageWindow struct {
	UsedPercent        *float64 `json:"usedPercent,omitempty"`
	LimitWindowSeconds *int64   `json:"limitWindowSeconds,omitempty"`
	ResetAfterSeconds  *int64   `json:"resetAfterSeconds,omitempty"`
	ResetAt            *int64   `json:"resetAt,omitempty"`
}

// CodexRateLimitInfo contains rate limit information
type CodexRateLimitInfo struct {
	Allowed         *bool             `json:"allowed,omitempty"`
	LimitReached    *bool             `json:"limitReached,omitempty"`
	PrimaryWindow   *CodexUsageWindow `json:"primaryWindow,omitempty"`
	SecondaryWindow *CodexUsageWindow `json:"secondaryWindow,omitempty"`
}

// CodexUsageResponse represents the usage API response
type CodexUsageResponse struct {
	PlanType            string              `json:"planType,omitempty"`
	RateLimit           *CodexRateLimitInfo `json:"rateLimit,omitempty"`
	CodeReviewRateLimit *CodexRateLimitInfo `json:"codeReviewRateLimit,omitempty"`
}

// codexUsageAPIResponse handles both camelCase and snake_case from API
type codexUsageAPIResponse struct {
	PlanType            string `json:"plan_type,omitempty"`
	PlanTypeCamel       string `json:"planType,omitempty"`
	RateLimit           *struct {
		Allowed            *bool `json:"allowed,omitempty"`
		LimitReached       *bool `json:"limit_reached,omitempty"`
		LimitReachedCamel  *bool `json:"limitReached,omitempty"`
		PrimaryWindow      *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"primary_window,omitempty"`
		PrimaryWindowCamel *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"primaryWindow,omitempty"`
		SecondaryWindow *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"secondary_window,omitempty"`
		SecondaryWindowCamel *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"secondaryWindow,omitempty"`
	} `json:"rate_limit,omitempty"`
	RateLimitCamel *struct {
		Allowed            *bool `json:"allowed,omitempty"`
		LimitReached       *bool `json:"limit_reached,omitempty"`
		LimitReachedCamel  *bool `json:"limitReached,omitempty"`
		PrimaryWindow      *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"primary_window,omitempty"`
		PrimaryWindowCamel *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"primaryWindow,omitempty"`
		SecondaryWindow *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"secondary_window,omitempty"`
		SecondaryWindowCamel *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"secondaryWindow,omitempty"`
	} `json:"rateLimit,omitempty"`
	CodeReviewRateLimit *struct {
		Allowed            *bool `json:"allowed,omitempty"`
		LimitReached       *bool `json:"limit_reached,omitempty"`
		LimitReachedCamel  *bool `json:"limitReached,omitempty"`
		PrimaryWindow      *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"primary_window,omitempty"`
		PrimaryWindowCamel *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"primaryWindow,omitempty"`
	} `json:"code_review_rate_limit,omitempty"`
	CodeReviewRateLimitCamel *struct {
		Allowed            *bool `json:"allowed,omitempty"`
		LimitReached       *bool `json:"limit_reached,omitempty"`
		LimitReachedCamel  *bool `json:"limitReached,omitempty"`
		PrimaryWindow      *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"primary_window,omitempty"`
		PrimaryWindowCamel *struct {
			UsedPercent             *float64 `json:"used_percent,omitempty"`
			UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
			LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
			LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
			ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
			ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
			ResetAt                 *int64   `json:"reset_at,omitempty"`
			ResetAtCamel            *int64   `json:"resetAt,omitempty"`
		} `json:"primaryWindow,omitempty"`
	} `json:"codeReviewRateLimit,omitempty"`
}

// parseWindow parses a window from API response (handles both snake_case and camelCase)
func parseWindow(w *struct {
	UsedPercent             *float64 `json:"used_percent,omitempty"`
	UsedPercentCamel        *float64 `json:"usedPercent,omitempty"`
	LimitWindowSeconds      *int64   `json:"limit_window_seconds,omitempty"`
	LimitWindowSecondsCamel *int64   `json:"limitWindowSeconds,omitempty"`
	ResetAfterSeconds       *int64   `json:"reset_after_seconds,omitempty"`
	ResetAfterSecondsCamel  *int64   `json:"resetAfterSeconds,omitempty"`
	ResetAt                 *int64   `json:"reset_at,omitempty"`
	ResetAtCamel            *int64   `json:"resetAt,omitempty"`
}) *CodexUsageWindow {
	if w == nil {
		return nil
	}
	result := &CodexUsageWindow{}
	if w.UsedPercent != nil {
		result.UsedPercent = w.UsedPercent
	} else if w.UsedPercentCamel != nil {
		result.UsedPercent = w.UsedPercentCamel
	}
	if w.LimitWindowSeconds != nil {
		result.LimitWindowSeconds = w.LimitWindowSeconds
	} else if w.LimitWindowSecondsCamel != nil {
		result.LimitWindowSeconds = w.LimitWindowSecondsCamel
	}
	if w.ResetAfterSeconds != nil {
		result.ResetAfterSeconds = w.ResetAfterSeconds
	} else if w.ResetAfterSecondsCamel != nil {
		result.ResetAfterSeconds = w.ResetAfterSecondsCamel
	}
	if w.ResetAt != nil {
		result.ResetAt = w.ResetAt
	} else if w.ResetAtCamel != nil {
		result.ResetAt = w.ResetAtCamel
	}
	return result
}

// normalizeUsageResponse normalizes the API response to CodexUsageResponse
func normalizeUsageResponse(raw *codexUsageAPIResponse) *CodexUsageResponse {
	if raw == nil {
		return nil
	}

	result := &CodexUsageResponse{}

	// Plan type
	if raw.PlanType != "" {
		result.PlanType = raw.PlanType
	} else if raw.PlanTypeCamel != "" {
		result.PlanType = raw.PlanTypeCamel
	}

	// Rate limit
	rl := raw.RateLimit
	if rl == nil {
		rl = raw.RateLimitCamel
	}
	if rl != nil {
		result.RateLimit = &CodexRateLimitInfo{
			Allowed: rl.Allowed,
		}
		if rl.LimitReached != nil {
			result.RateLimit.LimitReached = rl.LimitReached
		} else if rl.LimitReachedCamel != nil {
			result.RateLimit.LimitReached = rl.LimitReachedCamel
		}
		// Primary window
		pw := rl.PrimaryWindow
		if pw == nil {
			pw = rl.PrimaryWindowCamel
		}
		result.RateLimit.PrimaryWindow = parseWindow(pw)
		// Secondary window
		sw := rl.SecondaryWindow
		if sw == nil {
			sw = rl.SecondaryWindowCamel
		}
		result.RateLimit.SecondaryWindow = parseWindow(sw)
	}

	// Code review rate limit
	crl := raw.CodeReviewRateLimit
	if crl == nil {
		crl = raw.CodeReviewRateLimitCamel
	}
	if crl != nil {
		result.CodeReviewRateLimit = &CodexRateLimitInfo{
			Allowed: crl.Allowed,
		}
		if crl.LimitReached != nil {
			result.CodeReviewRateLimit.LimitReached = crl.LimitReached
		} else if crl.LimitReachedCamel != nil {
			result.CodeReviewRateLimit.LimitReached = crl.LimitReachedCamel
		}
		// Primary window only for code review
		pw := crl.PrimaryWindow
		if pw == nil {
			pw = crl.PrimaryWindowCamel
		}
		result.CodeReviewRateLimit.PrimaryWindow = parseWindow(pw)
	}

	return result
}

// FetchUsage fetches usage/quota information from Codex API
func FetchUsage(ctx context.Context, accessToken, accountID string) (*CodexUsageResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", CodexUsageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", CodexUserAgent)
	if accountID != "" {
		req.Header.Set("Chatgpt-Account-Id", accountID)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("usage request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var raw codexUsageAPIResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse usage response: %w", err)
	}

	return normalizeUsageResponse(&raw), nil
}
