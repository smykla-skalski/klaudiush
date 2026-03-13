package main

import (
	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

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
		hook.CanonicalEventPreCompress:
		return errs, cleanup
	case hook.CanonicalEventSessionStart:
		if err := store.Start(hookCtx.Provider, hookCtx.SessionID); err != nil {
			log.Info("failed to initialize hook session state", "error", err)
		}
	case hook.CanonicalEventAfterTool:
		if err := store.Append(hookCtx, errs); err != nil {
			log.Info("failed to persist hook session findings", "error", err)
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
			if err := store.Clear(hookCtx.Provider, hookCtx.SessionID); err != nil {
				log.Info("failed to clear hook session findings", "error", err)
			}
		}
	}

	return errs, cleanup
}
