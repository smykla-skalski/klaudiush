package evidence_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/checkers/evidence"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	geminiSelection = `{"hooks": {
		"BeforeTool": [{"matcher": "run_shell_command|write_file|replace|read_file", "hooks": [
			{"type": "command", "command": "/usr/local/bin/klaudiush --provider gemini --event BeforeTool"}]}],
		"BeforeToolSelection": [{"hooks": [
			{"type": "command", "command": "/usr/local/bin/klaudiush --provider gemini --event BeforeToolSelection"}]}]}}`
	geminiAllTools = `{"hooks": {
		"BeforeTool": [{"matcher": "*", "hooks": [
			{"type": "command", "command": "klaudiush --provider gemini --event BeforeTool"}]}],
		"BeforeToolSelection": [{"hooks": [
			{"type": "command", "command": "klaudiush --provider gemini --event BeforeToolSelection"}]}]}}`
	geminiNoSelection = `{"hooks": {"BeforeTool": [{"hooks": [
		{"type": "command", "command": "klaudiush --provider gemini --event BeforeTool"}]}]}}`
	geminiNarrow = `{"hooks": {
		"BeforeTool": [{"matcher": "read_file", "hooks": [
			{"type": "command", "command": "klaudiush --provider gemini --event BeforeTool"}]}],
		"BeforeToolSelection": [{"hooks": [
			{"type": "command", "command": "klaudiush --provider gemini --event BeforeToolSelection"}]}]}}`
)

var _ = Describe("ToolPhaseChecker", func() {
	var dir string

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
	})

	phaseConfig := func(settingsJSON string) *config.Config {
		enabled := true
		cfg := enabledConfig(&config.EvidenceCheckConfig{
			Name: "plan", Commands: []string{"test -s PLAN.md"},
		})
		cfg.Evidence.ToolPhase = &config.EvidenceToolPhaseConfig{
			Enabled: &enabled, Requires: []string{"plan"},
		}

		if settingsJSON != "" {
			path := filepath.Join(dir, "settings.json")
			Expect(os.WriteFile(path, []byte(settingsJSON), 0o600)).To(Succeed())

			cfg.Providers = &config.ProvidersConfig{
				Gemini: &config.GeminiProviderConfig{Enabled: &enabled, SettingsPath: path},
			}
		}

		return cfg
	}

	check := func(cfg *config.Config) doctor.CheckResult {
		checker := evidence.NewToolPhaseChecker(cfg)
		Expect(checker.Name()).To(Equal("Evidence tool phase reaches Gemini"))
		Expect(checker.Category()).To(Equal(doctor.CategoryEvidence))

		return checker.Check(context.Background())
	}

	It("skips a disabled phase", func() {
		Expect(check(nil).Status).To(Equal(doctor.StatusSkipped))
		Expect(check(enabledConfig()).Status).To(Equal(doctor.StatusSkipped))
	})

	It("fails an invalid phase", func() {
		cfg := phaseConfig("")
		cfg.Evidence.ToolPhase.Requires = []string{"nope"}

		result := check(cfg)
		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Severity).To(Equal(doctor.SeverityError))

		cfg.Evidence.Checks[0].Commands = nil
		Expect(check(cfg).Severity).To(Equal(doctor.SeverityError))
	})

	It("warns that only Gemini can have tools withheld", func() {
		result := check(phaseConfig(""))
		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Severity).To(Equal(doctor.SeverityWarning))
		Expect(result.Details).To(ContainElement(ContainSubstring("claude: not restricted")))
	})

	It("offers the install fix when BeforeToolSelection is not registered", func() {
		result := check(phaseConfig(geminiNoSelection))
		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Message).To(ContainSubstring("Gemini offers every tool"))
		Expect(result.FixID).To(Equal("install_hook"))
	})

	It("warns when the governed tools reach no klaudiush BeforeTool hook", func() {
		result := check(phaseConfig(geminiNarrow))
		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Message).To(ContainSubstring("write_file, replace, run_shell_command"))
		Expect(result.FixID).To(BeEmpty())
	})

	It("passes and names tools withheld only by the selection", func() {
		result := check(phaseConfig(geminiSelection))
		Expect(result.Status).To(Equal(doctor.StatusPass))
		Expect(result.Message).To(ContainSubstring("wait for plan"))
		Expect(result.Details).To(ContainElement(ContainSubstring("save_memory")))

		result = check(phaseConfig(geminiAllTools))
		Expect(result.Status).To(Equal(doctor.StatusPass))
		Expect(result.Details).NotTo(ContainElement(ContainSubstring("save_memory")))
	})

	It("reports unreadable settings", func() {
		result := check(phaseConfig(`{`))
		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Severity).To(Equal(doctor.SeverityError))
	})
})
