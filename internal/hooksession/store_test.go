package hooksession

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func TestStoreAppendAndCombinedErrorsDedupesFindings(t *testing.T) {
	tempDir := t.TempDir()
	now := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)

	store := NewStore(
		WithStateFile(filepath.Join(tempDir, "state.json")),
		WithTimeFunc(func() time.Time { return now }),
		WithRetention(7*24*time.Hour),
	)

	if err := store.Start(hook.ProviderCodex, "sess-1"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	hookCtx := &hook.Context{
		Provider:     hook.ProviderCodex,
		Event:        hook.CanonicalEventAfterTool,
		RawEventName: "AfterToolUse",
		SessionID:    "sess-1",
		ToolName:     hook.ToolTypeBash,
		ToolFamily:   hook.ToolFamilyShell,
		ToolInput: hook.ToolInput{
			Command: "git push origin main",
		},
	}

	errs := []*dispatcher.ValidationError{
		{
			Validator:   "git.push",
			Message:     "protected branch",
			ShouldBlock: true,
			Reference:   validator.RefGitKongOrgPush,
		},
	}

	if err := store.Append(hookCtx, errs); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if err := store.Append(hookCtx, errs); err != nil {
		t.Fatalf("Append() duplicate error = %v", err)
	}

	state, err := store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}

	entry := state.Sessions[sessionKey(hook.ProviderCodex, "sess-1")]
	if entry == nil {
		t.Fatalf("expected persisted session entry")
	}

	if len(entry.Findings) != 1 {
		t.Fatalf("len(entry.Findings) = %d, want 1", len(entry.Findings))
	}

	if entry.Findings[0].Count != 2 {
		t.Fatalf("entry.Findings[0].Count = %d, want 2", entry.Findings[0].Count)
	}

	combined, err := store.CombinedErrors(hook.ProviderCodex, "sess-1")
	if err != nil {
		t.Fatalf("CombinedErrors() error = %v", err)
	}

	if len(combined) != 1 {
		t.Fatalf("len(combined) = %d, want 1", len(combined))
	}

	if !combined[0].ShouldBlock {
		t.Fatalf("combined[0].ShouldBlock = false, want true")
	}
}

func TestStoreClearAndCleanupIsolateProviders(t *testing.T) {
	tempDir := t.TempDir()
	now := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)

	store := NewStore(
		WithStateFile(filepath.Join(tempDir, "state.json")),
		WithTimeFunc(func() time.Time { return now }),
		WithRetention(24*time.Hour),
	)

	if err := store.Start(hook.ProviderCodex, "shared"); err != nil {
		t.Fatalf("Start(codex) error = %v", err)
	}

	if err := store.Start(hook.ProviderClaude, "shared"); err != nil {
		t.Fatalf("Start(claude) error = %v", err)
	}

	errs := []*dispatcher.ValidationError{
		{
			Validator:   "git.push",
			Message:     "protected branch",
			ShouldBlock: true,
			Reference:   validator.RefGitKongOrgPush,
		},
	}

	if err := store.Append(&hook.Context{
		Provider:     hook.ProviderCodex,
		Event:        hook.CanonicalEventAfterTool,
		RawEventName: "AfterToolUse",
		SessionID:    "shared",
		ToolName:     hook.ToolTypeBash,
		ToolFamily:   hook.ToolFamilyShell,
		ToolInput:    hook.ToolInput{Command: "git push origin main"},
	}, errs); err != nil {
		t.Fatalf("Append(codex) error = %v", err)
	}

	if err := store.Append(&hook.Context{
		Provider:     hook.ProviderClaude,
		Event:        hook.CanonicalEventBeforeTool,
		RawEventName: "PreToolUse",
		SessionID:    "shared",
		ToolName:     hook.ToolTypeBash,
		ToolFamily:   hook.ToolFamilyShell,
		ToolInput:    hook.ToolInput{Command: "git push origin main"},
	}, errs); err != nil {
		t.Fatalf("Append(claude) error = %v", err)
	}

	if err := store.Clear(hook.ProviderCodex, "shared"); err != nil {
		t.Fatalf("Clear(codex) error = %v", err)
	}

	codexCombined, err := store.CombinedErrors(hook.ProviderCodex, "shared")
	if err != nil {
		t.Fatalf("CombinedErrors(codex) error = %v", err)
	}

	if len(codexCombined) != 0 {
		t.Fatalf("len(codexCombined) = %d, want 0", len(codexCombined))
	}

	claudeCombined, err := store.CombinedErrors(hook.ProviderClaude, "shared")
	if err != nil {
		t.Fatalf("CombinedErrors(claude) error = %v", err)
	}

	if len(claudeCombined) != 1 {
		t.Fatalf("len(claudeCombined) = %d, want 1", len(claudeCombined))
	}

	state, err := store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}

	staleKey := sessionKey(hook.ProviderCodex, "stale")
	state.Sessions[staleKey] = &sessionEntry{
		Provider:  string(hook.ProviderCodex),
		SessionID: "stale",
		UpdatedAt: now.Add(-48 * time.Hour),
	}

	saveErr := store.saveState(state)
	if saveErr != nil {
		t.Fatalf("saveState() error = %v", saveErr)
	}

	_, combinedErr := store.CombinedErrors(hook.ProviderCodex, "missing")
	if combinedErr != nil {
		t.Fatalf("CombinedErrors(missing) error = %v", combinedErr)
	}

	state, err = store.loadState()
	if err != nil {
		t.Fatalf("loadState() error after cleanup = %v", err)
	}

	if _, ok := state.Sessions[staleKey]; ok {
		t.Fatalf("expected stale session to be cleaned up")
	}
}
