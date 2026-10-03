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
	watchPoll     = 50 * time.Millisecond
)

// sandboxEnvKeys are the variables whose value places a process in a
// sandbox. A harness child that drops one still keeps the others.
var sandboxEnvKeys = []string{
	"HOME", "TMPDIR", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "XDG_STATE_HOME",
}

// process is one running process as the platform listing sees it. Env is
// nil when the platform hides it (macOS does for Apple binaries). Start
// tells it apart from a later process that reuses its pid.
type process struct {
	PID   int
	PPID  int
	SID   int
	Start int64
	Env   []string
}

// track makes a command run in a new session whose id the sandbox records,
// so what the command leaves running can be found after it exits even when
// its environment cannot be read. On Linux the command dies with the caller,
// and the sandbox keeper stops the rest. While the command runs its process
// tree is listed every watchPoll, so a child that starts its own session
// and outlives its parent is still known. The returned function ends that
// watch; call it once the command has finished.
func (s *Sandbox) track(opts *execpkg.RunOptions) (func(), error) {
	k, err := s.ensureKeeper()
	if err != nil {
		return nil, err
	}

	opts.NewSession = true
	opts.KillWithParent = true
	opts.Started = func(pid int) {
		s.mu.Lock()
		s.sessions[pid] = struct{}{}
		s.mu.Unlock()

		_ = k.send(keeperMessage{Session: pid})
	}

	return s.watch(), nil
}

// ensureKeeper starts the sandbox keeper on first use and tells it what the
// sandbox already knows.
func (s *Sandbox) ensureKeeper() (*keeper, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.keeper != nil || !keeperSupported {
		return s.keeper, nil
	}

	k, err := startKeeper(s.Root, s.aliases)
	if err != nil {
		return nil, err
	}

	var sendErr error

	for sid := range s.sessions {
		sendErr = errors.CombineErrors(sendErr, k.send(keeperMessage{Session: sid}))
	}

	for pid, start := range s.known {
		sendErr = errors.CombineErrors(sendErr, k.send(keeperMessage{PID: pid, Start: start}))
	}

	if sendErr != nil {
		return nil, errors.CombineErrors(sendErr, k.close())
	}

	s.keeper = k

	return k, nil
}

// closeKeeper ends the keeper and waits for its last sweep.
func (s *Sandbox) closeKeeper() error {
	s.mu.Lock()
	k := s.keeper
	s.keeper = nil
	s.mu.Unlock()

	return k.close()
}

// watch lists the sandbox processes every watchPoll until the returned
// function is called.
func (s *Sandbox) watch() func() {
	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)

		ticker := time.NewTicker(watchPoll)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_, _ = s.owned(false)
			}
		}
	}()

	return func() {
		close(stop)
		<-done
	}
}

// StopProcesses kills every process the sandbox started that is still
// running: background work a harness left behind (a Codex plugin clone, a
// shell job, a child that outlived a timeout). Those children are no longer
// in the harness process tree once it exits, so they are found by session,
// by the sandbox paths in their environment, by an earlier sighting with
// the same start time, and by descent from any of those. Each round stops
// the matches before killing them, so none can fork past the sweep.
func (s *Sandbox) StopProcesses() error {
	deadline := time.Now().Add(stopTimeout)

	for {
		procs, err := s.owned(true)
		if err != nil {
			return err
		}

		if len(procs) == 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return errors.Newf("sandbox processes still running after %s: %s",
				stopTimeout, joinPIDs(pidsOf(procs)))
		}

		for _, p := range procs {
			freezeProcess(p)
		}

		for _, p := range procs {
			killProcess(p)
		}

		time.Sleep(stopPoll)
	}
}

// Processes lists the live processes that belong to the sandbox, other
// than the caller. A recorded session with no live member is dropped, so a
// later process that reuses its id is never taken for a sandbox process.
func (s *Sandbox) Processes() ([]int, error) {
	procs, err := s.owned(true)
	if err != nil {
		return nil, err
	}

	return pidsOf(procs), nil
}

// owned lists the sandbox processes with the identity they were seen with,
// so a signal never reaches a later process that reuses one of their pids.
// Listings are serialized, so each one is newer than the known processes it
// prunes. Without withEnv a process is matched only by session, earlier
// sighting or descent, which follows a known tree without the cost of
// reading every environment.
func (s *Sandbox) owned(withEnv bool) ([]process, error) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()

	tracked := s.trackedSessions()

	procs, err := listProcesses(withEnv)
	if err != nil {
		return nil, errors.Wrap(err, "listing processes")
	}

	owned, skip := s.roots(procs, tracked)

	for grew := true; grew; {
		grew = false

		for _, p := range procs {
			if !skip[p.PID] && !owned[p.PID] && owned[p.PPID] {
				owned[p.PID] = true
				grew = true
			}
		}
	}

	out := make([]process, 0, len(owned))

	for _, p := range procs {
		if owned[p.PID] {
			out = append(out, p)
		}
	}

	s.remember(out, procs)

	return out, nil
}

// roots marks the processes that belong to the sandbox by their own
// session, environment or earlier sighting, and drops the tracked sessions
// with no live member. skip holds the caller and its keeper.
func (s *Sandbox) roots(procs []process, tracked []int) (owned, skip map[int]bool) {
	owned = map[int]bool{}
	skip = map[int]bool{os.Getpid(): true}
	live := map[int]bool{}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.keeper != nil {
		skip[s.keeper.pid] = true
	}

	for _, p := range procs {
		live[p.SID] = true

		_, inSession := s.sessions[p.SID]
		start, seen := s.known[p.PID]

		if !skip[p.PID] && (inSession || seen && start == p.Start || s.ownsEnv(p.Env)) {
			owned[p.PID] = true
		}
	}

	for _, sid := range tracked {
		if !live[sid] {
			delete(s.sessions, sid)
		}
	}

	return owned, skip
}

// remember records the owned processes, so one that leaves its session and
// loses its parent before the next listing is still found, passes the new
// ones to the keeper, and forgets those no longer running.
func (s *Sandbox) remember(owned, procs []process) {
	alive := make(map[int]int64, len(procs))
	for _, p := range procs {
		alive[p.PID] = p.Start
	}

	var added []keeperMessage

	s.mu.Lock()

	for _, p := range owned {
		if start, ok := s.known[p.PID]; !ok || start != p.Start {
			s.known[p.PID] = p.Start
			added = append(added, keeperMessage{PID: p.PID, Start: p.Start})
		}
	}

	for pid, start := range s.known {
		if got, ok := alive[pid]; !ok || got != start {
			delete(s.known, pid)
		}
	}

	k := s.keeper
	s.mu.Unlock()

	for _, msg := range added {
		_ = k.send(msg)
	}
}

func pidsOf(procs []process) []int {
	pids := make([]int, 0, len(procs))
	for _, p := range procs {
		pids = append(pids, p.PID)
	}

	slices.Sort(pids)

	return pids
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

// Close stops the sandbox processes and the keeper and removes every
// sandbox file. A process that escaped the sweep could recreate files, so
// the removal is checked again after a pause.
func (s *Sandbox) Close() error {
	var stopErr, removeErr error

	for range closeAttempts {
		stopErr = s.StopProcesses()
		removeErr = os.RemoveAll(s.Root)

		time.Sleep(closeSettle)

		if _, err := os.Lstat(s.Root); removeErr == nil && errors.Is(err, os.ErrNotExist) {
			return errors.CombineErrors(stopErr, s.closeKeeper())
		}
	}

	return errors.CombineErrors(
		errors.Newf("files reappeared in %s after removing it %d times", s.Root, closeAttempts),
		errors.CombineErrors(
			errors.CombineErrors(errors.Wrap(removeErr, "removing sandbox"), stopErr),
			s.closeKeeper(),
		),
	)
}

func joinPIDs(pids []int) string {
	parts := make([]string, 0, len(pids))

	for _, pid := range pids {
		parts = append(parts, strconv.Itoa(pid))
	}

	return strings.Join(parts, ", ")
}
