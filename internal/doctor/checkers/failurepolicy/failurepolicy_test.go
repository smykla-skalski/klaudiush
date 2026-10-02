package failurepolicy_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/checkers/failurepolicy"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const settingsJSON = `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "*", "hooks": [
        {"type": "command", "command": "/usr/local/bin/klaudiush --provider claude", "timeout": %d},
        {"type": "command", "command": "other-tool", "timeout": 1}
      ]}
    ],
    "Stop": [
      {"matcher": "", "hooks": [{"type": "command", "command": "klaudiush --event Stop"}]}
    ]
  }
}`

var _ = Describe("DeadlineChecker", func() {
	var dir string

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
	})

	locationsWith := func(timeout int) func() []settings.SettingsLocation {
		path := filepath.Join(dir, "settings.json")
		Expect(os.WriteFile(path, fmt.Appendf(nil, settingsJSON, timeout), 0o600)).To(Succeed())

		return func() []settings.SettingsLocation {
			return []settings.SettingsLocation{
				{Path: path, Type: "user", Exists: true},
				{Path: filepath.Join(dir, "missing.json"), Type: "project"},
				{Path: filepath.Join(dir, "bad.json"), Type: "local", Exists: true},
			}
		}
	}

	It("passes when the deadline leaves time to answer", func() {
		checker := failurepolicy.NewDeadlineCheckerWithLocations(nil, locationsWith(30))

		Expect(checker.Name()).NotTo(BeEmpty())
		Expect(checker.Category()).To(Equal(doctor.CategoryFailurePolicy))

		result := checker.Check(context.Background())
		Expect(result.Status).To(Equal(doctor.StatusPass))
	})

	It("warns when a klaudiush hook timeout is too short", func() {
		cfg := &config.Config{FailurePolicy: &config.FailurePolicyConfig{
			Deadline: config.Duration(20 * time.Second),
		}}
		checker := failurepolicy.NewDeadlineCheckerWithLocations(cfg, locationsWith(10))

		result := checker.Check(context.Background())
		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Severity).To(Equal(doctor.SeverityWarning))
		Expect(result.Details[0]).To(ContainSubstring("hook timeout 10s"))
	})

	It("can be built with the real settings locations", func() {
		Expect(failurepolicy.NewDeadlineChecker(nil)).NotTo(BeNil())
	})
})

var _ = Describe("CriticalToolsChecker", func() {
	var tools *exec.MockToolChecker

	BeforeEach(func() {
		tools = exec.NewMockToolChecker(gomock.NewController(GinkgoT()))
	})

	critical := func(names ...string) *config.Config {
		return &config.Config{FailurePolicy: &config.FailurePolicyConfig{Critical: names}}
	}

	It("skips without critical validators", func() {
		checker := failurepolicy.NewCriticalToolsCheckerWithTools(nil, tools)

		Expect(checker.Name()).NotTo(BeEmpty())
		Expect(checker.Category()).To(Equal(doctor.CategoryFailurePolicy))
		Expect(checker.Check(context.Background()).Status).To(Equal(doctor.StatusSkipped))
	})

	It("passes when every critical tool is installed", func() {
		tools.EXPECT().FindTool("tofu", "terraform").Return("tofu")
		tools.EXPECT().FindTool("tflint").Return("tflint")

		checker := failurepolicy.NewCriticalToolsCheckerWithTools(
			critical("file.terraform", "git.commit"),
			tools,
		)
		Expect(checker.Check(context.Background()).Status).To(Equal(doctor.StatusPass))
	})

	It("fails when a critical validator's tool is missing", func() {
		tools.EXPECT().FindTool("shellcheck").Return("")

		checker := failurepolicy.NewCriticalToolsCheckerWithTools(critical("shellscript"), tools)

		result := checker.Check(context.Background())
		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Severity).To(Equal(doctor.SeverityError))
		Expect(result.Details[0]).To(ContainSubstring("shellscript needs shellcheck"))
	})

	It("needs tflint for terraform only while use_tflint is on", func() {
		tools.EXPECT().FindTool("tofu", "terraform").Return("tofu").Times(2)
		tools.EXPECT().FindTool("tflint").Return("")

		cfg := critical("file.terraform")
		Expect(failurepolicy.NewCriticalToolsCheckerWithTools(cfg, tools).
			Check(context.Background()).Status).To(Equal(doctor.StatusFail))

		cfg.Validators = &config.ValidatorsConfig{File: &config.FileConfig{
			Terraform: &config.TerraformValidatorConfig{UseTflint: new(false)},
		}}
		Expect(failurepolicy.NewCriticalToolsCheckerWithTools(cfg, tools).
			Check(context.Background()).Status).To(Equal(doctor.StatusPass))
	})

	It("flags critical names that match no validator", func() {
		result := failurepolicy.NewCriticalToolsCheckerWithTools(critical("file.typo"), tools).
			Check(context.Background())

		Expect(result.Status).To(Equal(doctor.StatusFail))
		Expect(result.Details[0]).To(ContainSubstring("not a known validator name"))
	})

	It("can be built with the real tool lookup", func() {
		Expect(failurepolicy.NewCriticalToolsChecker(nil)).NotTo(BeNil())
	})
})
