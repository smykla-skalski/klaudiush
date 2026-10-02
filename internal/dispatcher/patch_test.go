package dispatcher_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("Dispatcher multi-file patches", func() {
	It("validates every patch file, not only the first", func() {
		log := logger.NewNoOpLogger()
		reg := validator.NewRegistry()
		reg.Register(
			&mockBlockingValidator{name: "rules", reference: "https://klaudiu.sh/e/FILE001"},
			validator.And(
				validator.EventIs(hook.CanonicalEventBeforeTool),
				validator.FilePathContains("protected/"),
			),
		)

		disp := dispatcher.NewDispatcher(reg, log)

		errs := disp.Dispatch(context.Background(), &hook.Context{
			Provider:      hook.ProviderCodex,
			Event:         hook.CanonicalEventBeforeTool,
			EventType:     hook.EventTypePreToolUse,
			RawToolName:   "apply_patch",
			ToolName:      hook.ToolTypeEdit,
			ToolFamily:    hook.ToolFamilyEdit,
			AffectedPaths: []string{"ok.txt", "protected/x.txt"},
			PatchFiles: []hook.PatchFile{
				{
					ToolName:   hook.ToolTypeEdit,
					ToolFamily: hook.ToolFamilyEdit,
					Input:      hook.ToolInput{FilePath: "ok.txt", NewString: "b"},
				},
				{
					ToolName:   hook.ToolTypeEdit,
					ToolFamily: hook.ToolFamilyEdit,
					Input:      hook.ToolInput{FilePath: "protected/x.txt"},
				},
			},
		})

		Expect(errs).To(HaveLen(1))
		Expect(errs[0].ShouldBlock).To(BeTrue())
	})
})
