package workbuddyai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/awsl-project/maxx/internal/domain"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const Endpoint = "https://www.workbuddy.ai"
const UserAgent = "WorkBuddy/5.5.6"

type Config = domain.ProviderConfigWorkBuddyAI
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}
type renewal struct {
	done chan struct{}
	next Config
	err  error
	at   time.Time
}

var globalRenewals = struct {
	mu      sync.Mutex
	results map[string]*renewal
}{results: map[string]*renewal{}}

type Client struct {
	endpoint string
	http     HTTPClient
}

func NewClient(endpoint string, h HTTPClient) *Client {
	if endpoint == "" {
		endpoint = Endpoint
	}
	if h == nil {
		h = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{endpoint: strings.TrimRight(endpoint, "/"), http: h}
}

var DefaultClient = NewClient("", nil)

type APIError struct {
	Code   int
	Status int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("WorkBuddy request failed (HTTP %d, code %d)", e.Status, e.Code)
}
func Pending(err error, code int) bool {
	e, ok := err.(*APIError)
	return ok && (e.Code == code || e.Status == 408 || e.Status == 429)
}
func (c *Client) call(ctx context.Context, method, path string, h http.Header, out any) error {
	var body io.Reader
	if method == "POST" {
		body = bytes.NewBufferString("{}")
	}
	r, e := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if e != nil {
		return e
	}
	r.Header = h.Clone()
	if r.Header == nil {
		r.Header = make(http.Header)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	if r.Header.Get("User-Agent") == "" {
		r.Header.Set("User-Agent", UserAgent)
	}
	res, e := c.http.Do(r)
	if e != nil {
		return fmt.Errorf("WorkBuddy transport: %w", e)
	}
	defer res.Body.Close()
	var env struct {
		Code json.Number
		Data json.RawMessage
	}
	e = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&env)
	n, _ := env.Code.Int64()
	if res.StatusCode < 200 || res.StatusCode >= 300 || n != 0 {
		return &APIError{int(n), res.StatusCode}
	}
	if e != nil {
		return fmt.Errorf("WorkBuddy invalid response")
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if json.Unmarshal(env.Data, out) != nil {
		return fmt.Errorf("WorkBuddy invalid data")
	}
	return nil
}
func domainOf(a Config) string {
	if a.Domain != "" {
		return a.Domain
	}
	return "www.workbuddy.ai"
}
func enterprise(h http.Header, a Config) {
	h.Del("X-Enterprise-Id")
	h.Del("X-Tenant-Id")
	if id := strings.TrimSpace(a.EnterpriseID); id != "" {
		h.Set("X-Enterprise-Id", id)
		h.Set("X-Tenant-Id", id)
	}
}
func SignHeaders(h http.Header, a Config) {
	for k, v := range map[string]string{"Authorization": "Bearer " + a.AccessToken, "X-User-Id": a.UID, "X-Domain": domainOf(a), "X-Product": "SaaS", "X-IDE-Type": "WorkBuddy", "User-Agent": UserAgent, "X-Requested-With": "XMLHttpRequest", "X-Agent-Intent": "craft", "X-Agent-Purpose": "conversation", "X-IDE-Name": "WorkBuddy", "X-IDE-Version": "5.5.6", "X-Agent-Type": "main"} {
		h.Set(k, v)
	}
	enterprise(h, a)
}

type token struct {
	AccessToken      string
	RefreshToken     string
	Domain           string
	TokenType        string
	ExpiresAt        int64
	ExpiresIn        int64
	RefreshExpiresAt int64
	RefreshExpiresIn int64
}

func merge(a Config, t token) Config {
	a.AccessToken = t.AccessToken
	if t.RefreshToken != "" {
		a.RefreshToken = t.RefreshToken
	}
	if t.Domain != "" {
		a.Domain = t.Domain
	}
	if t.TokenType != "" {
		a.TokenType = t.TokenType
	}
	if t.ExpiresAt > 0 {
		a.ExpiresAt = t.ExpiresAt
	} else if t.ExpiresIn > 0 {
		a.ExpiresAt = time.Now().UnixMilli() + t.ExpiresIn*1000
	}
	if t.RefreshExpiresAt > 0 {
		a.RefreshExpiresAt = t.RefreshExpiresAt
	} else if t.RefreshExpiresIn > 0 {
		a.RefreshExpiresAt = time.Now().UnixMilli() + t.RefreshExpiresIn*1000
	}
	return a
}

// Refresh shares renewal and reuses old-token results for one minute. Caller cancellation does not cancel other waiters.
func (c *Client) Refresh(ctx context.Context, a Config) (Config, error) {
	if e := ctx.Err(); e != nil {
		return a, e
	}
	if a.RefreshToken == "" || (a.RefreshExpiresAt > 0 && time.Now().UnixMilli() >= a.RefreshExpiresAt) {
		return a, fmt.Errorf("WorkBuddy account is signed out; sign in again")
	}
	// A renewal is authorized for its domain, user and enterprise, not just
	// its refresh token. Hash the tuple to avoid retaining raw tokens as keys.
	scope, _ := json.Marshal([]string{c.endpoint, domainOf(a), a.UID, strings.TrimSpace(a.EnterpriseID), a.RefreshToken})
	key := fmt.Sprintf("%x", sha256.Sum256(scope))
	globalRenewals.mu.Lock()
	r := globalRenewals.results[key]
	if r != nil && !r.at.IsZero() && time.Since(r.at) >= time.Minute {
		delete(globalRenewals.results, key)
		r = nil
	}
	if r == nil {
		r = &renewal{done: make(chan struct{})}
		globalRenewals.results[key] = r
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h := http.Header{"X-Refresh-Token": {a.RefreshToken}, "X-Auth-Refresh-Source": {"plugin"}, "X-Domain": {domainOf(a)}}
			enterprise(h, a)
			var t token
			e := c.call(ctx, "POST", "/v2/plugin/auth/token/refresh", h, &t)
			if e == nil && t.AccessToken == "" {
				e = fmt.Errorf("WorkBuddy gave no refreshed token")
			}
			globalRenewals.mu.Lock()
			r.next = merge(a, t)
			r.err = e
			r.at = time.Now()
			if e != nil {
				delete(globalRenewals.results, key)
			}
			close(r.done)
			for k, v := range globalRenewals.results {
				if !v.at.IsZero() && time.Since(v.at) >= time.Minute {
					delete(globalRenewals.results, k)
				}
			}
			globalRenewals.mu.Unlock()
		}()
	}
	globalRenewals.mu.Unlock()
	select {
	case <-ctx.Done():
		return a, ctx.Err()
	case <-r.done:
		next := r.next
		next.UID = a.UID
		next.EnterpriseID = a.EnterpriseID
		return next, r.err
	}
}

// Fresh calls save before returning rotated credentials. Persistence failures are not swallowed.
func (c *Client) Fresh(ctx context.Context, a Config, save func(context.Context, Config) error) (Config, error) {
	if a.AccessToken != "" && (a.ExpiresAt == 0 || time.Now().UnixMilli() < a.ExpiresAt-60000) {
		return a, nil
	}
	n, e := c.Refresh(ctx, a)
	if e != nil {
		if ctx.Err() != nil {
			return a, ctx.Err()
		}
		if a.AccessToken != "" && (a.ExpiresAt == 0 || time.Now().UnixMilli() < a.ExpiresAt) {
			return a, nil
		}
		return a, e
	}
	if save != nil {
		if e = save(ctx, n); e != nil {
			return n, e
		}
	}
	return n, nil
}

type StartResult struct {
	State     string `json:"state"`
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (c *Client) Start(ctx context.Context, id string) (*StartResult, error) {
	h := make(http.Header)
	for _, k := range []string{"X-No-Authorization", "X-No-User-Id", "X-No-Enterprise-Id", "X-No-Department-Info"} {
		h.Set(k, "true")
	}
	var s struct {
		State   string
		AuthURL string
	}
	if e := c.call(ctx, "POST", "/v2/plugin/auth/state?platform=workbuddy-ai", h, &s); e != nil {
		return nil, e
	}
	u, e := url.Parse(s.AuthURL)
	if e != nil || u.Scheme != "https" || u.Host == "" || s.State == "" {
		return nil, fmt.Errorf("WorkBuddy gave no secure sign-in page")
	}
	q := u.Query()
	q.Set("version", "2.0.0")
	q.Set("loginSessionId", id)
	u.RawQuery = q.Encode()
	return &StartResult{s.State, u.String(), time.Now().Add(5 * time.Minute).UnixMilli()}, nil
}

type Account struct {
	UID          string
	Type         string
	EnterpriseID string
	LastLogin    bool
}

func (c *Client) Poll(ctx context.Context, state string, a Config) (Config, bool, error) {
	q := "?state=" + url.QueryEscape(state)
	if a.AccessToken == "" {
		var t token
		e := c.call(ctx, "GET", "/v2/plugin/auth/token"+q, nil, &t)
		if Pending(e, 11217) {
			return a, false, nil
		}
		if e != nil {
			return a, false, e
		}
		if t.AccessToken == "" {
			return a, false, nil
		}
		a = merge(a, t)
	}
	h := http.Header{"Authorization": {"Bearer " + a.AccessToken}, "X-Domain": {domainOf(a)}, "X-No-User-Id": {"true"}, "X-No-Enterprise-Id": {"true"}}
	var who Account
	e := c.call(ctx, "GET", "/v2/plugin/login/account"+q, h, &who)
	if Pending(e, 12151) {
		return a, false, nil
	}
	if e != nil {
		return a, false, e
	}
	if who.UID == "" {
		return a, false, nil
	}
	a.UID = who.UID
	a.EnterpriseID = strings.TrimSpace(who.EnterpriseID)
	typ := strings.ToLower(strings.TrimSpace(who.Type))
	if a.EnterpriseID == "" && typ != "" && typ != "personal" {
		var list struct{ Accounts []Account }
		if c.call(ctx, "GET", "/v2/plugin/accounts", http.Header{"Authorization": {"Bearer " + a.AccessToken}, "X-Domain": {domainOf(a)}}, &list) == nil {
			var mine, same, last []Account
			for _, x := range list.Accounts {
				if x.UID != who.UID || strings.TrimSpace(x.EnterpriseID) == "" {
					continue
				}
				mine = append(mine, x)
				if strings.ToLower(strings.TrimSpace(x.Type)) == typ {
					same = append(same, x)
				}
				if x.LastLogin {
					last = append(last, x)
				}
			}
			var pick []Account
			if len(same) == 1 {
				pick = same
			} else if len(last) == 1 {
				pick = last
			} else if len(mine) == 1 {
				pick = mine
			}
			if len(pick) == 1 {
				a.EnterpriseID = strings.TrimSpace(pick[0].EnterpriseID)
			}
		}
	}
	return a, true, nil
}

type Model struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Context int64  `json:"context,omitempty"`
	Output  int64  `json:"output,omitempty"`
	Images  bool   `json:"images"`
}

func (c *Client) Models(ctx context.Context, a Config) ([]Model, error) {
	h := make(http.Header)
	SignHeaders(h, a)
	h.Set("User-Agent", "CLI/2.0.0 "+UserAgent)
	var cfg struct {
		Agents []struct {
			Name   string
			Models []string
		}
		Models []struct {
			ID              string
			Name            string
			MaxInputTokens  int64
			MaxOutputTokens int64
			SupportsImages  bool
		}
	}
	if e := c.call(ctx, "GET", "/v3/config", h, &cfg); e != nil {
		return nil, e
	}
	out := []Model{}
	seen := map[string]bool{}
	for _, agent := range cfg.Agents {
		if agent.Name != "cli" {
			continue
		}
		for _, id := range agent.Models {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			m := Model{ID: id, Name: id}
			for _, d := range cfg.Models {
				if d.ID == id {
					if d.Name != "" {
						m.Name = d.Name
					}
					m.Context = d.MaxInputTokens
					m.Output = d.MaxOutputTokens
					m.Images = d.SupportsImages && !strings.HasPrefix(id, "glm-")
					break
				}
			}
			out = append(out, m)
		}
	}
	return out, nil
}
