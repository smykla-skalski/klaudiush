package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/pelletier/go-toml/v2"
	"github.com/pmezard/go-difflib/difflib"

	internalconfig "github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/internal/prompt"
	"github.com/smykla-skalski/klaudiush/internal/schema"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	defaultCodexHooksPath     = "~/.codex/hooks.json"
	defaultGeminiSettingsPath = "~/.gemini/settings.json"
	configDiffContextLines    = 3
)

type providerSelection struct {
	ClaudeEnabled      bool
	CodexEnabled       bool
	CodexHooksPath     string
	GeminiEnabled      bool
	GeminiSettingsPath string
}

func providerSelectionFromConfig(cfg *pkgConfig.Config) providerSelection {
	selection := providerSelection{
		ClaudeEnabled: true,
	}

	if cfg == nil || cfg.Providers == nil {
		return selection
	}

	selection.ClaudeEnabled = cfg.GetProviders().GetClaude().IsEnabled()
	selection.CodexEnabled = cfg.GetProviders().GetCodex().IsEnabled()
	selection.CodexHooksPath = cfg.GetProviders().GetCodex().HooksConfigPath
	selection.GeminiEnabled = cfg.GetProviders().GetGemini().IsEnabled()
	selection.GeminiSettingsPath = cfg.GetProviders().GetGemini().SettingsPath

	return selection
}

func resolveProviderSelection(
	providers []string,
	codexHooksPath string,
	geminiSettingsPath string,
	existing *pkgConfig.Config,
) (providerSelection, error) {
	selection := providerSelectionFromConfig(existing)

	if len(providers) > 0 {
		selection = providerSelection{}

		if err := applyProviderTokens(&selection, providers); err != nil {
			return providerSelection{}, err
		}
	}

	applyExplicitProviderPaths(&selection, codexHooksPath, geminiSettingsPath)
	fillProviderPathDefaults(&selection, providerSelectionFromConfig(existing))

	return selection, nil
}

func applyProviderSelection(
	existing *pkgConfig.Config,
	selection providerSelection,
) (*pkgConfig.Config, error) {
	updated, err := cloneConfig(existing)
	if err != nil {
		return nil, err
	}

	if updated.Version == 0 {
		updated.Version = pkgConfig.CurrentConfigVersion
	}

	claudeEnabled := selection.ClaudeEnabled
	codexEnabled := selection.CodexEnabled
	codexExperimental := selection.CodexEnabled
	geminiEnabled := selection.GeminiEnabled

	updated.Providers = &pkgConfig.ProvidersConfig{
		Claude: &pkgConfig.ClaudeProviderConfig{
			Enabled: &claudeEnabled,
		},
		Codex: &pkgConfig.CodexProviderConfig{
			Enabled:      &codexEnabled,
			Experimental: &codexExperimental,
		},
		Gemini: &pkgConfig.GeminiProviderConfig{
			Enabled: &geminiEnabled,
		},
	}

	if selection.CodexEnabled {
		updated.Providers.Codex.HooksConfigPath = selection.CodexHooksPath
	}

	if selection.GeminiEnabled {
		updated.Providers.Gemini.SettingsPath = selection.GeminiSettingsPath
	}

	return updated, nil
}

func promptProviderUpdate(
	prompter prompt.Prompter,
	out io.Writer,
	configPath string,
	existing *pkgConfig.Config,
) (*pkgConfig.Config, bool, error) {
	configureProviders, err := prompter.Confirm(
		"Configuration already exists. Configure provider integrations only?",
		true,
	)
	if err != nil {
		return nil, false, err
	}

	if !configureProviders {
		return nil, false, nil
	}

	current := providerSelectionFromConfig(existing)

	selection, err := promptProviderToggleSelection(prompter, current)
	if err != nil {
		return nil, true, err
	}

	pathErr := promptProviderPaths(prompter, current, &selection)
	if pathErr != nil {
		return nil, true, pathErr
	}

	updated, err := applyProviderSelection(existing, selection)
	if err != nil {
		return nil, true, err
	}

	diff, err := renderConfigDiff(configPath, existing, updated)
	if err != nil {
		return nil, true, err
	}

	if diff == "" {
		if _, writeErr := fmt.Fprintf(
			out,
			"No configuration changes are needed for %s\n",
			configPath,
		); writeErr != nil {
			return nil, true, errors.Wrap(writeErr, "failed to write init update output")
		}

		return updated, true, nil
	}

	if _, writeErr := fmt.Fprintf(
		out,
		"Proposed changes for %s:\n%s",
		configPath,
		diff,
	); writeErr != nil {
		return nil, true, errors.Wrap(writeErr, "failed to write init update output")
	}

	approved, err := prompter.Confirm("Apply these changes?", false)
	if err != nil {
		return nil, true, err
	}

	if !approved {
		return nil, true, nil
	}

	return updated, true, nil
}

func applyProviderTokens(selection *providerSelection, providers []string) error {
	for _, provider := range providers {
		for token := range strings.SplitSeq(provider, ",") {
			switch strings.ToLower(strings.TrimSpace(token)) {
			case "":
				continue
			case "claude":
				selection.ClaudeEnabled = true
			case "codex":
				selection.CodexEnabled = true
			case "gemini":
				selection.GeminiEnabled = true
			default:
				return errors.Errorf("unknown provider %q", token)
			}
		}
	}

	return nil
}

func applyExplicitProviderPaths(
	selection *providerSelection,
	codexHooksPath string,
	geminiSettingsPath string,
) {
	if codexHooksPath != "" {
		selection.CodexEnabled = true
		selection.CodexHooksPath = codexHooksPath
	}

	if geminiSettingsPath != "" {
		selection.GeminiEnabled = true
		selection.GeminiSettingsPath = geminiSettingsPath
	}
}

func fillProviderPathDefaults(selection *providerSelection, existing providerSelection) {
	if selection.CodexEnabled && selection.CodexHooksPath == "" {
		selection.CodexHooksPath = existing.CodexHooksPath
		if selection.CodexHooksPath == "" {
			selection.CodexHooksPath = defaultCodexHooksPath
		}
	}

	if !selection.CodexEnabled {
		selection.CodexHooksPath = ""
	}

	if selection.GeminiEnabled && selection.GeminiSettingsPath == "" {
		selection.GeminiSettingsPath = existing.GeminiSettingsPath
		if selection.GeminiSettingsPath == "" {
			selection.GeminiSettingsPath = defaultGeminiSettingsPath
		}
	}

	if !selection.GeminiEnabled {
		selection.GeminiSettingsPath = ""
	}
}

func promptProviderToggleSelection(
	prompter prompt.Prompter,
	current providerSelection,
) (providerSelection, error) {
	selection := providerSelection{}

	var err error

	selection.ClaudeEnabled, err = prompter.Confirm(
		"Enable Claude integration",
		current.ClaudeEnabled,
	)
	if err != nil {
		return providerSelection{}, err
	}

	selection.CodexEnabled, err = prompter.Confirm(
		"Enable Codex integration",
		current.CodexEnabled,
	)
	if err != nil {
		return providerSelection{}, err
	}

	selection.GeminiEnabled, err = prompter.Confirm(
		"Enable Gemini integration",
		current.GeminiEnabled,
	)
	if err != nil {
		return providerSelection{}, err
	}

	return selection, nil
}

func promptProviderPaths(
	prompter prompt.Prompter,
	current providerSelection,
	selection *providerSelection,
) error {
	if selection.CodexEnabled {
		defaultPath := current.CodexHooksPath
		if defaultPath == "" {
			defaultPath = defaultCodexHooksPath
		}

		value, err := prompter.Input("Codex hooks.json path", defaultPath)
		if err != nil {
			return err
		}

		selection.CodexHooksPath = value
	}

	if selection.GeminiEnabled {
		defaultPath := current.GeminiSettingsPath
		if defaultPath == "" {
			defaultPath = defaultGeminiSettingsPath
		}

		value, err := prompter.Input("Gemini settings.json path", defaultPath)
		if err != nil {
			return err
		}

		selection.GeminiSettingsPath = value
	}

	return nil
}

func loadConfigFile(path string) (*pkgConfig.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from config writer resolution
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file %s", path)
	}

	var cfg pkgConfig.Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, errors.Wrapf(err, "failed to parse config file %s", path)
	}

	if cfg.Version == 0 {
		cfg.Version = pkgConfig.CurrentConfigVersion
	}

	return &cfg, nil
}

func renderConfigDiff(
	configPath string,
	current *pkgConfig.Config,
	updated *pkgConfig.Config,
) (string, error) {
	currentData, err := marshalConfigForPreview(current)
	if err != nil {
		return "", err
	}

	updatedData, err := marshalConfigForPreview(updated)
	if err != nil {
		return "", err
	}

	if bytes.Equal(currentData, updatedData) {
		return "", nil
	}

	return difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(currentData)),
		B:        difflib.SplitLines(string(updatedData)),
		FromFile: configPath,
		ToFile:   configPath + " (proposed)",
		Context:  configDiffContextLines,
	})
}

func marshalConfigForPreview(cfg *pkgConfig.Config) ([]byte, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	var buf bytes.Buffer
	buf.WriteString(schema.SchemaDirective())
	buf.WriteByte('\n')

	encoder := toml.NewEncoder(&buf)
	encoder.SetIndentTables(true)

	if err := encoder.Encode(cfg); err != nil {
		return nil, errors.Wrap(err, "failed to encode config preview")
	}

	return buf.Bytes(), nil
}

func cloneConfig(cfg *pkgConfig.Config) (*pkgConfig.Config, error) {
	if cfg == nil {
		return &pkgConfig.Config{}, nil
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal config clone")
	}

	var cloned pkgConfig.Config
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config clone")
	}

	return &cloned, nil
}

func validateConfigForWrite(cfg *pkgConfig.Config) error {
	return errors.Wrap(internalconfig.NewValidator().Validate(cfg), "configuration is invalid")
}
