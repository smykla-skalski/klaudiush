package session

import (
	"sync"
	"time"

	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// File permission constants.
const (
	// stateFilePermissions is the permission mode for the state file.
	stateFilePermissions = 0o600

	// stateDirPermissions is the permission mode for the state directory.
	stateDirPermissions = 0o700

	// defaultMaxSessionAge is the default maximum age for session entries.
	defaultMaxSessionAge = 24 * time.Hour
)

// Tracker manages session state, tracking which sessions have been poisoned
// by blocked commands and providing fast-fail for subsequent commands.
type Tracker struct {
	mu     sync.RWMutex
	state  *SessionState
	config *config.SessionConfig
	logger logger.Logger

	// stateFile is the resolved path for state persistence.
	stateFile string

	// maxSessionAge is the maximum age before a session is expired.
	maxSessionAge time.Duration

	// now is a function that returns the current time.
	// Used for testing to control time.
	now func() time.Time
}

// TrackerOption configures the Tracker.
type TrackerOption func(*Tracker)

// WithLogger sets the logger.
func WithLogger(log logger.Logger) TrackerOption {
	return func(t *Tracker) {
		if log != nil {
			t.logger = log
		}
	}
}

// WithStateFile sets a custom state file path.
func WithStateFile(path string) TrackerOption {
	return func(t *Tracker) {
		if path != "" {
			t.stateFile = path
		}
	}
}

// WithTimeFunc sets a custom time function for testing.
func WithTimeFunc(fn func() time.Time) TrackerOption {
	return func(t *Tracker) {
		if fn != nil {
			t.now = fn
		}
	}
}

// WithMaxSessionAge sets a custom maximum session age.
func WithMaxSessionAge(d time.Duration) TrackerOption {
	return func(t *Tracker) {
		if d > 0 {
			t.maxSessionAge = d
		}
	}
}

// NewTracker creates a new session tracker.
func NewTracker(cfg *config.SessionConfig, opts ...TrackerOption) *Tracker {
	t := &Tracker{
		state:         NewSessionState(),
		config:        cfg,
		logger:        logger.NewNoOpLogger(),
		maxSessionAge: defaultMaxSessionAge,
		now:           time.Now,
	}

	// Set defaults from config
	if cfg != nil {
		t.stateFile = cfg.GetStateFile()

		if maxAge := cfg.GetMaxSessionAge(); maxAge > 0 {
			t.maxSessionAge = maxAge
		}
	} else {
		t.stateFile = (&config.SessionConfig{}).GetStateFile()
	}

	for _, opt := range opts {
		opt(t)
	}

	// Initialize state with current time
	t.state.LastUpdated = t.now()

	return t
}

// IsPoisoned checks if a session is poisoned.
// Returns (isPoisoned, sessionInfo).
func (t *Tracker) IsPoisoned(sessionID string) (bool, *SessionInfo) {
	if sessionID == "" {
		return false, nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	info, exists := t.state.Sessions[sessionID]
	if !exists {
		return false, nil
	}

	// Check if session has expired
	if t.isExpiredLocked(info) {
		return false, nil
	}

	return info.IsPoisoned(), info
}

// Poison marks a session as poisoned with the given error codes and message.
func (t *Tracker) Poison(sessionID string, codes []string, message string) {
	if sessionID == "" {
		t.logger.Debug("cannot poison session with empty ID")

		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	info, exists := t.state.Sessions[sessionID]

	if !exists {
		info = &SessionInfo{
			SessionID:    sessionID,
			Status:       StatusPoisoned,
			CommandCount: 0,
			LastActivity: now,
		}
		t.state.Sessions[sessionID] = info
	}

	info.Status = StatusPoisoned
	info.PoisonedAt = &now
	info.PoisonCodes = codes
	info.PoisonMessage = message
	info.LastActivity = now
	t.state.LastUpdated = now

	t.logger.Debug("session poisoned",
		"session_id", sessionID,
		"codes", codes,
		"message", message,
	)
}

// Unpoison clears the poisoned state of a session, allowing it to continue.
// This is typically called when the user acknowledges all blocking error codes.
func (t *Tracker) Unpoison(sessionID string) {
	if sessionID == "" {
		t.logger.Debug("cannot unpoison session with empty ID")

		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	info, exists := t.state.Sessions[sessionID]
	if !exists {
		t.logger.Debug("cannot unpoison non-existent session",
			"session_id", sessionID,
		)

		return
	}

	info.Status = StatusClean
	info.PoisonedAt = nil
	info.PoisonCodes = nil
	info.PoisonMessage = ""
	info.LastActivity = t.now()
	t.state.LastUpdated = t.now()

	t.logger.Debug("session unpoisoned",
		"session_id", sessionID,
	)
}

// RecordCommand increments the command count for a session.
func (t *Tracker) RecordCommand(sessionID string) {
	if sessionID == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	info, exists := t.state.Sessions[sessionID]

	if !exists {
		info = &SessionInfo{
			SessionID:    sessionID,
			Status:       StatusClean,
			CommandCount: 0,
			LastActivity: now,
		}
		t.state.Sessions[sessionID] = info
	}

	// Check if session has expired - if so, reset it
	if t.isExpiredLocked(info) {
		info.Status = StatusClean
		info.PoisonedAt = nil
		info.PoisonCodes = nil
		info.PoisonMessage = ""
		info.CommandCount = 0
	}

	info.CommandCount++
	info.LastActivity = now
	t.state.LastUpdated = now

	t.logger.Debug("recorded command",
		"session_id", sessionID,
		"command_count", info.CommandCount,
	)
}

// GetInfo returns session information for a session ID.
// Returns nil if session doesn't exist.
func (t *Tracker) GetInfo(sessionID string) *SessionInfo {
	if sessionID == "" {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	info, exists := t.state.Sessions[sessionID]
	if !exists {
		return nil
	}

	// Return a deep copy
	infoCopy := *info
	if info.PoisonedAt != nil {
		poisonedAtCopy := *info.PoisonedAt
		infoCopy.PoisonedAt = &poisonedAtCopy
	}

	return &infoCopy
}

// GetState returns a copy of the current session state.
func (t *Tracker) GetState() SessionState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Create a deep copy
	state := *t.state
	state.Sessions = make(map[string]*SessionInfo, len(t.state.Sessions))

	for k, v := range t.state.Sessions {
		infoCopy := *v
		if v.PoisonedAt != nil {
			poisonedAtCopy := *v.PoisonedAt
			infoCopy.PoisonedAt = &poisonedAtCopy
		}

		state.Sessions[k] = &infoCopy
	}

	return state
}

// ClearSession removes a session from tracking.
func (t *Tracker) ClearSession(sessionID string) {
	if sessionID == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.state.Sessions, sessionID)
	t.state.LastUpdated = t.now()

	t.logger.Debug("cleared session",
		"session_id", sessionID,
	)
}

// CleanupExpired removes expired sessions from tracking.
// Returns the number of sessions removed.
func (t *Tracker) CleanupExpired() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	var removed int

	for sessionID, info := range t.state.Sessions {
		if t.isExpiredLocked(info) {
			delete(t.state.Sessions, sessionID)

			removed++

			t.logger.Debug("expired session removed",
				"session_id", sessionID,
			)
		}
	}

	if removed > 0 {
		t.state.LastUpdated = t.now()
	}

	return removed
}

// Reset clears all session state.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.state = NewSessionState()
	t.state.LastUpdated = t.now()

	t.logger.Debug("session state reset")
}

// isExpiredLocked checks if a session has expired based on maxSessionAge.
// Must be called with mu held (read or write lock).
func (t *Tracker) isExpiredLocked(info *SessionInfo) bool {
	if t.maxSessionAge <= 0 {
		return false
	}

	return t.now().Sub(info.LastActivity) > t.maxSessionAge
}

// IsEnabled returns true if session tracking is enabled.
func (t *Tracker) IsEnabled() bool {
	if t == nil {
		return false
	}

	if t.config == nil {
		return false // Disabled by default
	}

	return t.config.IsEnabled()
}
