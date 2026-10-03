package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/evidence"
	kexec "github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

func evidenceRepo() string {
	repo := GinkgoT().TempDir()

	run := func(args ...string) {
		cmd := osexec.Command("git", args...)
		cmd.Dir = repo

		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
	}

	run("init", "-q")
	Expect(os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\n"), 0o600)).To(Succeed())
	run("add", "-A")
	run("commit", "-qm", "init")

	resolved, err := filepath.EvalSymlinks(repo)
	Expect(err).NotTo(HaveOccurred())

	return resolved
}

func evidenceConfig(checks ...*config.EvidenceCheckConfig) *config.Config {
	enabled := true

	return &config.Config{Evidence: &config.EvidenceConfig{Enabled: &enabled, Checks: checks}}
}

func evidenceCtx(event hook.CanonicalEvent, raw, repo string) *hook.Context {
	return &hook.Context{
		Provider:     hook.ProviderClaude,
		Event:        event,
		RawEventName: raw,
		SessionID:    "s1",
		WorkingDir:   repo,
		ToolName:     hook.ToolTypeBash,
		ToolFamily:   hook.ToolFamilyShell,
		ToolUseID:    "t1",
		ToolInput:    hook.ToolInput{Command: "make test"},
	}
}

var _ = Describe("evidenceGate", func() {
	var (
		repo  string
		store *hooksession.Store
		log   logger.Logger
		check *config.EvidenceCheckConfig
	)

	BeforeEach(func() {
		repo = evidenceRepo()
		store = hooksession.NewStore(
			hooksession.WithStateFile(filepath.Join(GinkgoT().TempDir(), "state.json")),
		)
		log = logger.NewNoOpLogger()
		check = &config.EvidenceCheckConfig{Name: "tests", Commands: []string{"make test"}}
	})

	It("has no checks of its own unless enabled with valid checks", func() {
		Expect(newEvidenceGate(evidenceConfig(check), nil, nil, log)).To(BeNil())

		for _, cfg := range []*config.Config{
			nil,
			{},
			evidenceConfig(),
			evidenceConfig(&config.EvidenceCheckConfig{Name: "x"}),
		} {
			gate := newEvidenceGate(cfg, store, nil, log)
			Expect(gate).NotTo(BeNil())
			Expect(gate.checks).To(BeEmpty())
		}

		var gate *evidenceGate

		errs := []*dispatcher.ValidationError{{Message: "kept"}}
		Expect(gate.apply(context.Background(), &hook.Context{}, errs)).To(Equal(errs))
	})

	It("does nothing without checks unless a file tool edits or the turn stops", func() {
		gate := newEvidenceGate(&config.Config{}, store, nil, log)
		gate.loadChecks = func(string) ([]*evidence.Check, error) {
			Fail("no repository should be loaded")

			return nil, nil
		}

		pre := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)
		Expect(gate.apply(context.Background(), pre, nil)).To(BeEmpty())
	})

	It("follows edits into a gated repository from a directory without checks", func() {
		other := evidenceRepo()
		withChecks := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate := newEvidenceGate(&config.Config{}, store, nil, log)
		gate.loadChecks = func(root string) ([]*evidence.Check, error) {
			if root == other {
				return withChecks.checks, nil
			}

			return nil, nil
		}

		write := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)
		write.ToolName, write.ToolFamily = hook.ToolTypeWrite, hook.ToolFamilyWrite
		write.AffectedPaths = []string{filepath.Join(other, "a.go")}
		gate.apply(context.Background(), write, nil)

		Expect(os.WriteFile(filepath.Join(other, "a.go"), []byte("package b\n"), 0o600)).
			To(Succeed())

		stop := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo)
		errs := gate.apply(context.Background(), stop, nil)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Details[evidenceValidator]).To(ContainSubstring(other))
	})

	It("reports an edited repository whose checks cannot load as unavailable", func() {
		other := evidenceRepo()
		gate := newEvidenceGate(&config.Config{}, store, nil, log)
		gate.loadChecks = func(root string) ([]*evidence.Check, error) {
			if root == other {
				return nil, os.ErrPermission
			}

			return nil, nil
		}

		read := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)
		read.ToolName, read.ToolFamily = hook.ToolTypeRead, hook.ToolFamilyRead
		read.AffectedPaths = []string{filepath.Join(other, "a.go")}
		gate.apply(context.Background(), read, nil)

		stop := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo)
		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())

		write := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)
		write.ToolName, write.ToolFamily = hook.ToolTypeWrite, hook.ToolFamilyWrite
		write.AffectedPaths = []string{filepath.Join(other, "a.go")}
		gate.apply(context.Background(), write, nil)

		errs := gate.apply(context.Background(), stop, nil)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Reference).To(Equal(validator.RefValidationUnavailable))
		Expect(errs[0].UnavailableReason).To(Equal(validator.ReasonConfig))
		Expect(errs[0].ShouldBlock).To(BeTrue())
		Expect(errs[0].Message).To(ContainSubstring("load the evidence checks of " + other))

		gate.policy = failpolicy.New(&config.FailurePolicyConfig{Mode: config.FailureModeWarn})
		errs = gate.apply(context.Background(), stop, nil)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].ShouldBlock).To(BeFalse())

		gate.loadChecks = func(string) ([]*evidence.Check, error) { return nil, nil }
		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())
	})

	It("reports its own checks that do not compile at the gate", func() {
		invalid := evidenceConfig(&config.EvidenceCheckConfig{Name: "x"})
		gate := newEvidenceGate(invalid, store, nil, log)
		Expect(gate.configErr).To(HaveOccurred())

		errs := gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo),
			nil,
		)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].UnavailableReason).To(Equal(validator.ReasonConfig))
		Expect(errs[0].Message).To(ContainSubstring("compile the configured evidence checks"))
	})

	It("loads a repository's checks from its configuration", func() {
		isolateHome()

		other := evidenceRepo()
		configDir := filepath.Join(other, ".klaudiush")
		Expect(os.MkdirAll(configDir, 0o700)).To(Succeed())

		configFile := filepath.Join(configDir, "config.toml")
		write := func(content string) {
			Expect(os.WriteFile(configFile, []byte(content), 0o600)).To(Succeed())
		}

		checks, err := repoChecks(log, other)
		Expect(err).NotTo(HaveOccurred())
		Expect(checks).To(BeEmpty())

		write("[evidence]\nenabled = true\n\n[[evidence.checks]]\n" +
			"name = \"tests\"\ncommands = [\"make test\"]\n")

		checks, err = repoChecks(log, other)
		Expect(err).NotTo(HaveOccurred())
		Expect(checks).To(HaveLen(1))

		write("[evidence\nenabled = true\n")

		_, err = repoChecks(log, other)
		Expect(err).To(HaveOccurred())
	})

	It("does not gate a read-only session on files it cannot fingerprint", func() {
		if os.Geteuid() == 0 {
			Skip("root reads every file")
		}

		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		unreadable := filepath.Join(repo, "a.go")
		Expect(os.Chmod(unreadable, 0o000)).To(Succeed())
		DeferCleanup(os.Chmod, unreadable, os.FileMode(0o644))

		read := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)
		read.ToolName, read.ToolFamily = hook.ToolTypeRead, hook.ToolFamilyRead
		gate.apply(context.Background(), read, nil)

		stop := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo)
		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())

		Expect(os.Chmod(unreadable, 0o644)).To(Succeed())
		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())
	})

	It("ignores hooks without a session or outside a repository", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		Expect(gate).NotTo(BeNil())

		stop := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", GinkgoT().TempDir())
		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())

		stop.SessionID = ""
		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())
		Expect(gate.apply(context.Background(), nil, nil)).To(BeEmpty())
	})

	It("does not record runs klaudiush blocked or approval prompts", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		pre := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)

		blocked := []*dispatcher.ValidationError{{ShouldBlock: true, Message: "no"}}
		gate.apply(context.Background(), pre, blocked)

		permission := evidenceCtx(hook.CanonicalEventBeforeTool, "PermissionRequest", repo)
		gate.apply(context.Background(), permission, nil)

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipts).To(BeEmpty())
	})

	It("ignores a result for a run it did not see start", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		post := evidenceCtx(hook.CanonicalEventAfterTool, "PostToolUse", repo)
		post.ToolSucceeded = true

		gate.apply(context.Background(), post, nil)

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipts).To(BeEmpty())
	})

	It("ignores a result for another command under the same tool use ID", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo),
			nil,
		)

		post := evidenceCtx(hook.CanonicalEventAfterTool, "PostToolUse", repo)
		post.ToolInput.Command = "echo PASS"
		post.ToolSucceeded = true
		gate.apply(context.Background(), post, nil)

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipts["tests"].Status).To(Equal(evidence.StatusRunning))
	})

	It("leaves a run alone when the check definition changed", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo),
			nil,
		)

		changed := newEvidenceGate(evidenceConfig(&config.EvidenceCheckConfig{
			Name:     "tests",
			Commands: []string{"make test"},
			Paths:    []string{"*.go"},
		}), store, nil, log)
		post := evidenceCtx(hook.CanonicalEventAfterTool, "PostToolUse", repo)
		post.ToolSucceeded = true
		changed.apply(context.Background(), post, nil)

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipts["tests"].Status).To(Equal(evidence.StatusRunning))
	})

	It("lets a pass from another session on the same content count", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo),
			nil,
		)

		Expect(
			os.WriteFile(filepath.Join(repo, "a.go"), []byte("package b\n"), 0o600),
		).To(Succeed())

		stop := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo)
		Expect(gate.apply(context.Background(), stop, nil)).To(HaveLen(1))

		other := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)
		other.SessionID, other.ToolUseID = "s2", "t9"
		gate.apply(context.Background(), other, nil)

		post := evidenceCtx(hook.CanonicalEventAfterTool, "PostToolUse", repo)
		post.SessionID, post.ToolUseID, post.ToolSucceeded = "s2", "t9", true
		gate.apply(context.Background(), post, nil)

		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())
	})

	It("gates every repository the session edited, not only the one it stops in", func() {
		other := evidenceRepo()
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate.loadChecks = func(string) ([]*evidence.Check, error) { return gate.checks, nil }

		write := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)
		write.ToolName, write.ToolFamily = hook.ToolTypeWrite, hook.ToolFamilyWrite
		write.AffectedPaths = []string{filepath.Join(other, "a.go")}
		gate.apply(context.Background(), write, nil)

		Expect(os.WriteFile(filepath.Join(other, "a.go"), []byte("package b\n"), 0o600)).
			To(Succeed())

		errs := gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo),
			nil,
		)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Details[evidenceValidator]).To(ContainSubstring(other))

		gate.loadChecks = func(string) ([]*evidence.Check, error) { return nil, nil }
		Expect(
			gate.apply(
				context.Background(),
				evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo),
				nil,
			),
		).
			To(BeEmpty())
	})

	It("counts a non-zero exit Claude interpreted as success as a failure", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo),
			nil,
		)

		post := evidenceCtx(hook.CanonicalEventAfterTool, "PostToolUse", repo)
		post.ToolSucceeded, post.ToolExitNote = true, "Files differ"
		gate.apply(context.Background(), post, nil)

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipts["tests"].Status).To(Equal(evidence.StatusFailed))
		Expect(receipts["tests"].Detail).To(ContainSubstring("Files differ"))
	})

	It("keeps a pass when a later run of the same content never finishes", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo),
			nil,
		)

		Expect(os.WriteFile(filepath.Join(repo, "a.go"), []byte("package b\n"), 0o600)).
			To(Succeed())

		run := func(id string, finish func(*hook.Context)) {
			pre := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)
			pre.ToolUseID = id
			gate.apply(context.Background(), pre, nil)

			if finish != nil {
				post := evidenceCtx(hook.CanonicalEventAfterTool, "PostToolUse", repo)
				post.ToolUseID = id
				finish(post)
				gate.apply(context.Background(), post, nil)
			}
		}

		stop := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo)

		run("t1", func(post *hook.Context) { post.ToolSucceeded = true })
		run("t2", nil)
		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())

		run("t3", func(post *hook.Context) { post.ToolSucceeded = false })
		Expect(gate.apply(context.Background(), stop, nil)).To(HaveLen(1))
	})

	It("does not gate a read-only session on someone else's edits", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)

		read := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)
		read.ToolName, read.ToolFamily = hook.ToolTypeRead, hook.ToolFamilyRead
		gate.apply(context.Background(), read, nil)

		Expect(os.WriteFile(filepath.Join(repo, "a.go"), []byte("package b\n"), 0o600)).
			To(Succeed())

		stop := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo)
		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())

		gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo),
			nil,
		)
		Expect(gate.apply(context.Background(), stop, nil)).To(HaveLen(1))
	})

	It("requires a check whose baseline it could not fingerprint", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		hookCtx := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)
		scope := &repoScope{
			root:   repo,
			checks: gate.checks,
			snap:   &lazySnapshot{root: GinkgoT().TempDir()},
		}

		baselines, err := gate.ensureBaselines(context.Background(), hookCtx, scope)
		Expect(err).NotTo(HaveOccurred())
		Expect(baselines[gate.checks[0].ID()]).To(Equal(hooksession.BaselineUnknown))
	})

	It("requires a check whose definition changed during the session", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo),
			nil,
		)

		Expect(os.WriteFile(filepath.Join(repo, "a.go"), []byte("package b\n"), 0o600)).
			To(Succeed())

		changed := newEvidenceGate(evidenceConfig(&config.EvidenceCheckConfig{
			Name:     "tests",
			Commands: []string{"make test"},
			Exclude:  []string{"nothing/**"},
		}), store, nil, log)

		errs := changed.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo),
			nil,
		)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Reference).To(Equal(validator.RefEvidenceMissing))
	})

	It("reports evidence it cannot read according to the failure policy", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		err := os.ErrDeadlineExceeded

		warn := gate.unavailable("read things", err, false)
		Expect(warn.ShouldBlock).To(BeFalse())
		Expect(gate.unavailable("read things", err, true).ShouldBlock).To(BeTrue())
		Expect(warn.Unavailable).To(BeTrue())
		Expect(warn.Reference).To(Equal(validator.RefValidationUnavailable))
		Expect(warn.Message).To(ContainSubstring("could not read things"))

		gate.policy = failpolicy.New(&config.FailurePolicyConfig{Critical: []string{"evidence"}})
		Expect(gate.unavailable("read things", err, false).ShouldBlock).To(BeTrue())

		gate.policy = failpolicy.New(&config.FailurePolicyConfig{Mode: config.FailureModeWarn})
		Expect(gate.unavailable("read things", err, true).ShouldBlock).To(BeFalse())
	})

	It("reports a fingerprint failure at the gate", func() {
		review := &config.EvidenceCheckConfig{
			Name:     "review",
			Kind:     config.EvidenceKindReview,
			Commands: []string{"review"},
			Base:     "no-such-base",
		}
		gate := newEvidenceGate(evidenceConfig(review), store, nil, log)
		gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo),
			nil,
		)

		Expect(
			os.WriteFile(filepath.Join(repo, "a.go"), []byte("package b\n"), 0o600),
		).To(Succeed())

		errs := gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo),
			nil,
		)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Unavailable).To(BeTrue())
		Expect(errs[0].ShouldBlock).To(BeTrue())
		Expect(errs[0].Message).To(ContainSubstring("no-such-base"))
	})

	It("reports a work tree it cannot list at the gate", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		stop := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo)
		scope := &repoScope{
			root:   repo,
			checks: gate.checks,
			snap:   &lazySnapshot{root: GinkgoT().TempDir()},
		}

		errs := gate.verdicts(context.Background(), stop, scope,
			map[string]string{gate.checks[0].ID(): hooksession.BaselineUnknown})
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Unavailable).To(BeTrue())
	})

	It("formats repairs and file lists", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate.binary = "/opt/my tools/klaudiush"
		codex := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo)
		codex.Provider = hook.ProviderCodex

		repair := gate.repair(
			codex,
			repo,
			gate.checks[0],
			evidence.Verdict{Status: evidence.StatusMissing},
		)
		Expect(repair).To(HavePrefix("Run `'/opt/my tools/klaudiush' evidence run tests`"))
		Expect(repair).To(ContainSubstring("Codex does not tell hooks"))

		running := gate.repair(
			codex,
			repo,
			gate.checks[0],
			evidence.Verdict{Status: evidence.StatusRunning},
		)
		Expect(running).To(HavePrefix("Wait for the running check"))

		Expect(gate.repair(codex, repo, gate.checks[0], evidence.Verdict{Status: "other"})).
			To(HavePrefix("Run "))

		files := make([]string, 12)
		for i := range files {
			files[i] = "f.go"
		}

		Expect(listFiles(files)).To(HaveSuffix("and 2 more"))
		Expect(listFiles([]string{"a", "b"})).To(Equal("a, b"))
		Expect(providerTitle("")).To(Equal("this provider"))
		Expect(upperFirst("")).To(BeEmpty())
		Expect(klaudiushBinary()).NotTo(BeEmpty())
	})

	It("uses the process directory when the provider sends none", func() {
		Expect(evidenceWorkDir(&hook.Context{WorkingDir: "/x"})).To(Equal("/x"))

		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(evidenceWorkDir(&hook.Context{})).To(Equal(cwd))
	})
})

type fakeOptionsRunner struct {
	result kexec.CommandResult
	wait   bool
	opts   kexec.RunOptions
}

func (f *fakeOptionsRunner) RunWithOptions(
	ctx context.Context,
	opts kexec.RunOptions,
	_ string,
	_ ...string,
) kexec.CommandResult {
	f.opts = opts

	if f.wait {
		<-ctx.Done()
	}

	return f.result
}

var _ = Describe("checkVerifier", func() {
	var (
		repo  string
		store *hooksession.Store
		check *evidence.Check
	)

	BeforeEach(func() {
		repo = evidenceRepo()
		store = hooksession.NewStore(
			hooksession.WithStateFile(filepath.Join(GinkgoT().TempDir(), "state.json")),
		)

		checks, err := evidence.Compile(&config.EvidenceConfig{
			Checks: []*config.EvidenceCheckConfig{{
				Name:     "tests",
				Commands: []string{"make test"},
				Timeout:  config.Duration(50 * time.Millisecond),
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		check = checks[0]
	})

	verify := func(runner *fakeOptionsRunner) (int, *evidence.Receipt, string) {
		var notices bytes.Buffer

		verifier := &checkVerifier{
			store:  store,
			runner: runner,
			now:    time.Now,
			notify: func(format string, args ...any) {
				notices.WriteString(strings.TrimSpace(format))

				for range args {
					notices.WriteString(" ")
				}
			},
		}

		code, err := verifier.run(context.Background(), repo, check)
		Expect(err).NotTo(HaveOccurred())

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())

		return code, receipts["tests"], notices.String()
	}

	It("records a pass from the repository root", func() {
		runner := &fakeOptionsRunner{}
		code, receipt, _ := verify(runner)

		Expect(code).To(Equal(0))
		Expect(runner.opts.Dir).To(Equal(repo))
		Expect(receipt.Status).To(Equal(evidence.StatusPassed))
		Expect(receipt.Source).To(Equal(evidence.SourceVerifier))
		Expect(*receipt.ExitCode).To(Equal(0))
	})

	It("records a failure with the exit status", func() {
		runner := &fakeOptionsRunner{result: kexec.CommandResult{ExitCode: 4, Err: os.ErrInvalid}}
		code, receipt, _ := verify(runner)

		Expect(code).To(Equal(4))
		Expect(receipt.Status).To(Equal(evidence.StatusFailed))
	})

	It("records a command that could not start", func() {
		runner := &fakeOptionsRunner{result: kexec.CommandResult{Err: os.ErrNotExist}}
		code, receipt, _ := verify(runner)

		Expect(code).To(Equal(1))
		Expect(receipt.Status).To(Equal(evidence.StatusFailed))
		Expect(receipt.Detail).To(ContainSubstring("could not start"))
	})

	It("records a timeout as canceled", func() {
		runner := &fakeOptionsRunner{wait: true, result: kexec.CommandResult{Err: os.ErrClosed}}
		code, receipt, _ := verify(runner)

		Expect(code).To(Equal(1))
		Expect(receipt.Status).To(Equal(evidence.StatusCanceled))
		Expect(receipt.Detail).To(ContainSubstring("timed out"))
	})

	It("does not overwrite a newer run", func() {
		runner := &fakeOptionsRunner{}
		runner.result = kexec.CommandResult{}

		replacing := &replacingRunner{store: store, repo: repo}

		var notices bytes.Buffer

		verifier := &checkVerifier{
			store:  store,
			runner: replacing,
			now:    time.Now,
			notify: func(format string, _ ...any) { notices.WriteString(format) },
		}

		_, err := verifier.run(context.Background(), repo, check)
		Expect(err).NotTo(HaveOccurred())
		Expect(notices.String()).To(ContainSubstring("replaced this one"))

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipts["tests"].RunID).To(Equal("newer"))
	})

	It("refuses a work tree it cannot fingerprint", func() {
		verifier := &checkVerifier{store: store, runner: &fakeOptionsRunner{}, now: time.Now}

		_, err := verifier.run(context.Background(), GinkgoT().TempDir(), check)
		Expect(err).To(MatchError(ContainSubstring("failed to fingerprint")))
	})

	It("bounds the starting fingerprint by the check's timeout", func() {
		verifier := &checkVerifier{
			store:  store,
			runner: &fakeOptionsRunner{},
			now:    time.Now,
			fingerprint: func(ctx context.Context, _ string, _ *evidence.Check) (fingerprint, error) {
				_, ok := ctx.Deadline()
				Expect(ok).To(BeTrue())
				<-ctx.Done()

				return fingerprint{}, ctx.Err()
			},
		}

		_, err := verifier.run(context.Background(), repo, check)
		Expect(err).To(MatchError(context.DeadlineExceeded))
	})

	It("gives the end fingerprint its own deadline after a timeout", func() {
		calls := 0

		var notices bytes.Buffer

		verifier := &checkVerifier{
			store:   store,
			runner:  &fakeOptionsRunner{wait: true, result: kexec.CommandResult{Err: os.ErrClosed}},
			now:     time.Now,
			cleanup: 20 * time.Millisecond,
			notify:  func(format string, _ ...any) { notices.WriteString(format) },
			fingerprint: func(
				ctx context.Context,
				root string,
				item *evidence.Check,
			) (fingerprint, error) {
				calls++
				if calls == 1 {
					return worktreeFingerprint(ctx, root, item)
				}

				Expect(ctx.Err()).NotTo(HaveOccurred())

				deadline, ok := ctx.Deadline()
				Expect(ok).To(BeTrue())
				Expect(time.Until(deadline)).To(BeNumerically("<=", 20*time.Millisecond))
				<-ctx.Done()

				return fingerprint{}, ctx.Err()
			},
		}

		code, err := verifier.run(context.Background(), repo, check)
		Expect(err).NotTo(HaveOccurred())
		Expect(code).To(Equal(1))
		Expect(calls).To(Equal(2))

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipts["tests"].Status).To(Equal(evidence.StatusUnverified))
		Expect(receipts["tests"].Detail).To(ContainSubstring("after the run"))
	})

	It("shows the kept pass that satisfies the gate in the status", func() {
		checks := []*evidence.Check{check}

		_, _, _ = verify(&fakeOptionsRunner{})

		current, err := worktreeFingerprint(context.Background(), repo, check)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.PutReceipt(repo, &evidence.Receipt{
			RunID:     "later",
			CheckID:   check.ID(),
			Check:     check.Name,
			Kind:      check.Kind,
			Status:    evidence.StatusRunning,
			Digest:    current.digest,
			Source:    string(hook.ProviderClaude),
			Command:   check.RunCommand(),
			StartedAt: time.Now().Add(-time.Hour),
		})).To(Succeed())

		var out bytes.Buffer

		Expect(printCheckStatus(context.Background(), bufferPrintf(&out), store, repo, checks)).
			To(Succeed())
		Expect(out.String()).To(ContainSubstring("verdict: passed"))
		Expect(out.String()).To(ContainSubstring("latest:  tests running"))
		Expect(out.String()).To(ContainSubstring("kept:    tests passed"))

		Expect(store.PutReceipt(repo, &evidence.Receipt{
			RunID:   "failed",
			CheckID: check.ID(),
			Check:   check.Name,
			Status:  evidence.StatusRunning,
			Digest:  current.digest,
		})).To(Succeed())

		_, err = store.FinishReceipt(repo, check.Name,
			func(stored *evidence.Receipt) bool { return stored.RunID == "failed" },
			func(stored *evidence.Receipt) {
				code := 2
				stored.Finish(evidence.StatusFailed, &code, current.digest, "", time.Now())
			},
		)
		Expect(err).NotTo(HaveOccurred())

		out.Reset()
		Expect(printCheckStatus(context.Background(), bufferPrintf(&out), store, repo, checks)).
			To(Succeed())
		Expect(out.String()).To(ContainSubstring("verdict: failed"))
		Expect(out.String()).NotTo(ContainSubstring("kept:"))
	})

	It("reports a work tree it cannot fingerprint in the status", func() {
		var out bytes.Buffer

		checks := []*evidence.Check{check}

		dir := GinkgoT().TempDir()
		Expect(printCheckStatus(context.Background(), bufferPrintf(&out), store, dir, checks)).
			To(Succeed())
		Expect(out.String()).To(ContainSubstring("current: unavailable"))
	})

	It("passes exit codes through", func() {
		code, ok := commandExitCode(&exitCodeError{code: 7})
		Expect(ok).To(BeTrue())
		Expect(code).To(Equal(7))
		Expect((&exitCodeError{code: 7}).Error()).To(ContainSubstring("7"))

		_, ok = commandExitCode(os.ErrInvalid)
		Expect(ok).To(BeFalse())

		Expect(checkNames(nil)).To(Equal("none"))
	})
})

func bufferPrintf(out *bytes.Buffer) func(string, ...any) {
	return func(format string, args ...any) { fmt.Fprintf(out, format, args...) }
}

type replacingRunner struct {
	store *hooksession.Store
	repo  string
}

func (r *replacingRunner) RunWithOptions(
	context.Context,
	kexec.RunOptions,
	string,
	...string,
) kexec.CommandResult {
	Expect(r.store.PutReceipt(r.repo, &evidence.Receipt{RunID: "newer", Check: "tests"})).
		To(Succeed())

	return kexec.CommandResult{}
}
