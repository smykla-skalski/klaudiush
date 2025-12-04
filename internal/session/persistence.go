package session

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

// Load loads the session state from the configured state file.
func (t *Tracker) Load() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	path := t.resolveStatePath()

	// Path comes from trusted configuration, not user input.
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is from config
	if err != nil {
		if os.IsNotExist(err) {
			t.logger.Debug("state file does not exist, using fresh state",
				"path", path,
			)

			return nil
		}

		return errors.Wrap(err, "reading state file")
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		t.logger.Debug("failed to parse state file, using fresh state",
			"path", path,
			"error", err.Error(),
		)

		return nil
	}

	// Initialize map if nil (could happen with corrupted/old state files)
	if state.Sessions == nil {
		state.Sessions = make(map[string]*SessionInfo)
	}

	t.state = &state

	// Cleanup expired sessions after loading
	t.cleanupExpiredLocked()

	t.logger.Debug("loaded state from file",
		"path", path,
		"sessions", len(t.state.Sessions),
	)

	return nil
}

// Save persists the current session state to the configured state file.
func (t *Tracker) Save() error {
	t.mu.RLock()
	state := t.state
	path := t.resolveStatePath()
	t.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, stateDirPermissions); err != nil {
		return errors.Wrap(err, "creating state directory")
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return errors.Wrap(err, "marshaling state")
	}

	// Write to temp file first for atomic operation
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, stateFilePermissions); err != nil {
		return errors.Wrap(err, "writing temp state file")
	}

	// Rename for atomic replace
	if err := os.Rename(tmpPath, path); err != nil {
		// Clean up temp file on error
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, "renaming state file")
	}

	t.logger.Debug("saved state to file",
		"path", path,
		"sessions", len(state.Sessions),
	)

	return nil
}

// resolveStatePath expands ~ in the state file path.
func (t *Tracker) resolveStatePath() string {
	path := t.stateFile
	if len(path) > 1 && path[0] == '~' && path[1] == '/' {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	return path
}

// cleanupExpiredLocked removes expired sessions.
// Must be called with mu held (write lock).
func (t *Tracker) cleanupExpiredLocked() {
	for sessionID, info := range t.state.Sessions {
		if t.isExpiredLocked(info) {
			delete(t.state.Sessions, sessionID)

			t.logger.Debug("expired session removed during load",
				"session_id", sessionID,
			)
		}
	}
}
