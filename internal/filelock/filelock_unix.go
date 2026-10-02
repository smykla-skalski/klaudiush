//go:build unix

package filelock

import (
	"os"

	"github.com/cockroachdb/errors"
	"golang.org/x/sys/unix"
)

// tryLock uses flock, which locks per open file description: goroutines of
// one process that open the file separately exclude each other too.
func tryLock(file *os.File) error {
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)

		switch {
		case err == nil:
			return nil
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EWOULDBLOCK):
			return errLocked
		default:
			return errors.Wrap(err, "flock")
		}
	}
}

func unlock(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
