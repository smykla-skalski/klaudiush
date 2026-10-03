package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Shell startup files", func() {
	const payload = "git push --force"

	resolver := fakeResolver{
		env: map[string]string{
			"HOME":     "/home/u",
			"BASH_ENV": "/opaque.sh",
			"ENV":      "/opaque.sh",
		},
		files: map[string]string{
			"x.sh":             payload,
			"/abs/x.sh":        payload,
			"/work/x.sh":       payload,
			"~/x.sh":           payload,
			"/home/u/x.sh":     payload,
			"/abs/script.sh":   "true",
			"/abs/fn.sh":       "git() { command git push --force; }",
			"/abs/loop.sh":     "bash -c true\n" + payload,
			"/abs/bad.sh":      "if then",
			"/abs/benign.sh":   "echo hi",
			"script.sh":        "true",
			"/abs/runs-env.sh": "BASH_ENV=/abs/x.sh bash -c true",
		},
		opaque: map[string]bool{"/opaque.sh": true},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred(), command)

		return result
	}

	pushed := func(result *parser.ParseResult) bool {
		for _, op := range result.GitOperations {
			if len(op.Args) > 0 && op.Args[0] == "push" {
				return true
			}
		}

		return false
	}

	DescribeTable("follows a startup file the line sets",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(pushed(result)).To(BeTrue(), command)
		},
		Entry("prefix, relative to the shell", `BASH_ENV=./x.sh bash -c true`),
		Entry("prefix, absolute, sh", `BASH_ENV=/abs/x.sh sh -c true`),
		Entry("env operand", `env BASH_ENV=/abs/x.sh bash -c true`),
		Entry("env operand under sudo", `sudo env -u X BASH_ENV=/abs/x.sh bash -c true`),
		Entry("export", `export BASH_ENV=/abs/x.sh; bash -c true`),
		Entry("assignment then export", `BASH_ENV=/abs/x.sh; export BASH_ENV; bash -c true`),
		Entry("declare -x", `declare -x BASH_ENV=/abs/x.sh && bash -c true`),
		Entry("script run by path", `BASH_ENV=/abs/x.sh /abs/script.sh`),
		Entry("relative script run by path", `BASH_ENV=x.sh ./script.sh`),
		Entry("script handed to bash", `BASH_ENV=/abs/x.sh bash /abs/script.sh`),
		Entry("script on stdin", `echo true | BASH_ENV=/abs/x.sh bash`),
		Entry("shell that runs nothing", `BASH_ENV=/abs/x.sh bash`),
		Entry("git, whose hooks may run bash", `BASH_ENV=/abs/x.sh git commit -m x`),
		Entry("any program that may start bash", `export BASH_ENV=/abs/x.sh; make build`),
		Entry("relative to cd", `cd /work && BASH_ENV=x.sh bash -c true`),
		Entry("variable in the value", `BASH_ENV='${HOME}/x.sh' bash -c true`),
		Entry("home in the value", `BASH_ENV=~/x.sh bash -c true`),
		Entry("variable assigned earlier", `d=/abs; BASH_ENV=$d/x.sh bash -c true`),
		Entry("ENV for an interactive shell", `ENV=/abs/x.sh sh -i -c true`),
		Entry("ENV for an interactive cluster", `ENV=/abs/x.sh dash -ic true`),
		Entry("ENV for a long interactive option", `ENV=/abs/x.sh zsh --interactive -c true`),
		Entry("ENV after an option value", `ENV=/abs/x.sh bash -o posix -i -c true`),
		Entry("bash --rcfile", `bash --rcfile /abs/x.sh -i -c true`),
		Entry("bash --init-file", `bash --init-file /abs/x.sh -i`),
		Entry("relative path for a program", `BASH_ENV=./x.sh git commit -m x`),
		Entry("shell inside a substitution", `X=$(BASH_ENV=/abs/x.sh bash -c true)`),
		Entry("inherited by a nested shell", `export BASH_ENV=/abs/x.sh; bash -c 'bash -c true'`),
		Entry("function the startup file defines", `BASH_ENV=/abs/fn.sh bash -c 'git status'`),
		Entry("set inside a script", `bash -c 'export BASH_ENV=/abs/x.sh; bash -c true'`),
		Entry("set by a sourced file", `source /abs/runs-env.sh`),
		Entry("plain unset keeps the earlier value",
			`BASH_ENV=/abs/x.sh; unset BASH_ENV; bash -c true`),
	)

	It("validates the startup file before the script", func() {
		result := parse(`BASH_ENV=/abs/x.sh bash -c 'git status'`)

		Expect(result.GitOperations).To(HaveLen(2))
		Expect(result.GitOperations[0].Args).To(HaveExactElements("push", "--force"))
		Expect(result.GitOperations[1].Args).To(HaveExactElements("status"))
	})

	It("does not read a startup file again in the shells it starts", func() {
		result := parse(`BASH_ENV=/abs/loop.sh bash -c true`)

		Expect(result.Truncated).To(BeFalse())
		Expect(pushed(result)).To(BeTrue())
	})

	It("names the startup file in the origin of what it runs", func() {
		result := parse(`BASH_ENV=/abs/bad.sh bash -c true`)

		Expect(result.Opacities).To(ContainElement(SatisfyAll(
			HaveField("Cause", parser.OpacityScriptSyntax),
			HaveField("Operation", "BASH_ENV"),
			HaveField("Origin", HaveExactElements("bash", "BASH_ENV")),
		)))
	})

	It("reads a startup file once for programs that start shells themselves", func() {
		result := parse(`export BASH_ENV=/abs/x.sh; git status; git log; make`)

		Expect(result.Truncated).To(BeFalse())
		Expect(result.GetCommands("git")).To(HaveLen(3))
	})

	DescribeTable("fails closed on a startup file it cannot see",
		func(command, operation, detail string) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(SatisfyAll(
				HaveField("Cause", parser.OpacityStartupFile),
				HaveField("Operation", operation),
				HaveField("Detail", detail),
			)), command)
		},
		Entry("command output", `BASH_ENV=$(mktemp) bash -c true`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("exported command output", `export BASH_ENV="$(mktemp)"; bash -c true`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("process substitution", `BASH_ENV=<(echo git push) bash -c true`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("env operand with a substitution", `env BASH_ENV=<(echo git push) bash -c true`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("append", `BASH_ENV=/abs/x.sh; BASH_ENV+=.evil; bash -c true`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("read", `read BASH_ENV; bash -c true`, "BASH_ENV", parser.DetailStartupValue),
		Entry("read after unset", `read BASH_ENV; unset BASH_ENV; bash -c true`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("conditional assignment", `[ -f a ] && BASH_ENV=/abs/x.sh; bash -c true`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("set later in a loop", `for i in 1 2; do bash -c true; BASH_ENV=/abs/x.sh; done`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("set by eval later in a loop",
			`for i in 1 2; do bash -c true; eval "$c"; done`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("write to a computed name", `n=BASH_ENV; declare "$n=/x.sh"; bash -c true`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("nameref", `declare -n r=BASH_ENV; r=/x.sh; bash -c true`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("script after a computed export", `export $(cat .env) && ./run.sh`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("special builtin prefix", `BASH_ENV=/abs/x.sh :; bash -c true`,
			"BASH_ENV", parser.DetailStartupValue),
		Entry("unknown variable", `BASH_ENV=$UNSET bash -c true`,
			"BASH_ENV", parser.DetailScriptVariable),
		Entry("variable from command output", `d=$(pwd); BASH_ENV=$d/x.sh bash -c true`,
			"BASH_ENV", parser.DetailScriptVariable),
		Entry("expansion the shell runs", `BASH_ENV='$(echo /abs/x.sh)' bash -c true`,
			"BASH_ENV", parser.DetailStartupExpansion),
		Entry("backquote the shell runs", "BASH_ENV='`echo /abs/x.sh`' bash -c true",
			"BASH_ENV", parser.DetailStartupExpansion),
		Entry("arithmetic the shell runs", `BASH_ENV='$((1))' bash -c true`,
			"BASH_ENV", parser.DetailStartupExpansion),
		Entry("unreadable file", `BASH_ENV=/opaque.sh bash -c true`,
			"BASH_ENV", parser.DetailScriptRead),
		Entry("descriptor", `BASH_ENV=/dev/fd/0 bash -c true <<< 'git push'`,
			"BASH_ENV", parser.DetailScriptRead),
		Entry("proc file", `BASH_ENV=/proc/self/fd/0 bash -c true`,
			"BASH_ENV", parser.DetailScriptRead),
		Entry("unknown directory", `cd "$D" && BASH_ENV=x.sh bash -c true`,
			"BASH_ENV", parser.DetailScriptDirectory),
		Entry("written on the line", `cp a /abs/x.sh; BASH_ENV=/abs/x.sh bash -c true`,
			"BASH_ENV", parser.DetailScriptWritten),
		Entry("ENV from command output", `ENV=$(mktemp) sh -i -c true`,
			"ENV", parser.DetailStartupValue),
		Entry("unknown --rcfile", `bash --rcfile "$RC" -i -c true`,
			"--rcfile", parser.DetailScriptVariable),
		Entry("inside a script", `bash -c 'BASH_ENV=$(mktemp) bash -c true'`,
			"BASH_ENV", parser.DetailStartupValue),
	)

	DescribeTable("leaves commands that start no shell alone",
		func(command string) {
			Expect(parse(command).Truncated).To(BeFalse(), command)
		},
		Entry("builtin", `BASH_ENV=$(mktemp) echo hi`),
		Entry("data command", `export BASH_ENV=$(mktemp); cd /tmp; ls; cat f`),
		Entry("prefix on another command", `BASH_ENV=$(mktemp) ls; bash -c true`),
		Entry("environment value", `bash -c true`),
		Entry("missing file", `BASH_ENV=/missing.sh bash -c true`),
		Entry("dev null", `BASH_ENV=/dev/null bash -c true`),
		Entry("empty", `BASH_ENV= bash -c true`),
		Entry("unset", `unset BASH_ENV ENV; bash -c true`),
		Entry("benign file", `BASH_ENV=/abs/benign.sh bash -c true`),
		Entry("ENV for a program", `ENV=$STAGE make deploy`),
		Entry("ENV for a non-interactive shell", `ENV=$STAGE sh -c true`),
		Entry("ENV for a non-interactive shell with options", `ENV=$STAGE bash +o posix -- -i`),
		Entry("multi-line value", "export BASH_ENV=\"$(\nmktemp\n)\"; echo"),
		Entry("ENV in a loop without startup names", `for e in a b; do ENV=$e ./deploy.sh; done`),
		Entry("function call", `f() { :; }; BASH_ENV=$(mktemp) f`),
		Entry("command inside the value", `BASH_ENV=$(mktemp); echo x`),
		Entry("program inside the exported value", `export BASH_ENV="$(mktemp)"`),
		Entry("after source", `source /abs/benign.sh; bash -c true`),
		Entry("program after a write to a computed name", `export $(cat .env) && npm start`),
	)
})
