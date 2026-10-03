package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Dynamic words and redirects", func() {
	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParser().Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	DescribeTable("marks commands whose words come from command output",
		func(command string, dynamic bool) {
			result := parse(command)
			Expect(result.Commands).NotTo(BeEmpty())
			Expect(result.Commands[0].Dynamic).To(Equal(dynamic), command)
		},
		Entry("command substitution", `rm "$(echo a)/b"`, true),
		Entry("backticks", "rm `echo a`", true),
		Entry("arithmetic", `rm f$((1+1))`, true),
		Entry("process substitution", `diff <(ls) x`, true),
		Entry("substitution in a default", `rm "${X:-$(echo a)}"`, true),
		Entry("plain words", `rm a "$B" 'c'`, false),
	)

	DescribeTable("records file writes for every output redirect",
		func(command string, op parser.WriteOp) {
			result := parse(command)
			Expect(result.FileWrites).To(HaveLen(1), command)
			Expect(result.FileWrites[0].Path).To(Equal("out"))
			Expect(result.FileWrites[0].Operation).To(Equal(op))
		},
		Entry("clobber", `echo x >| out`, parser.WriteOpRedirect),
		Entry("both streams", `echo x &> out`, parser.WriteOpRedirect),
		Entry("append both", `echo x &>> out`, parser.WriteOpAppend),
		Entry("read write", `exec 3<> out`, parser.WriteOpAppend),
		Entry("duplicate to file", `echo x >& out`, parser.WriteOpRedirect),
	)

	It("renders redirect targets as the shell passes them", func() {
		Expect(parse(`echo x > a\b`).FileWrites[0].Path).To(Equal("ab"))
		Expect(parse("echo x > out\\").FileWrites[0].Path).To(Equal("out"))
		Expect(
			parse(`echo x > $'\x2ea\t\u00e9\101\q'`).FileWrites[0].Path,
		).To(Equal(".a\té" + "A\\q"))
		Expect(parse(`cat < in\put`).Commands[0].StdinFile).To(Equal("input"))
	})

	It("marks variables assigned from command output", func() {
		result := parse(
			`a=$(pwd); b=plain; c=x; c+=y; export d=$((1+1)); e=$(pwd); e=fixed; echo "$a"`,
		)
		Expect(result.DynamicVars).To(HaveKey("a"))
		Expect(result.DynamicVars).To(HaveKey("c"))
		Expect(result.DynamicVars).To(HaveKey("d"))
		Expect(result.DynamicVars).NotTo(HaveKey("b"))
		Expect(result.DynamicVars).NotTo(HaveKey("e"))
	})

	It("keeps the working directory unknown after an unresolved cd", func() {
		result := parse(`cd "$X" && rm a > b`)
		Expect(result.Commands[len(result.Commands)-1].DirUnknown).To(BeTrue())
		Expect(result.FileWrites[0].DirUnknown).To(BeTrue())
		Expect(parse(`sudo rm a`).Commands[1].DirUnknown).To(BeFalse())
	})

	It("keeps descriptor duplication out of file writes", func() {
		Expect(parse(`echo x >&2`).FileWrites).To(BeEmpty())
		Expect(parse(`echo x >&-`).FileWrites).To(BeEmpty())
	})

	It("counts redirects whose target comes from command output", func() {
		result := parse(`echo x > "$(mktemp)"; echo y > "$(pwd)/f"`)
		Expect(result.DynamicWrites).To(Equal(2))
		Expect(result.FileWrites).To(HaveLen(1))
		Expect(result.FileWrites[0].Dynamic).To(BeTrue())

		Expect(parse(`bash -c 'echo x > "$(mktemp)"'`).DynamicWrites).To(Equal(1))
		Expect(parse(`echo x > out`).DynamicWrites).To(BeZero())
	})
})
