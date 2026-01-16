package kiro

import (
	"time"
)

// SessionManager tracks upstream session metadata.
type SessionManager struct {
	sessionID string
	startTime time.Time
	endTime   *time.Time
	isActive  bool
}

// NewSessionManager creates a session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessionID: generateUUID(),
		startTime: time.Now(),
		isActive:  false,
	}
}

// SetSessionID overrides the session ID.
func (sm *SessionManager) SetSessionID(sessionID string) {
	sm.sessionID = sessionID
}

// StartSession marks the session active.
func (sm *SessionManager) StartSession() []SSEEvent {
	sm.isActive = true
	sm.startTime = time.Now()

	return []SSEEvent{{
		Event: EventTypes.SessionStart,
		Data: map[string]any{
			"type":       EventTypes.SessionStart,
			"session_id": sm.sessionID,
			"timestamp":  sm.startTime.Format(time.RFC3339),
		},
	}}
}

// EndSession marks the session as ended.
func (sm *SessionManager) EndSession() []SSEEvent {
	now := time.Now()
	sm.endTime = &now
	sm.isActive = false

	return []SSEEvent{{
		Event: EventTypes.SessionEnd,
		Data: map[string]any{
			"type":       EventTypes.SessionEnd,
			"session_id": sm.sessionID,
			"timestamp":  now.Format(time.RFC3339),
			"duration":   now.Sub(sm.startTime).Milliseconds(),
		},
	}}
}

// IsActive reports whether the session is active.
func (sm *SessionManager) IsActive() bool {
	return sm.isActive
}

// GetSessionInfo returns session metadata.
func (sm *SessionManager) GetSessionInfo() SessionInfo {
	return SessionInfo{
		SessionID: sm.sessionID,
		StartTime: sm.startTime,
		EndTime:   sm.endTime,
	}
}

// Reset resets session state.
func (sm *SessionManager) Reset() {
	sm.sessionID = generateUUID()
	sm.startTime = time.Now()
	sm.endTime = nil
	sm.isActive = false
}
