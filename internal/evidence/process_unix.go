//go:build unix

package evidence

import (
	"github.com/cockroachdb/errors"
	"golang.org/x/sys/unix"
)

// ProcessAlive reports whether a process with the given ID exists.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	err := unix.Kill(pid, 0)

	return err == nil || errors.Is(err, unix.EPERM)
}
