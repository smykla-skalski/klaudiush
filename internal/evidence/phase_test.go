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

		It("never opens configuration or the files a prerequisite runs", func() {
			phase.WritablePaths = []string{"**"}
			phase.Requires = []*evidence.Check{compileOne(&config.EvidenceCheckConfig{
				Name: "plan", Commands: []string{
					"sh -e scripts/check.sh PLAN.md /abs/x", "./bin/verify PLAN.md",
				},
			})}

			for _, rel := range []string{
				"docs/.klaudiush/config.toml",
				"docs/.Klaudiush/config.toml",
				"docs/klaudiush.toml",
				"KLAUDIUSH.TOML",
				".gemini/settings.json",
				"sub/.claude/settings.json",
				".codex/hooks.json",
				".mcp.json",
				"scripts/check.sh",
				"bin/verify",
			} {
				Expect(phase.Writable(repo, filepath.Join(repo, rel))).To(BeFalse(), rel)
			}

			Expect(phase.Writable(repo, filepath.Join(repo, "scripts", "other.sh"))).To(BeTrue())
			Expect(phase.Writable(repo, filepath.Join(repo, "PLAN.md"))).To(BeTrue())
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

		It("refuses a dangling symbolic link", func() {
			if runtime.GOOS == "windows" {
				Skip("symbolic links need privileges on Windows")
			}

			Expect(
				os.Symlink(filepath.Join(repo, "src", "new.go"), filepath.Join(repo, "PLAN.md")),
			).
				To(Succeed())
			Expect(phase.Writable(repo, filepath.Join(repo, "PLAN.md"))).To(BeFalse())
		})

		It("refuses a dangling symbolic link above the target", func() {
			if runtime.GOOS == "windows" {
				Skip("symbolic links need privileges on Windows")
			}

			Expect(os.MkdirAll(filepath.Join(repo, "docs"), 0o750)).To(Succeed())
			Expect(
				os.Symlink(filepath.Join(repo, "src", "new"), filepath.Join(repo, "docs", "plans")),
			).
				To(Succeed())

			Expect(phase.Writable(repo, filepath.Join(repo, "docs", "plans", "file.md"))).
				To(BeFalse())
			Expect(phase.Writable(repo, filepath.Join(repo, "docs", "plans", "a", "b.md"))).
				To(BeFalse())
		})

		It("refuses a path whose components cannot be checked", func() {
			Expect(os.WriteFile(filepath.Join(repo, "PLAN.md"), nil, 0o600)).To(Succeed())

			Expect(phase.Writable(repo, filepath.Join(repo, "PLAN.md", "x"))).To(BeFalse())
			Expect(phase.Writable(repo, "PLAN.md")).To(BeFalse())
		})

		It("checks the path as written and as resolved", func() {
			if runtime.GOOS == "windows" {
				Skip("symbolic links need privileges on Windows")
			}

			Expect(os.MkdirAll(filepath.Join(repo, "docs", "plans"), 0o750)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(repo, "docs", "plans", "a.md"), nil, 0o600)).
				To(Succeed())
			Expect(
				os.Symlink(
					filepath.Join(repo, "docs", "plans", "a.md"),
					filepath.Join(repo, "src.md"),
				),
			).
				To(Succeed())

			Expect(phase.Writable(repo, filepath.Join(repo, "src.md"))).To(BeFalse())
			Expect(phase.Writable(repo, filepath.Join(repo, "docs", "plans", "a.md"))).To(BeTrue())

			link := filepath.Join(GinkgoT().TempDir(), "repo")
			Expect(os.Symlink(repo, link)).To(Succeed())
			Expect(phase.Writable(link, filepath.Join(link, "docs", "plans", "a.md"))).To(BeTrue())
			Expect(phase.Writable(link, filepath.Join(repo, "docs", "plans", "a.md"))).To(BeTrue())
		})

		Describe("aliases of protected files", func() {
			BeforeEach(func() {
				if runtime.GOOS == "windows" {
					Skip("symbolic links need privileges on Windows")
				}

				Expect(os.MkdirAll(filepath.Join(repo, "docs", "plans"), 0o750)).To(Succeed())
				Expect(os.MkdirAll(filepath.Join(repo, ".klaudiush"), 0o750)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(repo, "docs", "plans", "a.md"), nil, 0o600)).
					To(Succeed())
			})

			It("refuses the target of a configuration symlink", func() {
				Expect(os.Symlink(
					filepath.Join("..", "docs", "plans", "config.toml"),
					filepath.Join(repo, ".klaudiush", "config.toml"),
				)).To(Succeed())

				Expect(phase.Writable(repo, filepath.Join(repo, "docs", "plans", "config.toml"))).
					To(BeFalse())

				Expect(
					os.WriteFile(filepath.Join(repo, "docs", "plans", "config.toml"), nil, 0o600),
				).
					To(Succeed())
				Expect(phase.Writable(repo, filepath.Join(repo, "docs", "plans", "config.toml"))).
					To(BeFalse())
				Expect(
					phase.Writable(repo, filepath.Join(repo, "docs", "plans", "a.md")),
				).To(BeTrue())
			})

			It("refuses files under a configuration directory symlink", func() {
				Expect(os.MkdirAll(filepath.Join(repo, "sub"), 0o750)).To(Succeed())
				Expect(os.Symlink(
					filepath.Join(repo, "docs", "plans", "gemini"),
					filepath.Join(repo, "sub", ".gemini"),
				)).To(Succeed())

				Expect(
					phase.Writable(
						repo,
						filepath.Join(repo, "docs", "plans", "gemini", "settings.json"),
					),
				).
					To(BeFalse())
				Expect(
					phase.Writable(repo, filepath.Join(repo, "docs", "plans", "a.md")),
				).To(BeTrue())
			})

			It("refuses everything when a configuration symlink cannot be followed", func() {
				loop := filepath.Join(repo, ".klaudiush", "loop")
				Expect(os.Symlink(loop, loop)).To(Succeed())

				Expect(
					phase.Writable(repo, filepath.Join(repo, "docs", "plans", "a.md")),
				).To(BeFalse())
			})

			It("refuses a hard link", func() {
				Expect(os.WriteFile(filepath.Join(repo, ".klaudiush", "config.toml"), nil, 0o600)).
					To(Succeed())
				Expect(os.Link(
					filepath.Join(repo, ".klaudiush", "config.toml"),
					filepath.Join(repo, "docs", "plans", "config.toml"),
				)).To(Succeed())

				Expect(phase.Writable(repo, filepath.Join(repo, "docs", "plans", "config.toml"))).
					To(BeFalse())
			})

			It("compares a file with protected identities", func() {
				config := filepath.Join(repo, ".klaudiush", "config.toml")
				script := filepath.Join(repo, "check.sh")

				Expect(os.WriteFile(config, nil, 0o600)).To(Succeed())
				Expect(os.WriteFile(script, nil, 0o600)).To(Succeed())

				for path, want := range map[string]bool{
					config: true,
					script: true,
					filepath.Join(repo, "docs", "plans", "a.md"): false,
				} {
					info, err := os.Stat(path)
					Expect(err).NotTo(HaveOccurred())
					Expect(
						evidence.SameAsProtected(repo, info, []string{script}),
					).To(Equal(want), path)
				}
			})

			It("refuses everything when the repository cannot be scanned", func() {
				if os.Geteuid() == 0 {
					Skip("root reads every directory")
				}

				locked := filepath.Join(repo, "locked")
				Expect(os.MkdirAll(locked, 0o750)).To(Succeed())
				Expect(os.Chmod(locked, 0o000)).To(Succeed())
				DeferCleanup(os.Chmod, locked, os.FileMode(0o750))

				Expect(
					phase.Writable(repo, filepath.Join(repo, "docs", "plans", "a.md")),
				).To(BeFalse())
			})
		})

		Describe("files a prerequisite runs", func() {
			writable := func(command, rel string) bool {
				p := &evidence.Phase{
					WritablePaths: []string{"docs/plans/**", "PLAN.md"},
					Requires: []*evidence.Check{compileOne(&config.EvidenceCheckConfig{
						Name: "plan", Commands: []string{command},
					})},
				}

				return p.Writable(repo, filepath.Join(repo, rel))
			}

			It("resolves absolute and linked script paths", func() {
				if runtime.GOOS == "windows" {
					Skip("symbolic links need privileges on Windows")
				}

				plans := filepath.Join(repo, "docs", "plans")
				Expect(os.MkdirAll(plans, 0o750)).To(Succeed())
				Expect(os.MkdirAll(filepath.Join(repo, "scripts"), 0o750)).To(Succeed())
				Expect(os.Symlink(
					filepath.Join("..", "docs", "plans", "real.sh"),
					filepath.Join(repo, "scripts", "check.sh"),
				)).To(Succeed())
				Expect(os.Symlink(plans, filepath.Join(repo, "tools"))).To(Succeed())

				Expect(writable("sh "+filepath.Join(plans, "check.sh"), "docs/plans/check.sh")).
					To(BeFalse())
				Expect(writable(filepath.Join(plans, "verify"), "docs/plans/verify")).To(BeFalse())
				Expect(writable("sh scripts/check.sh", "docs/plans/real.sh")).To(BeFalse())
				Expect(writable("bash tools/check.sh PLAN.md", "docs/plans/check.sh")).To(BeFalse())
				Expect(writable("bash tools/check.sh PLAN.md", "PLAN.md")).To(BeTrue())
				Expect(writable("sh scripts/check.sh", "docs/plans/other.sh")).To(BeTrue())
			})

			DescribeTable(
				"finds the script an interpreter runs",
				func(command, script string, scriptWritable bool, planWritable bool) {
					Expect(writable(command, script)).To(Equal(scriptWritable))
					Expect(writable(command, "PLAN.md")).To(Equal(planWritable))
				},
				Entry("python -W ignore", "python3 -W ignore docs/plans/check.py PLAN.md",
					"docs/plans/check.py", false, true),
				Entry("python attached -Wignore", "python3 -uWignore docs/plans/check.py PLAN.md",
					"docs/plans/check.py", false, true),
				Entry("python -X value", "python3.12 -X dev -B docs/plans/check.py PLAN.md",
					"docs/plans/check.py", false, true),
				Entry(
					"python long option",
					"python --check-hash-based-pycs never docs/plans/c.py PLAN.md",
					"docs/plans/c.py",
					false,
					true,
				),
				Entry(
					"python long option with =",
					"python --check-hash-based-pycs=never docs/plans/c.py PLAN.md",
					"docs/plans/c.py",
					false,
					true,
				),
				Entry("python inline code", "python3 -c pass docs/plans/x.md PLAN.md",
					"docs/plans/x.md", false, false),
				Entry("python module", "python3 -m docs.plans.check PLAN.md",
					"docs/plans/x.md", true, false),
				Entry("python stdin", "python3 - PLAN.md", "docs/plans/x.md", true, false),
				Entry("python value missing", "python3 -W", "docs/plans/x.md", true, true),
				Entry("node require", "node -r docs/plans/hook.js docs/plans/check.js PLAN.md",
					"docs/plans/hook.js", false, true),
				Entry("node unknown option", "node --inspect docs/plans/check.js PLAN.md",
					"docs/plans/check.js", false, false),
				Entry("node --require=", "node --require=docs/plans/hook.js main.js PLAN.md",
					"docs/plans/hook.js", false, true),
				Entry("ruby -I", "ruby -I lib -W0 docs/plans/check.rb PLAN.md",
					"docs/plans/check.rb", false, true),
				Entry("ruby -C", "ruby -C docs docs/plans/check.rb PLAN.md",
					"docs/plans/check.rb", false, false),
				Entry("perl -I", "perl -I lib -Mstrict docs/plans/check.pl PLAN.md",
					"docs/plans/check.pl", false, true),
				Entry("perl -e", "perl -e 1 PLAN.md", "docs/plans/x.md", true, false),
				Entry("bash -o", "bash -o pipefail docs/plans/check.sh PLAN.md",
					"docs/plans/check.sh", false, true),
				Entry("sh +o", "sh +o errexit -eu docs/plans/check.sh PLAN.md",
					"docs/plans/check.sh", false, true),
				Entry(
					"bash --rcfile",
					"bash --norc --rcfile docs/plans/rc docs/plans/check.sh PLAN.md",
					"docs/plans/rc",
					false,
					true,
				),
				Entry("bash long option with value", "bash --norc=x docs/plans/check.sh PLAN.md",
					"docs/plans/check.sh", false, false),
				Entry("sh after --", "sh -- docs/plans/check.sh PLAN.md",
					"docs/plans/check.sh", false, true),
				Entry("sh -- alone", "sh --", "docs/plans/x.md", true, true),
				Entry("sh -c", "sh -c true docs/plans/check.sh PLAN.md",
					"docs/plans/check.sh", false, false),
				Entry("other program", "make -C docs/plans check PLAN.md",
					"docs/plans/check", true, true),
			)
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
				false,
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

		It("resolves a running binary known only by name on PATH", func() {
			Expect(phase.AllowsVerifier("klaudiush evidence status", "klaudiush")).To(BeTrue())
			Expect(phase.AllowsVerifier("klaudiush evidence status", "nope-q7zx")).To(BeFalse())
		})

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
		).To(ContainSubstring("gemini: supported"))
		Expect(evidence.PhaseCoverage(hook.ProviderClaude)).
			To(ContainSubstring("claude: not supported, it has no tool-selection event"))
		Expect(evidence.PhaseCoverageLines()).To(HaveLen(len(evidence.Providers)))
	})
})
