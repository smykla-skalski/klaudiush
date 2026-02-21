package dispatcher_test

import (
	"context"
	"testing"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// passValidator always passes validation.
type passValidator struct {
	*validator.BaseValidator
}

func newPassValidator(name string) *passValidator {
	return &passValidator{
		BaseValidator: validator.NewBaseValidator(name, logger.NewNoOpLogger()),
	}
}

func (*passValidator) Validate(context.Context, *hook.Context) *validator.Result {
	return validator.Pass()
}

// failValidator always fails validation.
type failValidator struct {
	*validator.BaseValidator
}

func newFailValidator(name string) *failValidator {
	return &failValidator{
		BaseValidator: validator.NewBaseValidator(name, logger.NewNoOpLogger()),
	}
}

func (*failValidator) Validate(context.Context, *hook.Context) *validator.Result {
	return validator.FailWithRef("https://klaudiu.sh/e/GIT001", "benchmark error")
}

// BenchmarkDispatch benchmarks the full dispatch path excluding real validator I/O.
func BenchmarkDispatch(b *testing.B) {
	log := logger.NewNoOpLogger()

	bashCtx := &hook.Context{
		EventType: hook.EventTypePreToolUse,
		ToolName:  hook.ToolTypeBash,
		ToolInput: hook.ToolInput{Command: "echo hello"},
	}

	b.Run("NoValidatorsMatch", func(b *testing.B) {
		registry := validator.NewRegistry()
		registry.Register(
			newPassValidator("commit"),
			validator.And(
				validator.EventTypeIs(hook.EventTypePreToolUse),
				validator.ToolTypeIs(hook.ToolTypeBash),
				validator.CommandContains("git commit"),
			),
		)

		d := dispatcher.NewDispatcher(registry, log)
		ctx := context.Background()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = d.Dispatch(ctx, bashCtx)
		}
	})

	b.Run("SingleValidatorPass", func(b *testing.B) {
		registry := validator.NewRegistry()
		registry.Register(
			newPassValidator("always-pass"),
			validator.Always(),
		)

		d := dispatcher.NewDispatcher(registry, log)
		ctx := context.Background()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = d.Dispatch(ctx, bashCtx)
		}
	})

	b.Run("SingleValidatorFail", func(b *testing.B) {
		registry := validator.NewRegistry()
		registry.Register(
			newFailValidator("always-fail"),
			validator.Always(),
		)

		d := dispatcher.NewDispatcher(registry, log)
		ctx := context.Background()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = d.Dispatch(ctx, bashCtx)
		}
	})

	b.Run("FiveValidatorsMixed", func(b *testing.B) {
		registry := validator.NewRegistry()
		for i := range 3 {
			registry.Register(
				newPassValidator("pass-"+string(rune('a'+i))),
				validator.Always(),
			)
		}

		for i := range 2 {
			registry.Register(
				newFailValidator("fail-"+string(rune('a'+i))),
				validator.Always(),
			)
		}

		d := dispatcher.NewDispatcher(registry, log)
		ctx := context.Background()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = d.Dispatch(ctx, bashCtx)
		}
	})

	b.Run("BashFileWriteSynthetic", func(b *testing.B) {
		// Triggers validateBashFileWrites (extra BashParser parse)
		registry := validator.NewRegistry()
		registry.Register(
			newPassValidator("shell"),
			validator.And(
				validator.EventTypeIs(hook.EventTypePreToolUse),
				validator.ToolTypeIs(hook.ToolTypeWrite),
				validator.FileExtensionIs(".sh"),
			),
		)

		fileWriteCtx := &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{Command: `echo "#!/bin/bash" > script.sh`},
		}

		d := dispatcher.NewDispatcher(registry, log)
		ctx := context.Background()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = d.Dispatch(ctx, fileWriteCtx)
		}
	})
}
