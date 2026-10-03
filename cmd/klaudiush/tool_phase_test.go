package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

func toolPhaseConfig(writable ...string) *config.Config {
	enabled := true

	cfg := evidenceConfig(&config.EvidenceCheckConfig{
		Name:     "plan",
		Commands: []string{"test -s PLAN.md"},
		Paths:    []string{"PLAN.md"},
	})
	cfg.Evidence.ToolPhase = &config.EvidenceToolPhaseConfig{
		Enabled:       &enabled,
		Requires:      []string{"plan"},
		WritablePaths: writable,
	}

	return cfg
}

func geminiCtx(event hook.CanonicalEvent, repo, tool string) *hook.Context {
	toolType, family := hook.ResolveToolMetadata(tool)

	return &hook.Context{
		Provider:     hook.ProviderGemini,
		Event:        event,
		RawEventName: hook.DisplayEventName(hook.ProviderGemini, event, hook.EventTypeUnknown),
		SessionID:    "g1",
		WorkingDir:   repo,
		RawToolName:  tool,
		ToolName:     toolType,
		ToolFamily:   family,
	}
}

var _ = Describe("toolPhase", func() {
	var (
		repo  string
		store *hooksession.Store
		log   logger.Logger
		ctx   context.Context
	)

	BeforeEach(func() {
		repo = evidenceRepo()
		store = hooksession.NewStore(
			hooksession.WithStateFile(filepath.Join(GinkgoT().TempDir(), "state.json")),
		)
		log = logger.NewNoOpLogger()
		ctx = context.Background()
	})

	passPlan := func(gate *evidenceGate) {
		Expect(os.WriteFile(filepath.Join(repo, "PLAN.md"), []byte("plan\n"), 0o600)).To(Succeed())

		check := gate.phase.phase.Requires[0]
		fp, err := checkFingerprint(ctx, &lazySnapshot{root: repo}, check)
		Expect(err).NotTo(HaveOccurred())

		receipt := &evidence.Receipt{
			RunID: evidence.NewRunID(), CheckID: check.ID(), Check: check.Name, Kind: check.Kind,
			Status: evidence.StatusRunning, Source: evidence.SourceVerifier, Digest: fp.digest,
			StartedAt: time.Now(),
		}
		Expect(store.PutReceipt(repo, receipt)).To(Succeed())

		_, err = store.FinishReceipt(repo, check.Name,
			func(r *evidence.Receipt) bool { return r.RunID == receipt.RunID },
			func(r *evidence.Receipt) {
				r.Finish(evidence.StatusPassed, nil, fp.digest, "", time.Now())
			})
		Expect(err).NotTo(HaveOccurred())
	}

	It("exists only when the configuration enables it", func() {
		Expect(newEvidenceGate(evidenceConfig(), store, nil, log).toolPhase()).To(BeNil())
		Expect(newToolPhase(toolPhaseConfig(), nil)).To(BeNil())
		Expect(newToolPhase(nil, &evidenceGate{})).To(BeNil())

		var gate *evidenceGate
		Expect(gate.toolPhase()).To(BeNil())

		var phase *toolPhase

		errs := []*dispatcher.ValidationError{{Message: "kept"}}
		Expect(
			phase.apply(ctx, geminiCtx(hook.CanonicalEventBeforeTool, repo, "write_file"), errs),
		).
			To(Equal(errs))
		Expect(phase.selection(ctx, geminiCtx(hook.CanonicalEventToolSelection, repo, ""))).
			To(BeNil())
	})

	It("offers only the phase's tools until the prerequisite passes", func() {
		gate := newEvidenceGate(toolPhaseConfig("PLAN.md"), store, nil, log)
		phase := gate.toolPhase()
		Expect(phase).NotTo(BeNil())

		selection := phase.selection(ctx, geminiCtx(hook.CanonicalEventToolSelection, repo, ""))
		resp, ok := selection.(*hookresponse.GeminiCommandResponse)
		Expect(ok).To(BeTrue())
		Expect(resp.HookSpecificOutput.HookEventName).To(Equal("BeforeToolSelection"))
		Expect(resp.HookSpecificOutput.ToolConfig.AllowedFunctionNames).
			To(ContainElements("read_file", "run_shell_command", "write_file", "replace"))

		passPlan(gate)
		Expect(phase.selection(ctx, geminiCtx(hook.CanonicalEventToolSelection, repo, ""))).
			To(BeNil())

		Expect(os.WriteFile(filepath.Join(repo, "PLAN.md"), []byte("other\n"), 0o600)).To(Succeed())
		Expect(phase.selection(ctx, geminiCtx(hook.CanonicalEventToolSelection, repo, ""))).
			NotTo(BeNil())
	})

	It("leaves tool selection alone when filtering is off, still denying per call", func() {
		cfg := toolPhaseConfig()
		off := false
		cfg.Evidence.ToolPhase.FilterTools = &off

		phase := newEvidenceGate(cfg, store, nil, log).toolPhase()
		Expect(phase.selection(ctx, geminiCtx(hook.CanonicalEventToolSelection, repo, ""))).
			To(BeNil())
		Expect(phase.apply(ctx, geminiCtx(hook.CanonicalEventBeforeTool, repo, "write_file"), nil)).
			To(HaveLen(1))
	})

	It("answers only Gemini tool selection", func() {
		phase := newEvidenceGate(toolPhaseConfig(), store, nil, log).toolPhase()

		claude := geminiCtx(hook.CanonicalEventToolSelection, repo, "")
		claude.Provider = hook.ProviderClaude
		Expect(phase.selection(ctx, claude)).To(BeNil())
		Expect(phase.selection(ctx, geminiCtx(hook.CanonicalEventBeforeTool, repo, ""))).To(BeNil())
	})

	It("denies withheld calls per call and lets permitted ones through", func() {
		gate := newEvidenceGate(toolPhaseConfig("PLAN.md"), store, nil, log)
		phase := gate.toolPhase()

		write := geminiCtx(hook.CanonicalEventBeforeTool, repo, "write_file")
		write.AffectedPaths = []string{filepath.Join(repo, "a.go")}

		errs := phase.apply(ctx, write, nil)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].ShouldBlock).To(BeTrue())
		Expect(errs[0].Reference).To(Equal(validator.RefToolPhaseLocked))
		Expect(errs[0].Message).To(ContainSubstring(`Tool "write_file" is withheld for now`))
		Expect(errs[0].Findings[0].Repair).To(ContainSubstring("evidence run plan"))
		Expect(errs[0].Findings[0].Repair).To(ContainSubstring("from " + repo))
		Expect(errs[0].Findings[0].Repair).To(ContainSubstring("may change only PLAN.md"))

		plan := geminiCtx(hook.CanonicalEventBeforeTool, repo, "replace")
		plan.ToolInput.FilePath = "PLAN.md"
		Expect(phase.apply(ctx, plan, nil)).To(BeEmpty())

		noPath := geminiCtx(hook.CanonicalEventBeforeTool, repo, "write_file")
		Expect(phase.apply(ctx, noPath, nil)).To(HaveLen(1))

		read := geminiCtx(hook.CanonicalEventBeforeTool, repo, "read_file")
		Expect(phase.apply(ctx, read, nil)).To(BeEmpty())

		mcp := geminiCtx(hook.CanonicalEventBeforeTool, repo, "mcp_files_write")
		Expect(phase.apply(ctx, mcp, nil)).To(HaveLen(1))

		shell := geminiCtx(hook.CanonicalEventBeforeTool, repo, "run_shell_command")
		shell.ToolInput.Command = "rm a.go"
		Expect(phase.apply(ctx, shell, nil)).To(HaveLen(1))

		shell.ToolInput.Command = shellQuote(gate.binary) + " evidence run plan"
		Expect(phase.apply(ctx, shell, nil)).To(BeEmpty())

		for _, dir := range []string{`"."`, `""`} {
			shell.ToolInput.Additional = map[string]json.RawMessage{
				"dir_path": json.RawMessage(dir),
			}
			Expect(phase.apply(ctx, shell, nil)).To(BeEmpty())
		}

		for _, dir := range []string{`"docs"`, `42`} {
			shell.ToolInput.Additional = map[string]json.RawMessage{
				"dir_path": json.RawMessage(dir),
			}
			Expect(phase.apply(ctx, shell, nil)).To(HaveLen(1))
		}

		claude := geminiCtx(hook.CanonicalEventBeforeTool, repo, "write_file")
		claude.Provider = hook.ProviderClaude
		Expect(phase.apply(ctx, claude, nil)).To(BeEmpty())

		passPlan(gate)
		Expect(phase.apply(ctx, write, nil)).To(BeEmpty())
	})

	It("follows the failure policy when git is missing", func() {
		GinkgoT().Setenv("PATH", GinkgoT().TempDir())

		phase := newEvidenceGate(toolPhaseConfig(), store, nil, log).toolPhase()
		errs := phase.apply(ctx, geminiCtx(hook.CanonicalEventBeforeTool, repo, "write_file"), nil)
		Expect(errs).To(HaveLen(2))
		Expect(errs[0].Message).To(ContainSubstring("find git"))
	})

	It("leaves the phase open outside a repository", func() {
		phase := newEvidenceGate(toolPhaseConfig(), store, nil, log).toolPhase()

		outside := GinkgoT().TempDir()
		Expect(phase.selection(ctx, geminiCtx(hook.CanonicalEventToolSelection, outside, ""))).
			To(BeNil())
		Expect(
			phase.apply(ctx, geminiCtx(hook.CanonicalEventBeforeTool, outside, "write_file"), nil),
		).
			To(BeEmpty())
	})

	It("follows the failure policy when git cannot read the repository", func() {
		broken := GinkgoT().TempDir()
		Expect(
			os.WriteFile(filepath.Join(broken, ".git"), []byte("gitdir: /nonexistent\n"), 0o600),
		).
			To(Succeed())

		phase := newEvidenceGate(toolPhaseConfig(), store, nil, log).toolPhase()
		Expect(phase.selection(ctx, geminiCtx(hook.CanonicalEventToolSelection, broken, ""))).
			NotTo(BeNil())

		errs := phase.apply(
			ctx,
			geminiCtx(hook.CanonicalEventBeforeTool, broken, "write_file"),
			nil,
		)
		Expect(errs).To(HaveLen(2))
		Expect(errs[0].Unavailable).To(BeTrue())
		Expect(errs[0].Message).To(ContainSubstring("find the git repository"))
		Expect(errs[1].Reference).To(Equal(validator.RefToolPhaseLocked))

		warn := failpolicy.New(&config.FailurePolicyConfig{Mode: config.FailureModeWarn})
		open := newEvidenceGate(toolPhaseConfig(), store, warn, log).toolPhase()

		errs = open.apply(ctx, geminiCtx(hook.CanonicalEventBeforeTool, broken, "write_file"), nil)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].ShouldBlock).To(BeFalse())
	})

	It("restricts with read-only tools when the phase does not compile", func() {
		cfg := toolPhaseConfig()
		cfg.Evidence.ToolPhase.Requires = []string{"missing"}

		phase := newEvidenceGate(cfg, store, nil, log).toolPhase()
		Expect(phase.err).To(HaveOccurred())

		selection := phase.selection(ctx, geminiCtx(hook.CanonicalEventToolSelection, repo, ""))
		Expect(selection).NotTo(BeNil())

		errs := phase.apply(ctx, geminiCtx(hook.CanonicalEventBeforeTool, repo, "write_file"), nil)
		Expect(errs).To(HaveLen(2))
		Expect(errs[0].Reference).To(Equal(validator.RefValidationUnavailable))
		Expect(errs[1].Message).To(ContainSubstring("could not judge the evidence tool phase"))
		Expect(errs[1].Findings[0].Repair).To(ContainSubstring("Fix the evidence tool phase"))

		Expect(phase.apply(ctx, geminiCtx(hook.CanonicalEventBeforeTool, repo, "read_file"), nil)).
			To(BeEmpty())

		cfg.Evidence.Checks[0].Commands = nil
		Expect(newEvidenceGate(cfg, store, nil, log).toolPhase().err).To(HaveOccurred())
	})

	It("follows the failure policy when check results cannot be read", func() {
		blocker := filepath.Join(GinkgoT().TempDir(), "file")
		Expect(os.WriteFile(blocker, nil, 0o600)).To(Succeed())

		broken := hooksession.NewStore(
			hooksession.WithStateFile(filepath.Join(blocker, "state.json")),
		)

		phase := newEvidenceGate(toolPhaseConfig(), broken, nil, log).toolPhase()
		errs := phase.apply(ctx, geminiCtx(hook.CanonicalEventBeforeTool, repo, "write_file"), nil)
		Expect(errs).To(HaveLen(2))
		Expect(errs[0].Unavailable).To(BeTrue())
		Expect(errs[0].ShouldBlock).To(BeTrue())

		warn := failpolicy.New(&config.FailurePolicyConfig{Mode: config.FailureModeWarn})
		open := newEvidenceGate(toolPhaseConfig(), broken, warn, log).toolPhase()

		errs = open.apply(ctx, geminiCtx(hook.CanonicalEventBeforeTool, repo, "write_file"), nil)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].ShouldBlock).To(BeFalse())
		Expect(open.selection(ctx, geminiCtx(hook.CanonicalEventToolSelection, repo, ""))).
			To(BeNil())
	})

	It("restricts when the prerequisite's files cannot be fingerprinted", func() {
		if os.Geteuid() == 0 {
			Skip("root reads every file")
		}

		plan := filepath.Join(repo, "PLAN.md")
		Expect(os.WriteFile(plan, []byte("plan\n"), 0o600)).To(Succeed())
		Expect(os.Chmod(plan, 0o000)).To(Succeed())
		DeferCleanup(os.Chmod, plan, os.FileMode(0o600))

		phase := newEvidenceGate(toolPhaseConfig(), store, nil, log).toolPhase()

		st := phase.state(ctx, geminiCtx(hook.CanonicalEventBeforeTool, repo, "write_file"))
		Expect(st.restricted).To(BeTrue())
		Expect(st.unavailable).NotTo(BeNil())
		Expect(st.unmet).To(BeEmpty())

		read := geminiCtx(hook.CanonicalEventBeforeTool, repo, "read_file")
		Expect(phase.apply(ctx, read, nil)).To(BeEmpty())
	})
})
