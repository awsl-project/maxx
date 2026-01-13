package antigravity

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/Bowl42/maxx-next/internal/event"
)

// OAuthSession 表示一个 OAuth 授权会话
type OAuthSession struct {
	State     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// OAuthResult 表示 OAuth 授权的结果
type OAuthResult struct {
	State        string     `json:"state"`        // 用于前端匹配会话
	Success      bool       `json:"success"`
	AccessToken  string     `json:"accessToken,omitempty"`
	RefreshToken string     `json:"refreshToken,omitempty"`
	Email        string     `json:"email,omitempty"`
	ProjectID    string     `json:"projectID,omitempty"`
	UserInfo     *UserInfo  `json:"userInfo,omitempty"`
	Quota        *QuotaData `json:"quota,omitempty"`
	Error        string     `json:"error,omitempty"`
}

// OAuthManager 管理 OAuth 授权会话
type OAuthManager struct {
	sessions    sync.Map          // state -> *OAuthSession
	broadcaster event.Broadcaster // 用于推送 OAuth 结果
	mu          sync.RWMutex
}

// NewOAuthManager 创建 OAuth 管理器
func NewOAuthManager(broadcaster event.Broadcaster) *OAuthManager {
	manager := &OAuthManager{
		broadcaster: broadcaster,
	}

	// 启动清理 goroutine
	go manager.cleanupExpired()

	return manager
}

// GenerateState 生成随机 state token
func (m *OAuthManager) GenerateState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// CreateSession 创建新的 OAuth 会话
func (m *OAuthManager) CreateSession(state string) *OAuthSession {
	session := &OAuthSession{
		State:     state,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute), // 5分钟超时
	}

	m.sessions.Store(state, session)
	return session
}

// GetSession 获取指定 state 的会话
func (m *OAuthManager) GetSession(state string) (*OAuthSession, bool) {
	val, ok := m.sessions.Load(state)
	if !ok {
		return nil, false
	}

	session, ok := val.(*OAuthSession)
	if !ok {
		return nil, false
	}

	// 检查是否过期
	if time.Now().After(session.ExpiresAt) {
		m.sessions.Delete(state)
		return nil, false
	}

	return session, true
}

// CompleteSession 完成 OAuth 会话并通过 WebSocket 推送结果
func (m *OAuthManager) CompleteSession(state string, result *OAuthResult) {
	// 确保 state 匹配
	result.State = state

	// 删除会话
	m.sessions.Delete(state)

	// 通过 broadcaster 推送结果
	if m.broadcaster != nil {
		m.broadcaster.BroadcastMessage("antigravity_oauth_result", result)
	}
}

// cleanupExpired 定期清理过期的会话
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
