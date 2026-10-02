package hook_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/checkers/hook"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

func TestHook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hook Checker Suite")
}

var _ = Describe("RegistrationChecker", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewUserRegistrationChecker", func() {
		var checker *hook.RegistrationChecker

		BeforeEach(func() {
			checker = hook.NewUserRegistrationChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("Dispatcher registered in user settings"))
		})

		It("should have hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Dispatcher registered in user settings"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})

	Describe("NewProjectRegistrationChecker", func() {
		var checker *hook.RegistrationChecker

		BeforeEach(func() {
			checker = hook.NewProjectRegistrationChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("Dispatcher registered in project settings"))
		})

		It("should have hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Dispatcher registered in project settings"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})

	Describe("NewProjectLocalRegistrationChecker", func() {
		var checker *hook.RegistrationChecker

		BeforeEach(func() {
			checker = hook.NewProjectLocalRegistrationChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("Dispatcher registered in project-local settings"))
		})

		It("should have hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Dispatcher registered in project-local settings"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})
})

var _ = Describe("PreToolUseChecker", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewUserPreToolUseChecker", func() {
		var checker *hook.PreToolUseChecker

		BeforeEach(func() {
			checker = hook.NewUserPreToolUseChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("PreToolUse hook in user settings"))
		})

		It("should have hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("PreToolUse hook in user settings"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})

	Describe("NewProjectPreToolUseChecker", func() {
		var checker *hook.PreToolUseChecker

		BeforeEach(func() {
			checker = hook.NewProjectPreToolUseChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("PreToolUse hook in project settings"))
		})

		It("should have hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("PreToolUse hook in project settings"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})

	Describe("NewProjectLocalPreToolUseChecker", func() {
		var checker *hook.PreToolUseChecker

		BeforeEach(func() {
			checker = hook.NewProjectLocalPreToolUseChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("PreToolUse hook in project-local settings"))
		})
	})
})

var _ = Describe("PostToolUseChecker", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewUserPostToolUseChecker", func() {
		var checker *hook.PostToolUseChecker

		BeforeEach(func() {
			checker = hook.NewUserPostToolUseChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("PostToolUse hook in user settings"))
		})

		It("should have hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("PostToolUse hook in user settings"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})

	Describe("NewProjectPostToolUseChecker", func() {
		var checker *hook.PostToolUseChecker

		BeforeEach(func() {
			checker = hook.NewProjectPostToolUseChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("PostToolUse hook in project settings"))
		})
	})

	Describe("NewProjectLocalPostToolUseChecker", func() {
		var checker *hook.PostToolUseChecker

		BeforeEach(func() {
			checker = hook.NewProjectLocalPostToolUseChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("PostToolUse hook in project-local settings"))
		})
	})
})

var _ = Describe("PathValidationChecker", func() {
	var (
		checker *hook.PathValidationChecker
		ctx     context.Context
	)

	BeforeEach(func() {
		checker = hook.NewPathValidationChecker()
		ctx = context.Background()
	})

	Describe("Name", func() {
		It("should return the correct name", func() {
			Expect(checker.Name()).To(Equal("Dispatcher path is valid"))
		})
	})

	Describe("Category", func() {
		It("should return hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})
	})

	Describe("Check", func() {
		It("should check dispatcher path", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Dispatcher path is valid"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})
})

var _ = Describe("Codex hook checkers", func() {
	var (
		ctx          context.Context
		tempDir      string
		hooksPath    string
		originalPath string
		pathSet      bool
	)

	BeforeEach(func() {
		var err error

		ctx = context.Background()
		tempDir, err = os.MkdirTemp("", "codex-hook-checker-*")
		Expect(err).NotTo(HaveOccurred())

		hooksPath = filepath.Join(tempDir, "hooks.json")
		binDir := filepath.Join(tempDir, "bin")
		Expect(os.MkdirAll(binDir, 0o755)).To(Succeed())

		Expect(
			os.WriteFile(
				filepath.Join(binDir, "klaudiush"),
				[]byte("#!/bin/sh\nexit 0\n"),
				0o755,
			),
		).
			To(Succeed())

		originalPath, pathSet = os.LookupEnv("PATH")
		Expect(os.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)).To(Succeed())
	})

	AfterEach(func() {
		if pathSet {
			Expect(os.Setenv("PATH", originalPath)).To(Succeed())
		} else {
			Expect(os.Unsetenv("PATH")).To(Succeed())
		}

		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("passes when configured Codex hooks register klaudiush", func() {
		Expect(os.WriteFile(
			hooksPath,
			[]byte(`{
  "hooks": {
    "SessionStart": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event SessionStart","timeout":30}]}],
    "PreToolUse": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse","timeout":30}]}],
    "Stop": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event Stop","timeout":30}]}]
  }
}`),
			0o600,
		)).To(Succeed())

		enabled := true
		experimental := true
		cfg := &pkgConfig.CodexProviderConfig{
			Enabled:         &enabled,
			Experimental:    &experimental,
			HooksConfigPath: hooksPath,
		}

		registrationChecker := hook.NewCodexRegistrationChecker(cfg)
		sessionStartChecker := hook.NewCodexEventChecker(cfg, "SessionStart")
		preToolUseChecker := hook.NewCodexEventChecker(cfg, "PreToolUse")
		stopChecker := hook.NewCodexEventChecker(cfg, "Stop")
		enforcementChecker := hook.NewCodexEnforcementChecker(cfg)

		Expect(registrationChecker.Check(ctx).Status).To(Equal(doctor.StatusPass))
		Expect(sessionStartChecker.Check(ctx).Status).To(Equal(doctor.StatusPass))
		Expect(preToolUseChecker.Check(ctx).Status).To(Equal(doctor.StatusPass))
		Expect(stopChecker.Check(ctx).Status).To(Equal(doctor.StatusPass))

		enforcement := enforcementChecker.Check(ctx)
		Expect(enforcement.Status).To(Equal(doctor.StatusPass))
		Expect(enforcement.Details).To(ContainElement(
			"Blocked before running: shell (Bash), apply_patch, MCP tools, local function tools",
		))
	})

	It("fails when the configured Codex hooks file is missing an event", func() {
		Expect(os.WriteFile(
			hooksPath,
			[]byte(`{
  "hooks": {
    "SessionStart": [{"hooks":[{"type":"command","command":"klaudiush --provider codex --event SessionStart","timeout":30}]}]
  }
}`),
			0o600,
		)).To(Succeed())

		enabled := true
		experimental := true
		cfg := &pkgConfig.CodexProviderConfig{
			Enabled:         &enabled,
			Experimental:    &experimental,
			HooksConfigPath: hooksPath,
		}

		preToolUseChecker := hook.NewCodexEventChecker(cfg, "PreToolUse")
		result := preToolUseChecker.Check(ctx)

		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.FixID).To(Equal("install_hook"))
		Expect(result.Message).To(ContainSubstring("not configured"))
	})

	DescribeTable("Codex enforcement distinguishes registration from blocking",
		func(hooksJSON, configTOML string, status doctor.Status, message string, fixID string) {
			Expect(os.WriteFile(hooksPath, []byte(hooksJSON), 0o600)).To(Succeed())

			if configTOML != "" {
				Expect(os.WriteFile(
					filepath.Join(filepath.Dir(hooksPath), "config.toml"),
					[]byte(configTOML),
					0o600,
				)).To(Succeed())
			}

			enabled := true
			experimental := true
			result := hook.NewCodexEnforcementChecker(&pkgConfig.CodexProviderConfig{
				Enabled:         &enabled,
				Experimental:    &experimental,
				HooksConfigPath: hooksPath,
			}).Check(ctx)

			Expect(result.Status).To(Equal(status))
			Expect(result.Message).To(ContainSubstring(message))
			Expect(result.FixID).To(Equal(fixID))
		},
		Entry(
			"legacy AfterToolUse only",
			`{"hooks":{"AfterToolUse":[{"hooks":[{"type":"command","command":"klaudiush --provider codex --event AfterToolUse"}]}]}}`,
			"",
			doctor.StatusFail,
			"legacy AfterToolUse",
			"install_hook",
		),
		Entry(
			"no PreToolUse",
			`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"klaudiush --provider codex --event Stop"}]}]}}`,
			"",
			doctor.StatusFail,
			"PreToolUse hook not registered",
			"install_hook",
		),
		Entry(
			"async PreToolUse",
			`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse","async":true}]}]}}`,
			"",
			doctor.StatusFail,
			"async",
			"install_hook",
		),
		Entry(
			"Bash-only matcher",
			`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse"}]}]}}`,
			"",
			doctor.StatusFail,
			"matcher skips",
			"install_hook",
		),
		Entry(
			"hooks feature disabled",
			`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse"}]}]}}`,
			"[features]\nhooks = false\n",
			doctor.StatusFail,
			"feature is disabled",
			"",
		),
	)
})

var _ = Describe("Codex enforcement coverage", func() {
	var (
		ctx       context.Context
		hooksPath string
	)

	BeforeEach(func() {
		ctx = context.Background()
		tempDir := GinkgoT().TempDir()
		hooksPath = filepath.Join(tempDir, "hooks.json")
		binDir := filepath.Join(tempDir, "bin")
		Expect(os.MkdirAll(binDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(binDir, "klaudiush"),
			[]byte("#!/bin/sh\nexit 0\n"),
			0o755,
		)).To(Succeed())
		GinkgoT().Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	})

	check := func(matchers ...string) doctor.CheckResult {
		groups := make([]string, 0, len(matchers))
		for _, matcher := range matchers {
			groups = append(groups, `{"matcher":`+strconv.Quote(matcher)+
				`,"hooks":[{"type":"command","command":"klaudiush --provider codex --event PreToolUse"}]}`)
		}

		Expect(os.WriteFile(
			hooksPath,
			[]byte(`{"hooks":{"PreToolUse":[`+strings.Join(groups, ",")+`]}}`),
			0o600,
		)).To(Succeed())

		enabled := true
		experimental := true

		return hook.NewCodexEnforcementChecker(&pkgConfig.CodexProviderConfig{
			Enabled:         &enabled,
			Experimental:    &experimental,
			HooksConfigPath: hooksPath,
		}).Check(ctx)
	}

	DescribeTable("reports family coverage from the matcher",
		func(matchers []string, status doctor.Status, details []string) {
			result := check(matchers...)

			Expect(result.Status).To(Equal(status))

			for _, detail := range details {
				Expect(result.Details).To(ContainElement(detail))
			}
		},
		Entry(
			"exact representative names are not whole-family coverage",
			[]string{"Bash|apply_patch|mcp__server__tool|update_plan"},
			doctor.StatusFail,
			[]string{
				"Blocked before running: shell (Bash), apply_patch",
				"Only tools the matcher names: MCP tools, local function tools",
				"Not enforced: hosted tools (web search)",
			},
		),
		Entry(
			"one MCP server pattern misses the representative",
			[]string{"Bash|apply_patch", "mcp__github__.*"},
			doctor.StatusFail,
			[]string{
				"Blocked before running: shell (Bash), apply_patch",
				"Not enforced: MCP tools, local function tools, hosted tools (web search)",
			},
		),
		Entry(
			"family patterns across groups cover everything",
			[]string{"Bash|apply_patch", "mcp__.*", "[a-z_0-9]+"},
			doctor.StatusPass,
			[]string{
				"Blocked before running: shell (Bash), apply_patch, MCP tools, local function tools",
				"Not enforced: hosted tools (web search)",
			},
		),
		Entry(
			"dot-star covers everything",
			[]string{".*"},
			doctor.StatusPass,
			[]string{
				"Blocked before running: shell (Bash), apply_patch, MCP tools, local function tools",
			},
		),
	)

	It("skips when the binary is not on PATH", func() {
		GinkgoT().Setenv("PATH", GinkgoT().TempDir())

		result := check("")

		Expect(result.Status).To(Equal(doctor.StatusSkipped))
	})

	It("warns when Codex feature flags cannot be read", func() {
		Expect(os.WriteFile(
			filepath.Join(filepath.Dir(hooksPath), "config.toml"),
			[]byte("[features\n"),
			0o600,
		)).To(Succeed())

		result := check("")

		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Severity).To(Equal(doctor.SeverityWarning))
		Expect(result.Message).To(ContainSubstring("Cannot read Codex feature flags"))
	})

	It("never lists a partial family as blocked", func() {
		result := check("mcp__server__tool")

		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.FixID).To(Equal("install_hook"))
		Expect(result.Details).To(ContainElements(
			"Blocked before running: none",
			"Only tools the matcher names: MCP tools",
		))
	})
})

var _ = Describe("Gemini hook checkers", func() {
	var (
		ctx          context.Context
		tempDir      string
		settingsPath string
		originalPath string
		pathSet      bool
	)

	BeforeEach(func() {
		var err error

		ctx = context.Background()
		tempDir, err = os.MkdirTemp("", "gemini-hook-checker-*")
		Expect(err).NotTo(HaveOccurred())

		settingsPath = filepath.Join(tempDir, "settings.json")
		binDir := filepath.Join(tempDir, "bin")
		Expect(os.MkdirAll(binDir, 0o755)).To(Succeed())

		Expect(
			os.WriteFile(
				filepath.Join(binDir, "klaudiush"),
				[]byte("#!/bin/sh\nexit 0\n"),
				0o755,
			),
		).
			To(Succeed())

		originalPath, pathSet = os.LookupEnv("PATH")
		Expect(os.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)).To(Succeed())
	})

	AfterEach(func() {
		if pathSet {
			Expect(os.Setenv("PATH", originalPath)).To(Succeed())
		} else {
			Expect(os.Unsetenv("PATH")).To(Succeed())
		}

		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("passes when configured Gemini settings register klaudiush", func() {
		Expect(os.WriteFile(
			settingsPath,
			[]byte(`{
  "hooks": {
    "BeforeTool": [{"matcher":"run_shell_command|write_file|replace|read_file|glob|grep|ls","hooks":[{"type":"command","command":"klaudiush --provider gemini --event BeforeTool","timeout":30000}]}],
    "AfterTool": [{"matcher":"run_shell_command|write_file|replace|read_file|glob|grep|ls","hooks":[{"type":"command","command":"klaudiush --provider gemini --event AfterTool","timeout":30000}]}],
    "SessionStart": [{"hooks":[{"type":"command","command":"klaudiush --provider gemini --event SessionStart","timeout":30000}]}],
    "SessionEnd": [{"hooks":[{"type":"command","command":"klaudiush --provider gemini --event SessionEnd","timeout":30000}]}],
    "Notification": [{"hooks":[{"type":"command","command":"klaudiush --provider gemini --event Notification","timeout":30000}]}],
    "PreCompress": [{"hooks":[{"type":"command","command":"klaudiush --provider gemini --event PreCompress","timeout":30000}]}]
  }
}`),
			0o600,
		)).To(Succeed())

		enabled := true
		cfg := &pkgConfig.GeminiProviderConfig{
			Enabled:      &enabled,
			SettingsPath: settingsPath,
		}

		registrationChecker := hook.NewGeminiRegistrationChecker(cfg)
		beforeToolChecker := hook.NewGeminiEventChecker(cfg, "BeforeTool")
		preCompressChecker := hook.NewGeminiEventChecker(cfg, "PreCompress")

		Expect(registrationChecker.Check(ctx).Status).To(Equal(doctor.StatusPass))
		Expect(beforeToolChecker.Check(ctx).Status).To(Equal(doctor.StatusPass))
		Expect(preCompressChecker.Check(ctx).Status).To(Equal(doctor.StatusPass))
	})

	It("fails when the configured Gemini settings file is missing an event", func() {
		Expect(os.WriteFile(
			settingsPath,
			[]byte(`{
  "hooks": {
    "BeforeTool": [{"matcher":"run_shell_command|write_file|replace|read_file|glob|grep|ls","hooks":[{"type":"command","command":"klaudiush --provider gemini --event BeforeTool","timeout":30000}]}]
  }
}`),
			0o600,
		)).To(Succeed())

		enabled := true
		cfg := &pkgConfig.GeminiProviderConfig{
			Enabled:      &enabled,
			SettingsPath: settingsPath,
		}

		sessionEndChecker := hook.NewGeminiEventChecker(cfg, "SessionEnd")
		result := sessionEndChecker.Check(ctx)

		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.FixID).To(Equal("install_hook"))
		Expect(result.Message).To(ContainSubstring("not configured"))
	})
})
