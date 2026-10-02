package failpolicy_test

import (
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("Policy", func() {
	Describe("ParseMode", func() {
		DescribeTable("parses modes",
			func(mode string, want failpolicy.Action) {
				action, err := failpolicy.ParseMode(mode)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(want))
			},
			Entry("warn", "warn", failpolicy.ActionWarn),
			Entry("block", "block", failpolicy.ActionBlock),
			Entry("mixed case and spaces", " Block ", failpolicy.ActionBlock),
		)

		It("rejects anything else", func() {
			_, err := failpolicy.ParseMode("ignore")
			Expect(errors.Is(err, failpolicy.ErrInvalidMode)).To(BeTrue())
		})
	})

	It("spells actions as in config", func() {
		Expect(failpolicy.ActionIgnore.String()).To(Equal("ignore"))
		Expect(failpolicy.ActionWarn.String()).To(Equal("warn"))
		Expect(failpolicy.ActionBlock.String()).To(Equal("block"))
		Expect(failpolicy.Action(42).String()).To(Equal("warn"))
	})

	DescribeTable("normalizes validator names",
		func(name, want string) {
			Expect(failpolicy.NormalizeName(name)).To(Equal(want))
		},
		Entry("runtime name", "validate-commit", "commit"),
		Entry("short name", "Commit", "commit"),
		Entry("override name", "git.commit", "commit"),
		Entry("file override name", "file.workflow", "github-workflow"),
		Entry("plugins", "plugins", "plugin-registry"),
		Entry("unknown", " custom ", "custom"),
	)

	Describe("Resolve", func() {
		It("keeps each check's choice and ignores missing tools by default", func() {
			for _, policy := range []*failpolicy.Policy{nil, failpolicy.New(nil)} {
				Expect(policy.Resolve("validate-shellscript", validator.ReasonTimeout, false)).
					To(Equal(failpolicy.ActionWarn))
				Expect(policy.Resolve("plugin-registry", validator.ReasonError, true)).
					To(Equal(failpolicy.ActionBlock))
				Expect(policy.Resolve("validate-shellscript", validator.ReasonMissingTool, true)).
					To(Equal(failpolicy.ActionIgnore))
				Expect(policy.Mode()).To(Equal(failpolicy.ActionWarn))
				Expect(policy.ModeSet()).To(BeFalse())
			}
		})

		It("applies the mode to every unavailable check", func() {
			block := failpolicy.New(&config.FailurePolicyConfig{Mode: "block"})
			Expect(block.Resolve("validate-shellscript", validator.ReasonTimeout, false)).
				To(Equal(failpolicy.ActionBlock))
			Expect(block.Mode()).To(Equal(failpolicy.ActionBlock))

			warn := failpolicy.New(&config.FailurePolicyConfig{Mode: "warn"})
			Expect(warn.Resolve("plugin-registry", validator.ReasonError, true)).
				To(Equal(failpolicy.ActionWarn))
		})

		It("ignores an invalid mode", func() {
			policy := failpolicy.New(&config.FailurePolicyConfig{Mode: "nope"})
			Expect(policy.ModeSet()).To(BeFalse())
		})

		It("leaves missing tools to missing_tools even in block mode", func() {
			policy := failpolicy.New(&config.FailurePolicyConfig{Mode: "block"})
			Expect(policy.Resolve("validate-rust", validator.ReasonMissingTool, false)).
				To(Equal(failpolicy.ActionIgnore))

			policy = failpolicy.New(&config.FailurePolicyConfig{MissingTools: "warn"})
			Expect(policy.Resolve("validate-rust", validator.ReasonMissingTool, false)).
				To(Equal(failpolicy.ActionWarn))

			policy = failpolicy.New(&config.FailurePolicyConfig{MissingTools: "block"})
			Expect(policy.Resolve("validate-rust", validator.ReasonMissingTool, false)).
				To(Equal(failpolicy.ActionBlock))
		})

		It("blocks critical validators for every reason", func() {
			policy := failpolicy.New(&config.FailurePolicyConfig{
				Mode:     "warn",
				Critical: []string{"file.shellscript", "", "commit"},
			})

			Expect(policy.IsCritical("validate-shellscript")).To(BeTrue())
			Expect(policy.IsCritical("validate-commit")).To(BeTrue())
			Expect(policy.IsCritical("validate-rust")).To(BeFalse())
			Expect(policy.Critical()).To(ConsistOf("shellscript", "commit"))
			Expect(policy.Resolve("validate-shellscript", validator.ReasonMissingTool, false)).
				To(Equal(failpolicy.ActionBlock))
			Expect(policy.Resolve("validate-rust", validator.ReasonTimeout, false)).
				To(Equal(failpolicy.ActionWarn))
		})

		It("lets WithMode override the configured mode", func() {
			policy := failpolicy.New(&config.FailurePolicyConfig{Mode: "warn"}).
				WithMode(failpolicy.ActionBlock)

			Expect(policy.Mode()).To(Equal(failpolicy.ActionBlock))
		})
	})

	Describe("Deadline", func() {
		It("defaults below the registered hook timeout", func() {
			var policy *failpolicy.Policy
			Expect(policy.Deadline()).To(Equal(config.DefaultFailureDeadline))
			Expect(failpolicy.New(nil).Deadline()).To(Equal(config.DefaultFailureDeadline))
			Expect(policy.Critical()).To(BeNil())
		})

		It("uses the configured deadline", func() {
			policy := failpolicy.New(&config.FailurePolicyConfig{
				Deadline: config.Duration(5 * time.Second),
			})
			Expect(policy.Deadline()).To(Equal(5 * time.Second))
			Expect((&failpolicy.Policy{}).Deadline()).To(Equal(config.DefaultFailureDeadline))
		})
	})
})
