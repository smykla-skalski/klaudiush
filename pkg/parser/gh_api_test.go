package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// parseFirstGHCommand parses a shell command and returns the first gh command.
func parseFirstGHCommand(command string) parser.Command {
	GinkgoHelper()

	result, err := parser.NewBashParser().Parse(command)
	Expect(err).NotTo(HaveOccurred())

	commands := result.GetCommands("gh")
	Expect(commands).NotTo(BeEmpty())

	return commands[0]
}

var _ = Describe("ParseGHAPICommand", func() {
	DescribeTable(
		"resolves the endpoint and effective method",
		func(command, wantMethod, wantEndpoint string) {
			apiCmd, err := parser.ParseGHAPICommand(parseFirstGHCommand(command))
			Expect(err).NotTo(HaveOccurred())
			Expect(apiCmd.Method).To(Equal(wantMethod))
			Expect(apiCmd.Endpoint).To(Equal(wantEndpoint))
		},
		Entry(
			"plain read defaults to GET",
			"gh api repos/o/r/contents/README.md",
			"GET", "repos/o/r/contents/README.md",
		),
		Entry(
			"leading slash is stripped",
			"gh api /repos/o/r/contents/README.md",
			"GET", "repos/o/r/contents/README.md",
		),
		Entry(
			"a field flag makes the default POST",
			"gh api repos/o/r/merges -f base=main -f head=topic",
			"POST", "repos/o/r/merges",
		),
		Entry(
			"-X before the endpoint",
			"gh api -X PUT repos/o/r/contents/README.md",
			"PUT", "repos/o/r/contents/README.md",
		),
		Entry(
			"-X after the endpoint",
			"gh api repos/o/r/contents/README.md -X PUT",
			"PUT", "repos/o/r/contents/README.md",
		),
		Entry(
			"--method=VALUE form",
			"gh api --method=DELETE repos/o/r/contents/README.md",
			"DELETE", "repos/o/r/contents/README.md",
		),
		Entry(
			"attached short flag value",
			"gh api -XPUT repos/o/r/contents/README.md",
			"PUT", "repos/o/r/contents/README.md",
		),
		Entry(
			"lower-case method is upper-cased",
			"gh api --method put repos/o/r/contents/README.md",
			"PUT", "repos/o/r/contents/README.md",
		),
		Entry(
			"bare positional verb",
			"gh api PUT repos/o/r/contents/README.md",
			"PUT", "repos/o/r/contents/README.md",
		),
		Entry(
			"full api.github.com URL",
			"gh api https://api.github.com/repos/o/r/git/commits -f message=x",
			"POST", "repos/o/r/git/commits",
		),
		Entry(
			"GitHub Enterprise URL drops the api/v3 prefix",
			"gh api https://ghe.example.com/api/v3/repos/o/r/merges -f base=main",
			"POST", "repos/o/r/merges",
		),
		Entry(
			"query string is dropped",
			"gh api 'repos/o/r/commits?per_page=1'",
			"GET", "repos/o/r/commits",
		),
		Entry(
			"repository spelled from a variable",
			"gh api -X PUT repos/$OWNER/$REPO/contents/README.md",
			"PUT", "repos/${OWNER}/${REPO}/contents/README.md",
		),
		Entry(
			"header flag value is not mistaken for the endpoint",
			"gh api -H 'Accept: application/vnd.github+json' repos/o/r/merges -f base=main",
			"POST", "repos/o/r/merges",
		),
		Entry(
			"jq flag value is not mistaken for the endpoint",
			"gh api --jq .sha repos/o/r/commits/main",
			"GET", "repos/o/r/commits/main",
		),
		Entry(
			"pull request merge",
			"gh api --method PUT repos/o/r/pulls/42/merge",
			"PUT", "repos/o/r/pulls/42/merge",
		),
	)

	Describe("GraphQL", func() {
		It("collects the mutation from an inline query field", func() {
			apiCmd, err := parser.ParseGHAPICommand(parseFirstGHCommand(
				`gh api graphql -f query='mutation { createCommitOnBranch(input: $i) { commit { url } } }'`,
			))
			Expect(err).NotTo(HaveOccurred())
			Expect(apiCmd.IsGraphQL).To(BeTrue())
			Expect(apiCmd.Query).To(ContainSubstring("createCommitOnBranch"))
		})

		It("collects the mutation from a heredoc on stdin", func() {
			apiCmd, err := parser.ParseGHAPICommand(parseFirstGHCommand(
				"gh api graphql --input - <<'EOF'\n{\"query\": \"mutation { createCommitOnBranch }\"}\nEOF",
			))
			Expect(err).NotTo(HaveOccurred())
			Expect(apiCmd.IsGraphQL).To(BeTrue())
			Expect(apiCmd.Query).To(ContainSubstring("createCommitOnBranch"))
		})

		It("marks the graphql endpoint even with a leading slash", func() {
			apiCmd, err := parser.ParseGHAPICommand(
				parseFirstGHCommand("gh api /graphql -f query=x"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(apiCmd.IsGraphQL).To(BeTrue())
		})
	})

	Describe("non-api commands", func() {
		It("rejects gh pr create", func() {
			_, err := parser.ParseGHAPICommand(parseFirstGHCommand(`gh pr create --title x`))
			Expect(err).To(MatchError(parser.ErrNotGHAPICommand))
		})

		It("rejects a non-gh command", func() {
			_, err := parser.ParseGHAPICommand(
				parser.Command{Name: "git", Args: []string{"status"}},
			)
			Expect(err).To(MatchError(parser.ErrNotGHCommand))
		})
	})

	Describe("input file", func() {
		It("records a --input path", func() {
			apiCmd, err := parser.ParseGHAPICommand(
				parseFirstGHCommand("gh api graphql --input query.json"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(apiCmd.InputFile).To(Equal("query.json"))
			Expect(apiCmd.Method).To(Equal("POST"))
		})

		It("follows --field query=@path to the file holding the query", func() {
			apiCmd, err := parser.ParseGHAPICommand(
				parseFirstGHCommand("gh api graphql -F query=@q.graphql"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(apiCmd.InputFile).To(Equal("q.graphql"))
			Expect(apiCmd.Query).To(BeEmpty())
		})

		It("keeps a --raw-field value literal, since it does not expand @", func() {
			apiCmd, err := parser.ParseGHAPICommand(
				parseFirstGHCommand("gh api graphql -f query=@q.graphql"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(apiCmd.InputFile).To(BeEmpty())
			Expect(apiCmd.Query).To(Equal("@q.graphql"))
		})

		It("treats --input - as stdin rather than a path", func() {
			apiCmd, err := parser.ParseGHAPICommand(
				parseFirstGHCommand("gh api graphql --input -"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(apiCmd.InputFile).To(BeEmpty())
		})
	})

	Describe("IsGHAPI", func() {
		It("recognises gh api inside a pipeline", func() {
			cmd := parseFirstGHCommand("gh api repos/o/r/commits | jq .")
			Expect(parser.IsGHAPI(&cmd)).To(BeTrue())
		})

		It("does not recognise gh pr merge", func() {
			cmd := parseFirstGHCommand("gh pr merge 42 --squash")
			Expect(parser.IsGHAPI(&cmd)).To(BeFalse())
		})
	})
})

var _ = Describe("ParseGHAPICommand fields", func() {
	It("records field values, files and the hostname", func() {
		apiCmd, err := parser.ParseGHAPICommand(parseFirstGHCommand(
			`gh api --hostname ghe.example.com -X PUT repos/o/r/pulls/1/merge ` +
				`-f merge_method=squash -F commit_message=@body.txt --raw-field sha=@abc`,
		))
		Expect(err).NotTo(HaveOccurred())
		Expect(apiCmd.Hostname).To(Equal("ghe.example.com"))
		Expect(apiCmd.Fields).To(Equal(map[string]string{
			"merge_method": "squash",
			"sha":          "@abc",
		}))
		Expect(apiCmd.FieldFiles).To(Equal(map[string]string{"commit_message": "body.txt"}))
	})

	It("lets a later field replace an earlier one with the same key", func() {
		apiCmd, err := parser.ParseGHAPICommand(parseFirstGHCommand(
			`gh api -X PUT repos/o/r/pulls/1/merge -F commit_message=@a.txt -f commit_message=x`,
		))
		Expect(err).NotTo(HaveOccurred())
		Expect(apiCmd.Fields).To(Equal(map[string]string{"commit_message": "x"}))
		Expect(apiCmd.FieldFiles).To(BeEmpty())
	})
})
