package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// stubValidator answers every hook with validate.
type stubValidator struct {
	name     string
	validate func() *validator.Result
}

func (v stubValidator) Name() string { return v.name }

func (stubValidator) Category() validator.ValidatorCategory { return validator.CategoryCPU }

func (v stubValidator) Validate(context.Context, *hook.Context) *validator.Result {
	return v.validate()
}

// captureStdout returns what fn writes to stdout.
func captureStdout(fn func()) string {
	GinkgoHelper()

	read, write, err := os.Pipe()
	Expect(err).NotTo(HaveOccurred())

	original := os.Stdout
	os.Stdout = write

	defer func() { os.Stdout = original }()

	fn()

	Expect(write.Close()).To(Succeed())

	out, err := io.ReadAll(read)
	Expect(err).NotTo(HaveOccurred())

	return string(out)
}

// isolateHome points HOME and every XDG directory at a fresh temp dir, so
// nothing reads or writes the real user configuration.
func isolateHome() string {
	dir := GinkgoT().TempDir()

	GinkgoT().Setenv("HOME", dir)
	GinkgoT().Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	GinkgoT().Setenv("XDG_DATA_HOME", filepath.Join(dir, ".local", "share"))
	GinkgoT().Setenv("XDG_STATE_HOME", filepath.Join(dir, ".local", "state"))
	GinkgoT().Setenv("XDG_CACHE_HOME", filepath.Join(dir, ".cache"))
	GinkgoT().Setenv(failureModeEnv, "")
	Expect(os.Unsetenv(failureModeEnv)).To(Succeed())

	return dir
}

var _ = Describe("hook failures", func() {
	var (
		log         logger.Logger
		savedMode   string
		savedGrace  time.Duration
		savedConfig *config.Config
	)

	BeforeEach(func() {
		isolateHome()

		log = logger.NewNoOpLogger()
		savedMode, savedGrace, savedConfig = failureMode, watchdogGrace, crashConfig
		failureMode = ""

		crashConfig = &config.Config{CrashDump: &config.CrashDumpConfig{Enabled: new(false)}}
	})

	AfterEach(func() {
		failureMode, watchdogGrace, crashConfig = savedMode, savedGrace, savedConfig
	})

	newRun := func(provider hook.Provider, event string) *hookRun {
		return newHookRun(
			provider,
			hook.ResolveLegacyEventType(provider, event, hook.EventTypeUnknown),
			event,
			log,
		)
	}

	decode := func(out string) map[string]any {
		GinkgoHelper()

		var resp map[string]any
		Expect(json.Unmarshal([]byte(out), &resp)).To(Succeed(), out)

		return resp
	}

	permissionDecision := func(resp map[string]any) any {
		specific, _ := resp["hookSpecificOutput"].(map[string]any)

		return specific["permissionDecision"]
	}

	It("writes nothing when validation finished on its own", func() {
		h := newRun(hook.ProviderClaude, "PreToolUse")

		out := captureStdout(func() {
			Expect(h.supervise(func() error { return nil })).To(Succeed())
		})
		Expect(out).To(BeEmpty())
	})

	It("passes through errors that are not hook failures", func() {
		h := newRun(hook.ProviderClaude, "PreToolUse")
		boom := errors.New("bad flag")

		Expect(h.supervise(func() error { return boom })).To(MatchError(boom))
	})

	It("warns about unreadable input by default", func() {
		h := newRun(hook.ProviderClaude, "PreToolUse")

		out := captureStdout(func() {
			Expect(h.supervise(func() error {
				return failHook(validator.ReasonMalformedInput, errors.New("invalid JSON"))
			})).To(Succeed())
		})

		resp := decode(out)
		Expect(permissionDecision(resp)).To(BeNil())
		Expect(resp["systemMessage"]).To(ContainSubstring("HOOK001"))
		Expect(resp["systemMessage"]).To(ContainSubstring("unreadable hook input"))
	})

	It("denies when the flag asks to block", func() {
		failureMode = "block"
		h := newRun(hook.ProviderCodex, "PreToolUse")

		out := captureStdout(func() {
			Expect(h.supervise(func() error {
				return failHook(validator.ReasonConfig, errors.New("broken config"))
			})).To(Succeed())
		})

		Expect(permissionDecision(decode(out))).To(Equal("deny"))
	})

	It("never blocks a completion gate", func() {
		h := newRun(hook.ProviderClaude, "Stop")
		h.setPolicy(failpolicy.New(nil).WithMode(failpolicy.ActionBlock))

		out := captureStdout(func() {
			Expect(h.supervise(func() error {
				return failHook(validator.ReasonConfig, errors.New("broken config"))
			})).To(Succeed())
		})

		resp := decode(out)
		Expect(resp).NotTo(HaveKey("decision"))
		Expect(resp["systemMessage"]).To(ContainSubstring("Validation unavailable"))
	})

	It("answers for a validation that panicked", func() {
		h := newRun(hook.ProviderGemini, "BeforeTool")
		h.setPolicy(failpolicy.New(&config.FailurePolicyConfig{Mode: "block"}))

		out := captureStdout(func() {
			Expect(h.supervise(func() error { panic("kaboom") })).To(Succeed())
		})

		resp := decode(out)
		Expect(resp["decision"]).To(Equal("deny"))
		Expect(resp["reason"]).To(ContainSubstring("crashed"))
		Expect(resp["reason"]).To(ContainSubstring("kaboom"))
	})

	It("answers for a validation that overran its deadline", func() {
		watchdogGrace = 10 * time.Millisecond

		h := newRun(hook.ProviderClaude, "PreToolUse")
		h.setPolicy(failpolicy.New(&config.FailurePolicyConfig{
			Mode:     "block",
			Deadline: config.Duration(20 * time.Millisecond),
		}))

		release := make(chan struct{})
		defer close(release)

		out := captureStdout(func() {
			Expect(h.supervise(func() error {
				<-release

				return nil
			})).To(Succeed())
		})

		resp := decode(out)
		Expect(permissionDecision(resp)).To(Equal("deny"))
		Expect(resp["systemMessage"]).To(ContainSubstring("timed out"))
		Expect(h.claim()).To(BeFalse())
	})

	It("keeps a deny found before the watchdog answered", func() {
		watchdogGrace = 10 * time.Millisecond

		h := newRun(hook.ProviderClaude, "PreToolUse")
		h.setPolicy(failpolicy.New(&config.FailurePolicyConfig{
			Deadline: config.Duration(20 * time.Millisecond),
		}))

		found := []*dispatcher.ValidationError{{
			Validator:   "validate-shellscript",
			Message:     "bad script",
			ShouldBlock: true,
		}}
		h.errs.Store(&found)

		release := make(chan struct{})
		defer close(release)

		out := captureStdout(func() {
			Expect(h.supervise(func() error {
				<-release

				return nil
			})).To(Succeed())
		})

		resp := decode(out)
		Expect(permissionDecision(resp)).To(Equal("deny"))
		Expect(resp["systemMessage"]).To(ContainSubstring("bad script"))
		Expect(resp["systemMessage"]).To(ContainSubstring("timed out"))
	})

	It("keeps a deny published while a later validator never returns", func() {
		watchdogGrace = 10 * time.Millisecond

		h := newRun(hook.ProviderClaude, "PreToolUse")
		h.setPolicy(failpolicy.New(&config.FailurePolicyConfig{
			Deadline: config.Duration(20 * time.Millisecond),
		}))

		release := make(chan struct{})
		defer close(release)

		reg := validator.NewRegistry()
		reg.Register(
			stubValidator{name: "validate-deny", validate: func() *validator.Result {
				return validator.Fail("bad command")
			}},
			validator.EventTypeIs(hook.EventTypePreToolUse),
		)
		reg.Register(
			stubValidator{name: "validate-stuck", validate: func() *validator.Result {
				<-release

				return validator.Pass()
			}},
			validator.EventTypeIs(hook.EventTypePreToolUse),
		)

		disp := dispatcher.NewDispatcherWithOptions(reg, log,
			dispatcher.NewSequentialExecutor(log), dispatcher.WithProgress(h.publish))

		out := captureStdout(func() {
			Expect(h.supervise(func() error {
				disp.Dispatch(context.Background(), &hook.Context{
					Event:     hook.CanonicalEventBeforeTool,
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{Command: "ls"},
				})

				return nil
			})).To(Succeed())
		})

		resp := decode(out)
		Expect(permissionDecision(resp)).To(Equal("deny"))
		Expect(resp["systemMessage"]).To(ContainSubstring("bad command"))
		Expect(resp["systemMessage"]).To(ContainSubstring("timed out"))
	})

	It("waits for a response validation already started writing", func() {
		watchdogGrace = 10 * time.Millisecond

		h := newRun(hook.ProviderClaude, "PreToolUse")
		h.setPolicy(failpolicy.New(&config.FailurePolicyConfig{
			Deadline: config.Duration(time.Millisecond),
		}))

		out := captureStdout(func() {
			Expect(h.supervise(func() error {
				Expect(h.claim()).To(BeTrue())
				time.Sleep(15 * time.Millisecond)

				return nil
			})).To(Succeed())
		})
		Expect(out).To(BeEmpty())
	})

	It("writes one response when the watchdog already answered", func() {
		h := newRun(hook.ProviderClaude, "PreToolUse")
		Expect(h.claim()).To(BeTrue())

		out := captureStdout(func() {
			Expect(h.supervise(func() error {
				return failHook(validator.ReasonTimeout, errors.New("late"))
			})).To(Succeed())
		})
		Expect(out).To(BeEmpty())
	})

	Describe("failureContext", func() {
		It("prefers the parsed context", func() {
			h := newRun(hook.ProviderClaude, "PreToolUse")
			parsed := &hook.Context{Provider: hook.ProviderClaude, SessionID: "s"}
			h.hookCtx.Store(parsed)

			Expect(h.failureContext()).To(BeIdenticalTo(parsed))
		})

		It("builds one from the flags", func() {
			hookCtx := newRun(hook.ProviderCodex, "PreToolUse").failureContext()

			Expect(hookCtx.Provider).To(Equal(hook.ProviderCodex))
			Expect(hookCtx.Event).To(Equal(hook.CanonicalEventBeforeTool))
		})
	})

	DescribeTable("scanMode",
		func(content, want string) {
			Expect(scanMode(content)).To(Equal(want))
		},
		Entry("plain", "[failure_policy]\nmode = \"block\"\n", "block"),
		Entry("comment and quotes", "[ failure_policy ]\n  mode='warn' # note\n", "warn"),
		Entry("other section", "[output]\nmode = \"block\"\n", ""),
		Entry("broken file", "[failure_policy]\nmode = \"block\"\n[validators\n", "block"),
		Entry("missing", "", ""),
	)

	It("blocks a deadline overrun when a validator is critical", func() {
		watchdogGrace = 10 * time.Millisecond

		h := newRun(hook.ProviderClaude, "PreToolUse")
		h.setPolicy(failpolicy.New(&config.FailurePolicyConfig{
			Critical: []string{"plugins"},
			Deadline: config.Duration(10 * time.Millisecond),
		}))

		release := make(chan struct{})
		defer close(release)

		out := captureStdout(func() {
			Expect(h.supervise(func() error {
				<-release

				return nil
			})).To(Succeed())
		})

		Expect(permissionDecision(decode(out))).To(Equal("deny"))
	})

	It("knows where a deny stops the action", func() {
		Expect(canStopAction(&hook.Context{Event: hook.CanonicalEventBeforeTool})).To(BeTrue())
		Expect(canStopAction(&hook.Context{Event: hook.CanonicalEventElicitation})).To(BeTrue())
		Expect(canStopAction(&hook.Context{Event: hook.CanonicalEventAfterTool})).To(BeFalse())
		Expect(canStopAction(&hook.Context{Event: hook.CanonicalEventTurnStop})).To(BeFalse())
	})

	Describe("buildPolicy", func() {
		It("uses the configuration without a flag", func() {
			policy, err := buildPolicy(&config.Config{
				FailurePolicy: &config.FailurePolicyConfig{Mode: "block"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(policy.Mode()).To(Equal(failpolicy.ActionBlock))

			policy, err = buildPolicy(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(policy.ModeSet()).To(BeFalse())
		})

		It("lets the flag override the configuration", func() {
			failureMode = "warn"

			policy, err := buildPolicy(&config.Config{
				FailurePolicy: &config.FailurePolicyConfig{Mode: "block"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(policy.Mode()).To(Equal(failpolicy.ActionWarn))
		})

		It("blocks on an unreadable flag", func() {
			failureMode = "maybe"

			policy, err := buildPolicy(nil)
			Expect(err).To(HaveOccurred())
			Expect(policy.Mode()).To(Equal(failpolicy.ActionBlock))
		})
	})

	Describe("fallbackPolicy", func() {
		It("uses the flag first", func() {
			failureMode = "block"

			Expect(fallbackPolicy("", log).Mode()).To(Equal(failpolicy.ActionBlock))

			failureMode = "bogus"

			Expect(fallbackPolicy("", log).Mode()).To(Equal(failpolicy.ActionBlock))
		})

		It("uses the environment next", func() {
			GinkgoT().Setenv(failureModeEnv, "block")
			Expect(fallbackPolicy("", log).Mode()).To(Equal(failpolicy.ActionBlock))

			GinkgoT().Setenv(failureModeEnv, "bogus")
			Expect(fallbackPolicy("", log).Mode()).To(Equal(failpolicy.ActionBlock))
		})

		It("reads the mode of an invalid configuration", func() {
			workDir := GinkgoT().TempDir()
			Expect(os.MkdirAll(filepath.Join(workDir, ".klaudiush"), 0o700)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(workDir, ".klaudiush", "config.toml"),
				[]byte(
					"[failure_policy]\nmode = \"block\"\n[validators.git.commit.message]\ntitle_max_length = -1\n",
				),
				0o600,
			)).To(Succeed())

			Expect(fallbackPolicy(workDir, log).Mode()).To(Equal(failpolicy.ActionBlock))
		})

		It("reads an unreadable configured mode as block", func() {
			workDir := GinkgoT().TempDir()
			Expect(os.MkdirAll(filepath.Join(workDir, ".klaudiush"), 0o700)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(workDir, ".klaudiush", "config.toml"),
				[]byte("[failure_policy]\nmode = \"strict\"\n"),
				0o600,
			)).To(Succeed())

			Expect(fallbackPolicy(workDir, log).Mode()).To(Equal(failpolicy.ActionBlock))
		})

		It("falls back to the global configuration when the project file is broken", func() {
			home := os.Getenv("HOME")
			globalDir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "klaudiush")
			Expect(os.MkdirAll(globalDir, 0o700)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(globalDir, "config.toml"),
				[]byte("[failure_policy]\nmode = \"block\"\n"),
				0o600,
			)).To(Succeed())

			workDir := filepath.Join(home, "project")
			Expect(os.MkdirAll(filepath.Join(workDir, ".klaudiush"), 0o700)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(workDir, ".klaudiush", "config.toml"),
				[]byte("[validators\nbroken"),
				0o600,
			)).To(Succeed())

			Expect(fallbackPolicy(workDir, log).Mode()).To(Equal(failpolicy.ActionBlock))
		})

		It("scans the mode out of a file that does not decode", func() {
			workDir := GinkgoT().TempDir()
			Expect(os.MkdirAll(filepath.Join(workDir, ".klaudiush"), 0o700)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(workDir, ".klaudiush", "config.toml"),
				[]byte(
					"[failure_policy]\nmode = \"block\"\n[validators.git.commit.message]\ntitle_max_length = \"abc\"\n",
				),
				0o600,
			)).To(Succeed())

			Expect(fallbackPolicy(workDir, log).Mode()).To(Equal(failpolicy.ActionBlock))
		})

		It("defaults to warn", func() {
			Expect(fallbackPolicy(GinkgoT().TempDir(), log).Mode()).To(Equal(failpolicy.ActionWarn))
			Expect(fallbackPolicy("", log).ModeSet()).To(BeFalse())
		})
	})

	It("reports unusable session state as a warning", func() {
		verr := stateUnavailable(
			"read the session's unresolved findings",
			errors.New("lock held\nmore"),
		)

		Expect(verr.ShouldBlock).To(BeFalse())
		Expect(verr.Unavailable).To(BeTrue())
		Expect(verr.UnavailableReason).To(Equal(validator.ReasonState))
		Expect(verr.Message).To(ContainSubstring("lock held"))
		Expect(verr.Message).NotTo(ContainSubstring("more"))
	})
})
