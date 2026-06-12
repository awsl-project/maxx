package cooldown

import (
	"context"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

// Manager manages provider cooldown states.
//
// 分布式架构:
//   - 本地 cooldowns/reasons map 是热路径快查
//   - store (atomic) 在 distributed 模式下是 Redis,在 standalone 是 memory
//     —— 充当跨实例真值
//   - 每个 provider 有一个 generation 计数器,变化时其他实例通过
//     ListByProvider 全量重载该 provider 的本地条目
//   - coord (atomic) 仅用于 publish/subscribe 事件;丢事件不致命,
//     节流的 syncProviderGeneration 会最终收敛
//
// 公共方法签名一律不变(没有 ctx / 没有 error),以避免调用方大改。
// 内部实现的 context 都是 context.Background(),错误只 log。
type Manager struct {
	mu             sync.RWMutex
	cooldowns      map[CooldownKey]time.Time         // cooldown key -> end time
	reasons        map[CooldownKey]CooldownReason    // cooldown key -> reason
	failureTracker *FailureTracker                   // tracks failure counts
	policies       map[CooldownReason]CooldownPolicy // cooldown calculation strategies
	repository     repository.CooldownRepository

	// coord 和 store 都用 atomic.Pointer 存,使得 broadcast/store 调用可以在
	// 持有 m.mu 的路径内执行而不引入嵌套锁依赖。
	coord atomic.Pointer[coordinator.Coordinator]
	store atomic.Pointer[CooldownStore]

	// providerGen 记录本实例最近一次知道的每 provider 的 generation。
	// 收到事件或主动 syncProviderGeneration 后更新。读写均在 m.mu 内。
	providerGen  map[uint64]int64
	lastGenSync  map[uint64]time.Time
	genSyncEvery time.Duration
}

// NewManager creates a new cooldown manager
func NewManager() *Manager {
	return &Manager{
		cooldowns:      make(map[CooldownKey]time.Time),
		reasons:        make(map[CooldownKey]CooldownReason),
		failureTracker: NewFailureTracker(),
		policies:       DefaultPolicies(),
		providerGen:    make(map[uint64]int64),
		lastGenSync:    make(map[uint64]time.Time),
		genSyncEvery:   2 * time.Second, // RFC 默认值,SetCoordinator 时可覆盖
	}
}

// Default global manager
var defaultManager = NewManager()

// Default returns the default global cooldown manager
func Default() *Manager {
	return defaultManager
}

// SetRepository sets the repository for cooldown persistence
func (m *Manager) SetRepository(repo repository.CooldownRepository) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repository = repo
}

// SetFailureCountRepository sets the repository for failure count persistence
func (m *Manager) SetFailureCountRepository(repo repository.FailureCountRepository) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureTracker.SetRepository(repo)
}

// LoadFromDatabase loads all active cooldowns and failure counts from database into memory
func (m *Manager) LoadFromDatabase() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load cooldowns
	if m.repository != nil {
		cooldowns, err := m.repository.GetAll()
		if err != nil {
			return err
		}

		m.cooldowns = make(map[CooldownKey]time.Time)
		m.reasons = make(map[CooldownKey]CooldownReason)
		for _, cd := range cooldowns {
			key := CooldownKey{
				ProviderID: cd.ProviderID,
				ClientType: cd.ClientType,
				Model:      cd.Model,
			}
			m.cooldowns[key] = cd.UntilTime
			m.reasons[key] = CooldownReason(cd.Reason)
		}

		log.Printf("[Cooldown] Loaded %d cooldowns from database", len(cooldowns))
	}

	// Load failure counts
	if err := m.failureTracker.LoadFromDatabase(); err != nil {
		log.Printf("[Cooldown] Warning: Failed to load failure counts: %v", err)
	}

	return nil
}

// RecordFailure records a failure and applies cooldown based on the reason, scope, and policy.
// If explicitUntil is provided, it will be used directly (e.g., from Retry-After header).
// Otherwise, the cooldown duration is calculated using the policy for the given reason.
// The scope determines which cooldown key dimensions are used:
//   - ScopeRequest: no cooldown recorded (returns zero time)
//   - ScopeModel: key uses (providerID, clientType, model)
//   - ScopeKey/ScopeEndpoint: key uses (providerID, clientType, "")
//   - ScopeProvider: key uses (providerID, "", "")
//
// Returns the calculated cooldown end time.
func (m *Manager) RecordFailure(providerID uint64, clientType string, model string, reason CooldownReason, scope domain.ErrorScope, explicitUntil *time.Time) time.Time {
	// ScopeRequest: only this request is bad, no cooldown needed
	if scope == domain.ScopeRequest {
		return time.Time{}
	}

	// Determine the effective key dimensions based on scope
	effectiveClientType := clientType
	effectiveModel := model
	switch scope {
	case domain.ScopeModel:
		// key uses (providerID, clientType, model) — keep all dimensions
	case domain.ScopeKey, domain.ScopeEndpoint:
		// key uses (providerID, clientType, "") — clear model
		effectiveModel = ""
	case domain.ScopeProvider:
		// key uses (providerID, "", "") — clear both
		effectiveClientType = ""
		effectiveModel = ""
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// If explicit until time is provided (e.g., from 429 Retry-After), use it directly
	if explicitUntil != nil {
		// 显式覆盖:走 Set (无条件)
		m.setCooldownLocked(providerID, effectiveClientType, effectiveModel, *explicitUntil, reason, false)
		log.Printf("[Cooldown] Provider %d (clientType=%s, model=%s): Set explicit cooldown until %s (reason=%s, scope=%s)",
			providerID, clientType, model, explicitUntil.Format("2006-01-02 15:04:05"), reason, scope)
		return *explicitUntil
	}

	// Otherwise, calculate cooldown based on policy and failure count
	// Increment failure count (always track at the model level for accurate counting)
	failureCount := m.failureTracker.IncrementFailure(providerID, effectiveClientType, effectiveModel, reason)

	// Get policy for this reason
	policy, ok := m.policies[reason]
	if !ok {
		// Fallback to fixed 5-second cooldown if no policy found
		policy = &FixedDurationPolicy{Duration: 5 * time.Second}
		log.Printf("[Cooldown] Warning: No policy found for reason=%s, using default 5-second cooldown", reason)
	}

	// Calculate cooldown duration
	duration := policy.CalculateCooldown(failureCount)
	until := time.Now().Add(duration)

	// 策略计算的失败 cooldown:走 SetIfLater,避免覆盖另一实例刚设的更晚 cooldown
	m.setCooldownLocked(providerID, effectiveClientType, effectiveModel, until, reason, true)

	log.Printf("[Cooldown] Provider %d (clientType=%s, model=%s): Set cooldown for %v until %s (reason=%s, scope=%s, failureCount=%d)",
		providerID, clientType, model, duration, until.Format("2006-01-02 15:04:05"), reason, scope, failureCount)

	return until
}

// UpdateCooldown updates cooldown time without incrementing failure count
// This is used for async updates (e.g., when quota reset time is fetched asynchronously)
// Keeps the existing reason
func (m *Manager) UpdateCooldown(providerID uint64, clientType string, model string, until time.Time) {
	if !until.After(time.Now()) {
		// 已过期或刚好等于现在,不写入 store/本地;直接相当于 clear。
		m.ClearCooldown(providerID, clientType, model)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get existing reason or use Unknown
	key := CooldownKey{ProviderID: providerID, ClientType: clientType, Model: model}
	reason, ok := m.reasons[key]
	if !ok {
		reason = ReasonUnknown
	}

	m.setCooldownLocked(providerID, clientType, model, until, reason, false)
	log.Printf("[Cooldown] Provider %d (clientType=%s, model=%s): Updated cooldown to %s (async update, no count increment)",
		providerID, clientType, model, until.Format("2006-01-02 15:04:05"))
}

// RecordSuccess records a successful request and clears the model-level cooldown.
// Only clears the specific (providerID, clientType, model) cooldown entry.
// Key/provider level cooldowns are NOT auto-cleared — they have their own expiry.
func (m *Manager) RecordSuccess(providerID uint64, clientType string, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := CooldownKey{ProviderID: providerID, ClientType: clientType, Model: model}

	// Step 1: Redis store (distributed truth) — 必须先成功才能推进本地/DB。
	// 失败时和 setCooldownLocked 的失败分支对齐:不动本地、不动 DB、不 bump。
	// 否则 Redis 真值仍保留旧 cooldown,但本实例已经放行 + 其他实例 reload 又会
	// 把它们的本地状态拉回到旧值,造成跨实例分叉。
	if sp := m.store.Load(); sp != nil {
		ctx, cancel := context.WithTimeout(context.Background(), cooldownIOTimeout)
		err := (*sp).Delete(ctx, key)
		cancel()
		if err != nil {
			log.Printf("[Cooldown] store Delete failed for provider %d, clientType=%s, model=%s: %v (skipping local/DB/publish to avoid divergence)",
				providerID, clientType, model, err)
			// 让下次 IsInCooldown 触发 syncProviderGeneration 重新拉齐
			delete(m.lastGenSync, providerID)
			return
		}
	}

	// Step 2: 本地 + DB(此时已确认 Redis 删除成功)
	delete(m.cooldowns, key)
	delete(m.reasons, key)
	if m.repository != nil {
		if err := m.repository.Delete(providerID, clientType, model); err != nil {
			log.Printf("[Cooldown] Failed to delete cooldown for provider %d, client %s, model %s from database: %v", providerID, clientType, model, err)
		}
	}

	// Step 3: 重置失败计数 + 通知其他实例
	m.failureTracker.ResetFailures(providerID, clientType, model)
	m.bumpAndPublishLocked(providerID)

	log.Printf("[Cooldown] Provider %d (clientType=%s, model=%s): Cleared model-level cooldown after successful request", providerID, clientType, model)
}

// setCooldownLocked sets cooldown without acquiring lock (internal use only).
//
// 写入顺序:Redis store(分布式真值) → 本地 map → DB(持久化) → bump generation + publish。
//
// 三个分支:
//   - store 接受(写入或同值覆盖):本地与 store 一致,正常 bump + publish
//   - store 因 SetIfLater 拒绝(已有更晚 cooldown):**不能**用本实例的 until
//     覆盖本地,否则本地暂时低于真值。改为 schedule 一次该 provider 的
//     reload,把本地拉到 store 最新状态
//   - store 错误:Redis 暂时不可用。仍然更新本地保证单机正确,bump 本地
//     generation 并尝试 publish——这样 Redis 恢复后 syncProviderGeneration
//     会发现本地 generation 落后并 reload
//
// useIfLater=true:RecordFailure 路径,避免覆盖更晚的 cooldown。
// useIfLater=false:UpdateCooldown / SetCooldownUntil / SetCooldownDuration 等显式覆盖路径。
func (m *Manager) setCooldownLocked(providerID uint64, clientType string, model string, until time.Time, reason CooldownReason, useIfLater bool) {
	key := CooldownKey{ProviderID: providerID, ClientType: clientType, Model: model}

	// 0. until <= now 时退化为 clear:Redis store 的 Set/SetIfLater 对过期 until
	//    会 no-op(PEXPIRE 不允许 ttl <= 0),继续走 Set 路径 → 本地/DB 写成过期
	//    值,但 Redis 仍保留旧 cooldown,当前实例放行+其他实例从 Redis 读旧值继续
	//    拦截 = 分布式分叉。直接 inline clear semantics(已持写锁,不需重入)。
	if !until.After(time.Now()) {
		if sp := m.store.Load(); sp != nil {
			ctx, cancel := context.WithTimeout(context.Background(), cooldownIOTimeout)
			err := (*sp).Delete(ctx, key)
			cancel()
			if err != nil {
				log.Printf("[Cooldown] store Delete on past-until clear failed: %v (skipping local/DB/publish)", err)
				delete(m.lastGenSync, providerID)
				return
			}
		}
		delete(m.cooldowns, key)
		delete(m.reasons, key)
		if m.repository != nil {
			if err := m.repository.Delete(providerID, clientType, model); err != nil {
				log.Printf("[Cooldown] repository.Delete on past-until clear failed: %v", err)
			}
		}
		m.failureTracker.ResetFailures(providerID, clientType, model)
		m.bumpAndPublishLocked(providerID)
		return
	}

	// 1. Redis store (分布式真值)
	storeAccepted := true
	storeRejected := false // SetIfLater 明确拒绝(已有更晚 cooldown)
	if sp := m.store.Load(); sp != nil {
		s := *sp
		// 在 m.mu 写锁路径内调 Redis;用短 timeout 防止持锁阻塞读路径
		ctx, cancel := context.WithTimeout(context.Background(), cooldownIOTimeout)
		if useIfLater {
			ok, err := s.SetIfLater(ctx, key, until)
			if err != nil {
				log.Printf("[Cooldown] store SetIfLater failed: %v", err)
				storeAccepted = false
			} else {
				storeAccepted = ok
				storeRejected = !ok
			}
		} else {
			if err := s.Set(ctx, key, until); err != nil {
				log.Printf("[Cooldown] store Set failed: %v", err)
				storeAccepted = false
			}
		}
		cancel()
	}

	// 2. 本地 map
	// SetIfLater 拒绝时跳过本地写入,而是 reload —— 这样本地不会因为
	// "用了一个比 store 真值更早的 until 覆盖本地" 而变得不一致。
	// store 错误时仍写本地,保证单机视图正确(详见上面注释)。
	if !storeRejected {
		m.cooldowns[key] = until
		m.reasons[key] = reason
	}

	// 3. DB 持久化 (兼容旧的 LoadFromDatabase 启动恢复路径)
	// SetIfLater 拒绝时跳过 DB 写入:store 上更晚的 cooldown 是由另一个
	// 实例写入的,那个实例同样会执行 repository.Upsert,所以 DB 也已经
	// 是真值。当前架构下所有实例都连同一个 DB,这个假设成立。
	// 如果未来拆分成"只写 store 不写 DB"的角色,这里需要补一次 DB sync。
	if m.repository != nil && !storeRejected {
		cd := &domain.Cooldown{
			ProviderID: providerID,
			ClientType: clientType,
			Model:      model,
			UntilTime:  until,
			Reason:     domain.CooldownReason(reason),
		}
		if err := m.repository.Upsert(cd); err != nil {
			log.Printf("[Cooldown] Failed to persist cooldown for provider %d: %v", providerID, err)
		}
	}

	// 4. 通知其他实例 / 本地恢复
	switch {
	case storeRejected:
		// 已有更晚 cooldown:本地未更新。把 lastGenSync 清零,下次
		// IsInCooldown 立即触发 syncProviderGeneration,从 store reload。
		// 不在持锁路径直接 reload —— 避免在写锁内做 Redis I/O。
		delete(m.lastGenSync, providerID)
	case storeAccepted:
		// 正常写入,bump + publish
		m.bumpAndPublishLocked(providerID)
	default:
		// store 错误:绝对**不能** bump/publish。原因:
		// 如果 store.Set 失败(Redis 上没有这条 cooldown)但 BumpGeneration
		// 成功,事件会让其他实例 reload。它们 ListByProvider 拿到的是 Redis
		// 上的旧状态(没有本次 cooldown),会把它们本地刚记的该 provider
		// cooldown 全部 erase —— 真正的"漏封禁"。
		//
		// 把"分布式失败"局限在本实例的本地视图:本地仍有这条 cooldown
		// (单机正确),但不去污染其他实例。lastGenSync 清零,下次
		// IsInCooldown 触发 syncProviderGeneration,看到 store 上 generation
		// 没变就保持本地;Redis 恢复后,下次成功的 mutation 才会同步 generation。
		delete(m.lastGenSync, providerID)
	}
}

// SetCooldownDuration sets a cooldown for a provider with a duration from now
// clientType is optional - empty string means cooldown applies to all client types
func (m *Manager) SetCooldownDuration(providerID uint64, clientType string, model string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	until := time.Now().Add(duration)
	m.setCooldownLocked(providerID, clientType, model, until, ReasonUnknown, false)
}

// SetCooldownUntil sets a cooldown for a provider until a specific time
// This is used for manual freezing by admin
func (m *Manager) SetCooldownUntil(providerID uint64, clientType string, model string, until time.Time) {
	log.Printf("[Cooldown] SetCooldownUntil: providerID=%d, clientType=%q, model=%q, until=%v", providerID, clientType, model, until)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCooldownLocked(providerID, clientType, model, until, ReasonManual, false)
	log.Printf("[Cooldown] SetCooldownUntil: done, current cooldowns count=%d", len(m.cooldowns))
}

// ClearCooldown removes the cooldown for a provider.
// If clientType and model are both empty, clears ALL cooldowns for the provider.
// If model is specified, only clears that specific key.
func (m *Manager) ClearCooldown(providerID uint64, clientType string, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if clientType == "" && model == "" {
		// Clear all cooldowns for this provider.
		// 和 RecordSuccess 一样:Redis 删除失败必须中止,不能让本地/DB 抢跑。
		if sp := m.store.Load(); sp != nil {
			ctx, cancel := context.WithTimeout(context.Background(), cooldownIOTimeout)
			err := (*sp).DeleteByProvider(ctx, providerID)
			cancel()
			if err != nil {
				log.Printf("[Cooldown] store DeleteByProvider failed for provider %d: %v (skipping local/DB/publish)", providerID, err)
				delete(m.lastGenSync, providerID)
				return
			}
		}
		keysToDelete := []CooldownKey{}
		for key := range m.cooldowns {
			if key.ProviderID == providerID {
				keysToDelete = append(keysToDelete, key)
			}
		}
		for _, key := range keysToDelete {
			delete(m.cooldowns, key)
			delete(m.reasons, key)
		}

		// Delete from database
		if m.repository != nil {
			if err := m.repository.DeleteAll(providerID); err != nil {
				log.Printf("[Cooldown] Failed to delete all cooldowns for provider %d from database: %v", providerID, err)
			}
		}

		// Also reset all failure counts for this provider
		m.failureTracker.ResetFailures(providerID, "", "")

		m.bumpAndPublishLocked(providerID)
	} else {
		// Clear specific cooldown — 同样先确保 Redis 真值删除成功
		key := CooldownKey{ProviderID: providerID, ClientType: clientType, Model: model}
		if sp := m.store.Load(); sp != nil {
			ctx, cancel := context.WithTimeout(context.Background(), cooldownIOTimeout)
			err := (*sp).Delete(ctx, key)
			cancel()
			if err != nil {
				log.Printf("[Cooldown] store Delete failed for provider %d, clientType=%s, model=%s: %v (skipping local/DB/publish)",
					providerID, clientType, model, err)
				delete(m.lastGenSync, providerID)
				return
			}
		}
		delete(m.cooldowns, key)
		delete(m.reasons, key)

		// Delete from database
		if m.repository != nil {
			if err := m.repository.Delete(providerID, clientType, model); err != nil {
				log.Printf("[Cooldown] Failed to delete cooldown for provider %d, client %s, model %s from database: %v", providerID, clientType, model, err)
			}
		}

		// Also reset failure counts for this provider+clientType+model
		m.failureTracker.ResetFailures(providerID, clientType, model)

		m.bumpAndPublishLocked(providerID)
	}
}

// IsInCooldown checks if a provider is currently in cooldown for a specific client type and model.
// Checks 4 hierarchical levels (any match = frozen):
//  1. (providerID, "", "")            — provider-level
//  2. (providerID, clientType, "")    — key/endpoint-level
//  3. (providerID, "", model)         — model-level (all client types)
//  4. (providerID, clientType, model) — model+clientType-level
func (m *Manager) IsInCooldown(providerID uint64, clientType string, model string) bool {
	// 节流内 generation sync:发现 store 中该 provider gen 变了就先 reload 本地。
	// 不在 m.mu 内调用,内部会自管锁。
	m.syncProviderGeneration(providerID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()

	// 1. Provider-level cooldown (applies to all client types and models)
	if until, ok := m.cooldowns[CooldownKey{ProviderID: providerID}]; ok && now.Before(until) {
		return true
	}

	// 2. Key/endpoint-level cooldown (applies to all models for this client type)
	if clientType != "" {
		if until, ok := m.cooldowns[CooldownKey{ProviderID: providerID, ClientType: clientType}]; ok && now.Before(until) {
			return true
		}
	}

	// 3. Model-level cooldown (applies to all client types for this model)
	if model != "" {
		if until, ok := m.cooldowns[CooldownKey{ProviderID: providerID, Model: model}]; ok && now.Before(until) {
			return true
		}
	}

	// 4. Model+clientType-level cooldown
	if clientType != "" && model != "" {
		if until, ok := m.cooldowns[CooldownKey{ProviderID: providerID, ClientType: clientType, Model: model}]; ok && now.Before(until) {
			return true
		}
	}

	return false
}

// GetCooldownUntil returns the cooldown end time for a provider, client type, and model.
// Checks 4 hierarchical levels and returns the latest (most restrictive) time.
// Returns zero time if not in cooldown.
func (m *Manager) GetCooldownUntil(providerID uint64, clientType string, model string) time.Time {
	m.syncProviderGeneration(providerID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getCooldownUntilLocked(providerID, clientType, model)
}

// GetAllCooldowns returns all active cooldowns
// Returns map of CooldownKey -> end time
func (m *Manager) GetAllCooldowns() map[CooldownKey]time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	result := make(map[CooldownKey]time.Time)

	for key, until := range m.cooldowns {
		if now.Before(until) {
			result[key] = until
		}
	}

	return result
}

// CleanupExpired removes expired cooldowns from memory and database
// Also resets failure counts for expired cooldowns
func (m *Manager) CleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	expiredKeys := []CooldownKey{}

	for key, until := range m.cooldowns {
		if now.After(until) {
			delete(m.cooldowns, key)
			delete(m.reasons, key)
			expiredKeys = append(expiredKeys, key)
		}
	}

	// Reset failure counts for expired cooldowns
	for _, key := range expiredKeys {
		m.failureTracker.ResetFailures(key.ProviderID, key.ClientType, key.Model)
	}

	// Delete expired cooldowns from database
	if m.repository != nil {
		if err := m.repository.DeleteExpired(); err != nil {
			log.Printf("[Cooldown] Failed to delete expired cooldowns from database: %v", err)
		}
	}

	// Cleanup old failure counts (older than 24 hours)
	m.failureTracker.CleanupExpired(24 * 60 * 60)

	if len(expiredKeys) > 0 {
		log.Printf("[Cooldown] Cleaned up %d expired cooldowns and reset their failure counts", len(expiredKeys))
	}
}

// GetCooldownInfo returns cooldown info for a specific provider, client type, and model.
func (m *Manager) GetCooldownInfo(providerID uint64, clientType string, model string, providerName string) *CooldownInfo {
	m.syncProviderGeneration(providerID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	until := m.getCooldownUntilLocked(providerID, clientType, model)
	if until.IsZero() {
		return nil
	}

	remaining := time.Until(until)
	if remaining < 0 {
		return nil
	}

	// Get reason — check from most specific to least specific
	var reason CooldownReason

	keys := []CooldownKey{
		{ProviderID: providerID, ClientType: clientType, Model: model},
		{ProviderID: providerID, Model: model},
		{ProviderID: providerID, ClientType: clientType},
		{ProviderID: providerID},
	}
	reason = ReasonUnknown
	for _, k := range keys {
		if r, ok := m.reasons[k]; ok {
			reason = r
			break
		}
	}

	return &CooldownInfo{
		ProviderID:   providerID,
		ProviderName: providerName,
		ClientType:   clientType,
		Model:        model,
		Until:        until,
		Remaining:    formatDuration(remaining),
		Reason:       reason,
	}
}

// getCooldownUntilLocked is internal version without lock.
// Checks 4 hierarchical levels and returns the latest (most restrictive) time.
func (m *Manager) getCooldownUntilLocked(providerID uint64, clientType string, model string) time.Time {
	now := time.Now()
	var latestCooldown time.Time

	// Check all 4 hierarchical levels
	keys := []CooldownKey{
		{ProviderID: providerID},                                          // 1. provider-level
		{ProviderID: providerID, ClientType: clientType},                  // 2. key/endpoint-level
		{ProviderID: providerID, Model: model},                            // 3. model-level (all client types)
		{ProviderID: providerID, ClientType: clientType, Model: model},    // 4. model+clientType-level
	}

	for _, key := range keys {
		if until, ok := m.cooldowns[key]; ok && now.Before(until) {
			if until.After(latestCooldown) {
				latestCooldown = until
			}
		}
	}

	return latestCooldown
}

// formatDuration formats a duration as a human-readable string
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return formatWithUnits(int(h), "h", int(m), "m", int(s), "s")
	}
	if m > 0 {
		return formatWithUnits(int(m), "m", int(s), "s", 0, "")
	}
	return formatWithUnits(int(s), "s", 0, "", 0, "")
}

func formatWithUnits(val1 int, unit1 string, val2 int, unit2 string, val3 int, unit3 string) string {
	result := ""
	if val1 > 0 {
		result += formatInt(val1) + unit1
	}
	if val2 > 0 {
		if result != "" {
			result += " "
		}
		result += formatInt(val2) + unit2
	}
	if val3 > 0 && unit3 != "" {
		if result != "" {
			result += " "
		}
		result += formatInt(val3) + unit3
	}
	return result
}

func formatInt(i int) string {
	// strconv 处理任意范围(原始实现对 >= 100 会产生乱码 rune)
	return strconv.Itoa(i)
}

// GetAllCooldownsFromDB returns all active cooldowns from the repository
func (m *Manager) GetAllCooldownsFromDB() ([]*domain.Cooldown, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.repository == nil {
		return nil, nil
	}

	return m.repository.GetAll()
}
