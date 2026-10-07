package harness

import (
	"io"
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

// SetProcessWatchInterval changes the process polling interval for a test.
func (s *Sandbox) SetProcessWatchInterval(interval time.Duration) { s.watchEvery = interval }

// KnowsProcess reports whether a process was recorded by an earlier listing.
func (s *Sandbox) KnowsProcess(pid int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.known[pid]

	return ok
}

// KeeperPID returns the pid of the sandbox keeper, or 0 before one runs.
func (s *Sandbox) KeeperPID() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.keeper == nil {
		return 0
	}

	return s.keeper.pid
}

// KeeperMain runs the keeper loop on r and returns its exit code.
func KeeperMain(r io.Reader) int { return runKeeper(r) }

// StartOf returns the start identity of a live process.
func StartOf(pid int) (int64, bool) { return processStart(pid) }

// KillIfSame kills pid only while it is the process that started at start.
func KillIfSame(pid int, start int64) bool { return signalProcess(pid, start, syscall.SIGKILL) }
