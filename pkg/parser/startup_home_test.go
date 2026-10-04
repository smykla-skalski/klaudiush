package parser_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Home startup files", func() {
	const payload = "git push --force"

	resolver := fakeResolver{
		env: map[string]string{"HOME": "/home/u"},
		files: map[string]string{
			"/z/.zshenv":          payload,
			"/zl/.zprofile":       payload,
			"/zl2/.zlogin":        payload,
			"/zi/.zshrc":          payload,
			"/zd/.zshenv":         payload,
			"/zf/.zshenv":         "git() { command git push --force; }",
			"/zm/.zshenv":         "ZDOTDIR=/zm2",
			"/zm/.zshrc":          "true",
			"/zlo/.zlogout":       payload,
			"sub/.zshenv":         payload,
			"/s/envz.zsh":         "#!/usr/bin/env -S ZDOTDIR=/zd zsh\ntrue",
			"/h8/.zshenv":         "echo 'git push --force' > /h8/.zshrc",
			"/blo/.bash_logout":   payload,
			"/t/.tcshrc":          payload,
			"/c/.cshrc":           payload,
			"/zc/.zshenv":         "cd /e\nZDOTDIR=.",
			"/e/.zshrc":           payload,
			"/zm2/.zshrc":         payload,
			"/zx/.zshenv":         "ZDOTDIR=$(mktemp -d)",
			"/zs/.zshenv":         "zsh -c true",
			"/zbad/.zshenv":       "if then",
			"/b/.bashrc":          payload,
			"/bp/.bash_profile":   payload,
			"/bl/.bash_login":     payload,
			"/bpr/.profile":       payload,
			"/k/.kshrc":           payload,
			"/y/.yash_profile":    payload,
			"/s/run.zsh":          "#!/bin/zsh\ntrue",
			"/s/env.zsh":          "#!/usr/bin/env zsh\ntrue",
			"/s/login.bash":       "#!/bin/bash -l\ntrue",
			"/s/run.sh":           "#!/bin/sh\ntrue",
			"/s/plain":            "true",
			"/home/u/.zshenv.bak": payload,
		},
		opaque: map[string]bool{"/zo/.zshenv": true},
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

	DescribeTable(
		"follows the startup files a shell reads from its home",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(pushed(result)).To(BeTrue(), command)
		},
		Entry("zshenv written with ~", `echo 'git push --force' > ~/.zshenv; zsh -c true`),
		Entry("zshenv written with $HOME",
			`echo 'git push --force' > "$HOME/.zshenv"; zsh -c true`),
		Entry("zshenv written with an absolute path",
			`echo 'git push --force' > /home/u/.zshenv; zsh -c true`),
		Entry("zshenv written after cd home", `cd && echo 'git push --force' > .zshenv && zsh`),
		Entry(
			"zshenv written by a heredoc",
			"cat > ~/.zshenv <<'E'\ngit push --force\nE\nzsh -c true",
		),
		Entry("zshenv written under a HOME set on the line",
			`HOME=/h; echo 'git push --force' > ~/.zshenv; zsh -c true`),
		Entry(
			"zshenv written inside a shell",
			`bash -c "echo 'git push --force' > ~/.zshenv"; zsh`,
		),
		Entry("zshenv on disk", `HOME=/z zsh -c true`),
		Entry("zshenv for a script file", `HOME=/z zsh script.zsh`),
		Entry("zshenv for zsh on stdin", `echo true | HOME=/z zsh`),
		Entry("exported HOME", `export HOME=/z; zsh -c true`),
		Entry("env operand HOME", `env HOME=/z zsh -c true`),
		Entry("ZDOTDIR prefix", `ZDOTDIR=/zd zsh -c true`),
		Entry("exported ZDOTDIR", `export ZDOTDIR=/zd; zsh -c true`),
		Entry("ZDOTDIR assigned but maybe not exported", `ZDOTDIR=/none; HOME=/z zsh -c true`),
		Entry("ZDOTDIR unset", `export ZDOTDIR=/none; unset ZDOTDIR; HOME=/z zsh -c true`),
		Entry("zprofile for a login zsh", `HOME=/zl zsh -l -c true`),
		Entry("zlogin for a login zsh", `HOME=/zl2 zsh --login -c true`),
		Entry("zlogin for zsh -o login", `HOME=/zl2 zsh -o login -c true`),
		Entry("zshrc for an interactive zsh", `HOME=/zi zsh -i -c true`),
		Entry("zshrc in a cluster", `HOME=/zi zsh -ic true`),
		Entry("zshrc from the ZDOTDIR zshenv sets", `HOME=/zm zsh -i -c true`),
		Entry("zsh -f turned back on", `HOME=/z zsh -f -o rcs -c true`),
		Entry("function zshenv defines", `HOME=/zf zsh -c 'git status'`),
		Entry("bashrc for an interactive bash", `HOME=/b bash -i -c true`),
		Entry("bash_profile for a login bash", `HOME=/bp bash -lc true`),
		Entry("bash_login for a login bash", `HOME=/bl bash --login -c true`),
		Entry("profile for a login bash", `HOME=/bpr bash -l -c true`),
		Entry("login after -o with a value", `HOME=/bp bash -eo pipefail -l -c true`),
		Entry("profile for a login sh", `HOME=/bpr sh -l -c true`),
		Entry("profile for a login dash", `HOME=/bpr dash -l`),
		Entry("kshrc for an interactive ksh", `HOME=/k ksh -i -c true`),
		Entry("yash_profile for a login yash", `HOME=/y yash -l -c true`),
		Entry("zsh shebang", `HOME=/z /s/run.zsh`),
		Entry("env zsh shebang", `HOME=/z /s/env.zsh`),
		Entry("login bash shebang", `HOME=/bp /s/login.bash`),
		Entry("bare shell", `HOME=/z zsh`),
		Entry("nested shell", `bash -c 'HOME=/z zsh -c true'`),
		Entry("zlogout for a login zsh", `HOME=/zlo zsh -l -c true`),
		Entry("bash_logout the script writes",
			`HOME=/h bash --noprofile -lc 'echo "git push --force" > /h/.bash_logout; exit'`),
		Entry("login option from a variable", `f=-l; HOME=/bp bash "$f" -c true`),
		Entry("relative HOME after a relative cd", `cd sub; HOME=. zsh -c true`),
		Entry("ZDOTDIR set by env in a shebang", `/s/envz.zsh`),
		Entry("zshrc written by zshenv", `HOME=/h8 zsh -ic true`),
		Entry("bash_logout for a login bash", `HOME=/blo bash -l -c exit`),
		Entry("exec -l zsh", `HOME=/zl exec -l zsh -c true`),
		Entry("exec -a with a dash", `HOME=/zl exec -a -zsh zsh -c true`),
		Entry("exec -a from a variable", `HOME=/zl exec -a "$N" zsh -c true`),
		Entry("exec -l bash", `HOME=/bp exec -l bash`),
		Entry("tcshrc", `HOME=/t tcsh -c true`),
		Entry("cshrc", `HOME=/c csh -c true`),
		Entry("profile for a login oksh", `HOME=/bpr oksh -l -c true`),
		Entry("zsh -f turned back on by +f", `HOME=/z zsh -f +f -c true`),
		Entry("relative ZDOTDIR after a cd in zshenv", `HOME=/zc zsh -i -c true`),
		Entry("zsh --emulate sh with a script", `zsh --emulate sh -c 'git push --force'`),
		Entry("profile for zsh --emulate sh -l", `HOME=/bpr zsh --emulate sh -l -c true`),
	)

	It("reads login files when an unresolved word may be -l", func() {
		Expect(pushed(parse(`HOME=/bp bash "$UNSET" -c true`))).To(BeTrue())
		Expect(parse(`HOME=/bp bash "$UNSET"`).GitOperations).To(BeEmpty())
	})

	It("validates zshenv before the script", func() {
		result := parse(`HOME=/z zsh -c 'git status'`)

		Expect(result.GitOperations).To(HaveLen(2))
		Expect(result.GitOperations[0].Args).To(HaveExactElements("push", "--force"))
		Expect(result.GitOperations[1].Args).To(HaveExactElements("status"))
	})

	It("names the startup file in the origin of what it runs", func() {
		result := parse(`HOME=/zbad zsh -c true`)

		Expect(result.Opacities).To(ContainElement(SatisfyAll(
			HaveField("Cause", parser.OpacityScriptSyntax),
			HaveField("Operation", ".zshenv"),
			HaveField("Origin", HaveExactElements("zsh", ".zshenv")),
		)))
	})

	It("follows env removing ZDOTDIR or the whole environment", func() {
		exported := fakeResolver{
			env:   map[string]string{"HOME": "/z", "ZDOTDIR": "/benign"},
			files: map[string]string{"/z/.zshenv": payload},
		}

		for command, want := range map[string]bool{
			`env -u ZDOTDIR zsh -c true`:           true,
			`env --unset=ZDOTDIR zsh -c true`:      true,
			`env -uZDOTDIR zsh -c true`:            true,
			`env -i HOME=/none zsh -c true`:        false,
			`zsh -c true`:                          false,
			`env -u ZDOTDIR bash -c 'zsh -c true'`: true,
		} {
			result, err := parser.NewBashParserWithResolver(exported).Parse(command)
			Expect(err).NotTo(HaveOccurred(), command)
			Expect(result.Truncated).To(BeFalse(), command)
			Expect(pushed(result)).To(Equal(want), command)
		}
	})

	It("does not read zshenv again in the zsh it starts", func() {
		result := parse(`HOME=/zs zsh -c true`)

		Expect(result.Truncated).To(BeFalse())
		Expect(result.GetCommands("zsh")).To(HaveLen(2))
	})

	DescribeTable("fails closed on a home startup file it cannot see",
		func(command, operation, detail string) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(SatisfyAll(
				HaveField("Cause", parser.OpacityStartupFile),
				HaveField("Operation", operation),
				HaveField("Detail", detail),
			)), command)
		},
		Entry("HOME from command output", `HOME=$(mktemp -d) zsh -c true`,
			"HOME", parser.DetailStartupValue),
		Entry("HOME from command output for a login bash", `HOME=$(mktemp -d) bash -l`,
			"HOME", parser.DetailStartupValue),
		Entry("exported HOME from command output", `export HOME="$(pwd)"; zsh -c true`,
			"HOME", parser.DetailStartupValue),
		Entry("unknown ZDOTDIR", `ZDOTDIR=$UNSET zsh -c true`,
			"ZDOTDIR", parser.DetailScriptVariable),
		Entry("ZDOTDIR the shell expands", `ZDOTDIR='$(id)' zsh -c true`,
			"ZDOTDIR", parser.DetailStartupExpansion),
		Entry("HOME read", `read HOME; zsh -c true`, "HOME", parser.DetailStartupValue),
		Entry("HOME unset", `unset HOME; zsh -c true`, "HOME", parser.DetailStartupValue),
		Entry("HOME removed by env -u", `env -u HOME zsh -c true`,
			"HOME", parser.DetailStartupValue),
		Entry("environment cleared by env -i", `env -i zsh -c true`,
			"HOME", parser.DetailStartupValue),
		Entry("HOME set later in a loop", `for i in 1 2; do zsh -c true; HOME=/z; done`,
			"HOME", parser.DetailStartupValue),
		Entry("ZDOTDIR set by a default in a loop",
			`for i in 1 2; do zsh -c true; : "${ZDOTDIR:=/zd}"; done`,
			"ZDOTDIR", parser.DetailStartupValue),
		Entry("names from command output", `export $(cat .env); zsh -c true`,
			"HOME", parser.DetailStartupValue),
		Entry("copied over zshenv", `cp evil ~/.zshenv; zsh -c true`,
			".zshenv", parser.DetailScriptWritten),
		Entry("appended to zshenv", `echo 'git push' >> ~/.zshenv; zsh -c true`,
			".zshenv", parser.DetailScriptWritten),
		Entry("written under an unknown directory", `echo x > "$D/.zshenv"; zsh -c true`,
			".zshenv", parser.DetailScriptWritten),
		Entry("written after an unknown cd", `cd "$D"; echo x > .bashrc; bash -i -c true`,
			".bashrc", parser.DetailScriptWritten),
		Entry("written by command output", `cat a > ~/.profile; sh -l -c true`,
			".profile", parser.DetailScriptWritten),
		Entry("written to a path from command output",
			`echo x > "$(pwd)/.zshenv"; zsh -c true`, ".zshenv", parser.DetailScriptWritten),
		Entry("written under a variable from command output",
			`d=$(pwd); echo x > "$d/.zshenv"; zsh -c true`, ".zshenv", parser.DetailScriptWritten),
		Entry("written under a cd from command output",
			`d=$(pwd); cd "$d"; echo x > .zshenv; HOME=/z zsh -c true`,
			".zshenv", parser.DetailScriptWritten),
		Entry("written under another user's home", `echo x > ~root/.zshenv; zsh -c true`,
			".zshenv", parser.DetailScriptWritten),
		Entry("relative HOME after an unknown cd", `cd "$D"; HOME=. zsh -c true`,
			".zshenv", parser.DetailScriptDirectory),
		Entry("relative HOME after a computed cd", `cd "$(echo /z)"; HOME=. zsh -c true`,
			".zshenv", parser.DetailScriptDirectory),
		Entry("relative HOME after an unknown cd from a known one",
			`cd /safe; cd "$D"; HOME=. zsh -c true`, ".zshenv", parser.DetailScriptDirectory),
		Entry("written under a variable read on the line",
			`D=/benign; read D <<< /evil; echo 'git push --force' > "$D/.zshenv"; `+
				`ZDOTDIR=/evil zsh -c true`,
			".zshenv", parser.DetailScriptWritten),
		Entry("HOME on a device", `HOME=/dev/fd zsh -c true`,
			".zshenv", parser.DetailScriptRead),
		Entry("unreadable zshenv", `HOME=/zo zsh -c true`, ".zshenv", parser.DetailScriptRead),
		Entry("ZDOTDIR from command output in zshenv", `HOME=/zx zsh -i -c true`,
			"ZDOTDIR", parser.DetailStartupValue),
		Entry("tilde HOME after HOME changes", `HOME=/a; HOME=~/b zsh -c true`,
			"HOME", parser.DetailScriptVariable),
	)

	DescribeTable("leaves shells that read no such file alone",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(pushed(result)).To(BeFalse(), command)
		},
		Entry("zsh without startup files", `zsh -c true`),
		Entry("missing home", `HOME=/missing zsh -i -l -c true`),
		Entry("bash -c reads no home file", `HOME=/b bash -c true`),
		Entry("bash -c with HOME from command output", `HOME=$(mktemp -d) bash -c true`),
		Entry("program with HOME from command output", `HOME=$(mktemp -d) git status`),
		Entry("zshenv written for bash -c", `echo 'git push' > ~/.zshenv; bash -c true`),
		Entry("bashrc for a login bash", `HOME=/b bash -l -i -c true`),
		Entry("bashrc with --norc", `HOME=/b bash --norc -i -c true`),
		Entry("bashrc replaced by --rcfile", `HOME=/b bash --rcfile /dev/null -i -c true`),
		Entry("bashrc for a posix bash", `HOME=/b bash --posix -i -c true`),
		Entry("bashrc for bash -o posix", `HOME=/b bash -o posix -i -c true`),
		Entry("profiles with --noprofile", `HOME=/bp bash --noprofile -l -c true`),
		Entry("profile for sh -c", `HOME=/bpr sh -c true`),
		Entry("zprofile without login", `HOME=/zl zsh -c true`),
		Entry("zshrc without -i", `HOME=/zi zsh -c true`),
		Entry("zsh -f", `HOME=/z zsh -f -c true`),
		Entry("zsh -o no_rcs", `HOME=/z zsh -o no_rcs -c true`),
		Entry("zsh +o rcs", `HOME=/z zsh +o rcs -c true`),
		Entry("zsh --no-rcs", `HOME=/z zsh --no-rcs -c true`),
		Entry("-l after the command", `HOME=/bp bash -c true -l`),
		Entry("-l after --", `HOME=/bp bash -- -l`),
		Entry("sh shebang", `HOME=/bpr /s/run.sh`),
		Entry("no shebang", `HOME=/z /s/plain`),
		Entry("sourced zsh script", `HOME=/z; source /s/run.zsh`),
		Entry("loop reading HOME", `for d in a b; do zsh -c "ls $HOME/$d"; done`),
		Entry("prefix HOME on another command", `HOME=/z true; zsh -c true`),
		Entry("other file under home", `zsh -c 'cat ~/.zshenv.bak'`),
		Entry("zsh --emulate sh reads no zshenv", `HOME=/z zsh --emulate sh -c true`),
		Entry("csh -f", `HOME=/c csh -f -c true`),
		Entry("exec without -l", `HOME=/zl exec -a zsh -c true`),
	)
})

var _ = Describe("Home startup files the line never touched", func() {
	const brew = `eval "$(/opt/homebrew/bin/brew shellenv)"`

	resolver := fakeResolver{
		env: map[string]string{"HOME": "/env"},
		files: map[string]string{
			"/env/.zprofile": brew,
			"/env/.zshenv":   "path=(${(s.:.)PATH})\nZDOTDIR=$(mktemp -d)",
			"/env/.bashrc":   strings.Repeat("true\n", 3000) + "git push --force",
			"/env/.profile":  "git push --force\n" + brew,
		},
		opaque: map[string]bool{"/env/.zlogin": true},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred(), command)

		return result
	}

	DescribeTable("follows them without blocking on what it cannot see",
		func(command string, git ...string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(result.Opacities).To(BeEmpty(), command)

			ops := make([]string, 0, len(result.GitOperations))
			for _, op := range result.GitOperations {
				ops = append(ops, strings.Join(op.Args, " "))
			}

			if len(git) == 0 {
				Expect(ops).To(BeEmpty(), command)
			} else {
				Expect(ops).To(Equal(git), command)
			}
		},
		Entry("brew shellenv in .zprofile", `zsh -l -c true`),
		Entry("the script still checked", `zsh -l -c 'git status'`, "status"),
		Entry("zsh syntax and a computed ZDOTDIR in .zshenv", `zsh -i -c true`),
		Entry("unreadable .zlogin", `zsh -l -c true`),
		Entry("commands still recorded", `sh -l -c true`, "push --force"),
		Entry("past the work budget, leaving the line's own", `bash -i -c 'git status'`,
			"status"),
		Entry("a reader naming the file", `cat ~/.zprofile; zsh -l -c true`),
		Entry("git with a variable", `git log "$X"; zsh -l -c true`, "log ${X}"),
	)

	It("does not count the shell's own arguments as touching them", func() {
		result := parse(`sudo bash -l "$X/run.sh"`)

		Expect(result.Opacities).To(ConsistOf(
			HaveField("Cause", parser.OpacityUnreadableScript),
		))
	})

	DescribeTable("checks them strictly once the line may have changed them",
		func(command string) {
			Expect(parse(command).Truncated).To(BeTrue(), command)
		},
		Entry("sed -i", `sed -i 's/a/b/' ~/.zprofile; zsh -l -c true`),
		Entry("ln", `ln -sf /tmp/p ~/.zprofile; zsh -l -c true`),
		Entry("cp into home", `cp -r cfg/. ~; zsh -l -c true`),
		Entry("rsync into $HOME", `rsync -a cfg/ "$HOME/"; zsh -l -c true`),
		Entry("write to an unknown path", `echo x > "$F"; zsh -l -c true`),
		Entry("command on an unknown path", `mv a "$F"; zsh -l -c true`),
		Entry("HOME set on the line", `HOME=/env zsh -l -c true`),
		Entry("ZDOTDIR set on the line", `export ZDOTDIR=/env; zsh -l -c true`),
		Entry("written on the line",
			"echo 'eval \"$(x)\"' > ~/.zprofile; zsh -l -c true"),
	)
})
