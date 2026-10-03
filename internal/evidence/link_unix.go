//go:build !windows

package evidence

import (
	"os"
	"syscall"
)

// linkedElsewhere reports a regular file with more than one hard link. The
// other name may be any protected file, inside the repository or not, so a
// second name alone fails closed.
func linkedElsewhere(info os.FileInfo, _ func(os.FileInfo) bool) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)

	return info.Mode().IsRegular() && ok && stat.Nlink > 1
}
