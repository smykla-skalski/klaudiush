package hook_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/checkers/hook"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
	pkgHook "github.com/smykla-skalski/klaudiush/pkg/hook"
)

var _ = Describe("opencode hook checkers", func() {
	var (
		ctx          context.Context
		tempDir      string
		pluginPath   string
		binaryPath   string
		originalPath string
		pathSet      bool
	)

	enabledCfg := func() *pkgConfig.OpenCodeProviderConfig {
		enabled := true

		return &pkgConfig.OpenCodeProviderConfig{
			Enabled:    &enabled,
			PluginPath: pluginPath,
		}
	}

	BeforeEach(func() {
		var err error

		ctx = context.Background()
		tempDir, err = os.MkdirTemp("", "opencode-hook-checker-*")
		Expect(err).NotTo(HaveOccurred())

		pluginPath = filepath.Join(tempDir, "plugin", "klaudiush.ts")

		// The checkers resolve the dispatcher through PATH, so the plugin has
		// to embed this same resolved path to count as registered.
		binDir := filepath.Join(tempDir, "bin")
		Expect(os.MkdirAll(binDir, 0o755)).To(Succeed())
		binaryPath = filepath.Join(binDir, "klaudiush")
		Expect(os.WriteFile(binaryPath, []byte("#!/bin/sh\nexit 0\n"), 0o755)).To(Succeed())

		originalPath, pathSet = os.LookupEnv("PATH")

		Expect(os.Setenv("PATH", binDir)).To(Succeed())
		GinkgoT().Setenv("HOME", tempDir)
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

	Describe("OpenCodeConfigChecker", func() {
		It("skips when the provider is disabled", func() {
			disabled := false
			checker := hook.NewOpenCodeConfigChecker(
				&pkgConfig.OpenCodeProviderConfig{Enabled: &disabled},
			)

			Expect(checker.Name()).To(Equal("opencode plugin configuration"))
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
			Expect(checker.Check(ctx).Status).To(Equal(doctor.StatusSkipped))
		})

		It("skips on a nil config", func() {
			Expect(hook.NewOpenCodeConfigChecker(nil).Check(ctx).Status).
				To(Equal(doctor.StatusSkipped))
		})

		// plugin_path is optional, so an enabled provider without one is a
		// supported setup and must not be reported as misconfigured.
		It("passes with the default path when plugin_path is unset", func() {
			enabled := true
			result := hook.NewOpenCodeConfigChecker(
				&pkgConfig.OpenCodeProviderConfig{Enabled: &enabled},
			).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusPass))
			Expect(result.Message).To(Equal(settings.DefaultOpenCodePluginPath()))
		})

		It("passes when plugin_path is configured", func() {
			result := hook.NewOpenCodeConfigChecker(enabledCfg()).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusPass))
			Expect(result.Message).To(Equal(pluginPath))
		})
	})

	Describe("OpenCodeRegistrationChecker", func() {
		// Skipping here would leave doctor --fix with no fixable opencode
		// result, so the advertised enable-only setup would never install.
		It("checks the default path when plugin_path is not configured", func() {
			GinkgoT().Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, "xdg"))

			enabled := true
			result := hook.NewOpenCodeRegistrationChecker(
				&pkgConfig.OpenCodeProviderConfig{Enabled: &enabled},
			).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.FixID).To(Equal("install_hook"))
		})

		It("passes on the default path once installed there", func() {
			GinkgoT().Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, "xdg"))

			_, err := settings.InstallOpenCodeDispatcher(
				settings.DefaultOpenCodePluginPath(),
				binaryPath,
				settings.OpenCodeAPIV1,
			)
			Expect(err).NotTo(HaveOccurred())

			enabled := true
			result := hook.NewOpenCodeRegistrationChecker(
				&pkgConfig.OpenCodeProviderConfig{Enabled: &enabled},
			).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusPass))
		})

		It("fails with a fixable result when the plugin is absent", func() {
			checker := hook.NewOpenCodeRegistrationChecker(enabledCfg())

			Expect(checker.Name()).To(Equal("Dispatcher registered in opencode plugin"))
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))

			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.FixID).To(Equal("install_hook"))
		})

		It("passes once the bridge plugin is installed", func() {
			_, err := settings.InstallOpenCodeDispatcher(
				pluginPath,
				binaryPath,
				settings.OpenCodeAPIV1,
			)
			Expect(err).NotTo(HaveOccurred())

			result := hook.NewOpenCodeRegistrationChecker(enabledCfg()).Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusPass))
		})

		It("fails when the plugin calls a different binary", func() {
			_, err := settings.InstallOpenCodeDispatcher(
				pluginPath,
				"/elsewhere/klaudiush",
				settings.OpenCodeAPIV1,
			)
			Expect(err).NotTo(HaveOccurred())

			result := hook.NewOpenCodeRegistrationChecker(enabledCfg()).Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusFail))
		})
	})

	Describe("OpenCodeFreshnessChecker", func() {
		It("skips when the plugin is not installed", func() {
			result := hook.NewOpenCodeFreshnessChecker(enabledCfg()).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})

		It("passes for a freshly installed plugin", func() {
			_, err := settings.InstallOpenCodeDispatcher(
				pluginPath,
				binaryPath,
				settings.OpenCodeAPIV1,
			)
			Expect(err).NotTo(HaveOccurred())

			checker := hook.NewOpenCodeFreshnessChecker(enabledCfg())
			Expect(checker.Name()).To(Equal("opencode plugin is up to date"))
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
			Expect(checker.Check(ctx).Status).To(Equal(doctor.StatusPass))
		})

		// A plugin body left over from an older klaudiush still names the right
		// binary and events, so only a content comparison catches it.
		It("flags a stale plugin as fixable", func() {
			_, err := settings.InstallOpenCodeDispatcher(
				pluginPath,
				binaryPath,
				settings.OpenCodeAPIV1,
			)
			Expect(err).NotTo(HaveOccurred())

			current, err := os.ReadFile(pluginPath)
			Expect(err).NotTo(HaveOccurred())

			stale := string(current) + "\n// left over from an older release\n"
			Expect(os.WriteFile(pluginPath, []byte(stale), 0o600)).To(Succeed())

			result := hook.NewOpenCodeFreshnessChecker(enabledCfg()).Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.FixID).To(Equal("install_hook"))
			// doctor --fix only acts on errors, so a warning would be reported
			// and then left in place.
			Expect(result.IsError()).To(BeTrue())
		})

		It("skips when the provider is disabled", func() {
			disabled := false
			result := hook.NewOpenCodeFreshnessChecker(
				&pkgConfig.OpenCodeProviderConfig{Enabled: &disabled},
			).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})
	})

	Describe("OpenCodeEventChecker", func() {
		It("skips when the provider is disabled", func() {
			disabled := false
			result := hook.NewOpenCodeEventChecker(
				&pkgConfig.OpenCodeProviderConfig{Enabled: &disabled},
				"tool.execute.before",
			).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})

		It("checks the default path for events when plugin_path is unset", func() {
			GinkgoT().Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, "xdg"))

			enabled := true
			result := hook.NewOpenCodeEventChecker(
				&pkgConfig.OpenCodeProviderConfig{Enabled: &enabled},
				"tool.execute.before",
			).Check(ctx)

			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.FixID).To(Equal("install_hook"))
		})

		It("fails for every forwarded event when the plugin is absent", func() {
			for _, eventName := range pkgHook.OpenCodeEventNames() {
				checker := hook.NewOpenCodeEventChecker(enabledCfg(), eventName)

				Expect(checker.Name()).To(Equal(eventName + " hook in opencode plugin"))
				Expect(checker.Category()).To(Equal(doctor.CategoryHook))

				result := checker.Check(ctx)
				Expect(result.Status).To(Equal(doctor.StatusFail), "event %s", eventName)
				Expect(result.FixID).To(Equal("install_hook"), "event %s", eventName)
			}
		})

		It("passes for every forwarded event once installed", func() {
			_, err := settings.InstallOpenCodeDispatcher(
				pluginPath,
				binaryPath,
				settings.OpenCodeAPIV1,
			)
			Expect(err).NotTo(HaveOccurred())

			for _, eventName := range pkgHook.OpenCodeEventNames() {
				result := hook.NewOpenCodeEventChecker(enabledCfg(), eventName).Check(ctx)
				Expect(result.Status).To(Equal(doctor.StatusPass), "event %s", eventName)
			}
		})

		// The plugin names permission.ask in a comment but does not subscribe
		// to it, so a mention must not be reported as configured.
		It("does not pass an event the plugin only mentions", func() {
			_, err := settings.InstallOpenCodeDispatcher(
				pluginPath,
				binaryPath,
				settings.OpenCodeAPIV1,
			)
			Expect(err).NotTo(HaveOccurred())

			result := hook.NewOpenCodeEventChecker(enabledCfg(), "permission.ask").Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusFail))
		})
	})
})
