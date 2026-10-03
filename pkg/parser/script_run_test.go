package parser_test

import (
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Program word allowances", func() {
	resolver := fakeResolver{
		env: map[string]string{"HOME": "/home/u"},
		files: map[string]string{
			"/home/u/go/bin/tool":    "git push --force\n",
			"/repo/scripts/x.sh":     "git push --force\n",
			"/s/run.sh":              "\"$(dirname \"$0\")/helper.sh\"\n",
			"/s/run-dash.sh":         "\"$(dirname -- \"$0\")/helper.sh\"\n",
			"/s/run-param.sh":        "\"${0%/*}/helper.sh\"\n",
			"/s/run-other.sh":        "\"$(dirname \"$1\")/helper.sh\"\n",
			"/s/helper.sh":           "git push --force\n",
			"/s/wrap.sh":             "exec \"$@\"\n",
			"/s/wrap-plain.sh":       "set -euo pipefail\n\"$@\"\n",
			"/s/wrap-one.sh":         "\"$1\" push\n",
			"/s/wrap-shift.sh":       "shift\n\"$@\"\n",
			"/s/wrap-set.sh":         "set -- ls\n\"$@\"\n",
			"/s/wrap-loop.sh":        "while :; do \"$@\"; shift; done\n",
			"/s/wrap-cond.sh":        "if [ -n \"$X\" ]; then shift; fi\n\"$@\"\n",
			"/s/wrap-eval-set.sh":    "eval 'set -- ls'\n\"$@\"\n",
			"/s/wrap-source-set.sh":  ". /s/set-ls.sh\n\"$@\"\n",
			"/s/set-git.sh":          "set -- git push\n",
			"/s/git-dir.sh":          "export GIT_DIR=/x\n",
			"/s/self.sh":             "if [ -z \"$1\" ]; then exec \"$0\" git push; fi\n\"$@\"\n",
			"/s/set-ls.sh":           "set -- ls\n",
			"/s/top.sh":              "\"$(git rev-parse --show-toplevel)/scripts/x.sh\"\n",
			"/s/wrap-builtin-set.sh": "builtin set -- ls\n\"$@\"\n",
			"/s/wrap-eval-shift.sh":  "eval shift\n\"$@\"\n",
			"/s/wrap-for.sh":         "for a; do $a push; done\n",
			"/s/wrap-func.sh":        "f() { git \"$@\"; }\nf commit -m x\n",
			"/s/wrap-unquoted.sh":    "$1 push\n",
		},
		outputs: map[string]string{
			"go env GOPATH":                 "/home/u/go",
			"go env GOBIN":                  "/home/u/go/bin",
			"git rev-parse --show-toplevel": "/repo",
		},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	hasCommand := func(result *parser.ParseResult, name string, args ...string) bool {
		return slices.ContainsFunc(result.Commands, func(cmd parser.Command) bool {
			return cmd.Name == name && slices.Equal(cmd.Args, args)
		})
	}

	DescribeTable("resolves and checks what the allowance names",
		func(command, name string, args ...string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), "%s: %+v", command, result.Opacities)
			Expect(hasCommand(result, name, args...)).To(BeTrue(), "%s: %+v", command,
				result.Commands)
		},
		Entry("a tool under GOPATH", `$(go env GOPATH)/bin/golangci-lint run`,
			"golangci-lint", "run"),
		Entry("a script under GOPATH", `$(go env GOPATH)/bin/tool`, "git", "push", "--force"),
		Entry("git under GOPATH", `$(go env GOPATH)/bin/git push`, "git", "push"),
		Entry("git under GOBIN", `"$(go env GOBIN)/git" commit -m x`, "git", "commit", "-m", "x"),
		Entry("a script under the repository root",
			`"$(git rev-parse --show-toplevel)/scripts/x.sh"`, "git", "push", "--force"),
		Entry("after a literal cd", `cd /repo && "$(git rev-parse --show-toplevel)/scripts/x.sh"`,
			"git", "push", "--force"),
		Entry("dirname of $0", `bash /s/run.sh`, "git", "push", "--force"),
		Entry("dirname -- of $0", `/s/run-dash.sh`, "git", "push", "--force"),
		Entry("${0%/*}", `bash /s/run-param.sh`, "git", "push", "--force"),
		Entry(`a wrapper exec "$@"`, `bash /s/wrap.sh git push`, "git", "push"),
		Entry("a wrapper run by path", `/s/wrap.sh git commit -m x`, "git", "commit", "-m", "x"),
		Entry("a wrapper after set -euo", `bash /s/wrap-plain.sh git push`, "git", "push"),
		Entry(`a wrapper using "$1"`, `bash /s/wrap-one.sh git`, "git", "push"),
		Entry("a wrapper with no arguments", `bash /s/wrap.sh`, "bash", "/s/wrap.sh"),
		Entry("a function in a wrapper keeps its own arguments", `bash /s/wrap-func.sh push`,
			"git", "commit", "-m", "x"),
		Entry("an unquoted positional", `bash /s/wrap-unquoted.sh git`, "git", "push"),
	)

	DescribeTable(
		"stays opaque when the allowance does not hold",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), "%s: %+v", command, result.Commands)
		},
		Entry("GOPATH assigned on the line", `GOPATH=/x; $(go env GOPATH)/bin/t`),
		Entry("go env -w earlier", `go env -w GOPATH=/x && $(go env GOPATH)/bin/t`),
		Entry("another go env variable", `$(go env GOROOT)/bin/t`),
		Entry("an extra argument", `$(go env GOPATH GOBIN)/bin/t`),
		Entry("a variable argument", `$(go env $V)/bin/t`),
		Entry("a quoted variable argument", `$(go env "$V")/bin/t`),
		Entry("a redirect", `$(go env GOPATH 2>/dev/null)/bin/t`),
		Entry("an assignment prefix", `$(GOFLAGS=x go env GOPATH)/bin/t`),
		Entry("two commands", `$(cd /x; git rev-parse --show-toplevel)/t`),
		Entry(
			"git init earlier",
			`git init -q sub && cd sub && "$(git rev-parse --show-toplevel)/x"`,
		),
		Entry("git config earlier",
			`git config core.worktree /x && "$(git rev-parse --show-toplevel)/x"`),
		Entry("a write earlier", `echo x > .git/HEAD; "$(git rev-parse --show-toplevel)/x"`),
		Entry("GIT_DIR assigned", `export GIT_DIR=/x; "$(git rev-parse --show-toplevel)/x"`),
		Entry("a script that reruns itself with arguments", `bash /s/self.sh`),
		Entry("a startup file that sets the arguments",
			`BASH_ENV=/s/set-git.sh bash /s/wrap.sh ls`),
		Entry("a startup file that sets GIT_DIR",
			`BASH_ENV=/s/git-dir.sh bash /s/top.sh`),
		Entry("a computed cd", `cd "$(echo /repo)" && "$(git rev-parse --show-toplevel)/x"`),
		Entry("CDPATH assigned", `CDPATH=/z; cd sub && "$(git rev-parse --show-toplevel)/x"`),
		Entry("prefix GIT_WORK_TREE on the script", `GIT_WORK_TREE=/x bash /s/top.sh`),
		Entry("set through builtin", `bash /s/wrap-builtin-set.sh git push`),
		Entry("shift through eval", `bash /s/wrap-eval-shift.sh x git push`),
		Entry("a for loop over the arguments", `bash /s/wrap-for.sh git`),
		Entry("PATH changed", `PATH=/x:$PATH; "$(git rev-parse --show-toplevel)/x"`),
		Entry("a git function", `git() { echo /x; }; "$(git rev-parse --show-toplevel)/x"`),
		Entry("an unknown directory", `cd "$D" && "$(git rev-parse --show-toplevel)/x"`),
		Entry("inside a loop", `for i in 1 2; do "$(git rev-parse --show-toplevel)/x"; done`),
		Entry("dirname of $0 at the top level", `"$(dirname "$0")/x"`),
		Entry("dirname of $0 in a new shell", `bash -c '"$(dirname "$0")/x"'`),
		Entry("dirname of $1 in a script", `bash /s/run-other.sh /s/a`),
		Entry("a wrapper under xargs", `echo git push | xargs /s/wrap.sh`),
		Entry("a wrapper under sudo", `sudo /s/wrap.sh git push`),
		Entry("a wrapper given command output", `bash /s/wrap.sh $(echo git) push`),
		Entry("a wrapper that shifts", `bash /s/wrap-shift.sh x git push`),
		Entry("a wrapper that sets its arguments", `bash /s/wrap-set.sh git push`),
		Entry("a wrapper that loops", `bash /s/wrap-loop.sh x git push`),
		Entry("a wrapper that may shift", `bash /s/wrap-cond.sh x git push`),
		Entry("a wrapper that sets them through eval", `bash /s/wrap-eval-set.sh git push`),
		Entry("a wrapper that sets them in a sourced file", `bash /s/wrap-source-set.sh git push`),
		Entry("a sourced wrapper without arguments", `source /s/wrap.sh`),
		Entry(
			"a sourced wrapper, after which no variable is trusted",
			`source /s/wrap.sh git push`,
		),
	)

	It("leaves a lookup opaque when the resolver cannot run it", func() {
		bare := fakeResolver{}
		result, err := parser.NewBashParserWithResolver(bare).Parse(`$(go env GOPATH)/bin/t`)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Truncated).To(BeTrue())
	})

	It("leaves a lookup opaque when its output is not a plain absolute path", func() {
		odd := fakeResolver{outputs: map[string]string{"go env GOPATH": "/a b"}}
		result, err := parser.NewBashParserWithResolver(odd).Parse(`$(go env GOPATH)/bin/t`)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Truncated).To(BeTrue())
	})

	It("allows only the fixed lookups", func() {
		Expect(parser.AllowedLookup([]string{"go", "env", "GOPATH"})).To(BeTrue())
		Expect(parser.AllowedLookup([]string{"go", "env", "GOROOT"})).To(BeFalse())
		Expect(parser.AllowedLookup([]string{"git", "rev-parse", "--show-toplevel", "x"})).
			To(BeFalse())
	})
})
