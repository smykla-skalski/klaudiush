package main

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-skalski/klaudiush/internal/prompt"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("init provider updates", func() {
	Describe("resolveProviderSelection", func() {
		It("defaults the Codex hooks path when Codex is selected by flags", func() {
			selection, err := resolveProviderSelection(
				[]string{"claude", "codex"},
				"",
				nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(selection.ClaudeEnabled).To(BeTrue())
			Expect(selection.CodexEnabled).To(BeTrue())
			Expect(selection.CodexHooksPath).To(Equal(defaultCodexHooksPath))
		})

		It("rejects unknown provider names", func() {
			_, err := resolveProviderSelection([]string{"claude", "cursor"}, "", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown provider"))
		})
	})

	Describe("applyProviderSelection", func() {
		It("updates providers without mutating unrelated config", func() {
			bellEnabled := true
			existing := &pkgConfig.Config{
				Version: pkgConfig.CurrentConfigVersion,
				Validators: &pkgConfig.ValidatorsConfig{
					Notification: &pkgConfig.NotificationConfig{
						Bell: &pkgConfig.BellValidatorConfig{
							ValidatorConfig: pkgConfig.ValidatorConfig{
								Enabled: &bellEnabled,
							},
						},
					},
				},
			}

			updated, err := applyProviderSelection(existing, providerSelection{
				ClaudeEnabled:  true,
				CodexEnabled:   true,
				CodexHooksPath: defaultCodexHooksPath,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.GetProviders().GetClaude().IsEnabled()).To(BeTrue())
			Expect(updated.GetProviders().GetCodex().IsEnabled()).To(BeTrue())
			Expect(updated.GetProviders().GetCodex().HooksConfigPath).
				To(Equal(defaultCodexHooksPath))
			Expect(updated.GetValidators().GetNotification().Bell.Enabled).
				To(Equal(existing.GetValidators().GetNotification().Bell.Enabled))
			Expect(existing.Providers).To(BeNil())
		})
	})

	Describe("promptProviderUpdate", func() {
		It("shows a diff and returns an approved provider-only patch", func() {
			ctrl := gomock.NewController(GinkgoT())
			defer ctrl.Finish()

			prompter := prompt.NewMockPrompter(ctrl)

			var out bytes.Buffer

			existing := &pkgConfig.Config{
				Version: pkgConfig.CurrentConfigVersion,
				Validators: &pkgConfig.ValidatorsConfig{
					Notification: &pkgConfig.NotificationConfig{
						Bell: &pkgConfig.BellValidatorConfig{},
					},
				},
			}

			gomock.InOrder(
				prompter.EXPECT().
					Confirm("Configuration already exists. Configure provider integrations only?", true).
					Return(true, nil),
				prompter.EXPECT().
					Confirm("Enable Claude integration", true).
					Return(true, nil),
				prompter.EXPECT().
					Confirm("Enable Codex integration", false).
					Return(true, nil),
				prompter.EXPECT().
					Input("Codex hooks.json path", defaultCodexHooksPath).
					Return(defaultCodexHooksPath, nil),
				prompter.EXPECT().
					Confirm("Apply these changes?", false).
					Return(true, nil),
			)

			updated, handled, err := promptProviderUpdate(
				prompter,
				&out,
				"/tmp/config.toml",
				existing,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(handled).To(BeTrue())
			Expect(updated.GetProviders().GetClaude().IsEnabled()).To(BeTrue())
			Expect(updated.GetProviders().GetCodex().IsEnabled()).To(BeTrue())
			Expect(updated.GetProviders().GetCodex().HooksConfigPath).
				To(Equal(defaultCodexHooksPath))
			Expect(out.String()).
				To(ContainSubstring("Proposed changes for /tmp/config.toml"))
			Expect(out.String()).To(ContainSubstring("[providers]"))
			Expect(out.String()).To(ContainSubstring("hooks_config_path"))
		})
	})
})
