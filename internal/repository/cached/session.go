package cached

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type sessionCacheKey struct {
	TenantID  uint64
	SessionID string
}

// SessionRepository caches session records around a backing repository.
//
// 三层读取顺序(写时三层全写,读时本地未命中向下回退):
//  1. 本地内存 cache(进程内最快)
//  2. coordinator KV(跨实例共享,Redis 时常态毫秒级)
//  3. 底层 DB repo(权威 source of truth)
//
// coord 为 nil 时退化为两层(本地 + DB),行为等价于原实现。
type SessionRepository struct {
	repo  repository.SessionRepository
	cache map[sessionCacheKey]*domain.Session
	mu    sync.RWMutex
	// coord 用 atomic.Pointer 存,读路径不加锁。SetCoordinator 仅在启动时调用一次。
	coord atomic.Pointer[coordinator.Coordinator]
	// coordTTL 是写入 KV 的过期时间。冷数据让 TTL 自然回收,避免遍历 KV。
	coordTTL time.Duration
}

func NewSessionRepository(repo repository.SessionRepository) *SessionRepository {
	return &SessionRepository{
		repo:  repo,
		cache: make(map[sessionCacheKey]*domain.Session),
	}
}

// SetCoordinator 注入 coordinator,启用跨实例 session 共享。
// ttl 控制 KV 条目过期时间。建议 1 小时左右——足够热数据复用,冷数据
// 由 TTL 自然回收,DB 仍然是权威源。ttl <= 0 时使用默认 1 小时。
func (r *SessionRepository) SetCoordinator(c coordinator.Coordinator, ttl time.Duration) {
	if c == nil {
		return
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	r.coordTTL = ttl
	r.coord.Store(&c)
}

func (r *SessionRepository) coordinator() coordinator.Coordinator {
	p := r.coord.Load()
	if p == nil {
		return nil
	}
	return *p
}

func sessionCoordKey(tenantID uint64, sessionID string) string {
	return fmt.Sprintf("session:%d:%s", tenantID, sessionID)
}

// coordIOTimeout 是单次 KV 操作的最大等待时间。
// 这条路径在 proxy 请求的热路径上,Redis 抖动不能拖累整个请求,
// 失败的代价仅仅是降级到 DB 读取。
const coordIOTimeout = 200 * time.Millisecond

// writeCoord 把 session 写到 coordinator KV,coord 未注入时 no-op。
// 失败仅记日志:DB 已经写过,KV 只是加速层,丢失不影响正确性。
func (r *SessionRepository) writeCoord(s *domain.Session) {
	c := r.coordinator()
	if c == nil || s == nil {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		log.Printf("[SessionCache] marshal failed: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), coordIOTimeout)
	defer cancel()
	if err := c.Set(ctx, sessionCoordKey(s.TenantID, s.SessionID), data, r.coordTTL); err != nil {
		log.Printf("[SessionCache] set %s failed: %v", sessionCoordKey(s.TenantID, s.SessionID), err)
	}
}

// readCoord 从 coordinator KV 读 session;未命中或失败均返回 nil。
// TenantID == TenantIDAll 时不查 KV(KV 按 TenantID 分键,无法做"任意租户"匹配),
// 让调用方走 DB 路径。
func (r *SessionRepository) readCoord(tenantID uint64, sessionID string) *domain.Session {
	c := r.coordinator()
	if c == nil || tenantID == domain.TenantIDAll {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), coordIOTimeout)
	defer cancel()
	data, err := c.Get(ctx, sessionCoordKey(tenantID, sessionID))
	if err != nil {
		if !errors.Is(err, coordinator.ErrNotFound) {
			log.Printf("[SessionCache] get %s failed: %v", sessionCoordKey(tenantID, sessionID), err)
		}
		return nil
	}
	var s domain.Session
	if err := json.Unmarshal(data, &s); err != nil {
		log.Printf("[SessionCache] unmarshal failed: %v", err)
		return nil
	}
	return &s
}

func (r *SessionRepository) Create(s *domain.Session) error {
	if err := r.repo.Create(s); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = cloneSession(s)
	r.mu.Unlock()
	r.writeCoord(s)
	return nil
}

func (r *SessionRepository) Update(s *domain.Session) error {
	if err := r.repo.Update(s); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = cloneSession(s)
	r.mu.Unlock()
	r.writeCoord(s)
	return nil
}

func (r *SessionRepository) Touch(tenantID uint64, sessionID string, touchedAt time.Time) error {
	if touchedAt.IsZero() {
		touchedAt = time.Now()
	}

	if err := r.repo.Touch(tenantID, sessionID, touchedAt); err != nil {
		return err
	}

	r.mu.Lock()
	var touched *domain.Session
	if tenantID == domain.TenantIDAll {
		for key, session := range r.cache {
			if key.SessionID == sessionID {
				session.UpdatedAt = touchedAt
				touched = cloneSession(session)
				break
			}
		}
	} else if session, ok := r.cache[sessionCacheKey{TenantID: tenantID, SessionID: sessionID}]; ok {
		session.UpdatedAt = touchedAt
		touched = cloneSession(session)
	}
	r.mu.Unlock()

	// Refresh KV so the new UpdatedAt + extended TTL is visible to peers.
	// 本地缓存未命中(touched==nil)时不更新 KV:这种情况说明本地从未见过这条
	// session(或被 DeleteOlderThan 清过),让下次读路径自然走 DB 回填即可。
	if touched != nil {
		r.writeCoord(touched)
	}
	return nil
}

func (r *SessionRepository) GetBySessionID(tenantID uint64, sessionID string) (*domain.Session, error) {
	r.mu.RLock()
	if tenantID == domain.TenantIDAll {
		for key, s := range r.cache {
			if key.SessionID == sessionID {
				clonedSession := cloneSession(s)
				r.mu.RUnlock()
				return clonedSession, nil
			}
		}
	} else if s, ok := r.cache[sessionCacheKey{TenantID: tenantID, SessionID: sessionID}]; ok {
		clonedSession := cloneSession(s)
		r.mu.RUnlock()
		return clonedSession, nil
	}
	r.mu.RUnlock()

	// 本地未命中 → 查 coordinator KV(跨实例共享层)
	if s := r.readCoord(tenantID, sessionID); s != nil {
		cachedSession := cloneSession(s)
		r.mu.Lock()
		r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = cachedSession
		r.mu.Unlock()
		return cloneSession(cachedSession), nil
	}

	s, err := r.repo.GetBySessionID(tenantID, sessionID)
	if err != nil {
		return nil, err
	}

	cachedSession := cloneSession(s)
	r.mu.Lock()
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = cachedSession
	r.mu.Unlock()
	r.writeCoord(s) // 回填 KV 让其他实例后续命中
	return cloneSession(cachedSession), nil
}

func (r *SessionRepository) GetOrCreate(tenantID uint64, sessionID string, clientType domain.ClientType) (*domain.Session, error) {
	r.mu.RLock()
	if tenantID == domain.TenantIDAll {
		for key, s := range r.cache {
			if key.SessionID == sessionID {
				clonedSession := cloneSession(s)
				r.mu.RUnlock()
				return clonedSession, nil
			}
		}
	} else if s, ok := r.cache[sessionCacheKey{TenantID: tenantID, SessionID: sessionID}]; ok {
		clonedSession := cloneSession(s)
		r.mu.RUnlock()
		return clonedSession, nil
	}
	r.mu.RUnlock()

	// 本地未命中 → 查 coordinator KV
	if found := r.readCoord(tenantID, sessionID); found != nil {
		cachedSession := cloneSession(found)
		r.mu.Lock()
		r.cache[sessionCacheKey{TenantID: found.TenantID, SessionID: found.SessionID}] = cachedSession
		r.mu.Unlock()
		return cloneSession(cachedSession), nil
	}

	s, err := r.repo.GetBySessionID(tenantID, sessionID)
	if err == nil {
		cachedSession := cloneSession(s)
		r.mu.Lock()
		r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = cachedSession
		r.mu.Unlock()
		r.writeCoord(s)
		return cloneSession(cachedSession), nil
	}

	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	// Reject creation with TenantIDAll — it would store TenantID=0
	if tenantID == domain.TenantIDAll {
		return nil, domain.ErrNotFound
	}

	s = &domain.Session{
		TenantID:   tenantID,
		SessionID:  sessionID,
		ClientType: clientType,
		ProjectID:  0,
	}
	if err := r.repo.Create(s); err != nil {
		return nil, err
	}

	cachedSession := cloneSession(s)
	r.mu.Lock()
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = cachedSession
	r.mu.Unlock()
	r.writeCoord(s)
	return cloneSession(cachedSession), nil
}

func (r *SessionRepository) List(tenantID uint64) ([]*domain.Session, error) {
	return r.repo.List(tenantID)
}

func (r *SessionRepository) DeleteOlderThan(before time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	deleted, err := r.repo.DeleteOlderThan(before)
	if err != nil {
		return 0, err
	}
	if deleted > 0 {
		r.cache = make(map[sessionCacheKey]*domain.Session)
	}
	return deleted, nil
}

func cloneSession(session *domain.Session) *domain.Session {
	if session == nil {
		return nil
	}

	clone := *session
	if session.DeletedAt != nil {
		deletedAt := *session.DeletedAt
		clone.DeletedAt = &deletedAt
	}
	if session.RejectedAt != nil {
		rejectedAt := *session.RejectedAt
		clone.RejectedAt = &rejectedAt
	}
	return &clone
}
