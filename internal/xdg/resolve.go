package xdg

import "os"

// ResolveFile checks XDG path, then legacy ~/.klaudiush/ path.
// Returns XDG path if: file exists there, or file doesn't exist anywhere (new file).
// Returns legacy path only if: file exists there but not at the XDG location.
func ResolveFile(xdgPath, legacyPath string) string {
	if fileExists(xdgPath) {
		return xdgPath
	}

	if fileExists(legacyPath) {
		return legacyPath
	}

	// Neither exists - use XDG for new files
	return xdgPath
}

// ResolveDir checks XDG path, then legacy ~/.klaudiush/ path.
// Returns XDG path if: dir exists there, or dir doesn't exist anywhere (new dir).
// Returns legacy path only if: dir exists there but not at the XDG location.
func ResolveDir(xdgPath, legacyPath string) string {
	if dirExists(xdgPath) {
		return xdgPath
	}

	if dirExists(legacyPath) {
		return legacyPath
	}

	return xdgPath
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.IsDir()
}
