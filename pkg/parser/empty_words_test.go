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
		Entry("function $@ as program", `f() { $@ git push --force; }; f ""`, "push"),
		Entry("function $* as program", `f() { $* git push --force; }; f ""`, "push"),
		Entry("function $1 as program", `f() { $1 $2 push --force; }; f "" git`, "push"),
		Entry("function missing $1", `f() { $1 git push --force; }; f`, "push"),
		Entry("function braced $1", `f() { ${1} git push --force; }; f ""`, "push"),
		Entry("function splits $1", `f() { $1 push --force; }; f " git "`, "push"),
	)

	DescribeTable("function arguments match bash word splitting",
		func(command string, want []string) {
			result, err := p.Parse(command)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Truncated).To(BeFalse())
			Expect(gitArgs(command)).To(ContainElement(Equal(want)))
		},
		Entry("unquoted empty $1", `f() { git $1 push --force; }; f ""`,
			[]string{"push", "--force"}),
		Entry("unquoted empty $@", `f() { git $@ push --force; }; f ""`,
			[]string{"push", "--force"}),
		Entry("quoted empty $1", `f() { git -C "$1" push; }; f ""`,
			[]string{"-C", "", "push"}),
		Entry("quoted missing $1", `f() { git -C "$1" push; }; f`,
			[]string{"-C", "", "push"}),
		Entry("unquoted $1 with spaces", `f() { git $1; }; f "push --force"`,
			[]string{"push", "--force"}),
	)

	DescribeTable("keeps the directory on an empty cd operand",
		func(command, dir string) {
			result, err := p.Parse(command)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.GitOperations).NotTo(BeEmpty())
			Expect(result.GitOperations[0].WorkingDirectory).To(Equal(dir))
		},
		Entry("cd empty", `cd /repo && cd "" && git push origin main`, "/repo"),
		Entry("cd -- empty", `cd /repo && cd -- '' && git push origin main`, "/repo"),
		Entry("pushd empty", `cd /repo && pushd "" && git push origin main`, "/repo"),
		Entry("cd with no operand", `cd /repo && cd && git push origin main`, "~"),
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
