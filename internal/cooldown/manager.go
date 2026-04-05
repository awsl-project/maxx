package cooldown

import (
	"log"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

// Manager manages provider cooldown states
// Cooldown is stored in memory and persisted to database
type Manager struct {
	mu             sync.RWMutex
	cooldowns      map[CooldownKey]time.Time         // cooldown key -> end time
	reasons        map[CooldownKey]CooldownReason    // cooldown key -> reason
	failureTracker *FailureTracker                   // tracks failure counts
	policies       map[CooldownReason]CooldownPolicy // cooldown calculation strategies
	repository     repository.CooldownRepository
}

// NewManager creates a new cooldown manager
func NewManager() *Manager {
	return &Manager{
		cooldowns:      make(map[CooldownKey]time.Time),
		reasons:        make(map[CooldownKey]CooldownReason),
		failureTracker: NewFailureTracker(),
		policies:       DefaultPolicies(),
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
		m.setCooldownLocked(providerID, effectiveClientType, effectiveModel, *explicitUntil, reason)
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

	m.setCooldownLocked(providerID, effectiveClientType, effectiveModel, until, reason)

	log.Printf("[Cooldown] Provider %d (clientType=%s, model=%s): Set cooldown for %v until %s (reason=%s, scope=%s, failureCount=%d)",
		providerID, clientType, model, duration, until.Format("2006-01-02 15:04:05"), reason, scope, failureCount)

	return until
}

// UpdateCooldown updates cooldown time without incrementing failure count
// This is used for async updates (e.g., when quota reset time is fetched asynchronously)
// Keeps the existing reason
func (m *Manager) UpdateCooldown(providerID uint64, clientType string, model string, until time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get existing reason or use Unknown
	key := CooldownKey{ProviderID: providerID, ClientType: clientType, Model: model}
	reason, ok := m.reasons[key]
	if !ok {
		reason = ReasonUnknown
	}

	m.setCooldownLocked(providerID, clientType, model, until, reason)
	log.Printf("[Cooldown] Provider %d (clientType=%s, model=%s): Updated cooldown to %s (async update, no count increment)",
		providerID, clientType, model, until.Format("2006-01-02 15:04:05"))
}

// RecordSuccess records a successful request and clears the model-level cooldown.
// Only clears the specific (providerID, clientType, model) cooldown entry.
// Key/provider level cooldowns are NOT auto-cleared — they have their own expiry.
func (m *Manager) RecordSuccess(providerID uint64, clientType string, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear only the model-level cooldown from memory
	key := CooldownKey{ProviderID: providerID, ClientType: clientType, Model: model}
	delete(m.cooldowns, key)
	delete(m.reasons, key)

	// Delete from database
	if m.repository != nil {
		if err := m.repository.Delete(providerID, clientType, model); err != nil {
			log.Printf("[Cooldown] Failed to delete cooldown for provider %d, client %s, model %s from database: %v", providerID, clientType, model, err)
		}
	}

	// Reset failure counts for this specific model
	m.failureTracker.ResetFailures(providerID, clientType, model)

	log.Printf("[Cooldown] Provider %d (clientType=%s, model=%s): Cleared model-level cooldown after successful request", providerID, clientType, model)
}

// setCooldownLocked sets cooldown without acquiring lock (internal use only)
func (m *Manager) setCooldownLocked(providerID uint64, clientType string, model string, until time.Time, reason CooldownReason) {
	key := CooldownKey{ProviderID: providerID, ClientType: clientType, Model: model}
	m.cooldowns[key] = until
	m.reasons[key] = reason

	// Persist to database
	if m.repository != nil {
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
}

// SetCooldownDuration sets a cooldown for a provider with a duration from now
// clientType is optional - empty string means cooldown applies to all client types
func (m *Manager) SetCooldownDuration(providerID uint64, clientType string, model string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	until := time.Now().Add(duration)
	m.setCooldownLocked(providerID, clientType, model, until, ReasonUnknown)
}

// SetCooldownUntil sets a cooldown for a provider until a specific time
// This is used for manual freezing by admin
func (m *Manager) SetCooldownUntil(providerID uint64, clientType string, model string, until time.Time) {
	log.Printf("[Cooldown] SetCooldownUntil: providerID=%d, clientType=%q, model=%q, until=%v", providerID, clientType, model, until)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCooldownLocked(providerID, clientType, model, until, ReasonManual)
	log.Printf("[Cooldown] SetCooldownUntil: done, current cooldowns count=%d", len(m.cooldowns))
}

// ClearCooldown removes the cooldown for a provider.
// If clientType and model are both empty, clears ALL cooldowns for the provider.
// If model is specified, only clears that specific key.
func (m *Manager) ClearCooldown(providerID uint64, clientType string, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if clientType == "" && model == "" {
		// Clear all cooldowns for this provider
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
	} else {
		// Clear specific cooldown
		key := CooldownKey{ProviderID: providerID, ClientType: clientType, Model: model}
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
	}
}

// IsInCooldown checks if a provider is currently in cooldown for a specific client type and model.
// Checks 4 hierarchical levels (any match = frozen):
//  1. (providerID, "", "")            — provider-level
//  2. (providerID, clientType, "")    — key/endpoint-level
//  3. (providerID, "", model)         — model-level (all client types)
//  4. (providerID, clientType, model) — model+clientType-level
func (m *Manager) IsInCooldown(providerID uint64, clientType string, model string) bool {
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
	return string(rune('0' + i/10)) + string(rune('0' + i%10))
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
