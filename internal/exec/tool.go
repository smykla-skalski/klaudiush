package exec

//go:generate mockgen -source=tool.go -destination=tool_mock.go -package=exec

import "os/exec"

// ToolChecker checks for tool availability in PATH.
type ToolChecker interface {
	// IsAvailable checks if a tool is available in PATH.
	IsAvailable(tool string) bool

	// RequireTool returns an error if the tool is not available.
	RequireTool(tool string) error

	// FindTool returns the first available tool from the list of alternatives.
	// Returns empty string if none are available.
	FindTool(alternatives ...string) string
}

// toolChecker implements ToolChecker.
type toolChecker struct{}

// NewToolChecker creates a new ToolChecker.
func NewToolChecker() *toolChecker {
	return &toolChecker{}
}

// IsAvailable checks if a tool is available in PATH.
func (*toolChecker) IsAvailable(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

// RequireTool returns an error if the tool is not available.
func (t *toolChecker) RequireTool(tool string) error {
	if !t.IsAvailable(tool) {
		return &ToolNotFoundError{Tool: tool}
	}

	return nil
}

// FindTool returns the first available tool from the list of alternatives.
func (t *toolChecker) FindTool(alternatives ...string) string {
	for _, tool := range alternatives {
		if t.IsAvailable(tool) {
			return tool
		}
	}

	return ""
}

// ToolNotFoundError is returned when a required tool is not found.
type ToolNotFoundError struct {
	Tool string
}

// Error returns the error message.
func (e *ToolNotFoundError) Error() string {
	return "tool not found in PATH: " + e.Tool
}
