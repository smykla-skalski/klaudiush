package main

import (
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("applyHookSessionLifecycle", func() {
	It("records blocking AfterToolUse findings and blocks them only at Stop", func() {
		tempDir := GinkgoT().TempDir()
		currentTime := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
		store := hooksession.NewStore(
			hooksession.WithStateFile(filepath.Join(tempDir, "state.json")),
			hooksession.WithTimeFunc(func() time.Time { return currentTime }),
		)
		log := logger.NewNoOpLogger()

		_, cleanup := applyHookSessionLifecycle(store, &hook.Context{
			Provider:     hook.ProviderCodex,
			Event:        hook.CanonicalEventSessionStart,
			RawEventName: "SessionStart",
			SessionID:    "sess-1",
		}, nil, log)
		Expect(cleanup).NotTo(BeNil())

		afterToolErrs := []*dispatcher.ValidationError{
			{
				Validator:   "git.push",
				Message:     "protected branch",
				ShouldBlock: true,
				Reference:   validator.RefGitKongOrgPush,
			},
		}

		recordedErrs, cleanup := applyHookSessionLifecycle(store, &hook.Context{
			Provider:     hook.ProviderCodex,
			Event:        hook.CanonicalEventAfterTool,
			RawEventName: "AfterToolUse",
			SessionID:    "sess-1",
			ToolName:     hook.ToolTypeBash,
			ToolFamily:   hook.ToolFamilyShell,
			ToolInput:    hook.ToolInput{Command: "git push origin main"},
		}, afterToolErrs, log)
		Expect(recordedErrs).To(Equal(afterToolErrs))
		cleanup()

		stopErrs, cleanup := applyHookSessionLifecycle(store, &hook.Context{
			Provider:     hook.ProviderCodex,
			Event:        hook.CanonicalEventTurnStop,
			RawEventName: "Stop",
			SessionID:    "sess-1",
		}, []*dispatcher.ValidationError{
			{
				Validator:   "summary",
				Message:     "wrap up",
				ShouldBlock: false,
			},
		}, log)
		Expect(stopErrs).To(HaveLen(2))
		Expect(dispatcher.ShouldBlock(stopErrs)).To(BeTrue())
		cleanup()

		combined, err := store.CombinedErrors(hook.ProviderCodex, "sess-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(combined).To(BeEmpty())
	})
})
