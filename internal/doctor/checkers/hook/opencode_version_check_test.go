package hook_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/checkers/hook"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

type stubOpenCode struct {
	version string
	err     error
}

func (s stubOpenCode) Detect(context.Context) (string, error) {
	return s.version, s.err
}

var _ = Describe("opencode plugin API checks", func() {
	var (
		ctx        context.Context
		pluginPath string
		binaryPath string
	)

	enabledCfg := func() *pkgConfig.OpenCodeProviderConfig {
		enabled := true

		return &pkgConfig.OpenCodeProviderConfig{Enabled: &enabled, PluginPath: pluginPath}
	}

	apiChecker := func(detector settings.OpenCodeVersionDetector) *hook.OpenCodeAPIChecker {
		return hook.NewOpenCodeAPIChecker(enabledCfg()).WithOpenCodeVersionDetector(detector)
	}

	install := func(api settings.OpenCodeAPI) {
		_, err := settings.InstallOpenCodeDispatcher(pluginPath, binaryPath, api)
		Expect(err).NotTo(HaveOccurred())
	}

	BeforeEach(func() {
		ctx = context.Background()
		tempDir := GinkgoT().TempDir()
		pluginPath = filepath.Join(tempDir, "plugin", "klaudiush.ts")

		binDir := filepath.Join(tempDir, "bin")
		Expect(os.MkdirAll(binDir, 0o755)).To(Succeed())
		binaryPath = filepath.Join(binDir, "klaudiush")
		Expect(os.WriteFile(binaryPath, []byte("#!/bin/sh\nexit 0\n"), 0o755)).To(Succeed())

		GinkgoT().Setenv("PATH", binDir)
	})

	Describe("OpenCodeAPIChecker", func() {
		It("describes itself as a hook check", func() {
			checker := hook.NewOpenCodeAPIChecker(enabledCfg())

			Expect(checker.Name()).To(Equal("opencode loads the bridge plugin"))
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("skips when the provider is disabled", func() {
			disabled := false
			result := hook.NewOpenCodeAPIChecker(
				&pkgConfig.OpenCodeProviderConfig{Enabled: &disabled},
			).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})

		It("skips when the plugin is not installed", func() {
			result := apiChecker(stubOpenCode{version: "2.0.19"}).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})

		It("reports an unreadable plugin", func() {
			Expect(os.MkdirAll(pluginPath, 0o755)).To(Succeed())

			result := apiChecker(stubOpenCode{version: "2.0.19"}).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.IsError()).To(BeTrue())
		})

		It("skips when opencode is not on PATH", func() {
			install(settings.OpenCodeAPIV1)

			result := apiChecker(stubOpenCode{err: settings.ErrOpenCodeNotInstalled}).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusSkipped))
			Expect(result.Message).To(ContainSubstring("opencode not found"))
		})

		It("warns when the opencode version cannot be read", func() {
			install(settings.OpenCodeAPIV2)

			result := apiChecker(stubOpenCode{err: errors.New("boom")}).Check(ctx)

			Expect(result.IsWarning()).To(BeTrue())
		})

		It("warns on a version it does not recognize", func() {
			install(settings.OpenCodeAPIV2)

			result := apiChecker(stubOpenCode{version: "nightly"}).Check(ctx)

			Expect(result.IsWarning()).To(BeTrue())
		})

		DescribeTable("flags a bridge the installed opencode rejects",
			func(installed settings.OpenCodeAPI, version, plugin string) {
				install(installed)

				result := apiChecker(stubOpenCode{version: version}).Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.IsError()).To(BeTrue())
				Expect(result.FixID).To(Equal("install_hook"))
				Expect(result.Message).To(ContainSubstring("rejects the bridge plugin"))
				Expect(result.Details).To(ContainElement(ContainSubstring("Plugin API: " + plugin)))
			},
			Entry("1.x bridge under opencode 2.x", settings.OpenCodeAPIV1, "2.0.19", "1.x"),
			Entry("2.x bridge under opencode 1.x", settings.OpenCodeAPIV2, "1.14.0", "2.x"),
		)

		It("flags a plugin whose exports match neither API", func() {
			Expect(os.MkdirAll(filepath.Dir(pluginPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(pluginPath, []byte("const x = 1\n"), 0o600)).To(Succeed())

			result := apiChecker(stubOpenCode{version: "2.0.19"}).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Details).To(ContainElement(ContainSubstring("unrecognized")))
		})

		DescribeTable("passes when the bridge matches the installed opencode",
			func(installed settings.OpenCodeAPI, version string) {
				install(installed)

				result := apiChecker(stubOpenCode{version: version}).Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusPass))
				Expect(result.Message).To(ContainSubstring(version))
			},
			Entry("opencode 2.x", settings.OpenCodeAPIV2, "2.0.19"),
			Entry("opencode 1.x", settings.OpenCodeAPIV1, "1.14.0"),
		)
	})

	Describe("OpenCodeFreshnessChecker", func() {
		freshness := func(detector settings.OpenCodeVersionDetector) doctor.CheckResult {
			return hook.NewOpenCodeFreshnessChecker(enabledCfg()).
				WithOpenCodeVersionDetector(detector).
				Check(ctx)
		}

		It("compares against the bridge of the installed opencode", func() {
			install(settings.OpenCodeAPIV2)

			Expect(freshness(stubOpenCode{version: "2.0.19"}).Status).To(Equal(doctor.StatusPass))

			result := freshness(stubOpenCode{version: "1.14.0"})
			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Details).To(ContainElement(ContainSubstring("opencode 1.14.0")))
		})

		It("keeps the installed bridge API when opencode is not on PATH", func() {
			install(settings.OpenCodeAPIV2)

			result := freshness(stubOpenCode{err: settings.ErrOpenCodeNotInstalled})
			Expect(result.Status).To(Equal(doctor.StatusPass))
		})
	})

	Describe("OpenCodeEventChecker on the 2.x bridge", func() {
		It("passes for every forwarded event", func() {
			install(settings.OpenCodeAPIV2)

			for _, eventName := range settings.OpenCodeEventNames() {
				result := hook.NewOpenCodeEventChecker(enabledCfg(), eventName).Check(ctx)
				Expect(result.Status).To(Equal(doctor.StatusPass), "event %s", eventName)
			}
		})
	})
})
