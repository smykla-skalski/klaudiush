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

	It("records every output redirect of a statement", func() {
		result := parse(`: > first >> second 2> third > last`)
		paths := make([]string, 0, len(result.FileWrites))
		ops := make([]parser.WriteOp, 0, len(result.FileWrites))

		for _, fw := range result.FileWrites {
			paths = append(paths, fw.Path)
			ops = append(ops, fw.Operation)
		}

		Expect(paths).To(Equal([]string{"first", "second", "third", "last"}))
		Expect(ops).To(Equal([]parser.WriteOp{
			parser.WriteOpRedirect, parser.WriteOpAppend,
			parser.WriteOpRedirect, parser.WriteOpRedirect,
		}))

		heredoc := parse("cat > first > last <<EOF\nbody\nEOF")
		Expect(heredoc.FileWrites).To(HaveLen(2))
		Expect(heredoc.FileWrites[0].Path).To(Equal("first"))
		Expect(heredoc.FileWrites[0].Content).To(BeEmpty())
		Expect(heredoc.FileWrites[1].Path).To(Equal("last"))
		Expect(heredoc.FileWrites[1].Content).To(Equal("body\n"))
	})

	It("snapshots variables for each command and write", func() {
		result := parse(`d=$(echo a); rm "$d/x" > "$d/log"; d=b; rm "$d/y"; export e=1`)

		var removes []parser.Command

		for _, cmd := range result.Commands {
			if cmd.Name == "rm" {
				removes = append(removes, cmd)
			}
		}

		Expect(removes).To(HaveLen(2))

		first, second := removes[0], removes[1]
		Expect(first.Vars.IsDynamic("d")).To(BeTrue())
		Expect(result.FileWrites[0].Vars.IsDynamic("d")).To(BeTrue())
		Expect(second.Vars.IsDynamic("d")).To(BeFalse())
		Expect(second.Vars.ExpandVars(second.Args[0])).To(Equal("b/y"))
		Expect(second.Vars.Assignments).NotTo(HaveKey("e"))
		Expect(result.Assignments).To(HaveKeyWithValue("e", "1"))

		var none *parser.VarScope
		Expect(none.ExpandVars("${d}")).To(Equal("${d}"))
		Expect(none.IsDynamic("d")).To(BeFalse())
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
		Expect(result.FileWrites).To(HaveLen(2))
		Expect(result.FileWrites[0].TargetUnknown).To(BeTrue())
		Expect(result.FileWrites[1].Dynamic).To(BeTrue())

		Expect(parse(`bash -c 'echo x > "$(mktemp)"'`).DynamicWrites).To(Equal(1))
		Expect(parse(`echo x > out`).DynamicWrites).To(BeZero())
	})
})
