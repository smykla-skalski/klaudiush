package exceptions_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/exceptions"
)

var _ = Describe("Token Parser", func() {
	var parser *exceptions.Parser

	BeforeEach(func() {
		parser = exceptions.NewParser()
	})

	Describe("NewParser", func() {
		It("creates parser with default options", func() {
			p := exceptions.NewParser()
			Expect(p).NotTo(BeNil())
		})

		It("accepts custom token prefix", func() {
			p := exceptions.NewParser(exceptions.WithTokenPrefix("ACK"))
			result, err := p.Parse("git push # ACK:GIT022:reason")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Found).To(BeTrue())
			Expect(result.Token.Prefix).To(Equal("ACK"))
		})

		It("accepts custom env var name", func() {
			p := exceptions.NewParser(exceptions.WithEnvVarName("MY_ACK"))
			result, err := p.Parse(`MY_ACK="EXC:GIT022:reason" git push`)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Found).To(BeTrue())
		})
	})

	Describe("Parse", func() {
		Context("with empty input", func() {
			It("returns error for empty command", func() {
				_, err := parser.Parse("")
				Expect(err).To(MatchError(exceptions.ErrEmptyCommand))
			})

			It("returns error for whitespace-only command", func() {
				_, err := parser.Parse("   \t\n")
				Expect(err).To(MatchError(exceptions.ErrEmptyCommand))
			})
		})

		Context("with no token", func() {
			It("returns not found for simple command", func() {
				result, err := parser.Parse("git push origin main")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeFalse())
				Expect(result.Token).To(BeNil())
			})

			It("returns not found for command with unrelated comment", func() {
				result, err := parser.Parse("git push # this is a regular comment")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeFalse())
			})

			It("returns not found for command with unrelated env var", func() {
				result, err := parser.Parse(`FOO="bar" git push`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeFalse())
			})
		})

		Context("with token in comment", func() {
			It("parses simple token without reason", func() {
				result, err := parser.Parse("git push origin main # EXC:GIT022")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Source).To(Equal(exceptions.TokenSourceComment))
				Expect(result.Token.Prefix).To(Equal("EXC"))
				Expect(result.Token.ErrorCode).To(Equal("GIT022"))
				Expect(result.Token.Reason).To(BeEmpty())
			})

			It("parses token with reason", func() {
				result, err := parser.Parse("git push origin main # EXC:GIT022:Emergency+hotfix")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.ErrorCode).To(Equal("GIT022"))
				Expect(result.Token.Reason).To(Equal("Emergency hotfix"))
			})

			It("parses token with URL-encoded reason", func() {
				result, err := parser.Parse("git push # EXC:SEC001:Test%20fixture%20data")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Reason).To(Equal("Test fixture data"))
			})

			It("parses token with complex URL-encoded reason", func() {
				result, err := parser.Parse("git push # EXC:GIT022:Emergency%3A+production%21")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Reason).To(Equal("Emergency: production!"))
			})

			It("handles token at start of comment", func() {
				result, err := parser.Parse("git push #EXC:GIT022:reason")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.ErrorCode).To(Equal("GIT022"))
			})

			It("handles token with extra text after", func() {
				result, err := parser.Parse("git push # EXC:GIT022:reason more comment text")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.ErrorCode).To(Equal("GIT022"))
				Expect(result.Token.Reason).To(Equal("reason"))
			})
		})

		Context("with token in environment variable", func() {
			It("parses token from KLACK", func() {
				result, err := parser.Parse(
					`KLACK="EXC:SEC001:Test+fixture" git commit -sS -m "msg"`,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Source).To(Equal(exceptions.TokenSourceEnvVar))
				Expect(result.Token.ErrorCode).To(Equal("SEC001"))
				Expect(result.Token.Reason).To(Equal("Test fixture"))
			})

			It("parses token without quotes", func() {
				result, err := parser.Parse(`KLACK=EXC:GIT022:reason git push`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.ErrorCode).To(Equal("GIT022"))
			})

			It("parses token with single quotes", func() {
				result, err := parser.Parse(`KLACK='EXC:FILE001:reason' touch file.txt`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.ErrorCode).To(Equal("FILE001"))
			})

			It("ignores other environment variables", func() {
				result, err := parser.Parse(`OTHER_VAR="EXC:GIT022:reason" git push`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeFalse())
			})
		})

		Context("with env var priority over comment", func() {
			It("prefers env var when both present", func() {
				result, err := parser.Parse(
					`KLACK="EXC:SEC001:env" git push # EXC:GIT022:comment`,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Source).To(Equal(exceptions.TokenSourceEnvVar))
				Expect(result.Token.ErrorCode).To(Equal("SEC001"))
			})
		})

		Context("with error codes", func() {
			DescribeTable("accepts valid error codes",
				func(code string) {
					result, err := parser.Parse("git push # EXC:" + code + ":reason")
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Found).To(BeTrue())
					Expect(result.Token.ErrorCode).To(Equal(code))
				},
				Entry("GIT code", "GIT001"),
				Entry("GIT larger code", "GIT022"),
				Entry("SEC code", "SEC001"),
				Entry("FILE code", "FILE005"),
				Entry("SHELL code", "SHELL001"),
				Entry("long prefix", "VALIDATION12345"),
				Entry("short prefix", "AB1"),
			)

			DescribeTable("rejects invalid error codes",
				func(code string) {
					result, err := parser.Parse("git push # EXC:" + code + ":reason")
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Found).To(BeFalse())
				},
				Entry("lowercase", "git001"),
				Entry("no numbers", "GITABC"),
				Entry("only numbers", "12345"),
				Entry("single letter", "A1"),
				Entry("too many numbers", "GIT123456"),
				Entry("special chars", "GIT@01"),
				Entry("empty", ""),
			)
		})

		Context("with chained commands", func() {
			It("finds token in first command", func() {
				result, err := parser.Parse("git add . # EXC:GIT001:reason && git commit -m msg")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.ErrorCode).To(Equal("GIT001"))
			})

			It("finds token in second command", func() {
				result, err := parser.Parse("git add . && git push # EXC:GIT022:reason")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.ErrorCode).To(Equal("GIT022"))
			})

			It("finds token in env var before chain", func() {
				result, err := parser.Parse(`KLACK="EXC:GIT022" git add . && git push`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.ErrorCode).To(Equal("GIT022"))
			})
		})

		Context("with complex commands", func() {
			It("parses token in subshell", func() {
				result, err := parser.Parse("(git push) # EXC:GIT022:reason")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
			})

			It("parses token with heredoc", func() {
				cmd := `git commit -m "$(cat <<'EOF'
message
EOF
)" # EXC:GIT001:reason`
				result, err := parser.Parse(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
			})

			It("parses token with pipe", func() {
				result, err := parser.Parse("echo test | git commit --amend # EXC:GIT001:reason")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
			})
		})
	})

	Describe("Token Raw field", func() {
		It("contains the original token string", func() {
			result, err := parser.Parse("git push # EXC:GIT022:Emergency+hotfix")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Token.Raw).To(Equal("EXC:GIT022:Emergency+hotfix"))
		})

		It("contains encoded reason in raw", func() {
			result, err := parser.Parse("git push # EXC:GIT022:Test%20data")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Token.Raw).To(Equal("EXC:GIT022:Test%20data"))
			Expect(result.Token.Reason).To(Equal("Test data"))
		})
	})

	Describe("TokenSource String()", func() {
		It("returns correct string for comment source", func() {
			Expect(exceptions.TokenSourceComment.String()).To(Equal("comment"))
		})

		It("returns correct string for env var source", func() {
			Expect(exceptions.TokenSourceEnvVar.String()).To(Equal("env_var"))
		})

		It("returns unknown for invalid source", func() {
			Expect(exceptions.TokenSourceUnknown.String()).To(Equal("unknown"))
		})
	})
})
