package hookresponse_test

import (
	"testing"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
	"github.com/smykla-skalski/klaudiush/internal/validator"
)

// BenchmarkHookResponse benchmarks the hook response builder and formatter.
func BenchmarkHookResponse(b *testing.B) {
	b.Run("Build/SingleBlockingError", func(b *testing.B) {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "validate-commit",
				Message:     "Commit message title exceeds 50 characters",
				ShouldBlock: true,
				Reference:   validator.Reference("https://klaudiu.sh/e/GIT004"),
				FixHint:     "Shorten the commit message title to 50 characters or less",
			},
		}

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = hookresponse.Build("PreToolUse", errs)
		}
	})

	b.Run("Build/MultipleErrors", func(b *testing.B) {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "validate-commit",
				Message:     "Missing conventional commit prefix",
				ShouldBlock: true,
				Reference:   validator.Reference("https://klaudiu.sh/e/GIT005"),
				FixHint:     "Use format: type(scope): description",
			},
			{
				Validator:   "validate-commit",
				Message:     "Missing -s (signoff) flag",
				ShouldBlock: true,
				Reference:   validator.Reference("https://klaudiu.sh/e/GIT007"),
				FixHint:     "Add -s flag to git commit",
			},
			{
				Validator:   "validate-commit",
				Message:     "Missing -S (GPG sign) flag",
				ShouldBlock: true,
				Reference:   validator.Reference("https://klaudiu.sh/e/GIT008"),
				FixHint:     "Add -S flag to git commit",
			},
		}

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = hookresponse.Build("PreToolUse", errs)
		}
	})

	b.Run("Build/WarningsOnly", func(b *testing.B) {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "validate-markdown",
				Message:     "Line exceeds 120 characters",
				ShouldBlock: false,
			},
			{
				Validator:   "validate-shell",
				Message:     "Consider using shellcheck",
				ShouldBlock: false,
			},
		}

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = hookresponse.Build("PreToolUse", errs)
		}
	})

	b.Run("FormatSystemMessage/Complex", func(b *testing.B) {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "validate-commit",
				Message:     "Commit message has multiple issues\n\nTitle exceeds 50 characters\nBody line exceeds 72 characters",
				ShouldBlock: true,
				Reference:   validator.Reference("https://klaudiu.sh/e/GIT004"),
				FixHint:     "Shorten both title and body lines",
				Details: map[string]string{
					"commit_preview": "feat(very-long-scope): this is way too long\n\nThis body has a line that is definitely over 72 characters and needs wrapping",
					"errors":         "title too long, body line too long",
				},
			},
		}

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = hookresponse.FormatSystemMessage(errs)
		}
	})
}
