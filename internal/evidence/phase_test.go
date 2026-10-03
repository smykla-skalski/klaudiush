package evidence_test

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func phaseConfig(phase *config.EvidenceToolPhaseConfig) *config.EvidenceConfig {
	enabled := true

	return &config.EvidenceConfig{
		Enabled: &enabled,
		Checks: []*config.EvidenceCheckConfig{
			{Name: "plan", Commands: []string{"test -s PLAN.md"}, Paths: []string{"PLAN.md"}},
			{Name: "tests", Commands: []string{"make test"}},
		},
		ToolPhase: phase,
	}
}

func enabledPhase(phase config.EvidenceToolPhaseConfig) *config.EvidenceToolPhaseConfig {
	enabled := true
	phase.Enabled = &enabled

	return &phase
}

func compilePhase(cfg *config.EvidenceConfig) (*evidence.Phase, error) {
	checks, err := evidence.Compile(cfg)
	Expect(err).NotTo(HaveOccurred())

	return evidence.CompilePhase(cfg, checks)
}

var _ = Describe("CompilePhase", func() {
	It("reports a disabled phase", func() {
		for _, cfg := range []*config.EvidenceConfig{
			nil,
			{},
			phaseConfig(nil),
			phaseConfig(&config.EvidenceToolPhaseConfig{Requires: []string{"plan"}}),
		} {
			_, err := evidence.CompilePhase(cfg, nil)
			Expect(errors.Is(err, evidence.ErrPhaseDisabled)).To(BeTrue())
		}
	})

	DescribeTable("rejects unusable configurations",
		func(mutate func(*config.EvidenceConfig), want string) {
			cfg := phaseConfig(enabledPhase(config.EvidenceToolPhaseConfig{
				Requires: []string{"plan"},
			}))
			mutate(cfg)

			_, err := compilePhase(cfg)
			Expect(errors.Is(err, evidence.ErrInvalidPhase)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring(want))
		},
		Entry("evidence gate off", func(cfg *config.EvidenceConfig) {
			disabled := false
			cfg.Enabled = &disabled
		}, "needs evidence.enabled"),
		Entry("no prerequisites", func(cfg *config.EvidenceConfig) {
			cfg.ToolPhase.Requires = nil
		}, "must name at least one check"),
		Entry("unknown prerequisite", func(cfg *config.EvidenceConfig) {
			cfg.ToolPhase.Requires = []string{"nope"}
		}, `"nope", which is not a configured check`),
		Entry("empty read-only tool", func(cfg *config.EvidenceConfig) {
			cfg.ToolPhase.ReadOnlyTools = []string{" "}
		}, "empty name"),
		Entry("mutation tool listed as read-only", func(cfg *config.EvidenceConfig) {
			cfg.ToolPhase.ReadOnlyTools = []string{"read_file", "write_file"}
		}, `"write_file", which changes files`),
		Entry("absolute writable path", func(cfg *config.EvidenceConfig) {
			cfg.ToolPhase.WritablePaths = []string{"/etc/passwd"}
		}, "invalid pattern"),
		Entry("invalid writable pattern", func(cfg *config.EvidenceConfig) {
			cfg.ToolPhase.WritablePaths = []string{"docs/[a"}
		}, "invalid pattern"),
	)

	It("compiles prerequisites once each, with the default read-only tools", func() {
		phase, err := compilePhase(phaseConfig(enabledPhase(config.EvidenceToolPhaseConfig{
			Requires: []string{"plan", "tests", "plan"},
		})))
		Expect(err).NotTo(HaveOccurred())
		Expect(phase.RequiredNames()).To(Equal([]string{"plan", "tests"}))
		Expect(phase.ReadOnlyTools).To(Equal(config.DefaultToolPhaseReadOnlyTools))
		Expect(phase.AllowsReadOnly("read_file")).To(BeTrue())
		Expect(phase.AllowsReadOnly("write_file")).To(BeFalse())
	})
})

var _ = Describe("Phase", func() {
	Describe("AllowedTools", func() {
		It("offers the shell for the verifier but no file tools without writable paths", func() {
			phase := &evidence.Phase{ReadOnlyTools: []string{"read_file", "glob", "read_file"}}
			Expect(
				phase.AllowedTools(),
			).To(Equal([]string{"glob", "read_file", "run_shell_command"}))
		})

		It("offers the file tools when some paths stay writable", func() {
			phase := &evidence.Phase{
				ReadOnlyTools: []string{"read_file"},
				WritablePaths: []string{"*.md"},
			}
			Expect(phase.AllowedTools()).To(Equal(
				[]string{"read_file", "replace", "run_shell_command", "write_file"},
			))
		})
	})

	Describe("Writable", func() {
		var (
			repo  string
			phase *evidence.Phase
		)

		BeforeEach(func() {
			var err error

			repo, err = filepath.EvalSymlinks(GinkgoT().TempDir())
			Expect(err).NotTo(HaveOccurred())

			phase = &evidence.Phase{WritablePaths: []string{"docs/plans/**", "PLAN.md"}}
		})

		It("accepts only paths inside the repository matching a pattern", func() {
			Expect(phase.Writable(repo, filepath.Join(repo, "PLAN.md"))).To(BeTrue())
			Expect(phase.Writable(repo, filepath.Join(repo, "docs", "plans", "a", "b.md"))).
				To(BeTrue())
			Expect(phase.Writable(repo, filepath.Join(repo, "main.go"))).To(BeFalse())
			Expect(phase.Writable(repo, filepath.Join(repo, "docs", "plans", "..", "x.go"))).
				To(BeFalse())
			Expect(phase.Writable(repo, filepath.Join(filepath.Dir(repo), "PLAN.md"))).To(BeFalse())
			Expect(phase.Writable(repo, repo)).To(BeFalse())
			Expect(phase.Writable(repo, "")).To(BeFalse())
			Expect(phase.Writable("", filepath.Join(repo, "PLAN.md"))).To(BeFalse())
			Expect((&evidence.Phase{}).Writable(repo, filepath.Join(repo, "PLAN.md"))).To(BeFalse())
		})

		It("never opens klaudiush or git state", func() {
			phase.WritablePaths = []string{"**"}

			Expect(
				phase.Writable(repo, filepath.Join(repo, ".klaudiush", "config.toml")),
			).To(BeFalse())
			Expect(phase.Writable(repo, filepath.Join(repo, ".git", "hooks", "pre-commit"))).
				To(BeFalse())
			Expect(phase.Writable(repo, filepath.Join(repo, "src", "a.go"))).To(BeTrue())
		})

		It("follows symbolic links out of a writable directory", func() {
			if runtime.GOOS == "windows" {
				Skip("symbolic links need privileges on Windows")
			}

			Expect(os.MkdirAll(filepath.Join(repo, "docs"), 0o750)).To(Succeed())
			Expect(os.Symlink(filepath.Join(repo, "src"), filepath.Join(repo, "docs", "plans"))).
				To(Succeed())
			Expect(os.MkdirAll(filepath.Join(repo, "src"), 0o750)).To(Succeed())

			Expect(
				phase.Writable(repo, filepath.Join(repo, "docs", "plans", "main.go")),
			).To(BeFalse())
		})
	})

	Describe("AllowsVerifier", func() {
		var (
			binary string
			phase  *evidence.Phase
		)

		BeforeEach(func() {
			dir, err := filepath.EvalSymlinks(GinkgoT().TempDir())
			Expect(err).NotTo(HaveOccurred())

			binary = filepath.Join(dir, "klaudiush")
			Expect(os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700)).To(Succeed())
			GinkgoT().Setenv("PATH", dir)

			phase, err = compilePhase(phaseConfig(enabledPhase(config.EvidenceToolPhaseConfig{
				Requires: []string{"plan"},
			})))
			Expect(err).NotTo(HaveOccurred())
		})

		DescribeTable(
			"accepts only a plain verifier run of a prerequisite",
			func(command func(binary string) string, allowed bool) {
				Expect(phase.AllowsVerifier(command(binary), binary)).To(Equal(allowed))
			},
			Entry("absolute path", func(b string) string { return b + " evidence run plan" }, true),
			Entry(
				"name on PATH",
				func(string) string { return "klaudiush evidence run plan" },
				true,
			),
			Entry("status", func(string) string { return "klaudiush evidence status" }, true),
			Entry(
				"after cd",
				func(b string) string { return "cd /repo && " + b + " evidence run plan" },
				true,
			),
			Entry("check that is no prerequisite",
				func(string) string { return "klaudiush evidence run tests" }, false),
			Entry("extra argument",
				func(string) string { return "klaudiush evidence run plan x" }, false),
			Entry(
				"other subcommand",
				func(string) string { return "klaudiush init --force" },
				false,
			),
			Entry("no subcommand", func(string) string { return "klaudiush" }, false),
			Entry(
				"relative path",
				func(string) string { return "./klaudiush evidence run plan" },
				false,
			),
			Entry("output redirect",
				func(string) string { return "klaudiush evidence run plan > main.go" }, false),
			Entry("chained command",
				func(string) string { return "klaudiush evidence run plan && rm main.go" }, false),
			Entry("cd with extra words",
				func(string) string { return "cd a b && klaudiush evidence run plan" }, false),
			Entry("pipe", func(string) string { return "klaudiush evidence status | sh" }, false),
			Entry("substitution",
				func(string) string { return "klaudiush evidence run $(rm main.go)" }, false),
			Entry(
				"unparsable",
				func(string) string { return "klaudiush evidence run 'plan" },
				false,
			),
			Entry("other program",
				func(string) string { return "/bin/sh evidence run plan" }, false),
			Entry("missing program",
				func(string) string { return "nope-q7zx evidence run plan" }, false),
		)

		It("refuses when the running binary is unknown or missing", func() {
			Expect(phase.AllowsVerifier("klaudiush evidence status", "")).To(BeFalse())
			Expect(phase.AllowsVerifier("klaudiush evidence status", binary+".gone")).To(BeFalse())
		})
	})
})

var _ = Describe("PhaseCoverage", func() {
	It("restricts only providers with a tool-selection event", func() {
		Expect(
			evidence.PhaseCoverage(hook.ProviderGemini),
		).To(ContainSubstring("gemini: restricted"))
		Expect(evidence.PhaseCoverage(hook.ProviderClaude)).
			To(ContainSubstring("claude: not restricted, it has no tool-selection event"))
		Expect(evidence.PhaseCoverageLines()).To(HaveLen(len(evidence.Providers)))
	})
})
