//go:build windows

package protection

import "os"

// hasOtherNames reports true for every regular file: NTFS supports hard
// links, and the link count is not in os.FileInfo, so checkHardLink always
// compares the file with the protected ones through os.SameFile.
func hasOtherNames(info os.FileInfo) bool {
	return info.Mode().IsRegular()
}
