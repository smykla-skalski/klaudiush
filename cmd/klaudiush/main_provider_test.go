package main

import (
	"testing"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

func TestExtractEffectiveWorkDirUsesGeminiWorkingDir(t *testing.T) {
	workDir := extractEffectiveWorkDir(&hook.Context{
		Provider:   hook.ProviderGemini,
		WorkingDir: "/tmp/gemini-project",
		ToolName:   hook.ToolTypeBash,
		ToolFamily: hook.ToolFamilyShell,
		ToolInput:  hook.ToolInput{Command: "git status"},
	}, logger.NewNoOpLogger())

	if workDir != "/tmp/gemini-project" {
		t.Fatalf("expected Gemini working dir to be used, got %q", workDir)
	}
}
