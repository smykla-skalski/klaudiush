//go:build !windows

package protection

import (
	"os"
	"syscall"
)

// hasOtherNames reports whether the file has more than one hard link.
func hasOtherNames(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)

	return ok && stat.Nlink > 1
}
