package hooksession

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/filelock"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	helperEnv        = "KLAUDIUSH_HOOKSESSION_HELPER"
	helperStateEnv   = "KLAUDIUSH_HOOKSESSION_STATE"
	helperWorkerEnv  = "KLAUDIUSH_HOOKSESSION_WORKER"
	helperRoundsEnv  = "KLAUDIUSH_HOOKSESSION_ROUNDS"
	sharedSession    = "shared"
	concurrentRounds = 12
	workerCount      = 8
	processCount     = 6
	testLockTimeout  = 30 * time.Second
)

func TestStoreConcurrentRecordsKeepEveryFinding(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")

	var wg sync.WaitGroup

	errs := make(chan error, workerCount)

	for worker := range workerCount {
		wg.Go(func() {
			errs <- recordWorkerFindings(stateFile, worker, concurrentRounds)
		})
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("worker error = %v", err)
		}
	}

	assertAllWorkerFindings(t, stateFile, workerCount, concurrentRounds)
}

func TestStoreConcurrentProcessesKeepEveryFinding(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	cmds := make([]*exec.Cmd, 0, processCount)

	for worker := range processCount {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperRecordFindings$")

		cmd.Env = append(os.Environ(),
			helperEnv+"=1",
			helperStateEnv+"="+stateFile,
			helperWorkerEnv+"="+strconv.Itoa(worker),
			helperRoundsEnv+"="+strconv.Itoa(concurrentRounds),
		)

		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}

		cmds = append(cmds, cmd)
	}

	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("helper process error = %v", err)
		}
	}

	assertAllWorkerFindings(t, stateFile, processCount, concurrentRounds)
}

// TestHelperRecordFindings runs only as a child process of
// TestStoreConcurrentProcessesKeepEveryFinding.
func TestHelperRecordFindings(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("helper process only")
	}

	worker, err := strconv.Atoi(os.Getenv(helperWorkerEnv))
	if err != nil {
		t.Fatal(err)
	}

	rounds, err := strconv.Atoi(os.Getenv(helperRoundsEnv))
	if err != nil {
		t.Fatal(err)
	}

	if err := recordWorkerFindings(os.Getenv(helperStateEnv), worker, rounds); err != nil {
		t.Fatal(err)
	}
}

func TestStoreLockTimeoutLeavesStateUntouched(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	store := NewStore(WithStateFile(stateFile), WithLockTimeout(20*time.Millisecond))

	if err := store.Start(hook.ProviderClaude, "sess-1"); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}

	lock, err := filelock.Acquire(store.lockFile(), time.Second)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = lock.Release() }()

	err = store.Record(workerContext(hook.ProviderClaude, "sess-1"), workerErrors(0, 0), nil)
	if !errors.Is(err, filelock.ErrTimeout) {
		t.Fatalf("Record() error = %v, want filelock.ErrTimeout", err)
	}

	after, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}

	if string(before) != string(after) {
		t.Fatal("state changed although the lock was not taken")
	}
}

func TestStoreUpdateFailsWhenLockCannotBeCreated(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewStore(WithStateFile(filepath.Join(parent, "state.json")))
	if err := store.Start(hook.ProviderClaude, "sess-1"); err == nil {
		t.Fatal("Start() succeeded under a file, want error")
	}
}

func TestStoreInterruptedWriteKeepsPriorState(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	store := NewStore(WithStateFile(stateFile))

	if err := store.Record(
		workerContext(hook.ProviderClaude, "sess-1"),
		workerErrors(0, 0),
		nil,
	); err != nil {
		t.Fatal(err)
	}

	orphan := filepath.Join(dir, "state.json.123456.tmp")
	legacy := filepath.Join(dir, "state.json.tmp")
	fresh := filepath.Join(dir, "state.json.654321.tmp")

	for _, path := range []string{orphan, legacy, fresh} {
		if err := os.WriteFile(path, []byte(`{"sessions": {"trunc`), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	old := time.Now().Add(-2 * orphanedTempAge)
	for _, path := range []string{orphan, legacy} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	stored, err := store.CombinedErrors(hook.ProviderClaude, "sess-1")
	if err != nil {
		t.Fatalf("CombinedErrors() error = %v", err)
	}

	if len(stored) != 1 {
		t.Fatalf("len(stored) = %d, want 1 after an interrupted write", len(stored))
	}

	if err := store.Record(
		workerContext(hook.ProviderClaude, "sess-1"),
		workerErrors(0, 1),
		nil,
	); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{orphan, legacy} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("orphaned temp file %s still present (err = %v)", path, err)
		}
	}

	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("recent temp file removed: %v", err)
	}
}

func TestStoreWritesLeaveNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(WithStateFile(filepath.Join(dir, "state.json")))

	for round := range 3 {
		if err := store.Record(
			workerContext(hook.ProviderClaude, "sess-1"),
			workerErrors(0, round),
			nil,
		); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), tempSuffix) {
			t.Fatalf("temp file left behind: %s", entry.Name())
		}
	}

	info, err := os.Stat(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != stateFileMode {
		t.Fatalf("state file mode = %v, want %v", info.Mode().Perm(), os.FileMode(stateFileMode))
	}
}

func TestWriteFileAtomicConcurrentWritersUseOwnTempFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	var wg sync.WaitGroup

	errs := make(chan error, workerCount*concurrentRounds)

	for worker := range workerCount {
		wg.Go(func() {
			for round := range concurrentRounds {
				payload := fmt.Sprintf(`{"worker": %d, "round": %d}`, worker, round)
				errs <- writeFileAtomic(path, []byte(payload))
			}
		})
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("writeFileAtomic() error = %v", err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(string(data), `{"worker": `) {
		t.Fatalf("state content = %q, want one complete payload", data)
	}
}

func TestWriteFileAtomicErrors(t *testing.T) {
	dir := t.TempDir()

	if err := writeFileAtomic(filepath.Join(dir, "missing", "state.json"), nil); err == nil {
		t.Fatal("writeFileAtomic() into a missing directory succeeded")
	}

	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(target, "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(target, []byte("{}")); err == nil {
		t.Fatal("writeFileAtomic() over a non-empty directory succeeded")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), tempSuffix) {
			t.Fatalf("temp file left behind after a failed rename: %s", entry.Name())
		}
	}
}

func TestStoreKeepsActiveSessionsPastRetention(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	start := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	now := start
	retention := 4 * time.Hour

	store := NewStore(
		WithStateFile(stateFile),
		WithTimeFunc(func() time.Time { return now }),
		WithRetention(retention),
	)

	active := workerContext(hook.ProviderClaude, "active")
	idle := workerContext(hook.ProviderClaude, "idle")

	if err := store.Record(active, workerErrors(0, 0), nil); err != nil {
		t.Fatal(err)
	}

	if err := store.Record(idle, workerErrors(1, 0), nil); err != nil {
		t.Fatal(err)
	}

	for range 6 {
		now = now.Add(time.Hour)

		if err := store.Record(active, nil, nil); err != nil {
			t.Fatal(err)
		}
	}

	if now.Sub(start) <= retention {
		t.Fatalf("test did not advance past retention")
	}

	activeErrs, err := store.CombinedErrors(hook.ProviderClaude, "active")
	if err != nil {
		t.Fatal(err)
	}

	if len(activeErrs) != 1 {
		t.Fatalf("active session findings = %d, want 1", len(activeErrs))
	}

	idleErrs, err := store.CombinedErrors(hook.ProviderClaude, "idle")
	if err != nil {
		t.Fatal(err)
	}

	if len(idleErrs) != 0 {
		t.Fatalf("idle session findings = %d, want 0 after retention", len(idleErrs))
	}
}

func TestStoreCompletionGateReadsKeepSessionAlive(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	now := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	retention := 4 * time.Hour

	store := NewStore(
		WithStateFile(stateFile),
		WithTimeFunc(func() time.Time { return now }),
		WithRetention(retention),
	)

	if err := store.Record(
		workerContext(hook.ProviderClaude, "sess"),
		workerErrors(0, 0),
		nil,
	); err != nil {
		t.Fatal(err)
	}

	for range 6 {
		now = now.Add(time.Hour)

		stored, err := store.CombinedErrors(hook.ProviderClaude, "sess")
		if err != nil {
			t.Fatal(err)
		}

		if len(stored) != 1 {
			t.Fatalf("findings = %d at %s, want 1", len(stored), now)
		}
	}
}

func recordWorkerFindings(stateFile string, worker, rounds int) error {
	store := NewStore(WithStateFile(stateFile), WithLockTimeout(testLockTimeout))
	ownSession := "worker-" + strconv.Itoa(worker)

	for round := range rounds {
		errs := workerErrors(worker, round)

		if err := store.Record(
			workerContext(hook.ProviderClaude, sharedSession),
			errs,
			nil,
		); err != nil {
			return err
		}

		if err := store.Record(
			workerContext(hook.ProviderCodex, ownSession),
			errs,
			nil,
		); err != nil {
			return err
		}

		if _, err := store.RecordCompletionBlock(
			hook.ProviderClaude,
			sharedSession,
			"Stop",
			true,
		); err != nil {
			return err
		}
	}

	return nil
}

func assertAllWorkerFindings(t *testing.T, stateFile string, workers, rounds int) {
	t.Helper()

	store := NewStore(WithStateFile(stateFile))

	st, err := store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}

	shared := st.Sessions[sessionKey(hook.ProviderClaude, sharedSession)]
	if shared == nil {
		t.Fatal("shared session missing")
	}

	want := min(workers*rounds, maxUnresolved)
	if len(shared.Findings) != want {
		t.Fatalf("shared findings = %d, want %d", len(shared.Findings), want)
	}

	if got := shared.CompletionBlocks["Stop"]; got != workers*rounds {
		t.Fatalf("completion blocks = %d, want %d", got, workers*rounds)
	}

	for worker := range workers {
		own := st.Sessions[sessionKey(hook.ProviderCodex, "worker-"+strconv.Itoa(worker))]
		if own == nil {
			t.Fatalf("session of worker %d missing", worker)
		}

		if len(own.Findings) != rounds {
			t.Fatalf("worker %d findings = %d, want %d", worker, len(own.Findings), rounds)
		}
	}
}

func workerContext(provider hook.Provider, sessionID string) *hook.Context {
	return &hook.Context{
		Provider:   provider,
		Event:      hook.CanonicalEventAfterTool,
		SessionID:  sessionID,
		ToolName:   hook.ToolTypeBash,
		ToolFamily: hook.ToolFamilyShell,
		ToolInput:  hook.ToolInput{Command: "git status"},
	}
}

func workerErrors(worker, round int) []*dispatcher.ValidationError {
	return []*dispatcher.ValidationError{
		{
			Validator:   "test.concurrency",
			Resource:    fmt.Sprintf("command:worker-%d-round-%d", worker, round),
			Message:     fmt.Sprintf("finding %d/%d", worker, round),
			ShouldBlock: true,
		},
	}
}
