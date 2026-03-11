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
	defaultCodexHooksPath  = "~/.codex/hooks.json"
	configDiffContextLines = 3
)

type providerSelection struct {
	ClaudeEnabled  bool
	CodexEnabled   bool
	CodexHooksPath string
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

	return selection
}

func resolveProviderSelection(
	providers []string,
	codexHooksPath string,
	existing *pkgConfig.Config,
) (providerSelection, error) {
	selection := providerSelectionFromConfig(existing)

	if len(providers) > 0 {
		selection = providerSelection{}

		for _, provider := range providers {
			for token := range strings.SplitSeq(provider, ",") {
				switch strings.ToLower(strings.TrimSpace(token)) {
				case "":
					continue
				case "claude":
					selection.ClaudeEnabled = true
				case "codex":
					selection.CodexEnabled = true
				default:
					return providerSelection{}, errors.Errorf("unknown provider %q", token)
				}
			}
		}
	}

	if codexHooksPath != "" {
		selection.CodexEnabled = true
		selection.CodexHooksPath = codexHooksPath
	}

	if selection.CodexEnabled && selection.CodexHooksPath == "" {
		existingPath := providerSelectionFromConfig(existing).CodexHooksPath
		if existingPath != "" {
			selection.CodexHooksPath = existingPath
		} else {
			selection.CodexHooksPath = defaultCodexHooksPath
		}
	}

	if !selection.CodexEnabled {
		selection.CodexHooksPath = ""
	}

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

	updated.Providers = &pkgConfig.ProvidersConfig{
		Claude: &pkgConfig.ClaudeProviderConfig{
			Enabled: &claudeEnabled,
		},
		Codex: &pkgConfig.CodexProviderConfig{
			Enabled:      &codexEnabled,
			Experimental: &codexExperimental,
		},
	}

	if selection.CodexEnabled {
		updated.Providers.Codex.HooksConfigPath = selection.CodexHooksPath
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

	claudeEnabled, err := prompter.Confirm("Enable Claude integration", current.ClaudeEnabled)
	if err != nil {
		return nil, true, err
	}

	codexEnabled, err := prompter.Confirm("Enable Codex integration", current.CodexEnabled)
	if err != nil {
		return nil, true, err
	}

	selection := providerSelection{
		ClaudeEnabled: claudeEnabled,
		CodexEnabled:  codexEnabled,
	}

	if selection.CodexEnabled {
		defaultPath := current.CodexHooksPath
		if defaultPath == "" {
			defaultPath = defaultCodexHooksPath
		}

		selection.CodexHooksPath, err = prompter.Input("Codex hooks.json path", defaultPath)
		if err != nil {
			return nil, true, err
		}
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
