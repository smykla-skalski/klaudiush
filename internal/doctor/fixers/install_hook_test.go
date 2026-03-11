package fixers

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-skalski/klaudiush/internal/prompt"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("InstallHookFixer", func() {
	var (
		ctrl         *gomock.Controller
		mockPrompt   *prompt.MockPrompter
		tempDir      string
		hooksPath    string
		originalPath string
		pathSet      bool
	)

	BeforeEach(func() {
		var err error

		ctrl = gomock.NewController(GinkgoT())
		mockPrompt = prompt.NewMockPrompter(ctrl)

		tempDir, err = os.MkdirTemp("", "install-hook-fixer-*")
		Expect(err).NotTo(HaveOccurred())

		hooksPath = filepath.Join(tempDir, ".codex", "hooks.json")
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
		ctrl.Finish()

		if pathSet {
			Expect(os.Setenv("PATH", originalPath)).To(Succeed())
		} else {
			Expect(os.Unsetenv("PATH")).To(Succeed())
		}

		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	It("installs configured Codex hooks", func() {
		claudeEnabled := false
		codexEnabled := true
		codexExperimental := true
		cfg := &pkgConfig.Config{
			Providers: &pkgConfig.ProvidersConfig{
				Claude: &pkgConfig.ClaudeProviderConfig{Enabled: &claudeEnabled},
				Codex: &pkgConfig.CodexProviderConfig{
					Enabled:         &codexEnabled,
					Experimental:    &codexExperimental,
					HooksConfigPath: hooksPath,
				},
			},
		}

		fixer := NewInstallHookFixer(mockPrompt, cfg)
		Expect(fixer.Fix(context.Background(), false)).To(Succeed())
		Expect(hooksPath).To(BeAnExistingFile())
	})
})
