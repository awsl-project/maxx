package cooldown

import (
	"log"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

// FailureTracker manages failure counts and their persistence
type FailureTracker struct {
	failureCounts map[FailureKey]int
	repository    repository.FailureCountRepository
}

// NewFailureTracker creates a new failure tracker
func NewFailureTracker() *FailureTracker {
	return &FailureTracker{
		failureCounts: make(map[FailureKey]int),
	}
}

// SetRepository sets the repository for persistence
func (ft *FailureTracker) SetRepository(repo repository.FailureCountRepository) {
	ft.repository = repo
}

// LoadFromDatabase loads all failure counts from database into memory
func (ft *FailureTracker) LoadFromDatabase() error {
	if ft.repository == nil {
		return nil
	}

	failureCounts, err := ft.repository.GetAll()
	if err != nil {
		return err
	}

	ft.failureCounts = make(map[FailureKey]int)
	for _, fc := range failureCounts {
		key := FailureKey{
			ProviderID: fc.ProviderID,
			ClientType: fc.ClientType,
			Reason:     CooldownReason(fc.Reason),
		}
		ft.failureCounts[key] = fc.Count
	}

	log.Printf("[FailureTracker] Loaded %d failure counts from database", len(failureCounts))
	return nil
}

// IncrementFailure increments the failure count and persists to database
// Returns the new failure count
func (ft *FailureTracker) IncrementFailure(providerID uint64, clientType string, reason CooldownReason) int {
	key := FailureKey{
		ProviderID: providerID,
		ClientType: clientType,
		Reason:     reason,
	}

	ft.failureCounts[key]++
	newCount := ft.failureCounts[key]

	// Persist to database
	if ft.repository != nil {
		fc := &domain.FailureCount{
			ProviderID:    providerID,
			ClientType:    clientType,
			Reason:        string(reason),
			Count:         newCount,
			LastFailureAt: time.Now().UTC(),
		}
		if err := ft.repository.Upsert(fc); err != nil {
			log.Printf("[FailureTracker] Failed to persist failure count: %v", err)
		}
	}

	return newCount
}

// GetFailureCount returns the current failure count for a given key
func (ft *FailureTracker) GetFailureCount(providerID uint64, clientType string, reason CooldownReason) int {
	key := FailureKey{
		ProviderID: providerID,
		ClientType: clientType,
		Reason:     reason,
	}
	return ft.failureCounts[key]
}

// ResetFailures resets all failure counts for a provider+clientType
// If clientType is empty, resets ALL failure counts for the provider
func (ft *FailureTracker) ResetFailures(providerID uint64, clientType string) {
	// Clear failure counts for all reasons for this provider+clientType
	keysToDelete := []FailureKey{}
	for key := range ft.failureCounts {
		// If clientType is empty, match all clientTypes for this provider
		// Otherwise, only match the specific clientType
		if key.ProviderID == providerID && (clientType == "" || key.ClientType == clientType) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	if len(keysToDelete) > 0 {
		for _, key := range keysToDelete {
			delete(ft.failureCounts, key)
		}

		// Delete from database
		if ft.repository != nil {
			if err := ft.repository.DeleteAll(providerID, clientType); err != nil {
				log.Printf("[FailureTracker] Failed to delete failure counts from database: %v", err)
			}
		}

		log.Printf("[FailureTracker] Provider %d (clientType=%s): Reset %d failure counts",
			providerID, clientType, len(keysToDelete))
	}
}

// CleanupExpired removes failure counts that are too old
// This prevents indefinite accumulation of failures
func (ft *FailureTracker) CleanupExpired(olderThanSeconds int64) {
	// Only cleanup from database - memory will be cleaned on next load
	if ft.repository != nil {
		if err := ft.repository.DeleteExpired(olderThanSeconds); err != nil {
			log.Printf("[FailureTracker] Failed to cleanup expired failure counts: %v", err)
		} else {
			log.Printf("[FailureTracker] Cleaned up failure counts older than %d seconds", olderThanSeconds)
		}
	}

	// Reload from database to sync memory state
	if err := ft.LoadFromDatabase(); err != nil {
		log.Printf("[FailureTracker] Failed to reload after cleanup: %v", err)
	}
}
