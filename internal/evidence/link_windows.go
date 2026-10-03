//go:build windows

package evidence

import "os"

// linkedElsewhere compares a regular file with the protected files through
// os.SameFile: NTFS supports hard links, and os.FileInfo has no link count.
func linkedElsewhere(info os.FileInfo, protected func(os.FileInfo) bool) bool {
	return info.Mode().IsRegular() && protected(info)
}
