package ruleschecker

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/pkg/config"
)

func TestRulesChecker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rules Checker Suite")
}

//go:generate mockgen -source=rules_check.go -destination=rules_check_mock.go -package=ruleschecker

var _ = Describe("RulesChecker", func() {
	var (
		ctrl       *gomock.Controller
		mockLoader *MockConfigLoader
		checker    *RulesChecker
		ctx        context.Context
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockLoader = NewMockConfigLoader(ctrl)
		checker = NewRulesCheckerWithLoader(mockLoader)
		ctx = context.Background()
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("Name and Category", func() {
		It("should return correct name", func() {
			Expect(checker.Name()).To(Equal("Rules validation"))
		})

		It("should return config category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryConfig))
		})
	})

	Describe("Check", func() {
		Context("when no project config exists", func() {
			It("should skip", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(false)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusSkipped))
				Expect(result.Message).To(ContainSubstring("No project config"))
			})
		})

		Context("when config load fails", func() {
			It("should skip with error reference", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).Return(nil, context.DeadlineExceeded)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusSkipped))
				Expect(result.Message).To(ContainSubstring("Config load failed"))
			})
		})

		Context("when no rules configured", func() {
			It("should pass with nil rules config", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).
					Return(&config.Config{Rules: nil}, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusPass))
				Expect(result.Message).To(ContainSubstring("No rules configured"))
			})

			It("should pass with empty rules slice", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).Return(&config.Config{
					Rules: &config.RulesConfig{Rules: []config.RuleConfig{}},
				}, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusPass))
				Expect(result.Message).To(ContainSubstring("No rules configured"))
			})
		})

		Context("when rules are valid", func() {
			It("should pass with valid rules", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).Return(&config.Config{
					Rules: &config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name: "valid-rule",
								Match: &config.RuleMatchConfig{
									ValidatorType: "git.push",
								},
								Action: &config.RuleActionConfig{
									Type: "block",
								},
							},
						},
					},
				}, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusPass))
				Expect(result.Message).To(ContainSubstring("1 rule(s) validated"))
			})
		})

		Context("when rules have issues", func() {
			It("should fail when rule has no match section", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).Return(&config.Config{
					Rules: &config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name:  "no-match-rule",
								Match: nil,
								Action: &config.RuleActionConfig{
									Type: "block",
								},
							},
						},
					},
				}, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Severity).To(Equal(doctor.SeverityError))
				Expect(result.Details).To(ContainElement(ContainSubstring("missing match section")))
				Expect(result.FixID).To(Equal("fix_invalid_rules"))
			})

			It("should fail when match section is empty", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).Return(&config.Config{
					Rules: &config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name:  "empty-match-rule",
								Match: &config.RuleMatchConfig{},
								Action: &config.RuleActionConfig{
									Type: "block",
								},
							},
						},
					},
				}, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Details).To(ContainElement(ContainSubstring("empty")))
				Expect(result.FixID).To(Equal("fix_invalid_rules"))
			})

			It("should fail when tool_type is invalid", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).Return(&config.Config{
					Rules: &config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name: "invalid-tool-type",
								Match: &config.RuleMatchConfig{
									ToolType: "InvalidTool",
								},
							},
						},
					},
				}, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Details).To(ContainElement(ContainSubstring("invalid tool_type")))
				Expect(result.FixID).To(Equal("fix_invalid_rules"))
			})

			It("should fail when event_type is invalid", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).Return(&config.Config{
					Rules: &config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name: "invalid-event-type",
								Match: &config.RuleMatchConfig{
									EventType: "InvalidEvent",
								},
							},
						},
					},
				}, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Details).To(ContainElement(ContainSubstring("invalid event_type")))
			})

			It("should fail when action type is invalid", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).Return(&config.Config{
					Rules: &config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name: "invalid-action-type",
								Match: &config.RuleMatchConfig{
									ValidatorType: "git.push",
								},
								Action: &config.RuleActionConfig{
									Type: "invalid-action",
								},
							},
						},
					},
				}, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Details).To(ContainElement(ContainSubstring("invalid action type")))
			})

			It("should report multiple issues", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).Return(&config.Config{
					Rules: &config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name:  "no-match",
								Match: nil,
							},
							{
								Name: "invalid-tool",
								Match: &config.RuleMatchConfig{
									ToolType: "BadTool",
								},
							},
						},
					},
				}, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Message).To(ContainSubstring("2 invalid rule(s)"))
				Expect(len(result.Details)).To(BeNumerically(">=", 2))
			})

			It("should use rule name in details when available", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).Return(&config.Config{
					Rules: &config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name:  "named-rule",
								Match: nil,
							},
						},
					},
				}, nil)

				result := checker.Check(ctx)

				Expect(result.Details).To(ContainElement(ContainSubstring(`"named-rule"`)))
			})

			It("should use rule index when name is empty", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().LoadWithoutValidation(nil).Return(&config.Config{
					Rules: &config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name:  "",
								Match: nil,
							},
						},
					},
				}, nil)

				result := checker.Check(ctx)

				Expect(result.Details).To(ContainElement(ContainSubstring("Rule #1")))
			})
		})
	})

	Describe("GetIssues", func() {
		It("should return issues found during check", func() {
			mockLoader.EXPECT().HasProjectConfig().Return(true)
			mockLoader.EXPECT().LoadWithoutValidation(nil).Return(&config.Config{
				Rules: &config.RulesConfig{
					Rules: []config.RuleConfig{
						{Name: "bad-rule", Match: nil},
					},
				},
			}, nil)

			checker.Check(ctx)
			issues := checker.GetIssues()

			Expect(issues).To(HaveLen(1))
			Expect(issues[0].RuleName).To(Equal("bad-rule"))
			Expect(issues[0].IssueType).To(Equal("no_match_section"))
			Expect(issues[0].Fixable).To(BeTrue())
		})
	})
})

var _ = Describe("RuleMatchConfig.HasMatchConditions", func() {
	It("should return false for nil match", func() {
		var match *config.RuleMatchConfig
		Expect(match.HasMatchConditions()).To(BeFalse())
	})

	It("should return false for empty match", func() {
		match := &config.RuleMatchConfig{}
		Expect(match.HasMatchConditions()).To(BeFalse())
	})

	It("should return true when ValidatorType is set", func() {
		match := &config.RuleMatchConfig{ValidatorType: "git.push"}
		Expect(match.HasMatchConditions()).To(BeTrue())
	})

	It("should return true when ToolType is set", func() {
		match := &config.RuleMatchConfig{ToolType: "Bash"}
		Expect(match.HasMatchConditions()).To(BeTrue())
	})

	It("should return true when EventType is set", func() {
		match := &config.RuleMatchConfig{EventType: "PreToolUse"}
		Expect(match.HasMatchConditions()).To(BeTrue())
	})

	It("should return true when FilePattern is set", func() {
		match := &config.RuleMatchConfig{FilePattern: "*.go"}
		Expect(match.HasMatchConditions()).To(BeTrue())
	})

	It("should return true when CommandPattern is set", func() {
		match := &config.RuleMatchConfig{CommandPattern: "git push*"}
		Expect(match.HasMatchConditions()).To(BeTrue())
	})
})
