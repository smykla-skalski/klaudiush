package parser_test

import (
	"testing"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

func FuzzBashParse(f *testing.F) {
	// Seed from bash_test.go and common patterns
	f.Add("git status")
	f.Add("git commit -sS -m 'test message'")
	f.Add("git add . && git commit -m 'msg' && git push upstream main")
	f.Add("ls | grep foo | wc -l")
	f.Add("(cd dir && git commit -m 'msg')")
	f.Add("echo $(git log -1 --format=%h)")
	f.Add(`git commit -m "msg && trick"`)
	f.Add("echo 'test' > file.txt")
	f.Add("cat > file.txt << 'EOF'\nline 1\nline 2\nEOF")
	f.Add("echo 'test' | tee output.txt")
	f.Add("")
	f.Add("   \t\n")
	f.Add("ls -la")
	f.Add("command1 ; command2")
	f.Add("command1 || command2")
	f.Add("VAR=value command")
	f.Add("export FOO=bar")
	f.Add("for i in 1 2 3; do echo $i; done")
	f.Add("if [ -f file ]; then cat file; fi")
	f.Add("git commit -m \"$(date)\"")
	// Forms the parser resolves or follows
	f.Add("{git,commit,-m,x}")
	f.Add("a{b,c}d{e,f} {x,y}")
	f.Add(`cmd=(git commit -m x); "${cmd[@]}"`)
	f.Add(`x="git commit"; $x -m y`)
	f.Add(`bash <(echo "git commit -m x")`)
	f.Add(`bash <<< "git commit -m x"`)
	f.Add("cat <<'EOF' | bash\ngit commit -m x\nEOF")
	f.Add(`env -S'git commit' sudo -u root nice timeout 5 git push`)
	f.Add(`python3 -Sc 'import os; os.system("git commit")'`)
	f.Add(`perl -e 'system("git", "commit")'`)
	f.Add(`f() { git "$@"; }; alias g=git; f commit; g push`)
	f.Add(`git -c alias.ci=commit ci; git config alias.x '!sh -c "git push"'; git x`)
	f.Add(`bash -c git\ commit\ -m\ x`)
	f.Add("find . -exec git commit -m x \\; | xargs -I{} git add {}")

	f.Fuzz(func(_ *testing.T, command string) {
		p := parser.NewBashParser()
		result, err := p.Parse(command)

		if err == nil && result != nil {
			// Exercise all methods - should not panic
			_ = result.HasCommand("git")
			_ = result.HasCommand("ls")
			_ = result.HasGitCommand()
			_ = result.GetCommands("git")
			_ = result.GetCommands("echo")

			// Access fields
			_ = result.Commands
			_ = result.FileWrites
			_ = result.GitOperations
		}
	})
}
