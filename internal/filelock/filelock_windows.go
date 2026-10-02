//go:build windows

package filelock

import (
	"os"

	"github.com/cockroachdb/errors"
	"golang.org/x/sys/windows"
)

// lockRange locks one byte; LockFileEx only needs any agreed-on range.
const lockRange = 1

func tryLock(file *os.File) error {
	overlapped := new(windows.Overlapped)

	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		lockRange,
		0,
		overlapped,
	)

	switch {
	case err == nil:
		return nil
	case errors.Is(err, windows.ERROR_LOCK_VIOLATION):
		return errLocked
	default:
		return errors.Wrap(err, "LockFileEx")
	}
}

func unlock(file *os.File) error {
	return windows.UnlockFileEx(
		windows.Handle(file.Fd()),
		0,
		lockRange,
		0,
		new(windows.Overlapped),
	)
}
