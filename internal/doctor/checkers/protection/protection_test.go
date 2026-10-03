package protection_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/checkers/protection"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	narrowMatcher = `{"hooks": {"PreToolUse": [{"matcher": "Bash|Write|Edit|MultiEdit", "hooks": [
		{"type": "command", "command": "/usr/local/bin/klaudiush --provider claude"}]}]}}`
	fullSettings = `{"hooks": {
		"PreToolUse": [{"matcher": "Bash|Write|Edit|MultiEdit|mcp__.*", "hooks": [
			{"type": "command", "command": "klaudiush --provider claude"}]}],
		"ConfigChange": [{"hooks": [{"type": "command", "command": "klaudiush --event ConfigChange"}]}]}}`
	otherHooks = `{"hooks": {"PreToolUse": [{"hooks": [{"type": "command", "command": "./lint.sh"}]}]}}`
)

var _ = Describe("checkers", func() {
	var dir string

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
	})

	locations := func(contents ...string) protection.Locations {
		result := make([]settings.SettingsLocation, 0, len(contents)+2)

		for i, content := range contents {
			path := filepath.Join(dir, "settings"+string(rune('a'+i))+".json")
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

			result = append(
				result,
				settings.SettingsLocation{Path: path, Type: "user", Exists: true},
			)
		}

		broken := filepath.Join(dir, "broken.json")
		Expect(os.WriteFile(broken, []byte("{"), 0o600)).To(Succeed())

		result = append(result,
			settings.SettingsLocation{Path: filepath.Join(dir, "missing.json"), Type: "project"},
			settings.SettingsLocation{Path: broken, Type: "project-local", Exists: true},
		)

		return func() []settings.SettingsLocation { return result }
	}

	noManaged := func() []string { return nil }

	Describe("ProtectionChecker", func() {
		enabled := func(cfg *config.ProtectionConfig) *config.Config {
			cfg.Enabled = new(true)

			return &config.Config{Protection: cfg}
		}

		It("describes itself", func() {
			checker := protection.NewProtectionChecker(nil)
			Expect(checker.Name()).NotTo(BeEmpty())
			Expect(checker.Category()).To(Equal(doctor.CategoryProtection))
		})

		It("skips when disabled", func() {
			result := protection.NewProtectionCheckerWith(nil, locations(), noManaged).
				Check(context.Background())
			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})

		It("fails on invalid configuration", func() {
			cfg := enabled(&config.ProtectionConfig{Paths: []string{""}})
			result := protection.NewProtectionCheckerWith(cfg, locations(), noManaged).
				Check(context.Background())
			Expect(result.Severity).To(Equal(doctor.SeverityError))
		})

		It("warns about Claude settings without ConfigChange", func() {
			cfg := enabled(&config.ProtectionConfig{})
			result := protection.NewProtectionCheckerWith(cfg, locations(narrowMatcher, otherHooks), noManaged).
				Check(context.Background())

			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityWarning))
			Expect(result.Message).To(ContainSubstring("1 Claude settings file"))
			Expect(result.Details).To(ContainElement(ContainSubstring("--event ConfigChange")))
		})

		It("does not need ConfigChange when no source is blocked", func() {
			cfg := enabled(&config.ProtectionConfig{ConfigChangeSources: []string{}})
			result := protection.NewProtectionCheckerWith(cfg, locations(narrowMatcher), noManaged).
				Check(context.Background())

			Expect(result.Status).To(Equal(doctor.StatusPass))
		})

		It("passes and reports coverage and managed hooks", func() {
			managed := filepath.Join(dir, "managed-settings.json")
			Expect(os.WriteFile(managed, []byte(fullSettings), 0o600)).To(Succeed())

			requirements := filepath.Join(dir, "requirements.toml")
			Expect(os.WriteFile(requirements, []byte(
				"[[hooks.PreToolUse]]\n[[hooks.PreToolUse.hooks]]\ntype = \"command\"\ncommand = \"/opt/check.sh\"\n",
			), 0o600)).To(Succeed())

			cfg := enabled(&config.ProtectionConfig{})
			result := protection.NewProtectionCheckerWith(cfg, locations(fullSettings), func() []string {
				return []string{managed, requirements, filepath.Join(dir, "absent.json")}
			}).
				Check(context.Background())

			Expect(result.Status).To(Equal(doctor.StatusPass))
			Expect(
				result.Details,
			).To(ContainElement(ContainSubstring("Managed hooks run klaudiush")))
			Expect(result.Details).To(ContainElement(ContainSubstring("do not run klaudiush")))
			Expect(result.Details).To(ContainElement(HavePrefix("Codex:")))
		})

		It("recommends managed hooks when there are none", func() {
			cfg := enabled(&config.ProtectionConfig{})
			result := protection.NewProtectionCheckerWith(cfg, locations(fullSettings), noManaged).
				Check(context.Background())

			Expect(
				result.Details,
			).To(ContainElement(ContainSubstring("No managed hook configuration")))
		})
	})

	Describe("MCPTrustChecker", func() {
		enabled := func(cfg *config.MCPTrustConfig) *config.Config {
			cfg.Enabled = new(true)

			return &config.Config{MCPTrust: cfg}
		}

		It("describes itself", func() {
			checker := protection.NewMCPTrustChecker(nil)
			Expect(checker.Name()).NotTo(BeEmpty())
			Expect(checker.Category()).To(Equal(doctor.CategoryProtection))
		})

		It("skips when disabled", func() {
			result := protection.NewMCPTrustCheckerWith(nil, locations()).
				Check(context.Background())
			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})

		It("warns when Claude does not route MCP tools", func() {
			cfg := enabled(&config.MCPTrustConfig{TrustedSources: []string{"user"}})
			result := protection.NewMCPTrustCheckerWith(cfg, locations(narrowMatcher)).
				Check(context.Background())

			Expect(result.Severity).To(Equal(doctor.SeverityWarning))
			Expect(result.Message).To(ContainSubstring("MCP tools"))
		})

		It("warns when nothing is trusted", func() {
			cfg := enabled(&config.MCPTrustConfig{})
			result := protection.NewMCPTrustCheckerWith(cfg, locations(fullSettings)).
				Check(context.Background())

			Expect(result.Message).To(ContainSubstring("No MCP server is trusted"))
		})

		It("warns when unknown provenance is allowed", func() {
			cfg := enabled(&config.MCPTrustConfig{
				TrustedSources:    []string{"user"},
				UnknownProvenance: config.MCPTrustActionAllow,
			})
			result := protection.NewMCPTrustCheckerWith(cfg, locations(fullSettings)).
				Check(context.Background())

			Expect(result.Message).To(ContainSubstring("unknown_provenance"))
		})

		It("passes with provenance-based trust", func() {
			cfg := enabled(&config.MCPTrustConfig{TrustedSources: []string{"user"}})
			result := protection.NewMCPTrustCheckerWith(cfg, locations(fullSettings, otherHooks)).
				Check(context.Background())

			Expect(result.Status).To(Equal(doctor.StatusPass))
			Expect(
				result.Details,
			).To(ContainElement(ContainSubstring(`unknown_provenance = "block"`)))
		})
	})

	It("lists only managed files that exist", func() {
		for _, path := range protection.ManagedSettingsFiles() {
			_, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
		}
	})
})
