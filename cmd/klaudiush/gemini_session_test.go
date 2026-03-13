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

var _ = Describe("Gemini session lifecycle", func() {
	It("aggregates AfterTool findings until SessionEnd", func() {
		tempDir := GinkgoT().TempDir()
		currentTime := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
		store := hooksession.NewStore(
			hooksession.WithStateFile(filepath.Join(tempDir, "state.json")),
			hooksession.WithTimeFunc(func() time.Time { return currentTime }),
		)
		log := logger.NewNoOpLogger()

		_, cleanup := applyHookSessionLifecycle(store, &hook.Context{
			Provider:     hook.ProviderGemini,
			Event:        hook.CanonicalEventSessionStart,
			RawEventName: "SessionStart",
			SessionID:    "sess-gemini-1",
		}, nil, log)
		cleanup()

		afterToolErrs := []*dispatcher.ValidationError{
			{
				Validator:   "file.markdown",
				Message:     "missing heading",
				ShouldBlock: true,
				Reference:   validator.RefMarkdownLint,
			},
		}

		recordedErrs, cleanup := applyHookSessionLifecycle(store, &hook.Context{
			Provider:     hook.ProviderGemini,
			Event:        hook.CanonicalEventAfterTool,
			RawEventName: "AfterTool",
			SessionID:    "sess-gemini-1",
			ToolName:     hook.ToolTypeWrite,
			ToolFamily:   hook.ToolFamilyWrite,
			ToolInput: hook.ToolInput{
				FilePath: "README.md",
				Content:  "hello",
			},
		}, afterToolErrs, log)
		Expect(recordedErrs).To(Equal(afterToolErrs))
		cleanup()

		sessionEndErrs, cleanup := applyHookSessionLifecycle(store, &hook.Context{
			Provider:     hook.ProviderGemini,
			Event:        hook.CanonicalEventTurnStop,
			RawEventName: "SessionEnd",
			SessionID:    "sess-gemini-1",
		}, nil, log)
		Expect(sessionEndErrs).To(HaveLen(1))
		Expect(sessionEndErrs[0].Reference).To(Equal(validator.RefMarkdownLint))
		cleanup()

		combined, err := store.CombinedErrors(hook.ProviderGemini, "sess-gemini-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(combined).To(BeEmpty())
	})
})
