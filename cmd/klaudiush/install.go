// Package main provides the CLI entry point for klaudiush.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/charmbracelet/huh"
	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-skalski/klaudiush/internal/doctor/fixers"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/internal/tui"
)

const defaultHookTimeout = 30

var (
	installGlobal  bool
	installProject bool
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Register klaudiush in Claude Code settings",
	Long: `Register klaudiush as a PreToolUse hook in Claude Code settings.json.

By default, prompts to choose between global (~/.claude/settings.json) and
project (.claude/settings.json) installation. Use --global or --project to
skip the prompt.`,
	RunE: runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)

	installCmd.Flags().BoolVarP(
		&installGlobal,
		"global",
		"g",
		false,
		"Install to ~/.claude/settings.json",
	)

	installCmd.Flags().BoolVarP(
		&installProject,
		"project",
		"p",
		false,
		"Install to .claude/settings.json",
	)

	installCmd.MarkFlagsMutuallyExclusive("global", "project")
}

func runInstall(_ *cobra.Command, _ []string) error {
	settingsPath, err := resolveInstallTarget()
	if err != nil {
		return err
	}

	binaryPath, err := exec.LookPath("klaudiush")
	if err != nil {
		return errors.Wrap(err, "klaudiush not found in PATH")
	}

	return performInstall(settingsPath, binaryPath)
}

func resolveInstallTarget() (string, error) {
	if installGlobal {
		return settings.GetUserSettingsPath(), nil
	}

	if installProject {
		return settings.GetProjectSettingsPath(), nil
	}

	return promptInstallTarget()
}

func promptInstallTarget() (string, error) {
	if !tui.IsTerminal() {
		return "", errors.New("no terminal detected, use --global or --project")
	}

	var target string

	err := huh.NewSelect[string]().
		Title("Where should klaudiush be installed?").
		Options(
			huh.NewOption("Global (~/.claude/settings.json)", "global"),
			huh.NewOption("Project (.claude/settings.json)", "project"),
		).
		Value(&target).
		Run()
	if err != nil {
		return "", errors.Wrap(err, "prompt failed")
	}

	if target == "global" {
		return settings.GetUserSettingsPath(), nil
	}

	return settings.GetProjectSettingsPath(), nil
}

func performInstall(settingsPath, binaryPath string) error {
	// Check if already registered
	parser := settings.NewSettingsParser(settingsPath)

	registered, err := parser.IsDispatcherRegistered(binaryPath)
	if err != nil {
		return errors.Wrap(err, "failed to check settings")
	}

	if registered {
		fmt.Printf("klaudiush is already registered in %s\n", settingsPath)
		return nil
	}

	// Load existing settings as raw map to preserve unknown fields
	raw, err := loadRawSettings(settingsPath)
	if err != nil {
		return err
	}

	addHookToSettings(raw, binaryPath)

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal settings")
	}

	data = append(data, '\n')

	if err := fixers.AtomicWriteFile(settingsPath, data, true); err != nil {
		return errors.Wrap(err, "failed to write settings")
	}

	fmt.Printf("klaudiush registered in %s\n", settingsPath)

	return nil
}

func loadRawSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is from CLI flags or settings helper
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}

		return nil, errors.Wrap(err, "failed to read settings")
	}

	if len(data) == 0 {
		return make(map[string]any), nil
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.Wrap(err, "failed to parse settings")
	}

	return raw, nil
}

func addHookToSettings(raw map[string]any, binaryPath string) {
	hooks, ok := raw["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
		raw["hooks"] = hooks
	}

	entry := map[string]any{
		"matcher": "Bash|Write|Edit",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": binaryPath + " --hook-type PreToolUse",
				"timeout": defaultHookTimeout,
			},
		},
	}

	existing, ok := hooks["PreToolUse"].([]any)
	if !ok {
		existing = nil
	}

	hooks["PreToolUse"] = append(existing, entry)
}
