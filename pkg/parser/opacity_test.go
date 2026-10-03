package parser_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Opacity explanations", func() {
	resolver := fakeResolver{
		files: map[string]string{
			"ok.sh":     "git status\n",
			"broken.sh": "git commit -m x && (\n",
			"outer.sh":  "bash ./inner.sh\n",
			"inner.sh":  "git zz\n",
			"outer.py":  "import os\nos.system('bash ./huge.sh')\n",
		},
		opaque: map[string]bool{"huge.sh": true},
		programs: map[string]parser.Program{
			"git-zz": parser.ProgramMissing,
		},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	only := func(command string) parser.Opacity {
		result := parse(command)
		Expect(result.Truncated).To(BeTrue(), "not truncated: %q", command)
		Expect(result.Opacities).To(HaveLen(1), "opacities of %q", command)

		return result.Opacities[0]
	}

	It("has no opacity for an inspectable command", func() {
		result := parse("sudo bash -c 'git status' && bash ./ok.sh")

		Expect(result.Truncated).To(BeFalse())
		Expect(result.Opacities).To(BeEmpty())
	})

	DescribeTable("names the cause, operation and origin",
		func(command string, want parser.Opacity) {
			Expect(only(command)).To(Equal(want))
		},
		Entry("nesting past the limit",
			strings.Repeat("env ", 9)+"git commit -m x",
			parser.Opacity{
				Cause:     parser.OpacityDepthLimit,
				Operation: "env",
				Origin:    strings.Fields(strings.Repeat("env ", 8)),
			},
		),
		Entry("an unreadable script under a launcher",
			"sudo bash ./huge.sh",
			parser.Opacity{
				Cause:     parser.OpacityUnreadableScript,
				Operation: "huge.sh",
				Origin:    []string{"sudo", "bash"},
				Detail:    parser.DetailScriptRead,
			},
		),
		Entry("a script path from a variable",
			`bash "$DIR/run.sh"`,
			parser.Opacity{
				Cause:     parser.OpacityUnreadableScript,
				Operation: "run.sh",
				Origin:    []string{"bash"},
				Detail:    parser.DetailScriptVariable,
			},
		),
		Entry("a relative script after an unknown cd",
			`cd "$X" && bash run.sh`,
			parser.Opacity{
				Cause:     parser.OpacityUnreadableScript,
				Operation: "run.sh",
				Origin:    []string{"bash"},
				Detail:    parser.DetailScriptDirectory,
			},
		),
		Entry("a script written with unknown content",
			`echo "$BODY" > s.sh && bash s.sh`,
			parser.Opacity{
				Cause:     parser.OpacityUnreadableScript,
				Operation: "s.sh",
				Origin:    []string{"bash"},
				Detail:    parser.DetailScriptWritten,
			},
		),
		Entry("a script that does not parse",
			"bash ./broken.sh",
			parser.Opacity{
				Cause:     parser.OpacityScriptSyntax,
				Operation: "broken.sh",
				Origin:    []string{"bash"},
			},
		),
		Entry("an inline script that does not parse",
			`env bash -c 'git commit -m x && ('`,
			parser.Opacity{
				Cause:     parser.OpacityScriptSyntax,
				Operation: "inline script",
				Origin:    []string{"env", "bash"},
			},
		),
		Entry("an unknown git subcommand under a shell",
			"bash -c 'git zz'",
			parser.Opacity{
				Cause:     parser.OpacityUnresolvedProgram,
				Operation: "git zz",
				Origin:    []string{"bash"},
			},
		),
		Entry("a function forwarding arguments it cannot follow",
			`f() { git "${@:1}"; }; f commit`,
			parser.Opacity{
				Cause:     parser.OpacityUnresolvedArgs,
				Operation: "f",
			},
		),
	)

	DescribeTable("flags zsh glob qualifiers that run code",
		func(command, form string) {
			Expect(only(command)).To(Equal(parser.Opacity{
				Cause:     parser.OpacityZshGlobQualifier,
				Operation: form,
			}))
		},
		Entry("an e qualifier", `ls *(e:'git push':)`, "(e)"),
		Entry("an e qualifier with braces", `ls *(e{'git push'})`, "(e)"),
		Entry("an e qualifier after other qualifiers", `ls *(.Ne,git push,)`, "(e)"),
		Entry("a negated e qualifier", `ls ?(^e:'git push':)`, "(e)"),
		Entry("an e qualifier after an owner", `ls *(u:root:e:'git push':)`, "(e)"),
		Entry("an e qualifier after a subscript", `ls *([1]e:'git push':)`, "(e)"),
		Entry("an e qualifier after a size", `ls *(Lk+1e:'git push':)`, "(e)"),
		Entry("an e qualifier holding a pipe", `ls *(e:'git push || true':)`, "(e)"),
		Entry("an e qualifier under a directory", `ls "$D"/*(e:'git push':)`, "(e)"),
		Entry("a quoted glob-subst lookalike stays out", `ls "$~q" *(e:'git push':)`, "(e)"),
		Entry("a function qualifier", `ls *(+fn)`, "(+func)"),
		Entry("a function qualifier with modifiers", `ls x*(+fn:t)`, "(+func)"),
		Entry("a sort by code", `ls *(oe:'git push':)`, "(e)"),
		Entry("a reverse sort by a function", `ls *(O+fn)`, "(+func)"),
		Entry("a function after a sort by time", `ls *(om+uname)`, "(+func)"),
		Entry("a function after a reverse sort by size", `ls *(OL+fn)`, "(+func)"),
		Entry("a function after a time unit and a flag", `ls *(amM+fn)`, "(+func)"),
		Entry("a function after a group delimited by letters", `ls *(gdwheeld+fn)`, "(+func)"),
		Entry("a numeric function after a group", `ls *(gdwheeld+3)`, "(+func)"),
		Entry("a function after an owner", `ls *(udrootd+pwd)`, "(+func)"),
		Entry("a digit function after a sort", `ls *(om+3)`, "(+func)"),
		Entry("a digit function after a unit and a flag", `ls *(mmM+3)`, "(+func)"),
		Entry("a plus in a symbolic mode", `ls *(f:u+x:)`, "(+func)"),
		Entry("a common word holding an e qualifier", `ls !(tests)`, "(e)"),
		Entry("a function after a sort and a flag", `ls *(oLM+fn)`, "(+func)"),
		Entry("the #q form", `ls *(#qe:'git push':)`, "(e)"),
		Entry("the #q form with a pipe", `ls *(#q+fn|x)`, "(+func)"),
		Entry("a qualifier in an array", `a=(*(e:'git push':))`, "(e)"),
		Entry("an e qualifier after an octal mode", `ls *(f-0e:"git push":)`, "(e)"),
		Entry("an e qualifier after an exact mode", `ls *(f=644e:"git push":)`, "(e)"),
		Entry("an e qualifier after a wildcard mode", `ls *(f?44e:"git push":)`, "(e)"),
		Entry("an e qualifier holding a command substitution",
			`ls *(e:"git push"$(true):)`, parser.GlobCommandSubst),
		Entry("an e qualifier holding backticks",
			"ls *(e:\"git push\"`true|true`:)", parser.GlobCommandSubst),
		Entry("a command substitution in an alternation",
			"ls @(a|`git push`)", parser.GlobCommandSubst),
		Entry("a command substitution in a repeat", `ls *(a$(git push))`, parser.GlobCommandSubst),
		Entry("a quoted e", `ls *('e':"git push":)`, "(e)"),
		Entry("an ANSI-C quoted argument", `ls *(e$':git push:')`, parser.GlobVariable),
		Entry("an ANSI-C quoted pipe delimiter", `ls *(e$'|git push|')`, parser.GlobVariable),
		Entry("a function name starting with a digit", `ls *(+1x)`, "(+func)"),
		Entry("a sort by a function starting with a digit", `ls *(O+1x)`, "(+func)"),
		Entry("a quoted brace in a parameter expansion",
			`ls *(e:'git push #'${x:-"}|"}:)`, parser.GlobVariable),
		Entry("a delimiter from a variable", `ls *(e${d}git push${d})`, parser.GlobVariable),
		Entry("qualifiers from a glob-subst variable",
			`q='e:git push:'; ls *($~q)`, parser.GlobVariable),
		Entry("qualifiers from a variable", `ls *($q)`, parser.GlobVariable),
		Entry("a glob from a glob-subst variable",
			`q='*(e:git push:)'; ls $~q`, parser.GlobSubst),
		Entry("a glob-subst variable inside a word", `ls a$~q`, parser.GlobSubst),
		Entry("a non-ASCII function name", `ls *(+é)`, "(+func)"),
		Entry("a qualifier in a command substitution in a heredoc",
			"cat <<EOF\n$(ls *(+fn))\nEOF", "(+func)"),
		Entry("an e qualifier after a numeric glob", `ls <0-9>(e:'git push':)`, "(e)"),
		Entry("a function qualifier after an open numeric glob", `echo <->(+fn)`, "(+func)"),
		Entry("a qualifier after a half-open numeric glob", `ls a<1->(.e,x,)`, "(e)"),
		Entry("an e qualifier in a default value", `ls ${x:-*(e:'git push':)}`, "(e)"),
		Entry("a function qualifier in a default value", `ls ${x-*(+fn)}`, "(+func)"),
		Entry("a qualifier in an alternate value", `ls ${x:+*(.e:x:)}`, "(e)"),
		Entry("a qualifier in a replacement", `ls ${x/y/*(+fn)}`, "(+func)"),
		Entry("a qualifier in a nested default", `ls ${x:-${y:-*(#qe:x:)}}`, "(e)"),
		Entry("a subscript naming a variable",
			`x='path[$(git push)]'; ls *([x])`, "([...])"),
		Entry("a subscript range naming a variable", `ls *(.[1,x])`, "([...])"),
		Entry("a subscript in the #q form", `ls *(#q[x]).go`, "([...])"),
		Entry("a quoted pipe", `ls *(e:'git push|x':)`, "(e)"),
		Entry("an escaped e", `ls *(\e:"git push":)`, "(e)"),
		Entry("a quoted plus", `ls *("+"fn)`, "(+func)"),
		Entry("an escaped delimiter", `ls *(e\:"git push"\:)`, "(e)"),
		Entry("an e qualifier holding a parameter expansion",
			`ls *(e:"git push ${x:-a|b}":)`, parser.GlobVariable),
		Entry("an e qualifier after a qualifier it cannot read",
			`ls *(f<u+x>Ze:"git push":)`, "(+func)"),
	)

	It("flags a glob qualifier in a heredoc fed to a shell", func() {
		Expect(only("zsh <<'EOF'\nls *(+fn)\nEOF")).To(Equal(parser.Opacity{
			Cause:     parser.OpacityZshGlobQualifier,
			Operation: "(+func)",
			Origin:    []string{"zsh"},
		}))
	})

	It("flags a glob qualifier inside an inline script", func() {
		Expect(only(`zsh -c "ls *(e:'git push':)"`)).To(Equal(parser.Opacity{
			Cause:     parser.OpacityZshGlobQualifier,
			Operation: "(e)",
			Origin:    []string{"zsh"},
		}))
	})

	DescribeTable("leaves extended globs that run no code alone",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), "truncated: %q", command)
			Expect(result.Opacities).To(BeEmpty())
		},
		Entry("an alternation", `ls @(a|b).go`),
		Entry("a plain repeat", `ls *(foo)`),
		Entry("a group with e and a pipe", `ls *(e:x:|y)`),
		Entry("an e without a closing delimiter", `ls *(seen)`),
		Entry("an e at the end", `ls ?(ee)`),
		Entry("a mode with e in it", `ls *(feature)`),
		Entry("an owner named e", `ls *(u:e:)`),
		Entry("a prefix holding e", `ls *(P:e:)`),
		Entry("a size with a sign", `ls *(m+3)`),
		Entry("a history modifier", `ls *(N:e)`),
		Entry("a size with a unit and a sign", `ls *(Lk+1)`),
		Entry("a time with a unit and a sign", `ls *(mm+3)`),
		Entry("a sort by name", `ls *(on)`),
		Entry("a nested group", `ls *(e:(x):)`),
		Entry("a numeric subscript", `ls *([1,3])`),
		Entry("a bracket inside a word", `ls @([a-z]*).go`),
		Entry("a digit range", `ls +([0-9]).txt`),
		Entry("an alternation in a default value", `ls ${x:-@(a|b)}`),
		Entry("a harmless qualifier after a numeric glob", `ls <->(N)`),
		Entry("a qualifier in a quoted heredoc", "cat <<'EOF'\nls *(+fn)\nEOF"),
		Entry("a qualifier in a heredoc", "cat <<EOF\nls *(e:x:) $~q\nEOF"),
		Entry("a qualifier in a here-string", `cat <<< *(+fn)`),
		Entry("an escaped glob substitution", `printf '%s\n' \$~q`),
		Entry("a dollar and a tilde split by quotes", `echo $'x'~q`),
		Entry("a dollar and a tilde across a quoted part", `echo a$"b"~q`),
		Entry("a process substitution feeding tee", `ls | tee >(grep -e foo)`),
		Entry("parentheses in a default value", `echo ${x:-f(a)}`),
	)

	It("explains an exhausted budget once", func() {
		calls := func(name string) string {
			return strings.Repeat(name+"; ", 60)
		}
		command := "f() { " + calls("g") + "}; g() { " + calls("git status") + "}; f"
		result := parse(command)

		Expect(result.Truncated).To(BeTrue())

		var budget []parser.Opacity

		for _, o := range result.Opacities {
			if o.Cause == parser.OpacityWorkBudget {
				budget = append(budget, o)
			}
		}

		Expect(budget).To(HaveLen(1))
		Expect(budget[0].Operation).NotTo(BeEmpty())
	})

	DescribeTable("hides names that are not plain or short",
		func(command, operation string) {
			Expect(only(command).Operation).To(Equal(operation))
		},
		Entry("a long subcommand", "git "+strings.Repeat("z", 40), "git <hidden>"),
		Entry("a script named by a variable", `bash "$X"`, "<hidden>"),
		Entry("a long script name", `bash "$D/`+strings.Repeat("a", 40)+`.sh"`, "<hidden>"),
		Entry("a script under a directory", `bash "$D/x/run.sh"`, "run.sh"),
		Entry("a subcommand that looks like a password", "git hunter2pw", "git <hidden>"),
		Entry("a function that looks like a key",
			`AKIAIOSFODNN7EXAMPLE() { git "${@:1}"; }; AKIAIOSFODNN7EXAMPLE x`, "<hidden>"),
		Entry("a short mixed subcommand", "git zz9", "git zz9"),
	)

	It("hides origin names that look like secrets but keeps known programs", func() {
		o := only("ab12cd34() { python3 -c 'import os; os.system(\"bash ./huge.sh\")'; }; ab12cd34")

		Expect(o.Origin).To(Equal([]string{"<hidden>", "python3", "bash"}))
	})

	It("keeps a bounded number of distinct opacities", func() {
		result := parse(manyUnknown(20))

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities).To(HaveLen(parser.MaxOpacities - 1))
		Expect(result.MoreOpacities).To(BeTrue())
	})

	It("keeps a slot for an exhausted budget", func() {
		calls := strings.Repeat("g; ", 60)
		result := parse(manyUnknown(20) + "; f() { " + calls + "}; g() { " +
			strings.Repeat("git status; ", 60) + "}; f")

		Expect(result.Opacities).To(HaveLen(parser.MaxOpacities))
		Expect(result.Opacities[parser.MaxOpacities-1].Cause).To(Equal(parser.OpacityWorkBudget))
	})

	It("uses the last slot once the budget is reported", func() {
		calls := strings.Repeat("g; ", 60)
		result := parse(manyUnknown(6) + "; bash -c 'f() { " + calls + "}; g() { " +
			strings.Repeat("git status; ", 60) + "}; f; git status && ('")

		Expect(result.Opacities).To(HaveLen(parser.MaxOpacities))
		Expect(result.Opacities[parser.MaxOpacities-2].Cause).To(Equal(parser.OpacityWorkBudget))
		Expect(result.Opacities[parser.MaxOpacities-1]).To(Equal(parser.Opacity{
			Cause:     parser.OpacityScriptSyntax,
			Operation: "inline script",
			Origin:    []string{"bash"},
		}))
		Expect(result.MoreOpacities).To(BeFalse())
	})

	It("keeps other opacities past many setup evals", func() {
		result := parse(allSetupEvals() + "; " + manyUnknown(parser.MaxOpacities-1))

		Expect(setupTools(result.Opacities)).To(ConsistOf(parser.EvalSetupTools()))
		Expect(result.Opacities).To(HaveLen(len(parser.EvalSetupTools()) + parser.MaxOpacities - 1))
		Expect(result.Opacities[len(result.Opacities)-1].Operation).To(Equal(
			"git zz" + strings.Repeat("z", parser.MaxOpacities-2),
		))
		Expect(result.MoreOpacities).To(BeFalse())
	})

	It("keeps every setup tool past a full list of other opacities", func() {
		result := parse(manyUnknown(20) + "; " + allSetupEvals())

		Expect(setupTools(result.Opacities)).To(ConsistOf(parser.EvalSetupTools()))
		Expect(result.MoreOpacities).To(BeTrue())
	})

	It("bounds repeats of one setup tool but still shows a new tool", func() {
		shells := []string{"bash", "sh", "zsh", "dash", "ksh"}
		evals := make([]string, 0, 2*len(shells)+2)

		for _, shell := range shells {
			evals = append(evals,
				shell+` -c 'eval "$(mise activate bash)"'`,
				"env "+shell+` -c 'eval "$(mise activate bash)"'`,
			)
		}

		evals = append(evals, `eval "$(direnv export bash)"`, "git zz")
		result := parse(strings.Join(evals, "; "))

		Expect(setupTools(result.Opacities)).To(HaveLen(parser.MaxOpacities + 1))
		Expect(setupTools(result.Opacities)).To(ContainElements("mise", "direnv"))
		Expect(result.Opacities).To(ContainElement(HaveField("Operation", "git zz")))
		Expect(result.MoreOpacities).To(BeTrue())
	})

	DescribeTable("names the scripts and aliases on the way",
		func(command string, origin []string, operation string) {
			o := only(command)

			Expect(o.Origin).To(Equal(origin))
			Expect(o.Operation).To(Equal(operation))
		},
		Entry("a script running a script", "bash ./outer.sh",
			[]string{"bash", "outer.sh", "bash", "inner.sh"}, "git zz"),
		Entry("a git shell alias", `git -c alias.yy='!git zz' yy`,
			[]string{"git", "git alias yy"}, "git zz"),
		Entry("a git shell alias that does not parse", `git -c alias.yy='!git status && (' yy`,
			[]string{"git"}, "git alias yy"),
		Entry("a same-line alias that does not parse", "alias g='git status && ('; g",
			[]string{"g"}, "g"),
		Entry("a script on stdin", "bash /dev/stdin <<< 'git zz'",
			[]string{"bash", "stdin"}, "git zz"),
		Entry("a gh shell alias", "gh alias set --shell yy 'git zz'; gh yy",
			[]string{"gh", "gh alias yy"}, "git zz"),
		Entry("a gh shell alias that does not parse",
			"gh alias set --shell yy 'git status && ('; gh yy",
			[]string{"gh"}, "gh alias yy"),
		Entry("a script run by an interpreter file", "python3 outer.py",
			[]string{"python3", "outer.py", "bash"}, "huge.sh"),
	)

	It("ignores stdin with nothing on it", func() {
		Expect(parse("bash -").Truncated).To(BeFalse())
	})

	It("reports a repeated opacity once", func() {
		result := parse("git zz; git zz")

		Expect(result.Opacities).To(HaveLen(1))
	})
})

// manyUnknown runs count distinct unknown git subcommands.
func manyUnknown(count int) string {
	parts := make([]string, 0, count)
	for i := range count {
		parts = append(parts, "git zz"+strings.Repeat("z", i))
	}

	return strings.Join(parts, "; ")
}

// setupEvals print each setup tool's shell setup, keyed by tool.
var setupEvals = map[string]string{
	"ssh-agent": "ssh-agent -s",
	"mise":      "mise activate bash",
	"direnv":    "direnv export bash",
	"rbenv":     "rbenv init -",
	"pyenv":     "pyenv init -",
	"nodenv":    "nodenv init -",
	"conda":     "conda shell.bash hook",
	"brew":      "brew shellenv",
	"starship":  "starship init bash",
	"zoxide":    "zoxide init bash",
	"fnm":       "fnm env",
}

// allSetupEvals evals the shell setup of every tool the parser recognizes.
func allSetupEvals() string {
	tools := parser.EvalSetupTools()
	parts := make([]string, 0, len(tools))

	for _, tool := range tools {
		Expect(setupEvals).To(HaveKey(tool))
		parts = append(parts, `eval "$(`+setupEvals[tool]+`)"`)
	}

	return strings.Join(parts, "; ")
}

// setupTools lists the setup tools the opacities name, in order.
func setupTools(opacities []parser.Opacity) []string {
	tools := make([]string, 0, len(opacities))

	for _, o := range opacities {
		if o.Tool != "" {
			tools = append(tools, o.Tool)
		}
	}

	return tools
}
