package workbuddyai

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/usage"
)

func init() { provider.RegisterAdapterFactory("workbuddy-ai", NewAdapter) }

// Adapter speaks OpenAI chat only; the executor owns cross-protocol conversion.
type Adapter struct {
	client         *http.Client
	endpoint       string
	auth           *Client
	mu             sync.Mutex
	providerUpdate func(*domain.Provider) error
	providerReload func() (*domain.Provider, error)
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.WorkBuddyAI == nil {
		return nil, errors.New("missing WorkBuddy AI credentials")
	}
	client, err := provider.NewHTTPClient(p, 0, true)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: client, endpoint: "https://www.workbuddy.ai"}, nil
}
func (a *Adapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *Adapter) Execute(c *flow.Ctx, p *domain.Provider) error {
	if flow.GetClientType(c) != domain.ClientTypeOpenAI {
		return upstreamError("unsupported WorkBuddy protocol", 400)
	}
	path := strings.Split(flow.GetRequestURI(c), "?")[0]
	if path != "/v1/chat/completions" && path != "/chat/completions" {
		return upstreamError("WorkBuddy AI supports chat completions only", 400)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(flow.GetRequestBody(c), &body); err != nil {
		return upstreamError("invalid chat request", 400)
	}
	stream := string(body["stream"]) == "true"
	body["stream"] = json.RawMessage("true")
	if model := flow.GetMappedModel(c); model != "" {
		body["model"], _ = json.Marshal(model)
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	cfg := p.Config.WorkBuddyAI
	// Authentication refresh/persistence is shared with the login client.
	cfg, err = a.freshCredentials(c.Request.Context(), p)
	if err != nil {
		return upstreamError(err.Error(), 401)
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, a.endpoint+"/v2/chat/completions", bytes.NewReader(data))
	if err != nil {
		return err
	}
	SignHeaders(req.Header, *cfg)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	attend(req.Header, body, flow.GetSessionID(c))
	resp, err := a.client.Do(req)
	if err != nil {
		return &domain.ProxyError{Err: err, Retryable: false}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return readUpstreamError(resp.Body, resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readUpstreamError(resp.Body, 502)
	}
	collector := usage.StreamCollector{}
	defer func() {
		if m := collector.Metrics; m != nil {
			flow.GetEventChan(c).SendMetrics(&domain.AdapterMetrics{InputTokens: m.InputTokens, OutputTokens: m.OutputTokens, CacheReadCount: m.CacheReadCount, CacheCreationCount: m.CacheCreationCount})
		}
	}()
	aggregate := newChatAggregate()
	committed := false
	firstToken := false
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	var event strings.Builder
	process := func(payload string) (bool, error) {
		if payload == "[DONE]" {
			if stream {
				if !committed {
					c.Writer.Header().Set("Content-Type", "text/event-stream")
					committed = true
				}
				if _, err := io.WriteString(c.Writer, "data: [DONE]\n\n"); err != nil {
					return false, err
				}
				if f, ok := c.Writer.(http.Flusher); ok {
					f.Flush()
				}
			}
			return true, nil
		}
		var frame map[string]json.RawMessage
		if err := json.Unmarshal([]byte(payload), &frame); err != nil {
			return false, upstreamError("invalid WorkBuddy SSE frame", 502)
		}
		if _, ok := frame["error"]; ok {
			return false, readUpstreamError(strings.NewReader(payload), 502)
		}
		if code, ok := frame["code"]; ok && string(code) != "0" && string(code) != "null" {
			return false, readUpstreamError(strings.NewReader(payload), 502)
		}
		collector.ProcessSSEPayload([]byte(payload))
		if !stream {
			if err := aggregate.add(frame); err != nil {
				return false, err
			}
		}
		if model := frame["model"]; model != nil {
			var m string
			_ = json.Unmarshal(model, &m)
			flow.GetEventChan(c).SendResponseModel(m)
		}
		if !firstToken {
			flow.GetEventChan(c).SendFirstToken(time.Now().UnixMilli())
			firstToken = true
		}
		if stream {
			if !committed {
				c.Writer.Header().Set("Content-Type", "text/event-stream")
				c.Writer.Header().Set("Cache-Control", "no-cache")
				c.Writer.Header().Set("X-Accel-Buffering", "no")
				committed = true
			}
			if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
				return false, err
			}
			if f, ok := c.Writer.(http.Flusher); ok {
				f.Flush()
			}
		}
		return false, nil
	}
	done := false
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if event.Len() > 0 {
				done, err = process(strings.TrimSuffix(event.String(), "\n"))
				event.Reset()
				if err != nil {
					return err
				}
				if done {
					break
				}
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			event.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			event.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return &domain.ProxyError{Err: err, Retryable: false}
	}
	if !done {
		return upstreamError("WorkBuddy stream ended before [DONE]", 502)
	}
	if !stream {
		out, err := aggregate.finish()
		if err != nil {
			return err
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		_, err = c.Writer.Write(out)
		return err
	}
	return nil
}
func upstreamError(message string, status int) *domain.ProxyError {
	return &domain.ProxyError{Err: errors.New(message), HTTPStatusCode: status, Retryable: false, Scope: domain.ScopeRequest}
}
func readUpstreamError(r io.Reader, status int) error {
	b, _ := io.ReadAll(io.LimitReader(r, 64<<10))
	var v struct {
		Code    json.RawMessage `json:"code"`
		Msg     string          `json:"msg"`
		Message string          `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &v)
	msg := v.Msg
	if msg == "" {
		msg = v.Message
	}
	if msg == "" {
		msg = v.Error.Message
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	return upstreamError(fmt.Sprintf("WorkBuddy AI upstream HTTP %d, code %s: %s", status, string(v.Code), msg), status)
}

var conversationSalt = randomID()

func randomID() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func hashedID(kind, key string) string {
	s := sha256.Sum256([]byte(conversationSalt + "|" + kind + "|" + key))
	return hex.EncodeToString(s[:16])
}
func attend(h http.Header, body map[string]json.RawMessage, session string) {
	var messages []struct {
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		ToolCalls json.RawMessage `json:"tool_calls"`
	}
	_ = json.Unmarshal(body["messages"], &messages)
	conv := ""
	turn := ""
	if session != "" {
		conv = "session:" + session
	}
	for i, m := range messages {
		if m.Role != "user" {
			continue
		}
		if conv == "" {
			conv = "first:" + string(m.Content)
		}
		if i > 0 && (messages[i-1].Role == "tool" || (messages[i-1].Role == "assistant" && len(messages[i-1].ToolCalls) > 2)) {
			continue
		}
		turn = fmt.Sprintf("u%d:%s", i, m.Content)
	}
	msg := randomID()
	h.Set("X-Conversation-Message-ID", msg)
	h.Set("X-Request-ID", msg)
	if conv != "" {
		h.Set("X-Conversation-ID", hashedID("conversation", conv))
	}
	if turn != "" {
		h.Set("X-Conversation-Request-ID", hashedID("turn", conv+"|"+turn))
	} else {
		h.Set("X-Conversation-Request-ID", randomID())
	}
}

func (a *Adapter) SetProviderUpdateFunc(fn func(*domain.Provider) error)     { a.providerUpdate = fn }
func (a *Adapter) SetProviderReloadFunc(fn func() (*domain.Provider, error)) { a.providerReload = fn }
func (a *Adapter) freshCredentials(ctx context.Context, p *domain.Provider) (*Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.providerReload != nil {
		latest, err := a.providerReload()
		if err != nil {
			return nil, err
		}
		if latest != nil {
			p = latest
		}
	}
	if p == nil || p.Config == nil || p.Config.WorkBuddyAI == nil {
		return nil, errors.New("missing WorkBuddy credentials")
	}
	if a.auth == nil {
		a.auth = NewClient(a.endpoint, a.client)
	}
	next, err := a.auth.Fresh(ctx, *p.Config.WorkBuddyAI, func(ctx context.Context, next Config) error {
		if a.providerUpdate == nil {
			return errors.New("cannot persist refreshed WorkBuddy credentials: provider updater is not configured")
		}
		cp := *p
		cfg := *p.Config
		cfg.WorkBuddyAI = &next
		cp.Config = &cfg
		return a.providerUpdate(&cp)
	})
	return &next, err
}

// NewAdapterWithClient permits embedding and deterministic upstream integration tests.
func NewAdapterWithClient(p *domain.Provider, endpoint string, client *http.Client) (provider.ProviderAdapter, error) {
	if p == nil || p.Config == nil || p.Config.WorkBuddyAI == nil {
		return nil, errors.New("missing WorkBuddy AI credentials")
	}
	if client == nil {
		return nil, errors.New("missing WorkBuddy HTTP client")
	}
	return &Adapter{client: client, endpoint: strings.TrimRight(endpoint, "/")}, nil
}
