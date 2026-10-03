package evidence_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/checkers/evidence"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	withoutStop = `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [
		{"type": "command", "command": "/usr/local/bin/klaudiush --provider claude"}]}]}}`
	withStop = `{"hooks": {
		"PreToolUse": [{"matcher": "Bash", "hooks": [
			{"type": "command", "command": "/usr/local/bin/klaudiush --provider claude"}]}],
		"Stop": [{"hooks": [{"type": "command", "command": "klaudiush --event Stop"}]}]}}`
	otherTool = `{"hooks": {"PreToolUse": [{"hooks": [{"type": "command", "command": ""}]}]}}`
)

func enabledConfig(checks ...*config.EvidenceCheckConfig) *config.Config {
	enabled := true

	return &config.Config{Evidence: &config.EvidenceConfig{Enabled: &enabled, Checks: checks}}
}

var _ = Describe("GateChecker", func() {
	var dir string

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
	})

	locations := func(contents ...string) func() []settings.SettingsLocation {
		result := make([]settings.SettingsLocation, 0, len(contents)+1)

		for i, content := range contents {
			path := filepath.Join(dir, "settings"+string(rune('a'+i))+".json")
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

			result = append(
				result,
				settings.SettingsLocation{Path: path, Type: "user", Exists: true},
			)
		}

		result = append(result,
			settings.SettingsLocation{Path: filepath.Join(dir, "missing.json"), Type: "project"},
		)

		return func() []settings.SettingsLocation { return result }
	}

	tests := &config.EvidenceCheckConfig{Name: "tests", Commands: []string{"make test"}}

	It("skips when the gate is disabled", func() {
		checker := evidence.NewGateChecker(nil)

		Expect(checker.Name()).NotTo(BeEmpty())
		Expect(checker.Category()).To(Equal(doctor.CategoryEvidence))
		Expect(checker.Check(context.Background()).Status).To(Equal(doctor.StatusSkipped))
	})

	It("fails on invalid checks", func() {
		cfg := enabledConfig(&config.EvidenceCheckConfig{Name: "tests"})
		result := evidence.NewGateCheckerWithLocations(cfg, locations()).Check(context.Background())

		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Severity).To(Equal(doctor.SeverityError))
		Expect(result.Details[0]).To(ContainSubstring("commands must not be empty"))
	})

	It("warns when the gate has no checks", func() {
		result := evidence.NewGateCheckerWithLocations(enabledConfig(), locations()).
			Check(context.Background())

		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Severity).To(Equal(doctor.SeverityWarning))
	})

	It("warns about Claude settings without a Stop hook", func() {
		checker := evidence.NewGateCheckerWithLocations(
			enabledConfig(tests),
			locations(withoutStop, withStop, otherTool, "{not json"),
		)
		result := checker.Check(context.Background())

		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Severity).To(Equal(doctor.SeverityWarning))
		Expect(result.Message).To(ContainSubstring("1 Claude settings file(s)"))
		Expect(result.Details).To(ContainElement(ContainSubstring("settingsa.json")))
		Expect(result.Details).To(ContainElement(ContainSubstring("Add a Stop hook")))
		Expect(result.Details).To(ContainElement(ContainSubstring("codex: gated")))
	})

	It("passes when every klaudiush settings file runs on Stop", func() {
		result := evidence.NewGateCheckerWithLocations(enabledConfig(tests), locations(withStop)).
			Check(context.Background())

		Expect(result.Status).To(Equal(doctor.StatusPass))
		Expect(result.Message).To(ContainSubstring("1 required check(s)"))
		Expect(result.Details).To(ContainElement(ContainSubstring("claude: gated")))
	})
})
