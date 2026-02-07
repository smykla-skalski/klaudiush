// Package plugin provides the public API for klaudiush plugin authors.
//
// Plugins extend klaudiush with custom validation logic and can be written
// in any language that supports one of the plugin interfaces:
//   - Go plugins (.so files) for native performance
//   - gRPC plugins for persistent connections and cross-language support
//   - Exec plugins (JSON over stdin/stdout) for maximum compatibility
//
// Example Go plugin:
//
//	package main
//
//	import "github.com/smykla-labs/klaudiush/pkg/plugin"
//
//	type MyPlugin struct{}
//
//	func (p *MyPlugin) Info() plugin.Info {
//		return plugin.Info{
//			Name:        "my-plugin",
//			Version:     "1.0.0",
//			Description: "Custom validation logic",
//		}
//	}
//
//	func (p *MyPlugin) Validate(req *plugin.ValidateRequest) *plugin.ValidateResponse {
//		// Implement validation logic
//		return plugin.PassResponse()
//	}
//
//	var Plugin MyPlugin  // exported symbol "Plugin" required for Go plugins
package plugin

// Plugin is the interface that all plugins must implement.
type Plugin interface {
	// Info returns metadata about the plugin.
	Info() Info

	// Validate performs validation and returns a response.
	Validate(req *ValidateRequest) *ValidateResponse
}

// Info contains plugin metadata.
type Info struct {
	// Name is the unique plugin identifier.
	Name string `json:"name"`

	// Version is the plugin version (semver recommended).
	Version string `json:"version"`

	// Description is a human-readable description of what the plugin does.
	Description string `json:"description,omitempty"`

	// Author is the plugin author or organization.
	Author string `json:"author,omitempty"`

	// URL is a link to the plugin's homepage or documentation.
	URL string `json:"url,omitempty"`
}

// ValidateRequest contains the context passed to plugin validators.
type ValidateRequest struct {
	// EventType is the hook event type ("PreToolUse", "PostToolUse", "Notification").
	EventType string `json:"event_type"`

	// ToolName is the tool being invoked ("Bash", "Write", "Edit", etc.).
	ToolName string `json:"tool_name"`

	// Command is the shell command (for Bash tool).
	Command string `json:"command,omitempty"`

	// FilePath is the file path (for file operations).
	FilePath string `json:"file_path,omitempty"`

	// Content is the file content (for Write/Edit tools).
	Content string `json:"content,omitempty"`

	// OldString is the string to replace (for Edit tool).
	OldString string `json:"old_string,omitempty"`

	// NewString is the replacement string (for Edit tool).
	NewString string `json:"new_string,omitempty"`

	// Pattern is the search pattern (for Grep/Glob tools).
	Pattern string `json:"pattern,omitempty"`

	// Config contains plugin-specific configuration from the config file.
	// The structure depends on how the plugin is configured in config.toml.
	Config map[string]any `json:"config,omitempty"`
}

// ValidateResponse contains the validation result returned by a plugin.
type ValidateResponse struct {
	// Passed indicates whether the validation passed.
	// If false, the validation failed.
	Passed bool `json:"passed"`

	// ShouldBlock indicates whether this failure should block the operation.
	// Set to false to allow the operation with a warning.
	ShouldBlock bool `json:"should_block"`

	// Message is a human-readable message describing the result.
	Message string `json:"message,omitempty"`

	// ErrorCode is a unique identifier for this error type.
	// Used for programmatic error handling and documentation.
	ErrorCode string `json:"error_code,omitempty"`

	// FixHint provides a short suggestion for fixing the issue.
	FixHint string `json:"fix_hint,omitempty"`

	// DocLink is a URL to detailed documentation for this error.
	DocLink string `json:"doc_link,omitempty"`

	// Details contains additional structured information about the result.
	Details map[string]string `json:"details,omitempty"`
}

// PassResponse returns a response indicating validation passed.
func PassResponse() *ValidateResponse {
	return &ValidateResponse{
		Passed:      true,
		ShouldBlock: false,
	}
}

// PassWithMessage returns a response indicating validation passed with a message.
func PassWithMessage(message string) *ValidateResponse {
	return &ValidateResponse{
		Passed:      true,
		ShouldBlock: false,
		Message:     message,
	}
}

// FailResponse returns a response indicating validation failed and should block.
func FailResponse(message string) *ValidateResponse {
	return &ValidateResponse{
		Passed:      false,
		ShouldBlock: true,
		Message:     message,
	}
}

// WarnResponse returns a response indicating validation failed but should not block.
func WarnResponse(message string) *ValidateResponse {
	return &ValidateResponse{
		Passed:      false,
		ShouldBlock: false,
		Message:     message,
	}
}

// FailWithCode returns a response with an error code and optional hint/link.
func FailWithCode(code, message, fixHint, docLink string) *ValidateResponse {
	return &ValidateResponse{
		Passed:      false,
		ShouldBlock: true,
		Message:     message,
		ErrorCode:   code,
		FixHint:     fixHint,
		DocLink:     docLink,
	}
}

// WarnWithCode returns a warning response with an error code and optional hint/link.
func WarnWithCode(code, message, fixHint, docLink string) *ValidateResponse {
	return &ValidateResponse{
		Passed:      false,
		ShouldBlock: false,
		Message:     message,
		ErrorCode:   code,
		FixHint:     fixHint,
		DocLink:     docLink,
	}
}

// AddDetail adds a detail entry to the response.
func (r *ValidateResponse) AddDetail(key, value string) *ValidateResponse {
	if r.Details == nil {
		r.Details = make(map[string]string)
	}

	r.Details[key] = value

	return r
}
