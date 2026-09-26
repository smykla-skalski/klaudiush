package parser_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Command resolution", func() {
	var p *parser.BashParser

	BeforeEach(func() {
		p = parser.NewBashParser()
	})

	gitCommitArgs := func(command string) [][]string {
		result, err := p.Parse(command)
		Expect(err).NotTo(HaveOccurred())

		var found [][]string

		for _, cmd := range result.GitOperations {
			if len(cmd.Args) > 0 && cmd.Args[0] == "commit" {
				found = append(found, cmd.Args)
			}
		}

		return found
	}

	DescribeTable("finds git however it is invoked",
		func(command string) {
			found := gitCommitArgs(command)
			Expect(found).NotTo(BeEmpty(), "git commit not found in %q", command)
			Expect(found[0]).To(ContainElement("-S"))
		},
		Entry("absolute path", "/usr/bin/git commit -S -m x"),
		Entry("relative path", "./git commit -S -m x"),
		Entry("home path", "~/bin/git commit -S -m x"),
		Entry("backslash escape", `\git commit -S -m x`),
		Entry("escape inside the name", `g\it commit -S -m x`),
		Entry("variable assigned earlier", "G=/usr/bin/git; $G commit -S -m x"),
		Entry("command builtin", "command git commit -S -m x"),
		Entry("exec builtin", "exec git commit -S -m x"),
		Entry("exec with -a", "exec -a name git commit -S -m x"),
		Entry("env", "env git commit -S -m x"),
		Entry("env with options and assignments", "env -i -u HOME FOO=1 git commit -S -m x"),
		Entry("env by path", "/usr/bin/env git commit -S -m x"),
		Entry("env -S", `env -S "git commit -S -m x"`),
		Entry("sudo", "sudo git commit -S -m x"),
		Entry("sudo with value flag", "sudo -u root git commit -S -m x"),
		Entry("sudo with long flag", "sudo --user=root git commit -S -m x"),
		Entry("sudo with --", "sudo -- git commit -S -m x"),
		Entry("doas", "doas -u root git commit -S -m x"),
		Entry("nohup", "nohup git commit -S -m x"),
		Entry("setsid", "setsid git commit -S -m x"),
		Entry("nice", "nice -n 5 git commit -S -m x"),
		Entry("ionice", "ionice -c 3 git commit -S -m x"),
		Entry("timeout", "timeout 30 git commit -S -m x"),
		Entry("timeout with signal", "timeout -s KILL 30s git commit -S -m x"),
		Entry("time keyword", "time git commit -S -m x"),
		Entry("time binary", "/usr/bin/time -o out git commit -S -m x"),
		Entry("stdbuf", "stdbuf -oL git commit -S -m x"),
		Entry("xargs", "echo x | xargs -I{} git commit -S -m x"),
		Entry("flock", "flock /run/lock git commit -S -m x"),
		Entry("watch -x", "watch -x git commit -S -m x"),
		Entry("find -exec", `find . -exec git commit -S -m x \;`),
		Entry("chained launchers", "sudo env FOO=1 nohup timeout 5 nice git commit -S -m x"),
		Entry("bash -c", `bash -c "git commit -S -m x"`),
		Entry("bash -lc", `bash -lc "git commit -S -m x"`),
		Entry("sh -c with path", `sh -c "/usr/bin/git commit -S -m x"`),
		Entry("zsh -c", `zsh -c "git commit -S -m x"`),
		Entry("shell with -o before -c", `bash -o pipefail -c "git commit -S -m x"`),
		Entry("shell by path", `/bin/bash -c "git commit -S -m x"`),
		Entry("shell reading a pipe", `echo "git commit -S -m x" | bash`),
		Entry("shell reading a heredoc", "bash <<'EOF'\ngit commit -S -m x\nEOF"),
		Entry("busybox sh", `busybox sh -c "git commit -S -m x"`),
		Entry("eval", `eval "git commit -S -m x"`),
		Entry("su -c", `su - root -c "git commit -S -m x"`),
		Entry("flock -c", `flock /run/lock -c "git commit -S -m x"`),
		Entry("launcher running a shell", `sudo bash -c "exec /usr/bin/git commit -S -m x"`),
		Entry("nested shells", `bash -c "sh -c 'git commit -S -m x'"`),
	)

	DescribeTable("does not invent git where none runs",
		func(command string) {
			Expect(gitCommitArgs(command)).To(BeEmpty())
		},
		Entry("command -v lookup", "command -v git"),
		Entry("sudo -l listing", "sudo -l git commit -S -m x"),
		Entry("echo of a git command", "echo git commit -S -m x"),
		Entry("shell running a script file", "bash ./script.sh commit"),
		Entry("path to git as an argument", "ls -la /usr/bin/git"),
		Entry("su without -c", "su - root"),
		Entry("bare timeout", "timeout 30"),
	)

	It("keeps the launcher itself in the command list", func() {
		result, err := p.Parse("sudo -u root git status")
		Expect(err).NotTo(HaveOccurred())

		names := make([]string, 0, len(result.Commands))
		for _, cmd := range result.Commands {
			names = append(names, cmd.Name)
		}

		Expect(names).To(Equal([]string{"sudo", "git"}))
	})

	It("resolves a path-qualified name to the program", func() {
		result, err := p.Parse("/usr/local/bin/gh pr create --title x")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Commands[0].Name).To(Equal("gh"))
	})

	It("scopes cd inside a shell script to that script", func() {
		result, err := p.Parse(`cd /a && bash -c "cd /b && git status" && git log`)
		Expect(err).NotTo(HaveOccurred())

		dirs := map[string]string{}
		for _, cmd := range result.GitOperations {
			dirs[cmd.Args[0]] = cmd.WorkingDirectory
		}

		Expect(dirs).To(Equal(map[string]string{"status": "/b", "log": "/a"}))
	})

	It("records file writes made through a launcher", func() {
		result, err := p.Parse("echo data | sudo tee /etc/hosts")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.FileWrites).To(ContainElement(HaveField("Path", "/etc/hosts")))
	})

	It("stops following deeply nested shells", func() {
		// Quoting grows about fourfold per level, so keep the nesting small.
		command := "git commit -S -m x"
		for range 8 {
			command = "bash -c " + shellQuote(command)
		}

		result, err := p.Parse(command)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.GitOperations).To(BeEmpty())
	})
})

// shellQuote wraps s in single quotes, escaping any single quote inside.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
