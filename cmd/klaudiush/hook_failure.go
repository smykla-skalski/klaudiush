package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"

	internalconfig "github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/internal/parser"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// failureModeEnv sets the failure mode when the configuration cannot be
// read. The config loader maps it onto failure_policy.mode as well.
const failureModeEnv = "KLAUDIUSH_FAILURE_POLICY_MODE"

// watchdogGrace is how long past the deadline validators get to wind down
// and answer themselves before the watchdog answers for them.
var watchdogGrace = 3 * time.Second

const failureValidatorName = "klaudiush"

// failureMode is the --failure-mode flag. It lives in the hook command the
// provider runs, outside every config file, so it still applies when the
// configuration cannot be read.
var failureMode string

// hookFailureError is an error that stopped the hook before validation finished.
type hookFailureError struct {
	reason validator.UnavailableReason
	err    error
}

func (f *hookFailureError) Error() string {
	return f.err.Error()
}

func (f *hookFailureError) Unwrap() error {
	return f.err
}

func failHook(reason validator.UnavailableReason, err error) error {
	return &hookFailureError{reason: reason, err: err}
}

// hookRun is one hook invocation. It owns stdout: exactly one response is
// written, by the validation or by the watchdog when validation overruns
// (claimed records who wrote it). The policy, parsed context, output config
// and working directory are filled in as validation learns them, and
// deadlines carries the configured deadline to the watchdog.
type hookRun struct {
	provider  hook.Provider
	eventType hook.EventType
	eventName string
	log       logger.Logger
	start     time.Time
	claimed   atomic.Bool
	deadlines chan time.Duration
	policy    atomic.Pointer[failpolicy.Policy]
	hookCtx   atomic.Pointer[hook.Context]
	output    atomic.Pointer[config.OutputConfig]
	workDir   atomic.Pointer[string]
	errs      atomic.Pointer[[]*dispatcher.ValidationError]
}

func newHookRun(
	provider hook.Provider,
	eventType hook.EventType,
	eventName string,
	log logger.Logger,
) *hookRun {
	return &hookRun{
		provider:  provider,
		eventType: eventType,
		eventName: eventName,
		log:       log,
		start:     time.Now(),
		deadlines: make(chan time.Duration, 1),
	}
}

// claim reports whether the caller may write the response.
func (h *hookRun) claim() bool {
	return h.claimed.CompareAndSwap(false, true)
}

// setPolicy records the effective policy and tells the watchdog its deadline.
func (h *hookRun) setPolicy(policy *failpolicy.Policy) {
	h.policy.Store(policy)

	select {
	case h.deadlines <- policy.Deadline():
	default:
	}
}

// supervise runs validate and answers for it when it overruns its deadline
// or panics. Before the configuration is read the default deadline applies.
func (h *hookRun) supervise(validate func() error) error {
	done := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				handlePanic(r)
				h.log.Error("hook panicked", "panic", fmt.Sprint(r))

				done <- failHook(validator.ReasonPanic, errors.Newf("klaudiush crashed: %v", r))
			}
		}()

		done <- validate()
	}()

	timer := time.NewTimer(config.DefaultFailureDeadline + watchdogGrace)
	defer timer.Stop()

	for {
		select {
		case deadline := <-h.deadlines:
			timer.Reset(time.Until(h.start.Add(deadline + watchdogGrace)))
		case err := <-done:
			return h.finish(err)
		case <-timer.C:
			if h.claimed.Load() {
				return h.awaitWriter(done)
			}

			h.log.Error("hook deadline exceeded", "elapsed", time.Since(h.start).String())

			return h.finish(failHook(
				validator.ReasonTimeout,
				errors.Newf(
					"validation did not finish within %s",
					time.Since(h.start).Round(time.Second),
				),
			))
		}
	}
}

// awaitWriter gives validation, which already started writing its response,
// time to finish before the process exits.
func (h *hookRun) awaitWriter(done <-chan error) error {
	select {
	case err := <-done:
		return h.finish(err)
	case <-time.After(watchdogGrace):
		h.log.Error("response writer did not finish in time")

		return nil
	}
}

// finish answers a failure that stopped validation. Errors that do not come
// from the hook pipeline still exit non-zero.
func (h *hookRun) finish(err error) error {
	var failure *hookFailureError
	if !errors.As(err, &failure) {
		return err
	}

	h.log.Error("validation unavailable",
		"reason", string(failure.reason),
		"error", failure.err,
	)

	fmt.Fprintf(os.Stderr, "klaudiush: validation unavailable (%s): %v\n",
		failure.reason.Describe(), failure.err)

	if !h.claim() {
		return nil
	}

	hookCtx := h.failureContext()
	errs := []*dispatcher.ValidationError{h.failureError(hookCtx, failure)}

	// Findings validation already made still count: a deny found before the
	// deadline must not turn into a timeout warning.
	if found := h.errs.Load(); found != nil {
		errs = append(slices.Clone(*found), errs...)
	}

	return writeResponse(
		hookCtx,
		errs,
		nil,
		nil,
		h.output.Load(),
		h.log,
	)
}

// failureError describes the failure for the response. Blocking is only
// offered where the provider stops the action before it happens; at a
// completion gate a hook that always fails would keep the agent going
// forever, and after a tool nothing can be stopped.
func (h *hookRun) failureError(
	hookCtx *hook.Context,
	failure *hookFailureError,
) *dispatcher.ValidationError {
	action := h.effectivePolicy().Mode()
	if !canStopAction(hookCtx) {
		action = failpolicy.ActionWarn
	}

	return &dispatcher.ValidationError{
		Validator: failureValidatorName,
		Message: fmt.Sprintf(
			"klaudiush could not run (%s): %s",
			failure.reason.Describe(),
			firstLine(failure.err.Error()),
		),
		ShouldBlock:       action == failpolicy.ActionBlock,
		Reference:         validator.RefValidationUnavailable,
		FixHint:           validator.GetSuggestion(validator.RefValidationUnavailable),
		Unavailable:       true,
		UnavailableReason: failure.reason,
	}
}

// canStopAction reports events where a deny stops the action before it runs.
func canStopAction(hookCtx *hook.Context) bool {
	return hookCtx.Event == hook.CanonicalEventBeforeTool || hookCtx.IsElicitationEvent()
}

// failureContext returns the parsed hook context, or one built from the
// provider and event flags when the input could not be parsed.
func (h *hookRun) failureContext() *hook.Context {
	if hookCtx := h.hookCtx.Load(); hookCtx != nil {
		return hookCtx
	}

	hookCtx, err := parser.NewJSONParser(strings.NewReader("{}")).ParseWithOptions(
		parser.ParseOptions{
			Provider:  h.provider,
			EventType: h.eventType,
			EventName: h.eventName,
		},
	)
	if err != nil {
		return &hook.Context{Provider: h.provider, EventType: h.eventType}
	}

	return hookCtx
}

// effectivePolicy returns the policy from configuration, or the fallback
// when the configuration has not been read.
func (h *hookRun) effectivePolicy() *failpolicy.Policy {
	if policy := h.policy.Load(); policy != nil {
		return policy
	}

	workDir := ""
	if dir := h.workDir.Load(); dir != nil {
		workDir = *dir
	}

	policy := fallbackPolicy(workDir, h.log)
	h.policy.Store(policy)

	return policy
}

// buildPolicy builds the failure policy from configuration and applies the
// --failure-mode flag on top. An unreadable flag blocks: a mode was asked
// for, and blocking is the reading a typo cannot turn into a silent pass.
func buildPolicy(cfg *config.Config) (*failpolicy.Policy, error) {
	var policyCfg *config.FailurePolicyConfig
	if cfg != nil {
		policyCfg = cfg.FailurePolicy
	}

	policy := failpolicy.New(policyCfg)

	if failureMode == "" {
		return policy, nil
	}

	mode, err := failpolicy.ParseMode(failureMode)
	if err != nil {
		return policy.WithMode(failpolicy.ActionBlock), errors.Wrap(err, "--failure-mode")
	}

	return policy.WithMode(mode), nil
}

// fallbackPolicy finds the failure mode when the configuration cannot be
// loaded: the flag, then the environment, then whatever the configuration
// says once validation is skipped, then the global configuration alone.
func fallbackPolicy(workDir string, log logger.Logger) *failpolicy.Policy {
	if failureMode != "" {
		policy, err := buildPolicy(nil)
		if err != nil {
			log.Error("invalid failure mode flag, blocking", "error", err)
		}

		return policy
	}

	if mode := os.Getenv(failureModeEnv); mode != "" {
		action, err := failpolicy.ParseMode(mode)
		if err != nil {
			log.Error("invalid failure mode environment variable, blocking", "error", err)

			action = failpolicy.ActionBlock
		}

		return failpolicy.New(nil).WithMode(action)
	}

	if cfg := looseConfig(workDir); cfg != nil {
		return failpolicy.New(cfg.FailurePolicy)
	}

	return failpolicy.New(nil)
}

// looseConfig reads what it can of a configuration that failed to load.
func looseConfig(workDir string) *config.Config {
	var (
		loader *internalconfig.KoanfLoader
		err    error
	)

	if workDir != "" {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return nil
		}

		loader, err = internalconfig.NewKoanfLoaderWithDirs(homeDir, workDir)
	} else {
		loader, err = internalconfig.NewKoanfLoader()
	}

	if err != nil {
		return nil
	}

	if cfg, loadErr := loader.LoadWithoutValidation(buildFlagsMap()); loadErr == nil {
		return strictPolicyMode(cfg)
	}

	if cfg, _, loadErr := loader.LoadGlobalConfigOnly(); loadErr == nil && cfg != nil {
		return strictPolicyMode(cfg)
	}

	return nil
}

// strictPolicyMode reads an unreadable mode of a loose configuration as
// block: a mode was asked for, and the strictest reading cannot let an edit
// to the configuration switch enforcement off.
func strictPolicyMode(cfg *config.Config) *config.Config {
	if cfg.FailurePolicy == nil || cfg.FailurePolicy.Mode == "" {
		return cfg
	}

	if _, err := failpolicy.ParseMode(cfg.FailurePolicy.Mode); err != nil {
		cfg.FailurePolicy.Mode = config.FailureModeBlock
	}

	return cfg
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")

	return line
}
