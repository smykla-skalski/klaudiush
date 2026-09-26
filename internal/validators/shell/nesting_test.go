package shell_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/shell"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("NestingValidator", func() {
	var v *shell.NestingValidator

	BeforeEach(func() {
		v = shell.NewNestingValidator(logger.NewNoOpLogger())
	})

	bash := func(command string) *hook.Context {
		return &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{Command: command},
		}
	}

	It("blocks a command nested past the limit", func() {
		result := v.Validate(
			context.Background(),
			bash(strings.Repeat("env ", 12)+"git commit -m x"),
		)

		Expect(result.Passed).To(BeFalse())
		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Reference).To(Equal(validator.RefShellNesting))
	})

	It("blocks a command that does not parse", func() {
		result := v.Validate(context.Background(), bash(`git commit -m "x" && (`))

		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Reference).To(Equal(validator.RefShellNesting))
	})

	It("passes ordinary nesting", func() {
		result := v.Validate(context.Background(), bash(`sudo env FOO=1 bash -c "git status"`))

		Expect(result.Passed).To(BeTrue())
	})

	It("passes an empty command", func() {
		Expect(v.Validate(context.Background(), bash("")).Passed).To(BeTrue())
	})
})
