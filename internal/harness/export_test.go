package harness

import (
	"syscall"
	"time"
)

// SetCodexTrustLimit shortens the Codex hook listing timeout for a test and
// returns a function that restores it.
func SetCodexTrustLimit(limit time.Duration) func() {
	previous := codexTrustLimit
	codexTrustLimit = limit

	return func() { codexTrustLimit = previous }
}

// CodexCatalog is the model catalog the Codex driver writes.
const CodexCatalog = codexCatalog

// SessionCount returns how many session ids the sandbox still records.
func (s *Sandbox) SessionCount() int { return len(s.trackedSessions()) }

// StartOf returns the start identity of a live process.
func StartOf(pid int) (int64, bool) { return processStart(pid) }

// KillIfSame kills pid only while it is the process that started at start.
func KillIfSame(pid int, start int64) bool { return signalProcess(pid, start, syscall.SIGKILL) }
