package session_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/session"
)

var _ = Describe("Unpoison Parser", func() {
	var parser *session.UnpoisonParser

	BeforeEach(func() {
		parser = session.NewUnpoisonParser()
	})

	Describe("NewUnpoisonParser", func() {
		It("creates parser with default options", func() {
			p := session.NewUnpoisonParser()
			Expect(p).NotTo(BeNil())
		})

		It("accepts custom token prefix", func() {
			p := session.NewUnpoisonParser(session.WithUnpoisonPrefix("ACK"))
			result, err := p.Parse("git push # ACK:GIT022")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Found).To(BeTrue())
			Expect(result.Token.Raw).To(HavePrefix("ACK:"))
		})

		It("accepts custom env var name", func() {
			p := session.NewUnpoisonParser(session.WithUnpoisonEnvVar("MY_ACK"))
			result, err := p.Parse(`MY_ACK="SESS:GIT022" git push`)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Found).To(BeTrue())
		})
	})

	Describe("Parse", func() {
		Context("with empty input", func() {
			It("returns error for empty command", func() {
				_, err := parser.Parse("")
				Expect(err).To(MatchError(session.ErrUnpoisonEmptyCommand))
			})

			It("returns error for whitespace-only command", func() {
				_, err := parser.Parse("   \t\n")
				Expect(err).To(MatchError(session.ErrUnpoisonEmptyCommand))
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

			It("returns not found for EXC token (wrong prefix)", func() {
				result, err := parser.Parse("git push # EXC:GIT022:reason")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeFalse())
			})
		})

		Context("with token in comment", func() {
			It("parses single code", func() {
				result, err := parser.Parse("git push origin main # SESS:GIT022")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Source).To(Equal(session.UnpoisonTokenSourceComment))
				Expect(result.Token.Codes).To(ConsistOf("GIT022"))
			})

			It("parses multiple comma-separated codes", func() {
				result, err := parser.Parse("git push origin main # SESS:GIT001,GIT002,SEC001")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Codes).To(ConsistOf("GIT001", "GIT002", "SEC001"))
			})

			It("parses two codes", func() {
				result, err := parser.Parse("git push # SESS:GIT001,GIT002")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Codes).To(HaveLen(2))
				Expect(result.Token.Codes).To(ContainElements("GIT001", "GIT002"))
			})

			It("handles token at start of comment", func() {
				result, err := parser.Parse("git push #SESS:GIT022")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Codes).To(ConsistOf("GIT022"))
			})

			It("handles token with extra text after", func() {
				result, err := parser.Parse("git push # SESS:GIT022,SEC001 more comment text")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Codes).To(ConsistOf("GIT022", "SEC001"))
			})

			It("handles whitespace around codes", func() {
				result, err := parser.Parse("git push # SESS:GIT022, SEC001")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				// Note: spaces after comma break the token at "more comment"
				// This tests that we only get "GIT022," parsed
				Expect(result.Token.Codes).To(ConsistOf("GIT022"))
			})
		})

		Context("with token in environment variable", func() {
			It("parses token from KLACK", func() {
				result, err := parser.Parse(
					`KLACK="SESS:SEC001" git commit -sS -m "msg"`,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Source).To(Equal(session.UnpoisonTokenSourceEnvVar))
				Expect(result.Token.Codes).To(ConsistOf("SEC001"))
			})

			It("parses multiple codes from env var", func() {
				result, err := parser.Parse(
					`KLACK="SESS:GIT001,GIT002,SEC001" git push`,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Codes).To(ConsistOf("GIT001", "GIT002", "SEC001"))
			})

			It("parses token without quotes", func() {
				result, err := parser.Parse(`KLACK=SESS:GIT022 git push`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Codes).To(ConsistOf("GIT022"))
			})

			It("parses token with single quotes", func() {
				result, err := parser.Parse(`KLACK='SESS:FILE001' touch file.txt`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Codes).To(ConsistOf("FILE001"))
			})

			It("ignores other environment variables", func() {
				result, err := parser.Parse(`OTHER_VAR="SESS:GIT022" git push`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeFalse())
			})

			It("ignores old KLAUDIUSH_ACK env var", func() {
				result, err := parser.Parse(`KLAUDIUSH_ACK="SESS:GIT022" git push`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeFalse())
			})
		})

		Context("with env var priority over comment", func() {
			It("prefers env var when both present", func() {
				result, err := parser.Parse(
					`KLACK="SESS:SEC001,SEC002" git push # SESS:GIT022`,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Source).To(Equal(session.UnpoisonTokenSourceEnvVar))
				Expect(result.Token.Codes).To(ConsistOf("SEC001", "SEC002"))
			})
		})

		Context("with error codes", func() {
			DescribeTable("accepts valid error codes",
				func(code string) {
					result, err := parser.Parse("git push # SESS:" + code)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Found).To(BeTrue())
					Expect(result.Token.Codes).To(ContainElement(code))
				},
				Entry("GIT code", "GIT001"),
				Entry("GIT larger code", "GIT022"),
				Entry("SEC code", "SEC001"),
				Entry("FILE code", "FILE005"),
				Entry("SHELL code", "SHELL001"),
				Entry("SESS code", "SESS001"),
				Entry("long prefix", "VALIDATION12345"),
				Entry("short prefix", "AB1"),
			)

			DescribeTable("rejects invalid error codes",
				func(code string) {
					result, err := parser.Parse("git push # SESS:" + code)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Found).To(BeFalse())
				},
				Entry("lowercase", "git001"),
				Entry("no numbers", "GITABC"),
				Entry("only numbers", "12345"),
				Entry("single letter", "A1"),
				Entry("too many numbers", "GIT123456"),
				Entry("special chars", "GIT@01"),
			)

			It("rejects token with empty codes", func() {
				result, err := parser.Parse("git push # SESS:")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeFalse())
			})
		})

		Context("with chained commands", func() {
			It("finds token in first command", func() {
				result, err := parser.Parse("git add . # SESS:GIT001 && git commit -m msg")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Codes).To(ConsistOf("GIT001"))
			})

			It("finds token in second command", func() {
				result, err := parser.Parse("git add . && git push # SESS:GIT022")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Codes).To(ConsistOf("GIT022"))
			})

			It("finds token in env var before chain", func() {
				result, err := parser.Parse(`KLACK="SESS:GIT022" git add . && git push`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
				Expect(result.Token.Codes).To(ConsistOf("GIT022"))
			})
		})

		Context("with complex commands", func() {
			It("parses token in subshell", func() {
				result, err := parser.Parse("(git push) # SESS:GIT022")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
			})

			It("parses token with heredoc", func() {
				cmd := `git commit -m "$(cat <<'EOF'
message
EOF
)" # SESS:GIT001`
				result, err := parser.Parse(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
			})

			It("parses token with pipe", func() {
				result, err := parser.Parse("echo test | git commit --amend # SESS:GIT001")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Found).To(BeTrue())
			})
		})
	})

	Describe("Token Raw field", func() {
		It("contains the original token string", func() {
			result, err := parser.Parse("git push # SESS:GIT022")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Token.Raw).To(Equal("SESS:GIT022"))
		})

		It("contains multiple codes in raw", func() {
			result, err := parser.Parse("git push # SESS:GIT001,GIT002")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Token.Raw).To(Equal("SESS:GIT001,GIT002"))
		})
	})

	Describe("UnpoisonTokenSource String()", func() {
		It("returns correct string for comment source", func() {
			Expect(session.UnpoisonTokenSourceComment.String()).To(Equal("comment"))
		})

		It("returns correct string for env var source", func() {
			Expect(session.UnpoisonTokenSourceEnvVar.String()).To(Equal("env_var"))
		})

		It("returns unknown for invalid source", func() {
			var invalidSource session.UnpoisonTokenSource = 999
			Expect(invalidSource.String()).To(Equal("unknown"))
		})
	})
})

var _ = Describe("CheckUnpoisonAcknowledgment", func() {
	Context("with empty poison codes", func() {
		It("returns acknowledged for nil codes", func() {
			acked, unacked, err := session.CheckUnpoisonAcknowledgment("git push", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(acked).To(BeTrue())
			Expect(unacked).To(BeNil())
		})

		It("returns acknowledged for empty codes slice", func() {
			acked, unacked, err := session.CheckUnpoisonAcknowledgment("git push", []string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(acked).To(BeTrue())
			Expect(unacked).To(BeNil())
		})
	})

	Context("with no token in command", func() {
		It("returns not acknowledged with all codes", func() {
			poisonCodes := []string{"GIT001", "GIT002"}
			acked, unacked, err := session.CheckUnpoisonAcknowledgment("git push", poisonCodes)
			Expect(err).NotTo(HaveOccurred())
			Expect(acked).To(BeFalse())
			Expect(unacked).To(ConsistOf("GIT001", "GIT002"))
		})
	})

	Context("with full acknowledgment", func() {
		It("acknowledges single code", func() {
			poisonCodes := []string{"GIT001"}
			acked, unacked, err := session.CheckUnpoisonAcknowledgment(
				"git push # SESS:GIT001",
				poisonCodes,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(acked).To(BeTrue())
			Expect(unacked).To(BeNil())
		})

		It("acknowledges multiple codes", func() {
			poisonCodes := []string{"GIT001", "GIT002", "SEC001"}
			acked, unacked, err := session.CheckUnpoisonAcknowledgment(
				"git push # SESS:GIT001,GIT002,SEC001",
				poisonCodes,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(acked).To(BeTrue())
			Expect(unacked).To(BeNil())
		})

		It("accepts superset of acknowledgment codes", func() {
			poisonCodes := []string{"GIT001", "GIT002"}
			acked, unacked, err := session.CheckUnpoisonAcknowledgment(
				"git push # SESS:GIT001,GIT002,SEC001,FILE001",
				poisonCodes,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(acked).To(BeTrue())
			Expect(unacked).To(BeNil())
		})
	})

	Context("with partial acknowledgment", func() {
		It("returns unacknowledged codes for partial match", func() {
			poisonCodes := []string{"GIT001", "GIT002", "SEC001"}
			acked, unacked, err := session.CheckUnpoisonAcknowledgment(
				"git push # SESS:GIT001",
				poisonCodes,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(acked).To(BeFalse())
			Expect(unacked).To(ConsistOf("GIT002", "SEC001"))
		})

		It("returns single unacknowledged code", func() {
			poisonCodes := []string{"GIT001", "GIT002"}
			acked, unacked, err := session.CheckUnpoisonAcknowledgment(
				"git push # SESS:GIT001",
				poisonCodes,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(acked).To(BeFalse())
			Expect(unacked).To(ConsistOf("GIT002"))
		})
	})

	Context("with env var token", func() {
		It("acknowledges via KLACK env var", func() {
			poisonCodes := []string{"GIT001", "SEC001"}
			acked, unacked, err := session.CheckUnpoisonAcknowledgment(
				`KLACK="SESS:GIT001,SEC001" git push`,
				poisonCodes,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(acked).To(BeTrue())
			Expect(unacked).To(BeNil())
		})
	})

	Context("with invalid command", func() {
		It("returns error for empty command", func() {
			poisonCodes := []string{"GIT001"}
			acked, unacked, err := session.CheckUnpoisonAcknowledgment("", poisonCodes)
			Expect(err).To(HaveOccurred())
			Expect(acked).To(BeFalse())
			Expect(unacked).To(ConsistOf("GIT001"))
		})
	})
})
