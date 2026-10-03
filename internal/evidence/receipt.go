package evidence

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// Status is the outcome a receipt records.
type Status string

// Receipt statuses. Only StatusPassed on the current digest satisfies a
// check; stale, missing and unverified are derived when a receipt is judged.
const (
	StatusPassed     Status = "passed"
	StatusFailed     Status = "failed"
	StatusRunning    Status = "running"
	StatusCanceled   Status = "canceled"
	StatusStale      Status = "stale"
	StatusMissing    Status = "missing"
	StatusUnverified Status = "unverified"
)

// Receipt sources: a provider that reported the command's exit status, or
// klaudiush running the check itself.
const (
	SourceVerifier = "verifier"
)

// Receipt records one run of a check and exactly what it ran against.
type Receipt struct {
	RunID      string     `json:"run_id"`
	CheckID    string     `json:"check_id"`
	Check      string     `json:"check"`
	Kind       string     `json:"kind"`
	Status     Status     `json:"status"`
	Digest     string     `json:"digest"`
	EndDigest  string     `json:"end_digest,omitempty"`
	Files      int        `json:"files,omitempty"`
	Base       string     `json:"base,omitempty"`
	Changed    []string   `json:"changed,omitempty"`
	Source     string     `json:"source"`
	Command    string     `json:"command"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	Detail     string     `json:"detail,omitempty"`
	SessionID  string     `json:"session_id,omitempty"`
	AgentID    string     `json:"agent_id,omitempty"`
	ToolUseID  string     `json:"tool_use_id,omitempty"`
	PID        int        `json:"pid,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// NewRunID returns a random identifier for one run of a check.
func NewRunID() string {
	return rand.Text()
}

// Finish records the outcome of a run. A run whose covered content changed
// while it ran proves nothing about either version, so it ends stale.
func (r *Receipt) Finish(status Status, exitCode *int, endDigest, detail string, at time.Time) {
	r.Status = status
	r.ExitCode = exitCode
	r.EndDigest = endDigest
	r.Detail = detail
	r.FinishedAt = &at

	if status != StatusUnverified && endDigest != r.Digest {
		r.Status = StatusStale
		r.Detail = fmt.Sprintf("covered files changed while the check ran (%s)", status)
	}
}

// Verdict is what a receipt proves about the current content.
type Verdict struct {
	Status  Status
	Receipt *Receipt
	Reason  string
}

// Satisfied reports whether the verdict lets the agent finish.
func (v Verdict) Satisfied() bool {
	return v.Status == StatusPassed
}

// Judge decides whether receipt proves check passed against digest, the
// current digest of what the check covers. alive reports whether a verifier
// process still runs.
func Judge(
	check *Check,
	receipt *Receipt,
	digest string,
	now time.Time,
	alive func(pid int) bool,
) Verdict {
	if receipt == nil {
		return Verdict{Status: StatusMissing, Reason: "it has not run since the files changed"}
	}

	if receipt.CheckID != check.ID() {
		return Verdict{
			Status:  StatusMissing,
			Receipt: receipt,
			Reason:  "its last result predates the current definition of the check",
		}
	}

	verdict := Verdict{Status: receipt.Status, Receipt: receipt}

	if receipt.Digest != digest {
		verdict.Status = StatusStale
		verdict.Reason = fmt.Sprintf(
			"its %s result is for content that has changed since",
			receipt.Status,
		)

		return verdict
	}

	switch receipt.Status {
	case StatusPassed:
		verdict.Reason = "passed against the current content"
	case StatusRunning:
		verdict.Reason = runningReason(check, receipt, now, alive, &verdict)
	case StatusFailed:
		verdict.Reason = "failed" + exitSuffix(receipt.ExitCode)
	case StatusCanceled, StatusStale, StatusUnverified, StatusMissing:
		verdict.Reason = string(receipt.Status)
	default:
		verdict.Status = StatusUnverified
		verdict.Reason = "unknown result status " + string(receipt.Status)
	}

	if receipt.Detail != "" && receipt.Status != StatusPassed {
		verdict.Reason += ": " + receipt.Detail
	}

	return verdict
}

func runningReason(
	check *Check,
	receipt *Receipt,
	now time.Time,
	alive func(pid int) bool,
	verdict *Verdict,
) string {
	if receipt.Source == SourceVerifier && receipt.PID > 0 && alive != nil && !alive(receipt.PID) {
		verdict.Status = StatusCanceled

		return "the verifier stopped without reporting a result"
	}

	if now.Sub(receipt.StartedAt) > check.Timeout {
		verdict.Status = StatusCanceled

		return fmt.Sprintf("no result within %s", check.Timeout)
	}

	return "still running since " + receipt.StartedAt.UTC().Format(time.RFC3339)
}

func exitSuffix(code *int) string {
	if code == nil {
		return ""
	}

	return fmt.Sprintf(" with exit status %d", *code)
}

// Summary describes a receipt for people: what ran, on what, and how it
// ended.
func (r *Receipt) Summary() string {
	parts := []string{
		fmt.Sprintf("%s %s", r.Check, r.Status),
		"command: " + r.Command,
		"source: " + r.Source,
		"digest: " + r.Digest,
	}

	if r.Base != "" {
		parts = append(parts, fmt.Sprintf("base: %s, %d changed file(s)", r.Base, len(r.Changed)))
	} else if r.Files > 0 {
		parts = append(parts, fmt.Sprintf("%d file(s)", r.Files))
	}

	return strings.Join(parts, "; ")
}
