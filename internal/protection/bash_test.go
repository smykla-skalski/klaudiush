package protection_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/protection"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

func checkCommand(set *protection.Set, command string) []protection.Violation {
	result, err := parser.NewBashParser().Parse(command)
	Expect(err).NotTo(HaveOccurred())
	Expect(result.Truncated).To(BeFalse(), command)

	return set.CheckCommand(context.Background(), result, command)
}

var _ = Describe("CheckCommand", func() {
	var (
		e   *env
		set *protection.Set
	)

	BeforeEach(func() {
		e = newEnv(GinkgoT().TempDir(), "linux", nil)
		e.write("project/build/out.txt", "")
		set = e.set()
	})

	DescribeTable(
		"blocks commands that change policy files",
		func(command string) {
			Expect(checkCommand(set, command)).NotTo(BeEmpty(), command)
		},
		Entry("redirect", `echo '{}' > .claude/settings.json`),
		Entry("append", `echo x >> .klaudiush/config.toml`),
		Entry("clobber", `echo x >| .klaudiush/config.toml`),
		Entry("redirect both", `make &> .klaudiush/config.toml`),
		Entry("read-write redirect", `exec 3<> .klaudiush/config.toml`),
		Entry("redirect to a file name after >&", `echo x >& .klaudiush/config.toml`),
		Entry("heredoc", "cat <<EOF > .klaudiush/config.toml\nx\nEOF"),
		Entry("tee", `echo x | tee -a .claude/settings.json`),
		Entry("sed in place", `sed -i 's/a/b/' .klaudiush/config.toml`),
		Entry("sed in place with suffix", `sed -i.bak -e 's/a/b/' .klaudiush/config.toml`),
		Entry("sed bundled in place", `sed -Ei 's/a/b/' .klaudiush/config.toml`),
		Entry("sed write command", `sed -n 'w .klaudiush/config.toml' main.go`),
		Entry("perl in place", `perl -pi -e 's/a/b/' .claude/settings.json`),
		Entry("rm", `rm .claude/settings.json`),
		Entry("rm directory", `rm -rf .klaudiush`),
		Entry("rm parent of protected", `rm -rf .claude`),
		Entry("rm working directory", `rm -rf .`),
		Entry("mv source", `mv .klaudiush/config.toml /tmp/x`),
		Entry("mv destination", `mv x.toml .klaudiush/config.toml`),
		Entry("cp destination", `cp evil.json .claude/settings.json`),
		Entry("cp into protected name", `cp /tmp/settings.json .claude/`),
		Entry("cp recursive into project", `cp -r /tmp/evil/. .`),
		Entry("cp as hard link", `cp -l .klaudiush/config.toml x`),
		Entry("install", `install -m 644 x .claude/settings.json`),
		Entry("rsync with delete", `rsync -a --delete /tmp/empty/ .klaudiush/`),
		Entry("ln to protected", `ln -s .klaudiush/config.toml link.toml`),
		Entry("ln at protected", `ln -sf /tmp/x .claude/settings.json`),
		Entry("hard link", `ln .klaudiush/config.toml link.toml`),
		Entry("truncate", `truncate -s 0 .klaudiush/config.toml`),
		Entry("touch", `touch .klaudiush/config.toml`),
		Entry("chmod", `chmod 000 .claude/settings.json`),
		Entry("chmod recursive on project", `chmod -R 777 .`),
		Entry("dd output", `dd if=/dev/zero of=.klaudiush/config.toml count=1`),
		Entry("curl output", `curl -o .claude/settings.json https://example.com`),
		Entry("curl long output", `curl --output=.claude/settings.json https://example.com`),
		Entry("python code", `python3 -c "open('.claude/settings.json','w').write('{}')"`),
		Entry("node code", `node -e "require('fs').writeFileSync('.klaudiush/config.toml','')"`),
		Entry("absolute path", "rm "+filepath.Join("/", "etc", "codex", "requirements.toml")),
		Entry("home tilde", `rm ~/.codex/hooks.json`),
		Entry("home variable", `rm $HOME/.codex/hooks.json`),
		Entry("braced home variable", `rm "${HOME}/.claude/settings.json"`),
		Entry("assigned variable", `F=.claude/settings.json; rm "$F"`),
		Entry("chained variable", `D=.claude; F="$D/settings.json"; echo x > "$F"`),
		Entry("unknown prefix variable", `rm "$SOMEWHERE/.claude/settings.json"`),
		Entry(
			"unknown variable naming a mentioned file",
			`for f in .claude/settings.json; do rm "$f"; done`,
		),
		Entry("command substitution prefix", `rm "$(echo .claude)/settings.json"`),
		Entry("command substitution target", `echo x > "$(echo .klaudiush/config.toml)"`),
		Entry("backticks", "rm `echo .claude`/settings.json"),
		Entry("glob", `rm .cl?ude/settings*.json`),
		Entry("bracket glob", `rm .clau[d]e/settings.json`),
		Entry("brace list", `rm .c{l,x}aude/settings.json`),
		Entry("double star", `rm **/settings.json`),
		Entry("star in project", `rm -rf *`),
		Entry("parent directory", `rm sub/../.claude/settings.json`),
		Entry("double slash", `rm .claude//settings.json`),
		Entry("cd chain", `cd .claude && rm settings.json`),
		Entry("cd chain with subshell", `(cd .klaudiush; echo x > config.toml)`),
		Entry("env launcher", `env FOO=1 rm .claude/settings.json`),
		Entry("sudo", `sudo rm /etc/codex/requirements.toml`),
		Entry("xargs from echo", `echo .claude/settings.json | xargs rm`),
		Entry("xargs from here-string", `xargs rm <<< .claude/settings.json`),
		Entry("bash -c", `bash -c 'rm .claude/settings.json'`),
		Entry("eval", `eval "rm .claude/settings.json"`),
		Entry("find delete", `find .klaudiush -delete`),
		Entry("find exec", `find . -name config.toml -exec rm {} \;`),
		Entry("find delete everything", `find . -delete`),
		Entry("git checkout path", `git checkout -- .claude/settings.json`),
		Entry("git restore project", `git restore .`),
		Entry("git -C", `git -C .claude rm settings.json`),
		Entry("git config file", `git config -f .klaudiush/config.toml a.b c`),
		Entry("editor", `vim .claude/settings.json`),
		Entry("unknown program", `my-tool --write .klaudiush/config.toml`),
		Entry("program via variable", `$EDITOR .claude/settings.json`),
		Entry("yq in place", `yq -i '.a = 1' .klaudiush/config.toml`),
		Entry("sort output", `sort -o .klaudiush/config.toml main.go`),
		Entry("awk redirect", `awk '{print > ".klaudiush/config.toml"}' main.go`),
		Entry("new config in subdirectory", `echo '[protection]' > sub/klaudiush.toml`),
		Entry("state file", `rm `+"~/.local/state/klaudiush/hook_sessions/state.json"),
		Entry("binary", `cp /tmp/fake ~/bin/klaudiush`),
		Entry("cp target directory", `cp -t .claude settings.json`),
		Entry("cp long target directory", `cp --target-directory .claude settings.json`),
		Entry("install target directory", `install -m 644 -t .claude settings.json`),
		Entry("mv target directory", `mv -t .claude settings.json`),
		Entry("ditto contents", `ditto /tmp/evil .claude`),
		Entry("rsync contents", `rsync /tmp/evil/ .claude`),
		Entry("find protected name", `find /tmp/elsewhere -name settings.json -delete`),
		Entry("find protected dir name", `find . -name '.klau*' -exec rm -rf {} +`),
		Entry("suggest output", `klaudiush suggest --output .klaudiush/config.toml`),
		Entry("suggest output equals", `klaudiush suggest --output=.claude/settings.json`),
		Entry("hook mode", `echo '{}' | klaudiush --event SessionStart`),
		Entry("escaped redirect target", `echo x > .klaudiu\sh/config.toml`),
		Entry("escaped dot", `echo x > \.mcp.json`),
		Entry("escaped directory", `rm -rf .klaudiu\sh`),
		Entry("trailing backslash", "echo x > .claude/settings.json\\"),
		Entry("escaped path", `echo x > /etc/claude\-code/managed\ settings.json`),
		Entry("ANSI-C quoting", `rm $'\x2eclaude/settings.json'`),
		Entry("ANSI-C octal", `rm $'\056klaudiush/config.toml'`),
		Entry("variable from substitution", `d=$(echo .claude) && echo x > "$d/settings.json"`),
		Entry("appended variable", `d=.cl; d+=aude; rm "$d/settings.json"`),
		Entry("ln relative target", `ln -sf settings.json .claude/settings.local.json`),
		Entry("ln into directory", `ln -s ../.klaudiush/config.toml build`),
		Entry("dot slash glob", `rm -rf ./.k*`),
		Entry("dot slash dot glob", `rm -rf ./.??*`),
		Entry("parent glob", `rm -rf sub/../.k*`),
		Entry("dot slash nested glob", `rm ./.c*/settings.json`),
		Entry("posix class", `rm -rf .[[:alpha:]]*`),
		Entry("brace range", `rm .claude/settings.{i..k}son`),
		Entry("zsh negation", `rm .claude/^foo`),
		Entry("zsh clobber", `echo x >! .mcp.json`),
		Entry("glob loop", `for f in .m*; do rm "$f"; done`),
		Entry("python on stdin", "python3 - <<'X'\nopen('.mcp.json','w')\nX"),
		Entry("python here-string", `python3 <<< "open('.mcp.json','w')"`),
		Entry("xargs null", `printf '%s\0' .mcp.json | xargs -0 rm`),
		Entry(
			"interpreter symlink",
			`python3 -c "import os; os.symlink('settings.json','.claude/settings.local.json')"`,
		),
		Entry("path shim", `ln -sf /bin/true ~/bin/klaudiush`),
	)

	DescribeTable("lets other commands through",
		func(command string) {
			Expect(checkCommand(set, command)).To(BeEmpty(), command)
		},
		Entry("read", `cat .klaudiush/config.toml`),
		Entry("grep", `grep -r enabled .klaudiush`),
		Entry("sed read", `sed -n '1,5p' .claude/settings.json`),
		Entry("jq read", `jq . .claude/settings.json`),
		Entry("diff", `diff .claude/settings.json /tmp/x`),
		Entry("ls project", `ls -la .`),
		Entry("copy from protected", `cp .klaudiush/config.toml /tmp/backup.toml`),
		Entry("other redirect", `echo hi > notes.txt`),
		Entry("rm other", `rm -rf build`),
		Entry("go test", `go test ./...`),
		Entry("pytest on project", `pytest .`),
		Entry("find delete filtered", `find . -name '*.pyc' -delete`),
		Entry("unknown variable without mention", `rm -rf "$BUILD_DIR"/*`),
		Entry("unknown target without mention", `echo x > "$OUT"`),
		Entry("dynamic target without mention", `echo x > "$(mktemp)"`),
		Entry("cp into project", `cp /tmp/a.txt .`),
		Entry("git commit mentioning a path", `git commit -m "update .claude/settings.json docs"`),
		Entry("gh body mentioning a path", `gh pr create --body "edits .klaudiush/config.toml"`),
		Entry("mkdir", `mkdir -p .claude/agents`),
		Entry("klaudiush evidence", `klaudiush evidence run tests`),
		Entry("klaudiush suggest", `klaudiush suggest --output KLAUDIUSH.md`),
		Entry("cp file into agents dir", `cp agent.md .claude/agents/`),
		Entry("substitution read into other file", `echo $(cat .mcp.json) > out.txt`),
		Entry("find listing", `find . -type f`),
		Entry("find by protected name", `find . -name settings.json`),
		Entry("copy protected file out", `cp .mcp.json backup.json`),
		Entry("find other name", `find . -name '*.orig' -delete`),
		Entry("klaudiush doctor", `klaudiush doctor --verbose`),
		Entry("devnull", `make 2>/dev/null >/dev/null`),
		Entry("descriptor duplicate", `make >&2`),
	)

	DescribeTable(
		"blocks klaudiush commands that change policy",
		func(command, sub string) {
			violations := checkCommand(set, command)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Command).To(Equal(sub))
		},
		Entry("bypass skip", `klaudiush bypass skip --reason x`, "bypass skip"),
		Entry("disable", `klaudiush disable policy.protection`, "disable policy.protection"),
		Entry("init", `klaudiush init --force`, "init"),
		Entry("update", `klaudiush update`, "update"),
		Entry("doctor fix", `klaudiush doctor --fix`, "doctor --fix"),
		Entry("backup restore", `klaudiush backup restore abc`, "backup restore"),
		Entry("crash clean", `klaudiush debug crash clean`, "debug crash clean"),
		Entry(
			"with config flag",
			`klaudiush --config x.toml overrides disable`,
			"overrides disable",
		),
		Entry("dispatcher path", `~/.claude/hooks/dispatcher bypass enforce`, "bypass enforce"),
	)

	It("reports the protected file and the program", func() {
		violations := checkCommand(set, `rm .claude/settings.json`)
		Expect(violations).To(HaveLen(1))
		Expect(violations[0].Program).To(Equal("rm"))
		Expect(violations[0].Target).To(Equal(".claude/settings.json"))
		Expect(violations[0].Reason).To(Equal(protection.ReasonClaudeSettings))
	})

	It("resolves relative paths from the hook's working directory", func() {
		sub := filepath.Join(e.project, "sub")
		Expect(os.MkdirAll(sub, 0o755)).To(Succeed())

		e.opts.WorkDir = sub
		set = e.set()

		Expect(checkCommand(set, `rm ../.claude/settings.json`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `rm .claude/settings.json`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `rm ../main.go`)).To(BeEmpty())
	})

	It("follows a symlink created earlier", func() {
		Expect(os.Symlink(
			filepath.Join(e.project, ".klaudiush", "config.toml"),
			filepath.Join(e.project, "cfg"),
		)).To(Succeed())

		Expect(checkCommand(e.set(), `echo x > cfg`)).NotTo(BeEmpty())
	})
})

var _ = Describe("CheckCommand after an unresolved cd", func() {
	It("matches relative names in any directory", func() {
		set := newEnv(GinkgoT().TempDir(), "linux", nil).set()

		Expect(checkCommand(set, `cd "$DIR" && rm settings.json`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `cd "$DIR" && echo x > notes.txt`)).To(BeEmpty())
		Expect(checkCommand(set, `cd "$DIR" && echo x > .klaudiush/config.toml`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `cd "$DIR" && find . -name '*.tmp' -delete`)).To(BeEmpty())
		Expect(
			checkCommand(set, `cd "$DIR" && git -C .claude checkout settings.json`),
		).NotTo(BeEmpty())
		Expect(checkCommand(set, `cd "$DIR" && rm -rf build`)).To(BeEmpty())
	})
})

var _ = Describe("CheckCommand with a renamed klaudiush", func() {
	It("recognizes a copy of the binary", func() {
		e := newEnv(GinkgoT().TempDir(), "linux", nil)
		binary := e.write("home/bin/klaudiush", "#!/bin/sh\necho klaudiush\n")
		copied := e.write("project/k", "#!/bin/sh\necho klaudiush\n")
		Expect(os.Link(binary, filepath.Join(e.project, "k2"))).To(Succeed())
		e.write("project/other", "#!/bin/sh\necho other\n")

		set := e.set()

		violations := checkCommand(set, copied+" bypass skip")
		Expect(violations).To(HaveLen(1))
		Expect(violations[0].Command).To(Equal("bypass skip"))
		Expect(checkCommand(set, "./k2 disable x")).NotTo(BeEmpty())
		Expect(checkCommand(set, "./other bypass skip")).To(BeEmpty())
		Expect(checkCommand(set, "./k version")).To(BeEmpty())
	})
})
