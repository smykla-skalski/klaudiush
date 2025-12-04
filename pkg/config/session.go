package config

import "time"

// Default values for session configuration.
const (
	// DefaultSessionStateFile is the default state file path.
	DefaultSessionStateFile = "~/.klaudiush/session_state.json"

	// DefaultMaxSessionAge is the default maximum session age.
	DefaultMaxSessionAge = 24 * time.Hour
)

// SessionConfig contains configuration for session tracking.
// Session tracking enables fast-fail for subsequent commands after a
// blocking error occurs in the same Claude Code session.
type SessionConfig struct {
	// Enabled controls whether session tracking is active.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled"`

	// StateFile is the path to the session state file.
	// Default: "~/.klaudiush/session_state.json"
	StateFile string `json:"state_file,omitempty" koanf:"state_file" toml:"state_file"`

	// MaxSessionAge is the maximum age before a session is expired.
	// Default: "24h"
	MaxSessionAge Duration `json:"max_session_age,omitempty" koanf:"max_session_age" toml:"max_session_age"`
}

// IsEnabled returns true if session tracking is enabled.
// Returns true if Enabled is nil (default behavior).
func (s *SessionConfig) IsEnabled() bool {
	if s == nil || s.Enabled == nil {
		return true
	}

	return *s.Enabled
}

// GetStateFile returns the state file path.
// Returns DefaultSessionStateFile if StateFile is empty.
func (s *SessionConfig) GetStateFile() string {
	if s == nil || s.StateFile == "" {
		return DefaultSessionStateFile
	}

	return s.StateFile
}

// GetMaxSessionAge returns the maximum session age as a time.Duration.
// Returns DefaultMaxSessionAge if MaxSessionAge is zero.
func (s *SessionConfig) GetMaxSessionAge() time.Duration {
	if s == nil || s.MaxSessionAge == 0 {
		return DefaultMaxSessionAge
	}

	return time.Duration(s.MaxSessionAge)
}
