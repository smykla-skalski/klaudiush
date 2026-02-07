// Package session provides session state tracking for Claude Code hooks.
// It tracks whether a session has been "poisoned" by a blocked command,
// enabling fast-fail for subsequent commands in the same session.
package session

import (
	"time"
)

//go:generate enumer -type=Status -trimprefix=Status -json -text -yaml -sql
//go:generate go run github.com/smykla-labs/klaudiush/tools/enumerfix status_enumer.go

// Status represents the current state of a session.
type Status int

const (
	// StatusClean indicates a session with no blocking errors.
	StatusClean Status = iota

	// StatusPoisoned indicates a session that was blocked by a validation error.
	// Subsequent commands in this session will be fast-failed.
	StatusPoisoned
)

// SessionInfo contains the state information for a single session.
type SessionInfo struct {
	// SessionID is the unique identifier for the session (UUID format).
	SessionID string `json:"session_id"`

	// Status is the current state of the session.
	Status Status `json:"status"`

	// PoisonedAt is when the session was poisoned (nil if clean).
	PoisonedAt *time.Time `json:"poisoned_at,omitempty"`

	// PoisonCodes is the list of error codes that poisoned the session (e.g., ["GIT001", "GIT002"]).
	PoisonCodes []string `json:"poison_codes,omitempty"`

	// PoisonMessage is the original error message that caused the poisoning.
	PoisonMessage string `json:"poison_message,omitempty"`

	// CommandCount is the number of commands processed in this session.
	CommandCount int `json:"command_count"`

	// LastActivity is when the session was last accessed.
	LastActivity time.Time `json:"last_activity"`
}

// IsPoisoned returns true if the session is in poisoned state.
func (s *SessionInfo) IsPoisoned() bool {
	return s.Status == StatusPoisoned
}

// SessionState contains state for all tracked sessions.
type SessionState struct {
	// Sessions maps session ID to session info.
	Sessions map[string]*SessionInfo `json:"sessions"`

	// LastUpdated is when the state was last modified.
	LastUpdated time.Time `json:"last_updated"`
}

// NewSessionState creates a new empty session state.
func NewSessionState() *SessionState {
	return &SessionState{
		Sessions:    make(map[string]*SessionInfo),
		LastUpdated: time.Now(),
	}
}
