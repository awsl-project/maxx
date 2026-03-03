package waiter

import (
	"context"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/repository"
)

var (
	ErrProjectBindingTimeout  = errors.New("project binding timeout: please select a project in the UI")
	ErrProjectBindingRequired = errors.New("project binding required")
	ErrSessionRejected        = errors.New("session rejected by user")
)

const (
	SettingKeyForceProjectBinding = "force_project_binding"
	SettingKeyForceProjectTimeout = "force_project_timeout"
	DefaultTimeout                = 30 * time.Second
	PollInterval                  = 500 * time.Millisecond
	BroadcastCooldown             = 5 * time.Second // 距离上次广播或拒绝超过此时间后可再次广播
)

// ProjectWaiter handles waiting for session project binding
type ProjectWaiter struct {
	sessionRepo repository.SessionRepository
	settingRepo repository.SystemSettingRepository
	broadcaster event.Broadcaster

	// Track last broadcast time for each session to implement cooldown
	lastBroadcast map[string]time.Time
	mu            sync.Mutex
}

// NewProjectWaiter creates a new ProjectWaiter
func NewProjectWaiter(
	sessionRepo repository.SessionRepository,
	settingRepo repository.SystemSettingRepository,
	broadcaster event.Broadcaster,
) *ProjectWaiter {
	return &ProjectWaiter{
		sessionRepo:   sessionRepo,
		settingRepo:   settingRepo,
		broadcaster:   broadcaster,
		lastBroadcast: make(map[string]time.Time),
	}
}

// IsForceProjectEnabled checks if force project binding is enabled
func (w *ProjectWaiter) IsForceProjectEnabled() bool {
	if w.settingRepo == nil {
		return false
	}
	value, err := w.settingRepo.Get(SettingKeyForceProjectBinding)
	if err != nil || value == "" {
		return false
	}
	return value == "true"
}

// GetTimeout returns the configured timeout duration
func (w *ProjectWaiter) GetTimeout() time.Duration {
	if w.settingRepo == nil {
		return DefaultTimeout
	}
	value, err := w.settingRepo.Get(SettingKeyForceProjectTimeout)
	if err != nil || value == "" {
		return DefaultTimeout
	}
	// Parse as seconds
	if secs, err := strconv.Atoi(value); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return DefaultTimeout
}

// WaitForProject waits for session to be assigned a project
// Returns nil if project is assigned, error if timeout or context cancelled
func (w *ProjectWaiter) WaitForProject(ctx context.Context, session *domain.Session) error {
	// Already has a project
	if session.ProjectID > 0 {
		return nil
	}

	// Check if force project is enabled
	if !w.IsForceProjectEnabled() {
		return nil
	}

	// Check if session is already rejected or has project (from previous requests)
	latestSession, err := w.sessionRepo.GetBySessionID(session.TenantID, session.SessionID)
	if err == nil && latestSession != nil {
		// Check rejection status
		if latestSession.RejectedAt != nil {
			// If rejected within cooldown period, fail immediately
			if time.Since(*latestSession.RejectedAt) < BroadcastCooldown {
				log.Printf("[ProjectWaiter] Session %s: rejected recently (within %v), failing immediately", session.SessionID, BroadcastCooldown)
				return ErrSessionRejected
			}
			// If rejected but cooldown passed, clear rejection to allow retry
			log.Printf("[ProjectWaiter] Session %s: rejection expired, clearing and allowing retry", session.SessionID)
			latestSession.RejectedAt = nil
			w.sessionRepo.Update(latestSession)
		}
		if latestSession.ProjectID > 0 {
			session.ProjectID = latestSession.ProjectID
			return nil
		}
	}

	log.Printf("[ProjectWaiter] Session %s requires project binding, waiting...", session.SessionID)

	// Check if we should broadcast (cooldown-based)
	w.mu.Lock()
	lastTime, exists := w.lastBroadcast[session.SessionID]
	shouldBroadcast := !exists || time.Since(lastTime) >= BroadcastCooldown
	if shouldBroadcast {
		w.lastBroadcast[session.SessionID] = time.Now()
	}
	w.mu.Unlock()

	// Broadcast if cooldown has passed
	if shouldBroadcast && w.broadcaster != nil {
		w.broadcaster.BroadcastMessage("new_session_pending", map[string]interface{}{
			"sessionID":  session.SessionID,
			"clientType": session.ClientType,
			"createdAt":  session.CreatedAt.Format(time.RFC3339),
		})
	}

	// Create timeout context
	timeout := w.GetTimeout()
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Poll for project assignment
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			if timeoutCtx.Err() == context.DeadlineExceeded {
				log.Printf("[ProjectWaiter] Session %s: timeout waiting for project binding", session.SessionID)
				return ErrProjectBindingTimeout
			}
			return timeoutCtx.Err()
		case <-ticker.C:
			// Check if session now has a project or was rejected
			updatedSession, err := w.sessionRepo.GetBySessionID(session.TenantID, session.SessionID)
			if err != nil {
				continue // Retry on transient errors
			}
			if updatedSession != nil {
				// Check if rejected
				if updatedSession.RejectedAt != nil {
					log.Printf("[ProjectWaiter] Session %s: rejected by user", session.SessionID)
					return ErrSessionRejected
				}
				// Check if project bound
				if updatedSession.ProjectID > 0 {
					// Update the original session reference
					session.ProjectID = updatedSession.ProjectID
					log.Printf("[ProjectWaiter] Session %s: project %d bound, continuing", session.SessionID, session.ProjectID)
					return nil
				}
			}
		}
	}
}
