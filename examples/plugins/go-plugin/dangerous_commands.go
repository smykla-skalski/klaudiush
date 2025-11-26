// Package main implements a sample klaudiush Go plugin that blocks dangerous shell commands.
//
// Build:
//
//	go build -buildmode=plugin -o dangerous_commands.so dangerous_commands.go
//
// Install:
//
//	cp dangerous_commands.so ~/.klaudiush/plugins/
//
// Configure in ~/.klaudiush/config.toml:
//
//	[[plugins.plugins]]
//	name = "dangerous-commands"
//	type = "go"
//	path = "~/.klaudiush/plugins/dangerous_commands.so"
//
//	[plugins.plugins.predicate]
//	event_types = ["PreToolUse"]
//	tool_types = ["Bash"]
//
//	[plugins.plugins.config]
//	blocked_commands = ["rm -rf /", "dd if=/dev/zero", "mkfs", ":(){ :|:& };:"]
package main

import (
	"strings"

	"github.com/smykla-labs/klaudiush/pkg/plugin"
)

// DangerousCommandsPlugin blocks potentially dangerous shell commands.
type DangerousCommandsPlugin struct{}

// Info returns plugin metadata.
func (*DangerousCommandsPlugin) Info() plugin.Info {
	return plugin.Info{
		Name:        "dangerous-commands",
		Version:     "1.0.0",
		Description: "Blocks dangerous shell commands",
		Author:      "klaudiush",
		URL:         "https://github.com/smykla-labs/klaudiush/examples/plugins/go-plugin",
	}
}

// Validate checks if command contains dangerous patterns.
func (p *DangerousCommandsPlugin) Validate(req *plugin.ValidateRequest) *plugin.ValidateResponse {
	// Only validate bash commands
	if req.ToolName != "Bash" {
		return plugin.PassResponse()
	}

	// Get blocked commands from config, use defaults if not configured
	blockedCommands := p.getBlockedCommands(req.Config)

	// Check each blocked pattern
	for _, blocked := range blockedCommands {
		if strings.Contains(req.Command, blocked) {
			return plugin.FailWithCode(
				"DANGEROUS_CMD",
				"Dangerous command blocked: "+blocked,
				"Avoid destructive commands or use safer alternatives",
				"https://github.com/smykla-labs/klaudiush/blob/main/docs/PLUGIN_GUIDE.md",
			)
		}
	}

	return plugin.PassResponse()
}

func (*DangerousCommandsPlugin) getBlockedCommands(cfg map[string]any) []string {
	// Default blocked patterns
	defaults := []string{
		"rm -rf /",
		"dd if=/dev/zero",
		"mkfs",
		":(){ :|:& };:", // fork bomb
	}

	// Try to get from config
	if cfg == nil {
		return defaults
	}

	patterns, ok := cfg["blocked_commands"].([]any)
	if !ok {
		return defaults
	}

	// Convert any slice to string slice
	var result []string

	for _, p := range patterns {
		if str, ok := p.(string); ok {
			result = append(result, str)
		}
	}

	if len(result) == 0 {
		return defaults
	}

	return result
}

// Plugin is the exported symbol that klaudiush will load.
//
//nolint:gochecknoglobals // Required for Go plugin interface
var Plugin DangerousCommandsPlugin
