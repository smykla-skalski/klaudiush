package main

import (
	"slices"
	"testing"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

func TestBuildDoctorRegistrySkipsClaudeHookChecksWhenProviderDisabled(t *testing.T) {
	claudeEnabled := false
	codexEnabled := true
	codexExperimental := true
	cfg := &pkgConfig.Config{
		Providers: &pkgConfig.ProvidersConfig{
			Claude: &pkgConfig.ClaudeProviderConfig{Enabled: &claudeEnabled},
			Codex: &pkgConfig.CodexProviderConfig{
				Enabled:         &codexEnabled,
				Experimental:    &codexExperimental,
				HooksConfigPath: "/tmp/hooks.json",
			},
		},
	}

	registry := buildDoctorRegistry(cfg)
	names := checkerNames(registry.CheckersForCategories([]doctor.Category{doctor.CategoryHook}))

	for _, disallowed := range []string{
		"Dispatcher registered in user settings",
		"Dispatcher registered in project settings",
		"Dispatcher registered in project-local settings",
		"PreToolUse hook in user settings",
		"PreToolUse hook in project settings",
	} {
		if slices.Contains(names, disallowed) {
			t.Fatalf("did not expect Claude hook checker %q when provider is disabled", disallowed)
		}
	}

	for _, expected := range []string{
		"Codex hooks configuration",
		"Dispatcher registered in Codex hooks",
		"SessionStart hook in Codex hooks",
		"AfterToolUse hook in Codex hooks",
		"Stop hook in Codex hooks",
		"Dispatcher path is valid",
	} {
		if !slices.Contains(names, expected) {
			t.Fatalf("expected hook checker %q to be registered", expected)
		}
	}
}

func TestBuildDoctorRegistryRegistersGeminiHookChecks(t *testing.T) {
	claudeEnabled := false
	geminiEnabled := true
	cfg := &pkgConfig.Config{
		Providers: &pkgConfig.ProvidersConfig{
			Claude: &pkgConfig.ClaudeProviderConfig{Enabled: &claudeEnabled},
			Gemini: &pkgConfig.GeminiProviderConfig{
				Enabled:      &geminiEnabled,
				SettingsPath: "/tmp/settings.json",
			},
		},
	}

	registry := buildDoctorRegistry(cfg)
	names := checkerNames(registry.CheckersForCategories([]doctor.Category{doctor.CategoryHook}))

	for _, expected := range []string{
		"Gemini hooks configuration",
		"Dispatcher registered in Gemini settings",
		"BeforeTool hook in Gemini settings",
		"AfterTool hook in Gemini settings",
		"SessionStart hook in Gemini settings",
		"SessionEnd hook in Gemini settings",
		"Notification hook in Gemini settings",
		"PreCompress hook in Gemini settings",
	} {
		if !slices.Contains(names, expected) {
			t.Fatalf("expected hook checker %q to be registered", expected)
		}
	}
}

func checkerNames(checkers []doctor.HealthChecker) []string {
	names := make([]string, 0, len(checkers))
	for _, checker := range checkers {
		names = append(names, checker.Name())
	}

	return names
}
