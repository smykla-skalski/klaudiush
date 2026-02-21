package validator_test

import (
	"context"
	"testing"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// benchValidator is a minimal Validator for benchmarks.
type benchValidator struct {
	*validator.BaseValidator
	result *validator.Result
}

func newBenchValidator(name string, result *validator.Result) *benchValidator {
	return &benchValidator{
		BaseValidator: validator.NewBaseValidator(name, logger.NewNoOpLogger()),
		result:        result,
	}
}

func (v *benchValidator) Validate(context.Context, *hook.Context) *validator.Result {
	return v.result
}

// BenchmarkFindValidators benchmarks registry lookup with a realistic validator set.
func BenchmarkFindValidators(b *testing.B) {
	registry := validator.NewRegistry()

	// Register ~15 validators simulating realistic mix
	registry.Register(
		newBenchValidator("commit", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeBash),
			validator.CommandContains("git commit"),
		),
	)
	registry.Register(
		newBenchValidator("push", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeBash),
			validator.CommandContains("git push"),
		),
	)
	registry.Register(
		newBenchValidator("add", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeBash),
			validator.CommandContains("git add"),
		),
	)
	registry.Register(
		newBenchValidator("pr", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeBash),
			validator.CommandContains("gh pr"),
		),
	)
	registry.Register(
		newBenchValidator("markdown", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeWrite),
			validator.FileExtensionIs(".md"),
		),
	)
	registry.Register(
		newBenchValidator("shell", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeWrite),
			validator.FileExtensionIs(".sh"),
		),
	)
	registry.Register(
		newBenchValidator("terraform", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeWrite),
			validator.FileExtensionIn(".tf", ".tfvars"),
		),
	)
	registry.Register(
		newBenchValidator("go", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeWrite),
			validator.FileExtensionIs(".go"),
		),
	)
	registry.Register(
		newBenchValidator("python", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeWrite),
			validator.FileExtensionIs(".py"),
		),
	)
	registry.Register(
		newBenchValidator("javascript", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeWrite),
			validator.FileExtensionIn(".js", ".ts", ".jsx", ".tsx"),
		),
	)
	registry.Register(
		newBenchValidator("rust", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeWrite),
			validator.FileExtensionIs(".rs"),
		),
	)
	registry.Register(
		newBenchValidator("workflow", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeWrite),
			validator.FilePathContains(".github/workflows/"),
		),
	)
	registry.Register(
		newBenchValidator("secrets-bash", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeBash),
		),
	)
	registry.Register(
		newBenchValidator("secrets-write", validator.Pass()),
		validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit),
		),
	)
	registry.Register(
		newBenchValidator("bell", validator.Pass()),
		validator.EventTypeIs(hook.EventTypeNotification),
	)

	gitCommitCtx := &hook.Context{
		EventType: hook.EventTypePreToolUse,
		ToolName:  hook.ToolTypeBash,
		ToolInput: hook.ToolInput{Command: `git commit -sS -m "feat: add feature"`},
	}

	gitPushCtx := &hook.Context{
		EventType: hook.EventTypePreToolUse,
		ToolName:  hook.ToolTypeBash,
		ToolInput: hook.ToolInput{Command: "git push upstream main"},
	}

	writeGoCtx := &hook.Context{
		EventType: hook.EventTypePreToolUse,
		ToolName:  hook.ToolTypeWrite,
		ToolInput: hook.ToolInput{FilePath: "/project/src/main.go"},
	}

	grepCtx := &hook.Context{
		EventType: hook.EventTypePreToolUse,
		ToolName:  hook.ToolTypeGrep,
		ToolInput: hook.ToolInput{Command: "pattern"},
	}

	b.Run("BashGitCommit", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = registry.FindValidators(gitCommitCtx)
		}
	})

	b.Run("BashGitPush", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = registry.FindValidators(gitPushCtx)
		}
	})

	b.Run("WriteGoFile", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = registry.FindValidators(writeGoCtx)
		}
	})

	b.Run("NonMatchingTool", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = registry.FindValidators(grepCtx)
		}
	})
}

// BenchmarkPredicates benchmarks individual predicate evaluation cost.
func BenchmarkPredicates(b *testing.B) {
	bashCtx := &hook.Context{
		EventType: hook.EventTypePreToolUse,
		ToolName:  hook.ToolTypeBash,
		ToolInput: hook.ToolInput{Command: `git commit -sS -m "feat: msg"`},
	}

	b.Run("EventTypeIs", func(b *testing.B) {
		pred := validator.EventTypeIs(hook.EventTypePreToolUse)

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = pred(bashCtx)
		}
	})

	b.Run("CommandContains", func(b *testing.B) {
		pred := validator.CommandContains("git commit")

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = pred(bashCtx)
		}
	})

	b.Run("CommandMatches", func(b *testing.B) {
		pred := validator.CommandMatches(`git\s+(commit|push)`)

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = pred(bashCtx)
		}
	})

	b.Run("GitSubcommandIs", func(b *testing.B) {
		// This is the expensive one - creates BashParser + GitCommand per call.
		pred := validator.GitSubcommandIs("commit")

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = pred(bashCtx)
		}
	})

	writeCtx := &hook.Context{
		EventType: hook.EventTypePreToolUse,
		ToolName:  hook.ToolTypeBash,
		ToolInput: hook.ToolInput{Command: `echo "test" > output.sh`},
	}

	b.Run("BashWritesFileWithExtension", func(b *testing.B) {
		pred := validator.BashWritesFileWithExtension(".sh")

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = pred(writeCtx)
		}
	})

	b.Run("And/3Predicates", func(b *testing.B) {
		pred := validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeBash),
			validator.CommandContains("git commit"),
		)

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = pred(bashCtx)
		}
	})
}
