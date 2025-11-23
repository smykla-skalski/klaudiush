package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathViolation represents a protected path violation.
type PathViolation struct {
	Path       string
	Operation  WriteOp
	Location   Location
	Suggestion string
}

// String returns a string representation of the path violation.
func (v *PathViolation) String() string {
	return fmt.Sprintf(
		"❌ Cannot write to %s (line %d, col %d)\n%s",
		v.Path,
		v.Location.Line,
		v.Location.Column,
		v.Suggestion,
	)
}

// PathValidator validates file paths for protected locations.
type PathValidator struct {
	projectRoot string
}

// NewPathValidator creates a new PathValidator.
func NewPathValidator() *PathValidator {
	// Try to detect project root
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}

	return &PathValidator{
		projectRoot: root,
	}
}

// CheckProtectedPaths checks if any file writes target protected paths.
func (v *PathValidator) CheckProtectedPaths(writes []FileWrite) []PathViolation {
	violations := make([]PathViolation, 0)

	for _, write := range writes {
		if write.IsProtectedPath() {
			violations = append(violations, PathViolation{
				Path:       write.Path,
				Operation:  write.Operation,
				Location:   write.Location,
				Suggestion: v.getSuggestion(write.Path),
			})
		}
	}

	return violations
}

// getSuggestion generates a helpful suggestion for the user.
func (v *PathValidator) getSuggestion(path string) string {
	var suggestion strings.Builder

	suggestion.WriteString("💡 Use project-local tmp/ directory instead:\n")
	suggestion.WriteString("   - Create: mkdir -p tmp/\n")
	suggestion.WriteString(fmt.Sprintf("   - Use: %s\n", v.getLocalPath(path)))
	suggestion.WriteString("   - Add to .git/info/exclude if needed")

	return suggestion.String()
}

// getLocalPath converts a /tmp path to a local tmp/ path.
func (*PathValidator) getLocalPath(path string) string {
	// Extract filename from path
	filename := filepath.Base(path)

	return filepath.Join("tmp", filename)
}
