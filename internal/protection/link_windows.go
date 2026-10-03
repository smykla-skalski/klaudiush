//go:build windows

package protection

import "os"

// hasOtherNames reports false: Windows hard links are not inspected.
func hasOtherNames(os.FileInfo) bool {
	return false
}
