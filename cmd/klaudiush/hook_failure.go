package main

import (
	"fmt"
	"os"
	"path/filepath"
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
	policyvalidators "github.com/smykla-skalski/klaudiush/internal/validators/policy"
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

// hookFailureError is an error that stopped the hook before validation
// finished. critical marks failures that may hide a policy guard: they
// block whatever the failure mode says.
type hookFailureError struct {
	reason   validator.UnavailableReason
	err      error
	critical bool
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

func failHookCritical(reason validator.UnavailableReason, err error) error {
	return &hookFailureError{reason: reason, err: err, critical: true}
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
	metrics   atomic.Pointer[config.MetricsConfig]
	outcome   atomic.Pointer[dispatcher.Outcome]
	released  atomic.Bool
	skipped   atomic.Bool
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

// publish records the findings validation made so far, which the watchdog
// keeps when it answers for a validation that overran its deadline.
func (h *hookRun) publish(errs []*dispatcher.ValidationError) {
	h.errs.Store(&errs)
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

	stopped, err := writeResponse(
		hookCtx,
		errs,
		nil,
		nil,
		h.output.Load(),
		h.log,
	)

	h.recordMetrics(hookCtx, errs, stopped)

	return err
}

// failureError describes the failure for the response. Blocking is only
// offered where the provider stops the action before it happens; at a
// completion gate a hook that always fails would keep the agent going
// forever, and after a tool nothing can be stopped.
func (h *hookRun) failureError(
	hookCtx *hook.Context,
	failure *hookFailureError,
) *dispatcher.ValidationError {
	policy := h.effectivePolicy()

	action := policy.Mode()

	// The watchdog cannot tell which check hung, so any critical validator
	// makes an overrun block.
	if failure.critical ||
		(failure.reason == validator.ReasonTimeout && len(policy.Critical()) > 0) {
		action = failpolicy.ActionBlock
	}

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

	policy := failpolicy.New(policyCfg).WithCritical(policyGuards(cfg)...)

	if failureMode == "" {
		return policy, nil
	}

	mode, err := failpolicy.ParseMode(failureMode)
	if err != nil {
		return policy.WithMode(failpolicy.ActionBlock), errors.Wrap(err, "--failure-mode")
	}

	return policy.WithMode(mode), nil
}

// policyGuards names the enabled validators that guard policy itself. They
// are always critical: a crash or a hook timeout must not be a way past them.
func policyGuards(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}

	var names []string

	if cfg.Protection.IsEnabled() {
		names = append(names, policyvalidators.ProtectionValidatorName)
	}

	if cfg.MCPTrust.IsEnabled() {
		names = append(names, policyvalidators.MCPTrustValidatorName)
	}

	return names
}

// projectDirEnv are the variables harnesses set to the session's project
// directory in the hook environment.
var projectDirEnv = []string{"CLAUDE_PROJECT_DIR", "GEMINI_PROJECT_DIR"}

// policyConfigDirs lists the other directories whose project configuration
// can turn the policy guards on: the hook's working directory and the
// harness project directory, when they differ from the directory the
// configuration was loaded for (a cd target).
func policyConfigDirs(hookCtx *hook.Context, workDir string) []string {
	candidates := []string{hookCtx.GetWorkingDir()}

	for _, name := range projectDirEnv {
		candidates = append(candidates, os.Getenv(name))
	}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}

	loaded := workDir
	if loaded == "" {
		loaded, _ = os.Getwd()
	}

	var dirs []string

	for _, dir := range candidates {
		if dir == "" || !filepath.IsAbs(dir) || filepath.Clean(dir) == filepath.Clean(loaded) ||
			slices.Contains(dirs, filepath.Clean(dir)) {
			continue
		}

		dirs = append(dirs, filepath.Clean(dir))
	}

	return dirs
}

// inheritPolicyGuards turns on protection and MCP trust when the project
// configuration of any of dirs enables them. Loading configuration from a
// cd target must not drop the guards of the project the agent works in:
// "cd /tmp && git status; rm .klaudiush/config.toml" would otherwise run
// under /tmp's configuration. Every directory whose protection is on is
// kept as a policy source, so its hook, evidence and plugin files stay
// protected. A configuration that cannot be read may be the one enabling
// the guards, so it fails the hook instead of being skipped.
func inheritPolicyGuards(cfg *config.Config, dirs []string, log logger.Logger) error {
	if cfg == nil {
		return nil
	}

	for _, dir := range dirs {
		other, err := loadConfig(log, dir)
		if err != nil {
			return errors.Wrapf(err, "failed to load policy guards from %s", dir)
		}

		if other.Protection.IsEnabled() {
			if !cfg.Protection.IsEnabled() {
				cfg.Protection = other.Protection
			}

			cfg.PolicySources = append(cfg.PolicySources, other)
		}

		if !cfg.MCPTrust.IsEnabled() && other.MCPTrust.IsEnabled() {
			cfg.MCPTrust = other.MCPTrust
		}
	}

	return nil
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

	if cfg := looseConfig(workDir); cfg != nil && cfg.FailurePolicy.GetMode() != "" {
		return failpolicy.New(cfg.FailurePolicy)
	}

	if mode := scannedMode(workDir); mode != "" {
		action, err := failpolicy.ParseMode(mode)
		if err != nil {
			action = failpolicy.ActionBlock
		}

		return failpolicy.New(nil).WithMode(action)
	}

	return failpolicy.New(nil)
}

// scannedMode reads failure_policy.mode line by line from the project and
// global config files, for files that do not parse or decode. The strictest
// mode found wins.
func scannedMode(workDir string) string {
	loader, err := configLoader(workDir)
	if err != nil {
		return ""
	}

	found := ""

	for _, path := range append(loader.ProjectConfigPaths(), loader.GlobalConfigPath()) {
		data, readErr := readConfigFile(path)
		if readErr != nil {
			continue
		}

		mode := scanMode(string(data))
		if mode == "" {
			continue
		}

		if action, parseErr := failpolicy.ParseMode(mode); parseErr != nil ||
			action == failpolicy.ActionBlock {
			return config.FailureModeBlock
		}

		found = mode
	}

	return found
}

// readConfigFile reads one config file through a root at its directory, so
// the read cannot leave it.
func readConfigFile(path string) ([]byte, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, errors.Wrap(err, "open config directory")
	}

	data, err := root.ReadFile(filepath.Base(path))

	return data, errors.CombineErrors(
		errors.Wrap(err, "read config file"),
		errors.Wrap(root.Close(), "close config directory"),
	)
}

// scanMode finds mode = "..." inside a [failure_policy] table.
func scanMode(content string) string {
	inSection := false

	for line := range strings.Lines(content) {
		line = withoutComment(line)

		if strings.HasPrefix(line, "[") {
			inSection = strings.Trim(line, "[] \t") == "failure_policy"

			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !inSection || !ok || strings.TrimSpace(key) != "mode" {
			continue
		}

		return strings.Trim(strings.TrimSpace(value), `"'`)
	}

	return ""
}

// withoutComment trims a TOML line and drops its comment: the first # that
// is not inside a basic ("...") or literal ('...') string.
func withoutComment(line string) string {
	var quote rune

	escaped := false

	for i, r := range line {
		switch {
		case escaped:
			escaped = false
		case quote == '"' && r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#':
			return strings.TrimSpace(line[:i])
		}
	}

	return strings.TrimSpace(line)
}

// configLoader returns a loader for workDir, or the process directory.
func configLoader(workDir string) (*internalconfig.KoanfLoader, error) {
	if workDir == "" {
		return internalconfig.NewKoanfLoader()
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.Wrap(err, "home directory")
	}

	return internalconfig.NewKoanfLoaderWithDirs(homeDir, workDir)
}

// looseConfig reads what it can of a configuration that failed to load.
func looseConfig(workDir string) *config.Config {
	loader, err := configLoader(workDir)
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
