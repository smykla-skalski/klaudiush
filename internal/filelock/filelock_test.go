package filelock

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
)

const (
	helperEnv      = "KLAUDIUSH_FILELOCK_HELPER"
	helperLockName = "state.lock"
	helperReady    = "locked"
	shortTimeout   = 50 * time.Millisecond
	generousWait   = 10 * time.Second
	counterWorkers = 16
	counterRounds  = 25
)

func TestAcquireAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")

	lock, err := Acquire(path, time.Second)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("lock file not created: %v", statErr)
	}

	if _, err = Acquire(path, shortTimeout); !errors.Is(err, ErrTimeout) {
		t.Fatalf("second Acquire() error = %v, want ErrTimeout", err)
	}

	if err = lock.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	if err = lock.Release(); err != nil {
		t.Fatalf("second Release() error = %v", err)
	}

	again, err := Acquire(path, shortTimeout)
	if err != nil {
		t.Fatalf("Acquire() after release error = %v", err)
	}

	if err := again.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}

func TestReleaseNilLock(t *testing.T) {
	var lock *Lock
	if err := lock.Release(); err != nil {
		t.Fatalf("nil Release() error = %v", err)
	}
}

func TestAcquireFailsWhenDirectoryIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "state.lock")

	_, err := Acquire(path, shortTimeout)
	if err == nil || errors.Is(err, ErrTimeout) {
		t.Fatalf("Acquire() in a missing directory error = %v, want open error", err)
	}
}

func TestAcquireFailsWhenLockPathIsADirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := Acquire(path, shortTimeout)
	if err == nil || errors.Is(err, ErrTimeout) {
		t.Fatalf("Acquire() on a directory error = %v, want open error", err)
	}
}

func TestAcquireSerializesGoroutines(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "counter.lock")
	counterPath := filepath.Join(dir, "counter")

	var wg sync.WaitGroup

	errs := make(chan error, counterWorkers*counterRounds)

	for range counterWorkers {
		wg.Go(func() {
			for range counterRounds {
				errs <- incrementCounter(lockPath, counterPath)
			}
		})
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("increment error = %v", err)
		}
	}

	if got := readCounter(t, counterPath); got != counterWorkers*counterRounds {
		t.Fatalf("counter = %d, want %d", got, counterWorkers*counterRounds)
	}
}

func TestLockHeldByAnotherProcessUntilItDies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, helperLockName)

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperHoldLock$")
	cmd.Dir = dir

	cmd.Env = append(os.Environ(), helperEnv+"=1")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	waitForLine(t, bufio.NewScanner(stdout), helperReady)

	if _, err = Acquire(path, shortTimeout); !errors.Is(err, ErrTimeout) {
		t.Fatalf("Acquire() while another process holds it error = %v, want ErrTimeout", err)
	}

	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	_ = cmd.Wait()

	lock, err := Acquire(path, generousWait)
	if err != nil {
		t.Fatalf("Acquire() after holder died error = %v", err)
	}

	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
}

// TestHelperHoldLock runs only as a child of
// TestLockHeldByAnotherProcessUntilItDies: it takes the lock and waits to be
// killed.
func TestHelperHoldLock(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("helper process only")
	}

	lock, err := Acquire(helperLockName, generousWait)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = lock.Release() }()

	_, _ = os.Stdout.WriteString(helperReady + "\n")

	time.Sleep(time.Minute)
}

func incrementCounter(lockPath, counterPath string) (err error) {
	lock, err := Acquire(lockPath, generousWait)
	if err != nil {
		return err
	}

	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil && err == nil {
			err = releaseErr
		}
	}()

	value := 0

	data, err := os.ReadFile(counterPath)
	if err == nil {
		value, err = strconv.Atoi(string(data))
		if err != nil {
			return errors.Wrap(err, "corrupt counter")
		}
	} else if !os.IsNotExist(err) {
		return errors.Wrap(err, "read counter")
	}

	return os.WriteFile(counterPath, []byte(strconv.Itoa(value+1)), 0o600)
}

func waitForLine(t *testing.T, scanner *bufio.Scanner, want string) {
	t.Helper()

	for scanner.Scan() {
		if scanner.Text() == want {
			return
		}
	}

	t.Fatalf("helper exited before printing %q", want)
}

func readCounter(t *testing.T, path string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	value, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}

	return value
}
