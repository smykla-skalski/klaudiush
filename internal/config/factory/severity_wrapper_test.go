package factory

import (
	"context"
	"testing"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

type fakeSeverityConfig struct {
	severity config.Severity
}

func (c fakeSeverityConfig) GetSeverity() config.Severity {
	return c.severity
}

type fakeValidator struct {
	name     string
	category validator.ValidatorCategory
	result   *validator.Result
}

func (v fakeValidator) Name() string {
	return v.name
}

func (v fakeValidator) Validate(context.Context, *hook.Context) *validator.Result {
	return v.result
}

func (v fakeValidator) Category() validator.ValidatorCategory {
	return v.category
}

func TestWrapValidatorWithSeverityDowngradesBlockingResults(t *testing.T) {
	base := fakeValidator{
		name:     "fake",
		category: validator.CategoryCPU,
		result:   validator.FailWithRef(validator.RefGitMissingFlags, "missing flags"),
	}

	wrapped := wrapValidatorWithSeverity(base, fakeSeverityConfig{severity: config.SeverityWarning})
	result := wrapped.Validate(context.Background(), &hook.Context{})

	if result == nil {
		t.Fatal("expected result")
	}

	if result.ShouldBlock {
		t.Fatal("expected warning severity to downgrade blocking result")
	}

	if result.Reference != validator.RefGitMissingFlags {
		t.Fatalf("expected reference to be preserved, got %q", result.Reference)
	}
}

func TestWrapValidatorWithSeverityLeavesWarningResultsUnchanged(t *testing.T) {
	base := fakeValidator{
		name:     "fake",
		category: validator.CategoryCPU,
		result:   validator.WarnWithRef(validator.RefGitMissingFlags, "missing flags"),
	}

	wrapped := wrapValidatorWithSeverity(base, fakeSeverityConfig{severity: config.SeverityWarning})
	result := wrapped.Validate(context.Background(), &hook.Context{})

	if result == nil {
		t.Fatal("expected result")
	}

	if result.ShouldBlock {
		t.Fatal("expected warning result to remain non-blocking")
	}
}

func TestWrapValidatorWithSeveritySkipsDefaultErrorSeverity(t *testing.T) {
	base := fakeValidator{
		name:     "fake",
		category: validator.CategoryCPU,
		result:   validator.Fail("still blocking"),
	}

	wrapped := wrapValidatorWithSeverity(base, fakeSeverityConfig{severity: config.SeverityError})
	result := wrapped.Validate(context.Background(), &hook.Context{})

	if result == nil {
		t.Fatal("expected result")
	}

	if !result.ShouldBlock {
		t.Fatal("expected error severity to preserve blocking result")
	}
}
