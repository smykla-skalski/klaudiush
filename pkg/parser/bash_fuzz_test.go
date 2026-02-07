package parser_test

import (
	"testing"

	"github.com/smykla-labs/klaudiush/pkg/parser"
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
