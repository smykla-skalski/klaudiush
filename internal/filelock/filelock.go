// Package filelock serializes writers of a shared file across processes with
// an advisory lock held on a sibling lock file.
//
// The operating system drops the lock when its holder exits or crashes, so a
// lock file left on disk never blocks anyone. Lock files are never removed:
// removing one would let two processes lock different files of the same name.
package filelock

import (
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
)

const (
	lockFileMode = 0o600

	minRetryDelay = time.Millisecond
	maxRetryDelay = 25 * time.Millisecond
	retryBackoff  = 2
)

// ErrTimeout reports that another holder kept the lock past the timeout.
var ErrTimeout = errors.New("timed out waiting for file lock")

// errLocked reports that another holder has the lock right now.
var errLocked = errors.New("file lock held by another holder")

// Lock is an acquired exclusive lock. Release it exactly once.
type Lock struct {
	file *os.File
}

// Acquire takes the exclusive lock on path, creating the file when missing;
// its directory must exist. It waits up to timeout for other holders and
// returns an error wrapping ErrTimeout when they keep it longer.
func Acquire(path string, timeout time.Duration) (*Lock, error) {
	file, err := os.OpenFile(filepath.Clean(path), os.O_RDWR|os.O_CREATE, lockFileMode)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open lock file")
	}

	deadline := time.Now().Add(timeout)
	delay := minRetryDelay

	for {
		err = tryLock(file)
		if err == nil {
			return &Lock{file: file}, nil
		}

		if !errors.Is(err, errLocked) {
			_ = file.Close()

			return nil, errors.Wrapf(err, "failed to lock %s", path)
		}

		if !time.Now().Before(deadline) {
			_ = file.Close()

			return nil, errors.Wrapf(ErrTimeout, "lock %s held for over %s", path, timeout)
		}

		time.Sleep(min(delay, time.Until(deadline)))

		delay = min(delay*retryBackoff, maxRetryDelay)
	}
}

// Release unlocks and closes the lock file.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}

	file := l.file
	l.file = nil

	unlockErr := unlock(file)
	closeErr := file.Close()

	if unlockErr != nil {
		return errors.Wrap(unlockErr, "failed to unlock file")
	}

	if closeErr != nil {
		return errors.Wrap(closeErr, "failed to close lock file")
	}

	return nil
}
