package hooksession

import (
	"os"
	"path/filepath"
	"strconv"
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

func TestStoreCompletionBlocksSurviveResolvedFindings(t *testing.T) {
	store := NewStore(WithStateFile(filepath.Join(t.TempDir(), "state.json")))

	count, err := store.RecordCompletionBlock(hook.ProviderClaude, "sess", "turn_stop", false)
	if err != nil || count != 1 {
		t.Fatalf("first block = %d, %v; want 1, nil", count, err)
	}

	afterTool := &hook.Context{
		Provider:  hook.ProviderClaude,
		Event:     hook.CanonicalEventAfterTool,
		SessionID: "sess",
	}

	err = store.Append(afterTool, []*dispatcher.ValidationError{
		{Validator: "v", Message: "m", ShouldBlock: true},
	})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	err = store.Record(afterTool, nil, []dispatcher.Check{
		{Validator: "v", Resource: afterTool.Resource()},
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	combined, err := store.CombinedErrors(hook.ProviderClaude, "sess")
	if err != nil || len(combined) != 0 {
		t.Fatalf("CombinedErrors() = %v, %v; want empty", combined, err)
	}

	count, err = store.RecordCompletionBlock(hook.ProviderClaude, "sess", "turn_stop", true)
	if err != nil || count != 2 {
		t.Fatalf("continued block = %d, %v; want 2, nil", count, err)
	}

	count, err = store.RecordCompletionBlock(hook.ProviderClaude, "sess", "turn_stop", false)
	if err != nil || count != 1 {
		t.Fatalf("fresh block = %d, %v; want 1, nil", count, err)
	}

	err = store.ResetCompletionBlocks(hook.ProviderClaude, "sess", "turn_stop")
	if err != nil {
		t.Fatalf("ResetCompletionBlocks() error = %v", err)
	}

	count, err = store.RecordCompletionBlock(hook.ProviderClaude, "sess", "turn_stop", true)
	if err != nil || count != 1 {
		t.Fatalf("block after reset = %d, %v; want 1, nil", count, err)
	}
}

func TestStoreCompletionBlocksIgnoreMissingSessions(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	store := NewStore(WithStateFile(stateFile))

	count, err := store.RecordCompletionBlock(hook.ProviderUnknown, "sess", "turn_stop", false)
	if err != nil || count != 0 {
		t.Fatalf("unknown provider = %d, %v; want 0, nil", count, err)
	}

	if err := store.ResetCompletionBlocks(hook.ProviderClaude, "missing", "turn_stop"); err != nil {
		t.Fatalf("ResetCompletionBlocks() error = %v", err)
	}

	if err := store.Clear(hook.ProviderClaude, ""); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	if err := store.Clear(hook.ProviderClaude, "missing"); err != nil {
		t.Fatalf("Clear(missing) error = %v", err)
	}

	if err := store.Record(nil, nil, nil); err != nil {
		t.Fatalf("Record(nil) error = %v", err)
	}
}

func TestStoreStartKeepsCompletionBlocks(t *testing.T) {
	store := NewStore(WithStateFile(filepath.Join(t.TempDir(), "state.json")))

	if _, err := store.RecordCompletionBlock(
		hook.ProviderClaude,
		"sess",
		"turn_stop",
		false,
	); err != nil {
		t.Fatalf("RecordCompletionBlock() error = %v", err)
	}

	if err := store.Start(hook.ProviderClaude, "sess"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	count, err := store.RecordCompletionBlock(hook.ProviderClaude, "sess", "turn_stop", true)
	if err != nil || count != 2 {
		t.Fatalf("block after restart = %d, %v; want 2, nil", count, err)
	}
}

func TestStoreKeepsStructuredFindings(t *testing.T) {
	store := NewStore(WithStateFile(filepath.Join(t.TempDir(), "state.json")))

	hookCtx := &hook.Context{
		Provider:  hook.ProviderCodex,
		Event:     hook.CanonicalEventBeforeTool,
		SessionID: "sess-structured",
		ToolName:  hook.ToolTypeBash,
	}

	finding := validator.Finding{
		Reference: validator.RefGitBadTitle,
		Location:  "title",
		Message:   "Title is 80 characters long",
		Required:  "at most 72 characters",
		Repair:    "Shorten the title",
	}

	errs := []*dispatcher.ValidationError{{
		Validator:   "git.commit",
		Message:     "title too long",
		ShouldBlock: true,
		Reference:   validator.RefGitBadTitle,
		Findings:    []validator.Finding{finding},
		Unavailable: true,
	}}

	if err := store.Append(hookCtx, errs); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	combined, err := store.CombinedErrors(hook.ProviderCodex, "sess-structured")
	if err != nil {
		t.Fatalf("CombinedErrors() error = %v", err)
	}

	if len(combined) != 1 || len(combined[0].Findings) != 1 {
		t.Fatalf("combined = %+v, want one error with one finding", combined)
	}

	if combined[0].Findings[0] != finding {
		t.Fatalf("finding = %+v, want %+v", combined[0].Findings[0], finding)
	}

	if !combined[0].Unavailable {
		t.Fatalf("Unavailable lost in round trip")
	}
}

func TestStoreRecordResolvesCheckedFindings(t *testing.T) {
	now := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	store := NewStore(
		WithStateFile(filepath.Join(t.TempDir(), "state.json")),
		WithTimeFunc(func() time.Time { return now }),
	)

	afterTool := func(path, agentID string) *hook.Context {
		return &hook.Context{
			Provider:   hook.ProviderCodex,
			Event:      hook.CanonicalEventAfterTool,
			SessionID:  "sess",
			AgentID:    agentID,
			WorkingDir: "/repo",
			ToolName:   hook.ToolTypeWrite,
			ToolFamily: hook.ToolFamilyWrite,
			ToolInput:  hook.ToolInput{FilePath: path},
		}
	}

	first := afterTool("a.md", "")
	second := afterTool("b.md", "agent-1")

	fail := func(msg string) []*dispatcher.ValidationError {
		return []*dispatcher.ValidationError{{Validator: "file.markdown", Message: msg}}
	}

	checks := func(hookCtx *hook.Context) []dispatcher.Check {
		return []dispatcher.Check{{Validator: "file.markdown", Resource: hookCtx.Resource()}}
	}

	mustRecord := func(hookCtx *hook.Context, errs []*dispatcher.ValidationError, c []dispatcher.Check) {
		t.Helper()

		if err := store.Record(hookCtx, errs, c); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	mustRecord(first, fail("line 1"), checks(first))
	mustRecord(second, fail("line 2"), checks(second))

	if err := store.Start(hook.ProviderCodex, "sess"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	files, err := store.UnresolvedFiles(hook.ProviderCodex, "sess")
	if err != nil || len(files) != 2 || files[0] != "/repo/a.md" || files[1] != "/repo/b.md" {
		t.Fatalf("UnresolvedFiles() = %v, %v; want both files", files, err)
	}

	agentErrs, err := store.AgentErrors(hook.ProviderCodex, "sess", "agent-1")
	if err != nil || len(agentErrs) != 1 || agentErrs[0].Resource != "file:/repo/b.md" {
		t.Fatalf("AgentErrors() = %+v, %v; want b.md only", agentErrs, err)
	}

	mustRecord(first, fail("line 9"), checks(first))

	combined, err := store.CombinedErrors(hook.ProviderCodex, "sess")
	if err != nil || len(combined) != 2 || combined[0].Message != "line 2" ||
		combined[1].Message != "line 9" {
		t.Fatalf("CombinedErrors() = %+v, %v; want line 2 and line 9", combined, err)
	}

	mustRecord(first, nil, checks(first))

	st, err := store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}

	entry := st.Sessions[sessionKey(hook.ProviderCodex, "sess")]
	if len(entry.Findings) != 1 || entry.Findings[0].Message != "line 2" {
		t.Fatalf("Findings = %+v, want only b.md", entry.Findings)
	}

	if len(entry.Resolved) != 2 || !entry.Resolved[1].ResolvedAt.Equal(now) {
		t.Fatalf("Resolved = %+v, want two resolved findings", entry.Resolved)
	}

	if none, err := store.UnresolvedFiles(
		hook.ProviderCodex,
		"missing",
	); err != nil ||
		none != nil {
		t.Fatalf("UnresolvedFiles(missing) = %v, %v; want nil", none, err)
	}

	if none, err := store.UnresolvedFiles(hook.ProviderUnknown, "sess"); err != nil || none != nil {
		t.Fatalf("UnresolvedFiles(unknown) = %v, %v; want nil", none, err)
	}
}

func TestStoreRefreshesEvidenceAndCapsHistory(t *testing.T) {
	store := NewStore(WithStateFile(filepath.Join(t.TempDir(), "state.json")))
	hookCtx := &hook.Context{
		Provider:  hook.ProviderCodex,
		Event:     hook.CanonicalEventAfterTool,
		SessionID: "sess",
		ToolName:  hook.ToolTypeBash,
		ToolInput: hook.ToolInput{Command: "make lint"},
	}
	resource := hookCtx.Resource()

	for i := range maxResolvedHistory + 5 {
		errs := []*dispatcher.ValidationError{{
			Validator: "shell.lint",
			Message:   "problem",
			FixHint:   strconv.Itoa(i),
			Details:   map[string]string{"run": strconv.Itoa(i)},
		}}

		if err := store.Record(hookCtx, errs, nil); err != nil {
			t.Fatalf("Record() error = %v", err)
		}

		if err := store.Record(hookCtx, nil, []dispatcher.Check{
			{Validator: "shell.lint", Resource: resource},
		}); err != nil {
			t.Fatalf("Record(resolve) error = %v", err)
		}
	}

	if err := store.Record(hookCtx, []*dispatcher.ValidationError{
		{Validator: "shell.lint", Message: "problem", FixHint: "a"},
	}, nil); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	if err := store.Record(hookCtx, []*dispatcher.ValidationError{
		{
			Validator: "shell.lint",
			Message:   "problem",
			FixHint:   "b",
			Details:   map[string]string{"k": "v"},
		},
	}, nil); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	st, err := store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}

	entry := st.Sessions[sessionKey(hook.ProviderCodex, "sess")]
	if len(entry.Resolved) != maxResolvedHistory {
		t.Fatalf("len(Resolved) = %d, want %d", len(entry.Resolved), maxResolvedHistory)
	}

	if entry.Resolved[0].FixHint != "5" {
		t.Fatalf("oldest kept = %q, want 5", entry.Resolved[0].FixHint)
	}

	if len(entry.Findings) != 1 || entry.Findings[0].FixHint != "b" ||
		entry.Findings[0].Count != 2 || entry.Findings[0].Details["k"] != "v" {
		t.Fatalf("Findings = %+v, want one refreshed finding", entry.Findings)
	}
}

func TestFindingLegacyResource(t *testing.T) {
	tests := []struct {
		name string
		item finding
		want string
	}{
		{"file", finding{FilePath: "/repo/a.md"}, "file:/repo/a.md"},
		{"command", finding{Command: "git push", FilePath: "x"}, "command:git push"},
		{"tool", finding{ToolName: "Grep"}, "tool:Grep"},
		{"recorded", finding{Resource: "file:/x", Command: "y"}, "file:/x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.resource(); got != tt.want {
				t.Fatalf("resource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStoreResolvesLegacyFindings(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "state.json")
	legacy := `{"sessions":{"codex:sess":{"provider":"codex","session_id":"sess",` +
		`"findings":[{"validator":"git.push","message":"m","should_block":true,` +
		`"command":"git push","count":1}]}}}`

	if err := os.WriteFile(stateFile, []byte(legacy), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := NewStore(WithStateFile(stateFile))
	hookCtx := &hook.Context{
		Provider:  hook.ProviderCodex,
		Event:     hook.CanonicalEventAfterTool,
		SessionID: "sess",
		ToolName:  hook.ToolTypeBash,
		ToolInput: hook.ToolInput{Command: "git push"},
	}

	if err := store.Record(hookCtx, nil, []dispatcher.Check{
		{Validator: "git.push", Resource: hookCtx.Resource()},
	}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	combined, err := store.CombinedErrors(hook.ProviderCodex, "sess")
	if err != nil || len(combined) != 0 {
		t.Fatalf("CombinedErrors() = %+v, %v; want empty", combined, err)
	}
}
