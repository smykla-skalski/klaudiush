package main

import (
	"fmt"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// maxCompletionBlocks is how many consecutive times a completion gate keeps
// the agent working before it lets the turn end. Claude caps Stop
// continuations at 8 on its own; Codex and Gemini document no cap.
const maxCompletionBlocks = 3

func applyHookSessionLifecycle(
	store *hooksession.Store,
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
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
		hook.CanonicalEventSubagentStop,
		hook.CanonicalEventStopFailure:
		return errs, cleanup
	case hook.CanonicalEventSessionStart:
		if err := store.Start(hookCtx.Provider, hookCtx.SessionID); err != nil {
			log.Info("failed to initialize hook session state", "error", err)
		}
	case hook.CanonicalEventAfterTool:
		if err := store.Append(hookCtx, errs); err != nil {
			log.Info("failed to persist hook session findings", "error", err)
		}
	case hook.CanonicalEventSessionEnd:
		cleanup = func() {
			if err := store.Clear(hookCtx.Provider, hookCtx.SessionID); err != nil {
				log.Info("failed to clear hook session state", "error", err)
			}
		}
	case hook.CanonicalEventTurnStop:
		storedErrs, err := store.CombinedErrors(hookCtx.Provider, hookCtx.SessionID)
		if err != nil {
			log.Info("failed to load hook session findings", "error", err)
			return errs, cleanup
		}

		if len(storedErrs) > 0 {
			errs = append(storedErrs, errs...)
		}

		cleanup = func() {
			if err := store.ClearFindings(hookCtx.Provider, hookCtx.SessionID); err != nil {
				log.Info("failed to clear hook session findings", "error", err)
			}
		}
	}

	return errs, cleanup
}

// applyCompletionGate bounds how often a completion gate (Claude Stop and
// SubagentStop, Codex Stop and SubagentStop, Gemini AfterAgent) keeps the
// agent working. Blocks count per session and reset whenever the provider
// reaches the gate without a prior block (stop_hook_active false) or the gate
// passes. Past maxCompletionBlocks the findings are downgraded to warnings so
// the turn ends, and the returned notice tells the user why. Without a session
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

	gate := string(hookCtx.Event)
	tracked := store != nil && hookCtx.SessionID != ""

	if !dispatcher.ShouldBlock(errs) {
		if tracked {
			resetCompletionBlocks(store, hookCtx, gate, log)
		}

		return errs, ""
	}

	attempt := 1

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
		attempt = maxCompletionBlocks + 1
	}

	if attempt <= maxCompletionBlocks {
		log.Info("completion gate blocked", "gate", gate, "attempt", attempt)

		return errs, ""
	}

	if tracked {
		resetCompletionBlocks(store, hookCtx, gate, log)
	}

	log.Info("completion gate released after repeated blocks",
		"gate", gate,
		"limit", maxCompletionBlocks,
	)

	return releaseBlocking(errs), completionReleaseNotice(hookCtx.EventName())
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

func completionReleaseNotice(eventName string) string {
	if eventName == "" {
		eventName = "completion"
	}

	return fmt.Sprintf(
		"klaudiush: the %s check kept failing after %d continuations, so the agent "+
			"was allowed to stop. The findings in this message are still unresolved; "+
			"fix them before relying on this result.",
		eventName,
		maxCompletionBlocks,
	)
}
