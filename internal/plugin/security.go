package plugin

import (
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
)

// Constants for plugin directory configuration.
const (
	// GlobalPluginDir is the user's global plugin directory relative to home.
	GlobalPluginDir = ".klaudiush/plugins"

	// ProjectPluginDir is the project-local plugin directory.
	ProjectPluginDir = ".klaudiush/plugins"

	// maxPanicMessageLen is the maximum length for sanitized panic messages.
	maxPanicMessageLen = 200

	// maxAllowedDirs is the maximum number of allowed directories (global + project).
	maxAllowedDirs = 2
)

// Sentinel errors for security validation.
var (
	// ErrPathTraversal is returned when path traversal patterns are detected.
	ErrPathTraversal = errors.New("path traversal detected")

	// ErrPathNotAllowed is returned when the plugin path is not in an allowed directory.
	ErrPathNotAllowed = errors.New("plugin path not in allowed directory")

	// ErrInvalidExtension is returned when the plugin file extension is not allowed.
	ErrInvalidExtension = errors.New("invalid plugin file extension")

	// ErrDangerousChars is returned when dangerous characters are found in the path.
	ErrDangerousChars = errors.New("dangerous characters in path")

	// ErrLoaderClosed is returned when attempting to use a closed loader.
	ErrLoaderClosed = errors.New("loader has been closed")

	// ErrInsecureRemote is returned when attempting insecure connection to remote host.
	ErrInsecureRemote = errors.New("insecure connection to remote host")
)

// dangerousChars contains shell metacharacters to reject in paths.
// While exec.Command doesn't interpret these, rejecting them adds defense-in-depth.
var dangerousChars = []byte{';', '|', '&', '$', '`', '"', '\'', '<', '>', '(', ')'}

// pathTraversalPattern matches common path traversal attempts.
var pathTraversalPattern = regexp.MustCompile(`(?:^|/)\.\.(?:/|$)`)

// filePathPattern matches typical file paths to remove from panic messages.
var filePathPattern = regexp.MustCompile(`(?:/[a-zA-Z0-9._-]+)+(?:\.[a-zA-Z0-9]+)?`)

// ValidatePath performs comprehensive path validation for plugin files.
// It checks for:
//   - Path traversal attempts (../)
//   - Path containment within allowed directories
//   - Symlink resolution
func ValidatePath(path string, allowedDirs []string) error {
	if path == "" {
		return errors.New("path is required")
	}

	resolvedPath, err := resolvePath(path)
	if err != nil {
		return err
	}

	if len(allowedDirs) == 0 {
		return nil
	}

	if !isPathInAllowedDirs(resolvedPath, allowedDirs) {
		return errors.Wrapf(ErrPathNotAllowed, "path %s not in allowed directories", path)
	}

	return nil
}

// resolvePath expands, validates, and resolves a path to its canonical form.
func resolvePath(path string) (string, error) {
	expandedPath, err := expandPath(path)
	if err != nil {
		return "", errors.Wrap(err, "failed to expand path")
	}

	if pathTraversalPattern.MatchString(expandedPath) {
		return "", errors.Wrapf(ErrPathTraversal, "path contains traversal pattern: %s", path)
	}

	resolvedPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return "", errors.Wrap(err, "failed to resolve absolute path")
	}

	resolvedPath, err = evalSymlinksIfExists(resolvedPath)
	if err != nil {
		return "", err
	}

	return resolvedPath, nil
}

// evalSymlinksIfExists resolves symlinks for the path.
// If the file doesn't exist, it resolves symlinks on the parent directory
// and joins with the filename to ensure consistent path representation.
func evalSymlinksIfExists(path string) (string, error) {
	_, statErr := os.Stat(path)
	if statErr == nil {
		// File exists, resolve the full path
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", errors.Wrap(err, "failed to evaluate symlinks")
		}

		return realPath, nil
	}

	// File doesn't exist; resolve symlinks on the parent directory
	// This ensures consistent path representation even for new files
	parentDir := filepath.Dir(path)
	fileName := filepath.Base(path)

	realParentDir, err := filepath.EvalSymlinks(parentDir)
	if err != nil {
		// Parent directory doesn't exist either; return original path
		return path, nil //nolint:nilerr // intentional: return original if parent doesn't exist
	}

	return filepath.Join(realParentDir, fileName), nil
}

// isPathInAllowedDirs checks if the resolved path is within any allowed directory.
func isPathInAllowedDirs(resolvedPath string, allowedDirs []string) bool {
	for _, allowedDir := range allowedDirs {
		if isPathUnderDir(resolvedPath, allowedDir) {
			return true
		}
	}

	return false
}

// isPathUnderDir checks if resolvedPath is under the given directory.
func isPathUnderDir(resolvedPath, dir string) bool {
	expandedDir, err := expandPath(dir)
	if err != nil {
		return false
	}

	absDir, err := filepath.Abs(expandedDir)
	if err != nil {
		return false
	}

	// Resolve symlinks on the allowed directory if it exists
	// This ensures consistency with the resolved path (which also has symlinks resolved)
	if _, statErr := os.Stat(absDir); statErr == nil {
		if realDir, evalErr := filepath.EvalSymlinks(absDir); evalErr == nil {
			absDir = realDir
		}
	}

	// Normalize directory path with trailing separator
	normalizedDir := absDir
	if !strings.HasSuffix(normalizedDir, string(filepath.Separator)) {
		normalizedDir += string(filepath.Separator)
	}

	// Check for exact directory match (path equals allowed directory)
	if resolvedPath == absDir ||
		resolvedPath == strings.TrimSuffix(normalizedDir, string(filepath.Separator)) {
		return true
	}

	return strings.HasPrefix(resolvedPath, normalizedDir)
}

// ValidateExtension checks if the file has an allowed extension.
func ValidateExtension(path string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}

	ext := filepath.Ext(path)
	if ext == "" {
		return errors.Wrap(ErrInvalidExtension, "file has no extension")
	}

	for _, allowedExt := range allowed {
		if strings.EqualFold(ext, allowedExt) {
			return nil
		}
	}

	return errors.Wrapf(ErrInvalidExtension,
		"extension %q not in allowed list %v", ext, allowed)
}

// ValidateMetachars rejects paths containing shell metacharacters.
// This is defense-in-depth since exec.Command doesn't interpret these,
// but it prevents accidental issues and suspicious paths.
func ValidateMetachars(path string) error {
	for _, char := range dangerousChars {
		if strings.ContainsRune(path, rune(char)) {
			return errors.Wrapf(ErrDangerousChars, "path contains forbidden character: %c", char)
		}
	}

	return nil
}

// GetAllowedDirs returns the list of allowed plugin directories.
// Returns both the global (~/.klaudiush/plugins) and project (.klaudiush/plugins) directories.
func GetAllowedDirs(projectRoot string) ([]string, error) {
	dirs := make([]string, 0, maxAllowedDirs)

	// Add global plugin directory
	homeDir, err := os.UserHomeDir()
	if err == nil {
		globalDir := filepath.Join(homeDir, GlobalPluginDir)
		dirs = append(dirs, globalDir)
	}

	// Add project plugin directory if projectRoot is provided
	if projectRoot != "" {
		projectDir := filepath.Join(projectRoot, ProjectPluginDir)
		dirs = append(dirs, projectDir)
	}

	if len(dirs) == 0 {
		return nil, errors.New("no allowed directories could be determined")
	}

	return dirs, nil
}

// IsLocalAddress checks if the address refers to localhost.
// Supports:
//   - localhost (with or without port)
//   - 127.0.0.1 (with or without port)
//   - ::1 and [::1] (with or without port)
//   - 0.0.0.0 (with or without port) - typically used for binding, but treated as local
func IsLocalAddress(address string) bool {
	if address == "" {
		return false
	}

	// Extract host from host:port if present
	host := address
	if h, _, err := net.SplitHostPort(address); err == nil {
		host = h
	}

	// Check various localhost representations
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0":
		return true
	}

	// Handle IPv6 with brackets (e.g., [::1])
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		innerHost := host[1 : len(host)-1]
		if innerHost == "::1" {
			return true
		}
	}

	// Parse as IP and check if loopback
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}

	return false
}

// SanitizePanicMessage removes sensitive data from panic messages.
// It removes file paths and limits the message length.
func SanitizePanicMessage(msg string) string {
	if msg == "" {
		return msg
	}

	// Remove file paths
	sanitized := filePathPattern.ReplaceAllString(msg, "[path]")

	// Limit length
	if len(sanitized) > maxPanicMessageLen {
		sanitized = sanitized[:maxPanicMessageLen] + "..."
	}

	return sanitized
}

// expandPath expands ~ at the beginning of a path to the user's home directory.
func expandPath(path string) (string, error) {
	if path == "" {
		return path, nil
	}

	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, "failed to get home directory")
	}

	if path == "~" {
		return homeDir, nil
	}

	if len(path) > 1 && (path[1] == '/' || path[1] == filepath.Separator) {
		return filepath.Join(homeDir, path[2:]), nil
	}

	// ~user style paths not supported
	return path, nil
}
