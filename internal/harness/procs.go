package harness

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
)

const (
	stopTimeout   = 10 * time.Second
	stopPoll      = 50 * time.Millisecond
	closeSettle   = 100 * time.Millisecond
	closeAttempts = 3
)

// sandboxEnvKeys are the variables whose value places a process in a
// sandbox. A harness child that drops one still keeps the others.
var sandboxEnvKeys = []string{
	"HOME", "TMPDIR", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "XDG_STATE_HOME",
}

// process is one running process as the platform listing sees it. Env is
// nil when the platform hides it (macOS does for Apple binaries).
type process struct {
	PID  int
	PPID int
	SID  int
	Env  []string
}

// track makes a command run in a new session whose id the sandbox records,
// so what the command leaves running can be found after it exits even when
// its environment cannot be read.
func (s *Sandbox) track(opts *execpkg.RunOptions) {
	opts.NewSession = true
	opts.Started = func(pid int) {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.sessions[pid] = struct{}{}
	}
}

// StopProcesses kills every process the sandbox started that is still
// running: background work a harness left behind (a Codex plugin clone, a
// shell job, a child that outlived a timeout). Those children are no longer
// in the harness process tree once it exits, so they are found by session,
// by the sandbox paths in their environment, and by descent from either.
// Each round stops the matches before killing them, so none can fork past
// the sweep.
func (s *Sandbox) StopProcesses() error {
	deadline := time.Now().Add(stopTimeout)

	for {
		pids, err := s.Processes()
		if err != nil {
			return err
		}

		if len(pids) == 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return errors.Newf("sandbox processes still running after %s: %s",
				stopTimeout, joinPIDs(pids))
		}

		for _, pid := range pids {
			freezeProcess(pid)
		}

		for _, pid := range pids {
			killProcess(pid)
		}

		time.Sleep(stopPoll)
	}
}

// Processes lists the live processes that belong to the sandbox, other
// than the caller. A recorded session with no live member is dropped, so a
// later process that reuses its id is never taken for a sandbox process.
func (s *Sandbox) Processes() ([]int, error) {
	tracked := s.trackedSessions()

	procs, err := listProcesses()
	if err != nil {
		return nil, errors.Wrap(err, "listing processes")
	}

	self := os.Getpid()
	owned := map[int]bool{}
	live := map[int]bool{}

	s.mu.Lock()
	for _, p := range procs {
		live[p.SID] = true

		_, tracked := s.sessions[p.SID]
		if p.PID != self && (tracked || s.ownsEnv(p.Env)) {
			owned[p.PID] = true
		}
	}

	for _, sid := range tracked {
		if !live[sid] {
			delete(s.sessions, sid)
		}
	}
	s.mu.Unlock()

	for grew := true; grew; {
		grew = false

		for _, p := range procs {
			if p.PID != self && !owned[p.PID] && owned[p.PPID] {
				owned[p.PID] = true
				grew = true
			}
		}
	}

	pids := make([]int, 0, len(owned))
	for pid := range owned {
		pids = append(pids, pid)
	}

	slices.Sort(pids)

	return pids, nil
}

// trackedSessions copies the recorded session ids. Only ids recorded before
// a listing may be dropped for having no member in it.
func (s *Sandbox) trackedSessions() []int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Collect(maps.Keys(s.sessions))
}

func (s *Sandbox) ownsEnv(env []string) bool {
	roots := append([]string{s.Root}, s.aliases...)

	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !slices.Contains(sandboxEnvKeys, key) {
			continue
		}

		for _, root := range roots {
			if value == root || strings.HasPrefix(value, root+string(filepath.Separator)) {
				return true
			}
		}
	}

	return false
}

// Close stops the sandbox processes and removes every sandbox file. A
// process that escaped the sweep could recreate files, so the removal is
// checked again after a pause.
func (s *Sandbox) Close() error {
	var stopErr, removeErr error

	for range closeAttempts {
		stopErr = s.StopProcesses()
		removeErr = os.RemoveAll(s.Root)

		time.Sleep(closeSettle)

		if _, err := os.Lstat(s.Root); removeErr == nil && errors.Is(err, os.ErrNotExist) {
			return stopErr
		}
	}

	return errors.CombineErrors(
		errors.Newf("files reappeared in %s after removing it %d times", s.Root, closeAttempts),
		errors.CombineErrors(errors.Wrap(removeErr, "removing sandbox"), stopErr),
	)
}

func joinPIDs(pids []int) string {
	parts := make([]string, 0, len(pids))

	for _, pid := range pids {
		parts = append(parts, strconv.Itoa(pid))
	}

	return strings.Join(parts, ", ")
}
