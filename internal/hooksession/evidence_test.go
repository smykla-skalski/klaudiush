package hooksession

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func newEvidenceStore(t *testing.T, now *time.Time) *Store {
	t.Helper()

	return NewStore(
		WithStateFile(filepath.Join(t.TempDir(), "state.json")),
		WithTimeFunc(func() time.Time { return *now }),
		WithRetention(24*time.Hour),
	)
}

func TestEvidenceBaselinesKeepTheFirstDigest(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	store := newEvidenceStore(t, &now)

	got, err := store.EvidenceBaselines(hook.ProviderClaude, "s1", "/repo")
	if err != nil || got != nil {
		t.Fatalf("EvidenceBaselines() on an empty store = %v, %v", got, err)
	}

	if setErr := store.AddEvidenceBaselines(
		hook.ProviderClaude,
		"s1",
		"/repo",
		nil,
	); setErr != nil {
		t.Fatalf("AddEvidenceBaselines(nil) error = %v", setErr)
	}

	first := map[string]string{"tests@1": "sha256:a"}
	if setErr := store.AddEvidenceBaselines(
		hook.ProviderClaude,
		"s1",
		"/repo",
		first,
	); setErr != nil {
		t.Fatalf("AddEvidenceBaselines() error = %v", setErr)
	}

	second := map[string]string{"tests@1": "sha256:b", "review@1": BaselineUnknown}
	if setErr := store.AddEvidenceBaselines(
		hook.ProviderClaude,
		"s1",
		"/repo",
		second,
	); setErr != nil {
		t.Fatalf("AddEvidenceBaselines() error = %v", setErr)
	}

	if setErr := store.AddEvidenceBaselines(
		hook.ProviderClaude,
		"s1",
		"/repo",
		first,
	); setErr != nil {
		t.Fatalf("AddEvidenceBaselines() unchanged error = %v", setErr)
	}

	got, err = store.EvidenceBaselines(hook.ProviderClaude, "s1", "/repo")
	if err != nil {
		t.Fatalf("EvidenceBaselines() error = %v", err)
	}

	if got["tests@1"] != "sha256:a" || got["review@1"] != BaselineUnknown || len(got) != 2 {
		t.Fatalf("baselines = %v", got)
	}

	other, err := store.EvidenceBaselines(hook.ProviderClaude, "s2", "/repo")
	if err != nil || other != nil {
		t.Fatalf("another session sees baselines %v, %v", other, err)
	}

	otherRepo, err := store.EvidenceBaselines(hook.ProviderClaude, "s1", "/other")
	if err != nil || otherRepo != nil {
		t.Fatalf("another repository sees baselines %v, %v", otherRepo, err)
	}

	if setErr := store.Clear(hook.ProviderClaude, "s1"); setErr != nil {
		t.Fatalf("Clear() error = %v", setErr)
	}

	got, err = store.EvidenceBaselines(hook.ProviderClaude, "s1", "/repo")
	if err != nil || got != nil {
		t.Fatalf("baselines after Clear() = %v, %v", got, err)
	}
}

func TestReceiptsAreKeptPerRepositoryAndCheck(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	store := newEvidenceStore(t, &now)

	receipts, err := store.Receipts("/repo")
	if err != nil || receipts != nil {
		t.Fatalf("Receipts() on an empty store = %v, %v", receipts, err)
	}

	running := &evidence.Receipt{
		RunID:     "run-1",
		Check:     "tests",
		Status:    evidence.StatusRunning,
		ToolUseID: "tool-1",
		SessionID: "s1",
	}
	if setErr := store.PutReceipt("/repo", running); setErr != nil {
		t.Fatalf("PutReceipt() error = %v", setErr)
	}

	found, err := store.FindReceipt("/repo", func(r *evidence.Receipt) bool {
		return r.ToolUseID == "tool-1"
	})
	if err != nil || found == nil || found.RunID != "run-1" {
		t.Fatalf("FindReceipt() = %v, %v", found, err)
	}

	missing, err := store.FindReceipt("/other", func(*evidence.Receipt) bool { return true })
	if err != nil || missing != nil {
		t.Fatalf("FindReceipt() in another repository = %v, %v", missing, err)
	}

	none, err := store.FindReceipt("/repo", func(*evidence.Receipt) bool { return false })
	if err != nil || none != nil {
		t.Fatalf("FindReceipt() without a match = %v, %v", none, err)
	}

	finished, err := store.FinishReceipt("/repo", "tests",
		func(r *evidence.Receipt) bool { return r.RunID == "run-2" },
		func(r *evidence.Receipt) { r.Status = evidence.StatusPassed },
	)
	if err != nil || finished {
		t.Fatalf("FinishReceipt() of another run = %v, %v", finished, err)
	}

	finished, err = store.FinishReceipt("/repo", "tests",
		func(r *evidence.Receipt) bool { return r.RunID == "run-1" },
		func(r *evidence.Receipt) { r.Status = evidence.StatusPassed },
	)
	if err != nil || !finished {
		t.Fatalf("FinishReceipt() = %v, %v", finished, err)
	}

	finished, err = store.FinishReceipt("/other", "tests",
		func(*evidence.Receipt) bool { return true },
		func(*evidence.Receipt) {},
	)
	if err != nil || finished {
		t.Fatalf("FinishReceipt() in another repository = %v, %v", finished, err)
	}

	receipts, err = store.Receipts("/repo")
	if err != nil || receipts["tests"].Status != evidence.StatusPassed {
		t.Fatalf("Receipts() = %v, %v", receipts, err)
	}

	newer := &evidence.Receipt{RunID: "run-3", Check: "tests", Status: evidence.StatusFailed}
	if setErr := store.PutReceipt("/repo", newer); setErr != nil {
		t.Fatalf("PutReceipt() error = %v", setErr)
	}

	receipts, err = store.Receipts("/repo")
	if err != nil || receipts["tests"].RunID != "run-3" || len(receipts) != 1 {
		t.Fatalf("latest receipt = %v, %v", receipts, err)
	}
}

func TestReceiptsExpireAfterRetention(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	store := newEvidenceStore(t, &now)

	if setErr := store.PutReceipt("/repo", &evidence.Receipt{Check: "tests"}); setErr != nil {
		t.Fatalf("PutReceipt() error = %v", setErr)
	}

	now = now.Add(25 * time.Hour)

	receipts, err := store.Receipts("/repo")
	if err != nil || receipts != nil {
		t.Fatalf("expired receipts = %v, %v", receipts, err)
	}

	state, err := store.loadState()
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}

	if len(state.Evidence) != 0 {
		t.Fatalf("expired repository kept: %v", state.Evidence)
	}
}
