package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/awsl-project/maxx/internal/adapter/provider/workbuddyai"
	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"net/http"
	"strings"
	"sync"
	"time"
)

type workBuddySession struct {
	mu       sync.Mutex
	state    string
	owner    string
	expires  int64
	next     time.Time
	config   workbuddyai.Config
	complete bool
}
type WorkBuddyAIHandler struct {
	client   *workbuddyai.Client
	mu       sync.Mutex
	sessions map[string]*workBuddySession
	admin    interface {
		GetProvider(uint64, uint64) (*domain.Provider, error)
		UpdateProvider(uint64, *domain.Provider) error
	}
}

func NewWorkBuddyAIHandler(clients ...*workbuddyai.Client) *WorkBuddyAIHandler {
	c := workbuddyai.DefaultClient
	if len(clients) > 0 && clients[0] != nil {
		c = clients[0]
	}
	return &WorkBuddyAIHandler{client: c, sessions: map[string]*workBuddySession{}}
}
func (h *WorkBuddyAIHandler) SetAdminService(s interface {
	GetProvider(uint64, uint64) (*domain.Provider, error)
	UpdateProvider(uint64, *domain.Provider) error
}) *WorkBuddyAIHandler { h.admin = s; return h }
func RegisterWorkBuddyAIRoutes(mux *http.ServeMux, wrap func(http.Handler) http.Handler, h *WorkBuddyAIHandler) {
	mux.Handle("/api/admin/workbuddy-ai/", wrap(h))
}
func (h *WorkBuddyAIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if maxxctx.GetUserRole(r.Context()) != string(domain.UserRoleAdmin) {
		http.Error(w, "forbidden", 403)
		return
	}
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", 405)
		return
	}
	owner := fmt.Sprintf("%d/%d", maxxctx.GetTenantID(r.Context()), maxxctx.GetUserID(r.Context()))
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/workbuddy-ai")
	switch path {
	case "/auth/start":
		id, e := randomURLSafe(24)
		if e != nil {
			http.Error(w, "cannot create session", 500)
			return
		}
		h.mu.Lock()
		for k, s := range h.sessions {
			if s.expires <= time.Now().UnixMilli() {
				delete(h.sessions, k)
			}
		}
		count := 0
		for _, s := range h.sessions {
			if s.owner == owner {
				count++
			}
		}
		if len(h.sessions) >= 256 || count >= 8 {
			h.mu.Unlock()
			http.Error(w, "too many login sessions", 429)
			return
		}
		s := &workBuddySession{owner: owner, expires: time.Now().Add(5 * time.Minute).UnixMilli()}
		h.sessions[id] = s
		h.mu.Unlock()
		result, e := h.client.Start(r.Context(), id)
		if e != nil {
			h.mu.Lock()
			delete(h.sessions, id)
			h.mu.Unlock()
			http.Error(w, "WorkBuddy login start failed", 502)
			return
		}
		s.mu.Lock()
		s.state = result.State
		s.mu.Unlock()
		result.State = id
		json.NewEncoder(w).Encode(result)
	case "/auth/poll":
		var req struct{ State string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		h.mu.Lock()
		s := h.sessions[req.State]
		h.mu.Unlock()
		if s == nil || s.owner != owner {
			http.Error(w, "invalid login session", 400)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.expires <= time.Now().UnixMilli() {
			json.NewEncoder(w).Encode(map[string]any{"status": "expired"})
			return
		}
		if s.complete {
			json.NewEncoder(w).Encode(map[string]any{"status": "authorized", "config": s.config})
			return
		}
		if s.state == "" || time.Now().Before(s.next) {
			json.NewEncoder(w).Encode(map[string]any{"status": "pending"})
			return
		}
		s.next = time.Now().Add(time.Second)
		a, done, e := h.client.Poll(r.Context(), s.state, s.config)
		s.config = a
		if e != nil {
			http.Error(w, "WorkBuddy login polling failed", 502)
			return
		}
		s.complete = done
		if done {
			json.NewEncoder(w).Encode(map[string]any{"status": "authorized", "config": a})
		} else {
			json.NewEncoder(w).Encode(map[string]any{"status": "pending"})
		}
	case "/models":
		var req struct {
			workbuddyai.Config
			ProviderID uint64 `json:"providerID"`
		}
		var a workbuddyai.Config
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&req) != nil {
			http.Error(w, "invalid configuration", 400)
			return
		}
		a = req.Config
		var saved *domain.Provider
		if req.ProviderID != 0 {
			if h.admin == nil {
				http.Error(w, "provider lookup unavailable", 503)
				return
			}
			var err error
			saved, err = h.admin.GetProvider(maxxctx.GetTenantID(r.Context()), req.ProviderID)
			if err != nil || saved == nil || saved.Type != "workbuddy-ai" || saved.Config == nil || saved.Config.WorkBuddyAI == nil {
				http.Error(w, "provider not found", 404)
				return
			}
			a = *saved.Config.WorkBuddyAI
		}
		if a.AccessToken == "" {
			http.Error(w, "access token required", 400)
			return
		}
		a, e := h.client.Fresh(r.Context(), a, func(ctx context.Context, next workbuddyai.Config) error {
			if saved == nil {
				return nil
			}
			cp := *saved
			cfg := *saved.Config
			cfg.WorkBuddyAI = &next
			cp.Config = &cfg
			return h.admin.UpdateProvider(maxxctx.GetTenantID(ctx), &cp)
		})
		if e != nil {
			http.Error(w, "WorkBuddy token refresh failed", 502)
			return
		}
		models, e := h.client.Models(r.Context(), a)
		if e != nil {
			http.Error(w, "WorkBuddy model discovery failed", 502)
			return
		}
		ids := make([]string, 0, len(models))
		for _, model := range models {
			ids = append(ids, model.ID)
		}
		json.NewEncoder(w).Encode(map[string]any{"models": ids, "config": a})
	default:
		http.NotFound(w, r)
	}
}
