package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Startup variables a loop may set", func() {
	resolver := fakeResolver{
		env:   map[string]string{"HOME": "/home/u"},
		files: map[string]string{"/abs/run.sh": "echo hi"},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred(), command)

		return result
	}

	DescribeTable(
		"trusts an unset startup variable nothing in the loop sets",
		func(command string) {
			Expect(parse(command).Truncated).To(BeFalse(), command)
		},
		Entry(
			"printf with command output after the format",
			`for i in $(seq 737 746); do printf "%s:%s " $i "$(gh issue view $i --repo o/r --json state -q .state)"; done; echo`,
		),
		Entry(
			"until and for with printf operands",
			`R=x; until [ "$(timeout 30 gh pr view 759 --repo o/r --json state -q .state)" = MERGED ]; do sleep 20; done; for i in $(seq 737 746); do printf "%s:%s " $i $(gh issue view $i --repo o/r --json state -q .state); done; echo`,
		),
		Entry(
			"function called from a while read loop",
			"probe() { jq -nc --arg c \"$1\" '{c:$c}' | /opt/homebrew/bin/klaudiush --hook-type PreToolUse | jq -r .x; }\n"+
				"while read -r line; do printf '%s => ' \"$line\"; probe \"$line\"; done <<'EOF'\nls\nEOF",
		),
		Entry(
			"printf variable operand before git",
			`for i in 1 2; do printf x $i; git status; done`,
		),
		Entry(
			"printf variable operand before a script",
			`for i in 1 2; do printf x "$i"; bash /abs/run.sh; done`,
		),
		Entry("printf after --", `for i in 1 2; do printf -- "$i"; gh x; done`),
		Entry(
			"printf -v with a literal name",
			`for i in 1 2; do printf -v out '%s' "$i"; gh x; done`,
		),
		Entry("select with printf", `select x in a b; do printf '%s' "$x"; gh x; done`),
		Entry("function that sets nothing", `f() { echo hi; }; for i in 1 2; do f; gh x; done`),
		Entry(
			"function reading HOME",
			`f() { ls "$HOME"; }; for i in 1 2; do f; zsh -c true; done`,
		),
		Entry("recursive function", `f() { g; }; g() { f; }; for i in 1 2; do f; gh x; done`),
		Entry(
			"function printing its argument",
			`f() { printf '%s\n' "$1"; }; for i in 1 2; do f "$i"; gh x; done`,
		),
		Entry("alias for a program", `alias g=git; for i in 1 2; do g status; done`),
		Entry(
			"trap with a literal action",
			`for i in 1 2; do trap 'rm -f /tmp/x' EXIT; gh x; done`,
		),
		Entry(
			"printf -- before a computed word",
			`for i in 1 2; do printf -- -v "$i" x; gh x; done`,
		),
		Entry("printf operand before a shell reading HOME files",
			`for i in 1 2; do printf x $i; zsh -c true; done`),
		Entry("read with a computed prompt", `for i in 1 2; do read -p "$i> " x; gh x; done`),
		Entry(
			"read with computed option values",
			`for i in 1 2; do read -rt "$t" -n "$n" -N "$n" -d "$d" -u "$fd" -i "$i" -e x; gh x; done`,
		),
		Entry(
			"read with an attached prompt cluster",
			`for i in 1 2; do read -rp "$i: " x; gh x; done`,
		),
		Entry(
			"read -a with a literal array",
			`for i in 1 2; do read -ra parts <<< "$i"; gh x; done`,
		),
		Entry(
			"mapfile with computed option values",
			`for i in 1 2; do mapfile -t -n "$n" -O "$o" -s "$s" -u "$fd" -d "$d" lines; gh x; done`,
		),
		Entry(
			"readarray with a computed count",
			`for i in 1 2; do readarray -n "$i" lines < f; gh x; done`,
		),
	)

	DescribeTable(
		"fails closed when the loop may set one",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(
				HaveField("Cause", parser.OpacityStartupFile),
			), command)
		},
		Entry(
			"assignment in the loop",
			`for f in a b; do BASH_ENV=$f; export BASH_ENV; bash /abs/run.sh; done`,
		),
		Entry("export before gh", `for f in a b; do export BASH_ENV=$f; gh pr view 1; done`),
		Entry("printf -v with a computed name", `for i in 1 2; do printf -v "$i" x; gh x; done`),
		Entry("printf with a computed first word", `for i in 1 2; do printf $i; gh x; done`),
		Entry(
			"printf -v value attached",
			`for i in 1 2; do gh x; printf -vBASH_ENV /abs/run.sh; done`,
		),
		Entry("read -a value attached", `for i in 1 2; do gh x; read -aBASH_ENV < f; done`),
		Entry("read into a computed name", `for i in 1 2; do read $i; gh x; done`),
		Entry(
			"read with a computed name after a prompt",
			`for i in 1 2; do read -p "$i> " "$i"; gh x; done`,
		),
		Entry("read with a prompt that may split", `for i in 1 2; do read -p $i x; gh x; done`),
		Entry("read with a prompt from a list", `for i in 1 2; do read -p "$@" x; gh x; done`),
		Entry("read with a computed option word", `for i in 1 2; do read "$o" x; gh x; done`),
		Entry("read -a with a computed array", `for i in 1 2; do read -a "$i"; gh x; done`),
		Entry("read with a glob name", `for i in 1 2; do gh x; read -r BASH_EN[V] < f; done`),
		Entry("mapfile into a computed array", `for i in 1 2; do mapfile -n "$n" "$i"; gh x; done`),
		Entry(
			"mapfile with a count that may split",
			`for i in 1 2; do mapfile -n $n lines; gh x; done`,
		),
		Entry("mapfile into BASH_ENV", `for i in 1 2; do gh x; mapfile -t BASH_ENV < f; done`),
		Entry("builtin read into a computed name", `for i in 1 2; do builtin read $i; gh x; done`),
		Entry("command printf -v", `for i in 1 2; do command printf -v $i x; gh x; done`),
		Entry("command -p declare", `for i in 1 2; do command -p declare "$i=x"; gh x; done`),
		Entry(
			"function that exports BASH_ENV",
			`f() { export BASH_ENV=/abs/run.sh; }; for i in 1 2; do gh x; f; done`,
		),
		Entry(
			"function reached through another",
			`g() { export BASH_ENV=$1; }; f() { g "$1"; }; for i in 1 2; do gh x; f a; done`,
		),
		Entry(
			"function running its arguments",
			`f() { "$@"; }; for i in 1 2; do gh x; f read "$i"; done`,
		),
		Entry(
			"function writing a computed name",
			`f() { read "$1"; }; for i in 1 2; do gh x; f "$i"; done`,
		),
		Entry("function running eval", `f() { eval "$1"; }; for i in 1 2; do gh x; f "$i"; done`),
		Entry(
			"function moving ZDOTDIR",
			`f() { export ZDOTDIR=/abs; }; for i in 1 2; do zsh -c true; f; done`,
		),
		Entry("function defined in the same group",
			`{ f() { export BASH_ENV=$(mktemp); }; for i in 1 2; do gh x; f; done; }`),
		Entry("function defined in the same subshell",
			`(f() { read "$1"; }; for i in 1 2; do gh x; f "$i"; done)`),
		Entry(
			"alias called again with a computed name",
			"n=BASH_ENV\nalias p='printf -v'\nf() { p y z; p \"$1\" /tmp/p; }\nfor i in 1 2; do gh x; f \"$n\"; done",
		),
		Entry("printf option from brace expansion",
			`for i in 1 2; do gh x; printf {-v,} "$i" /tmp/p; done`),
		Entry("printf option from a glob", `for i in 1 2; do gh x; printf [-]v "$i" /tmp/p; done`),
		Entry(
			"printf -v target from a glob",
			`for i in 1 2; do gh x; printf -v BASH_EN[V] /tmp/p; done`,
		),
		Entry("local nameref in a function",
			`f() { local -n q="$1"; q=/tmp/p; }; for i in 1 2; do gh x; f "$i"; done`),
		Entry("declare -n then assign in a function",
			`f() { declare -n q; q="$1"; q=/tmp/p; }; for i in 1 2; do gh x; f "$i"; done`),
		Entry("nameref in the loop", `for i in 1 2; do gh x; declare -n q="$i"; q=/tmp/p; done`),
		Entry(
			"mapfile callback",
			`g() { read -r "$1"; }; for i in 1 2; do gh x; mapfile -C g -c 1 a <<< y; done`,
		),
		Entry(
			"trap running a function",
			`g() { read -r "$n" <<< /tmp/p; }; f() { trap g DEBUG; }; for i in 1 2; do gh x; f; done`,
		),
		Entry("trap with a computed action", `for i in 1 2; do gh x; trap "$i" DEBUG; done`),
		Entry(
			"backslash call of a function",
			`g() { read -r "$1" <<< /tmp/p; }; f() { \g "$1"; }; for i in 1 2; do gh x; f "$i"; done`,
		),
		Entry("function shadowing read",
			`read() { :; }; for i in 1 2; do gh x; read -r "$i" <<< /tmp/p; unset -f read; done`),
		Entry(
			"alias shadowing read",
			`alias read=true; for i in 1 2; do gh x; read -r "$i" <<< /tmp/p; done`,
		),
		Entry(
			"indirect default assignment",
			`f() { : ${!1:=/tmp/p}; }; for i in 1 2; do gh x; f "$i"; done`,
		),
		Entry("self-expanding alias", `alias p='p x'; for i in 1 2; do gh x; p; done`),
		Entry("alias for eval", `alias e=eval; for i in 1 2; do gh x; e "$i"; done`),
		Entry("alias for read", `alias r=read; for i in 1 2; do gh x; r "$i"; done`),
		Entry("alias ending in a blank", `alias s='sudo '; for i in 1 2; do gh x; s ls; done`),
	)
	DescribeTable("treats quoted read and mapfile option values as no target",
		func(command string, blocked bool) {
			Expect(parse(command).Truncated).To(Equal(blocked), command)
		},
		Entry("quoted prompt", `read -p "$P> " x; bash /abs/run.sh`, false),
		Entry("quoted prompt in a cluster", `read -rp "$P> " x; bash /abs/run.sh`, false),
		Entry("quoted mapfile count", `mapfile -n "$N" lines < f; bash /abs/run.sh`, false),
		Entry("unquoted prompt", `read -p $P x; bash /abs/run.sh`, true),
		Entry("prompt partly unquoted", `read -p "a"$P x; bash /abs/run.sh`, true),
		Entry("prompt from a list", `read -p "$@" x; bash /abs/run.sh`, true),
		Entry("computed name after a quoted prompt", `read -p "$P" "$N"; bash /abs/run.sh`, true),
	)
})
