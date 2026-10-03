package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// Statuses of a harness or scenario in the report.
const (
	StatusPassed   = "passed"
	StatusFailed   = "failed"
	StatusSkipped  = "skipped"
	StatusKnownGap = "known_gap"
	StatusRan      = "ran"
)

// unsupportedPaths are paths klaudiush cannot see or does not register on,
// per harness. The live suite reports them next to each version so a reader
// knows what a pass does not cover.
var unsupportedPaths = map[string][]string{
	"claude": {
		"Stop is not registered by klaudiush init; the completion gate needs it added by hand",
		"PreToolUse matcher covers Bash|Write|Edit|MultiEdit only; other tools are not checked",
	},
	"codex": {
		"write_stdin input to a running unified-exec session does not fire PreToolUse",
		"hosted tools (web_search) never reach command hooks",
		"PostToolUse is not registered, so files a shell command broke are not reported after the tool",
		"new or changed hooks run only after /hooks trust review",
		"subagents spawned through the multi-agent tools are not exercised",
	},
	"opencode": {
		"session.idle cannot keep the agent working: no completion gate",
		"no declarative hook config, so unrelated-hook coexistence is not checked",
	},
	"gemini": {
		"Gemini CLI is not run locally: payloads come from the published hook reference",
	},
}

// ScenarioResult is one scenario outcome in the report.
type ScenarioResult struct {
	Scenario string  `json:"scenario"`
	Status   string  `json:"status"`
	Detail   string  `json:"detail,omitempty"`
	Seconds  float64 `json:"seconds"`
}

// HarnessReport records one harness: exact version, what klaudiush
// registers, the provider events it has contracts for, and the results.
type HarnessReport struct {
	Name        string           `json:"name"`
	Provider    hook.Provider    `json:"provider"`
	Binary      string           `json:"binary,omitempty"`
	Version     string           `json:"version,omitempty"`
	Status      string           `json:"status"`
	Reason      string           `json:"reason,omitempty"`
	Registered  []string         `json:"registered_events"`
	Contracts   []string         `json:"contract_events"`
	Unsupported []string         `json:"unsupported"`
	Results     []ScenarioResult `json:"results"`
}

// Report is the live suite report.
type Report struct {
	mu          sync.Mutex
	GeneratedAt time.Time        `json:"generated_at"`
	Klaudiush   string           `json:"klaudiush"`
	Harnesses   []*HarnessReport `json:"harnesses"`
}

// NewReport starts a report for a klaudiush build.
func NewReport(klaudiushVersion string) *Report {
	return &Report{GeneratedAt: time.Now().UTC(), Klaudiush: klaudiushVersion}
}

// Harness returns the entry for a harness, creating it on first use.
func (r *Report) Harness(name string, provider hook.Provider) *HarnessReport {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, h := range r.Harnesses {
		if h.Name == name {
			return h
		}
	}

	h := &HarnessReport{
		Name:        name,
		Provider:    provider,
		Status:      StatusSkipped,
		Registered:  RegisteredEvents(provider),
		Contracts:   hook.NativeEventNames(provider),
		Unsupported: unsupportedPaths[name],
	}
	r.Harnesses = append(r.Harnesses, h)

	return h
}

// Add records a scenario result for a harness.
func (r *Report) Add(name string, result ScenarioResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, h := range r.Harnesses {
		if h.Name == name {
			h.Results = append(h.Results, result)

			return
		}
	}
}

// RegisteredEvents lists the events `klaudiush init` registers for a provider.
func RegisteredEvents(provider hook.Provider) []string {
	switch provider {
	case hook.ProviderClaude:
		return settings.ClaudeDispatcherEvents()
	case hook.ProviderCodex:
		return settings.CodexDispatcherEvents()
	case hook.ProviderGemini:
		return settings.GeminiDispatcherEvents()
	case hook.ProviderOpenCode:
		return hook.OpenCodeEventNames()
	case hook.ProviderUnknown:
		return nil
	default:
		return nil
	}
}

// Write stores the report as indented JSON.
func (r *Report) Write(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return errors.Wrap(err, "encoding report")
	}

	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return errors.Wrap(err, "creating report directory")
	}

	return errors.Wrap(os.WriteFile(path, append(data, '\n'), filePerm), "writing report")
}

// Summary prints a version matrix and per-scenario results.
func (r *Report) Summary(w io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, _ = fmt.Fprintf(w, "build: %s, %s\n", r.Klaudiush, r.GeneratedAt.Format(time.RFC3339))

	for _, h := range r.Harnesses {
		version := h.Version
		if version == "" {
			version = "-"
		}

		line := fmt.Sprintf("%-9s %-10s %s", h.Name, version, h.Status)
		if h.Reason != "" {
			line += ": " + h.Reason
		}

		_, _ = fmt.Fprintln(w, line)

		results := slices.Clone(h.Results)
		slices.SortFunc(
			results,
			func(a, b ScenarioResult) int { return strings.Compare(a.Scenario, b.Scenario) },
		)

		for _, result := range results {
			detail := ""
			if result.Detail != "" {
				detail = " (" + result.Detail + ")"
			}

			_, _ = fmt.Fprintf(w, "  %-20s %s%s\n", result.Scenario, result.Status, detail)
		}
	}
}
