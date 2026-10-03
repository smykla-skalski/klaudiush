package main

import (
	"fmt"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// maxCompletionBlocks is how many consecutive times a completion gate keeps
// the agent working before it lets the turn end. Claude caps Stop
// continuations at 8 on its own; Codex and Gemini document no cap.
const maxCompletionBlocks = 3

// untrackedCompletionBlocks applies when there is no session to count in:
// stop_hook_active alone only says whether a previous block happened.
const untrackedCompletionBlocks = 1

// prepareRecheck lists the files the session still has unresolved findings
// for, so validation after a tool checks them again and notices a repair.
func prepareRecheck(store *hooksession.Store, hookCtx *hook.Context, log logger.Logger) {
	if store == nil || hookCtx == nil || !hookCtx.IsAfterTool() {
		return
	}

	files, err := store.FilesToRecheck(hookCtx.Provider, hookCtx.SessionID)
	if err != nil {
		log.Info("failed to load unresolved hook session files", "error", err)

		return
	}

	hookCtx.RecheckFiles = files
}

// applyHookSessionLifecycle keeps the session's unresolved findings. After a
// tool, findings are recorded and those a check no longer reports are
// resolved. Completion gates replay what is still unresolved: Stop all of it,
// SubagentStop what that subagent recorded. Nothing is dropped at a gate, so
// an unresolved finding blocks every attempt until it is repaired; only the
// parent's SessionEnd clears the session.
func applyHookSessionLifecycle(
	store *hooksession.Store,
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	checks []dispatcher.Check,
	log logger.Logger,
) ([]*dispatcher.ValidationError, func()) {
	cleanup := func() {}
	if store == nil ||
		hookCtx == nil ||
		hookCtx.Provider == hook.ProviderUnknown ||
		hookCtx.SessionID == "" {
		return errs, cleanup
	}

	switch hookCtx.Event {
	case hook.CanonicalEventUnknown,
		hook.CanonicalEventBeforeTool,
		hook.CanonicalEventNotification,
		hook.CanonicalEventPreCompress,
		hook.CanonicalEventElicitation,
		hook.CanonicalEventElicitationResult,
		hook.CanonicalEventPostCompact,
		hook.CanonicalEventUserPromptSubmit,
		hook.CanonicalEventStopFailure,
		hook.CanonicalEventConfigChange,
		hook.CanonicalEventToolSelection:
		return errs, cleanup
	case hook.CanonicalEventSessionStart:
		if err := store.Start(hookCtx.Provider, hookCtx.SessionID); err != nil {
			log.Info("failed to initialize hook session state", "error", err)
		}
	case hook.CanonicalEventAfterTool:
		if err := store.Record(hookCtx, errs, checks); err != nil {
			log.Info("failed to persist hook session findings", "error", err)

			return append(errs, stateUnavailable("record this check for the session", err)), cleanup
		}
	case hook.CanonicalEventSessionEnd:
		// A subagent shares the parent's session ID; its end must not
		// discard the parent's unresolved work.
		if hookCtx.AgentID != "" {
			return errs, cleanup
		}

		cleanup = func() {
			if err := store.Clear(hookCtx.Provider, hookCtx.SessionID); err != nil {
				log.Info("failed to clear hook session state", "error", err)
			}
		}
	case hook.CanonicalEventTurnStop:
		stored, err := store.CombinedErrors(hookCtx.Provider, hookCtx.SessionID)

		return withStoredErrors(errs, stored, err, log), cleanup
	case hook.CanonicalEventSubagentStop:
		if hookCtx.AgentID == "" {
			return errs, cleanup
		}

		stored, err := store.AgentErrors(hookCtx.Provider, hookCtx.SessionID, hookCtx.AgentID)

		return withStoredErrors(errs, stored, err, log), cleanup
	}

	return errs, cleanup
}

// withStoredErrors puts unresolved stored findings ahead of errs.
func withStoredErrors(
	errs, stored []*dispatcher.ValidationError,
	err error,
	log logger.Logger,
) []*dispatcher.ValidationError {
	if err != nil {
		log.Info("failed to load hook session findings", "error", err)

		return append(errs, stateUnavailable("read the session's unresolved findings", err))
	}

	if len(stored) == 0 {
		return errs
	}

	return append(stored, errs...)
}

// stateUnavailable reports session state klaudiush could not use. It warns
// rather than blocks: state errors such as a held lock are transient, and a
// gate that blocks on them could keep the agent working on nothing.
func stateUnavailable(what string, err error) *dispatcher.ValidationError {
	return &dispatcher.ValidationError{
		Validator: "session-state",
		Message: fmt.Sprintf(
			"klaudiush could not %s, so findings from earlier tool calls may be missing: %s",
			what,
			firstLine(err.Error()),
		),
		Reference:         validator.RefValidationUnavailable,
		FixHint:           validator.GetSuggestion(validator.RefValidationUnavailable),
		Unavailable:       true,
		UnavailableReason: validator.ReasonState,
	}
}

// applyCompletionGate bounds how often a completion gate (Claude Stop and
// SubagentStop, Codex Stop and SubagentStop, Gemini AfterAgent) keeps the
// agent working. Blocks count per session and reset whenever the provider
// reaches the gate without a prior block (stop_hook_active false) or the gate
// passes. Past maxCompletionBlocks the findings are downgraded to warnings so
// the turn ends, and the returned notice tells the user why; the streak stays
// exhausted until the provider reaches the gate fresh again. Without a session
// to count in, one continuation is allowed.
func applyCompletionGate(
	store *hooksession.Store,
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	log logger.Logger,
) ([]*dispatcher.ValidationError, string) {
	if hookCtx == nil ||
		!hook.IsCompletionGate(hookCtx.Provider, hookCtx.Event, hookCtx.RawEventName) {
		return errs, ""
	}

	gate := completionGateKey(hookCtx)
	tracked := store != nil && hookCtx.SessionID != ""

	if !dispatcher.ShouldBlock(errs) {
		if tracked {
			resetCompletionBlocks(store, hookCtx, gate, log)
		}

		return errs, ""
	}

	attempt, limit := 1, maxCompletionBlocks

	switch {
	case tracked:
		count, err := store.RecordCompletionBlock(
			hookCtx.Provider,
			hookCtx.SessionID,
			gate,
			hookCtx.StopHookActive,
		)
		if err != nil {
			log.Info("failed to record completion gate block", "error", err)
		}

		attempt = max(count, 1)

		if err != nil && hookCtx.StopHookActive {
			attempt = maxCompletionBlocks + 1
		}
	case hookCtx.StopHookActive:
		attempt, limit = untrackedCompletionBlocks+1, untrackedCompletionBlocks
	}

	if attempt <= limit {
		log.Info("completion gate blocked", "gate", gate, "attempt", attempt)

		return errs, ""
	}

	log.Info("completion gate released after repeated blocks",
		"gate", gate,
		"limit", limit,
	)

	return releaseBlocking(errs), completionReleaseNotice(hookCtx.EventName(), limit)
}

// completionGateKey separates the counters of gates that can interleave:
// parallel subagents share a session but each has its own stop streak.
func completionGateKey(hookCtx *hook.Context) string {
	if hookCtx.Event == hook.CanonicalEventSubagentStop && hookCtx.AgentID != "" {
		return string(hookCtx.Event) + ":" + hookCtx.AgentID
	}

	return string(hookCtx.Event)
}

func resetCompletionBlocks(
	store *hooksession.Store,
	hookCtx *hook.Context,
	gate string,
	log logger.Logger,
) {
	if err := store.ResetCompletionBlocks(hookCtx.Provider, hookCtx.SessionID, gate); err != nil {
		log.Info("failed to reset completion gate counter", "error", err)
	}
}

// releaseBlocking returns copies of errs with every blocking finding turned
// into a warning, so the findings are still reported but no longer block.
func releaseBlocking(errs []*dispatcher.ValidationError) []*dispatcher.ValidationError {
	released := make([]*dispatcher.ValidationError, 0, len(errs))

	for _, verr := range errs {
		if verr == nil || !verr.ShouldBlock {
			released = append(released, verr)

			continue
		}

		downgraded := *verr
		downgraded.ShouldBlock = false
		released = append(released, &downgraded)
	}

	return released
}

func completionReleaseNotice(eventName string, limit int) string {
	if eventName == "" {
		eventName = "completion"
	}

	return fmt.Sprintf(
		"klaudiush: the %s check kept failing after %d continuation(s), so the agent "+
			"was allowed to stop. The findings in this message are still unresolved; "+
			"fix them before relying on this result.",
		eventName,
		limit,
	)
}
