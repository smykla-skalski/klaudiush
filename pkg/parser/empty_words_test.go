package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Empty words", func() {
	var p *parser.BashParser

	BeforeEach(func() {
		p = parser.NewBashParser()
	})

	gitArgs := func(command string) [][]string {
		result, err := p.Parse(command)
		Expect(err).NotTo(HaveOccurred())

		found := make([][]string, 0, len(result.GitOperations))

		for _, cmd := range result.GitOperations {
			found = append(found, cmd.Args)
		}

		return found
	}

	DescribeTable("keeps quoted empty words in place",
		func(command string, want []string) {
			result, err := p.Parse(command)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Commands).NotTo(BeEmpty())
			Expect(result.Commands[0].Args).To(Equal(want))
		},
		Entry("double quotes", `echo "" x`, []string{"", "x"}),
		Entry("single quotes", `echo '' x`, []string{"", "x"}),
		Entry("ANSI-C quotes", `echo $'' x`, []string{"", "x"}),
		Entry("locale quotes", `echo $"" x`, []string{"", "x"}),
		Entry("adjacent empty quotes", `echo ""'' x`, []string{"", "x"}),
		Entry("trailing", `echo x ""`, []string{"x", ""}),
		Entry("several", `echo "" '' x`, []string{"", "", "x"}),
		Entry("git flag value", `git commit -m "" -sS`, []string{"commit", "-m", "", "-sS"}),
		Entry("git global option", `git -C "" status`, []string{"-C", "", "status"}),
	)

	DescribeTable("sees the command after an empty option value",
		func(command, sub, flag string) {
			found := gitArgs(command)
			Expect(found).To(ContainElement(SatisfyAll(
				ContainElement(sub),
				ContainElement(flag),
			)), "git %s %s not found in %q", sub, flag, command)
		},
		Entry("sudo -u double quotes", `sudo -u "" git push --force`, "push", "--force"),
		Entry("sudo -u single quotes", `sudo -u '' git push --force`, "push", "--force"),
		Entry("doas -u", `doas -u "" git push --force`, "push", "--force"),
		Entry("env -u", `env -u "" git push --force`, "push", "--force"),
		Entry("nice -n", `nice -n '' git push --force`, "push", "--force"),
		Entry("git -C", `git -C "" push --force`, "push", "--force"),
		Entry("git -c", `git -c '' push --force`, "push", "--force"),
		Entry("bash -c with sudo", `bash -c 'sudo -u "" git push --force'`, "push", "--force"),
		Entry(
			"docker --name before the image",
			`docker run --entrypoint git --name "" img push --force`,
			"push", "--force",
		),
		Entry(
			"docker --name before --entrypoint",
			`docker run --name '' --entrypoint git img push --force`,
			"push", "--force",
		),
		Entry(
			"docker -w",
			`docker run -w "" --entrypoint git img push --force`,
			"push", "--force",
		),
		Entry("positional empty argument", `bash -c 'sudo -u "$1" git push --force' _ ""`,
			"push", "--force"),
	)

	DescribeTable("drops unquoted words that expand to nothing",
		func(command, sub string) {
			Expect(gitArgs(command)).To(ContainElement(ContainElement(sub)))
		},
		Entry("empty variable as program", `X=; $X git push`, "push"),
		Entry("empty variable in a script", `bash -c 'X=; $X git push'`, "push"),
		Entry("unquoted empty positional", `bash -c 'git $1 push' _ ""`, "push"),
	)

	It("does not run an empty program word", func() {
		result, err := p.Parse(`"" git push --force`)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.GitOperations).To(BeEmpty())
	})

	It("keeps empty git global option values in place", func() {
		want := []string{"-C", "", "-c", "", "push", "origin", "main"}
		Expect(gitArgs(`git -C '' -c "" push origin main`)).To(ContainElement(Equal(want)))
	})
})
