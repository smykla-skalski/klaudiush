package parser_test

import (
	"strings"
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
	// Forms the parser cannot see through
	f.Add(strings.Repeat("env ", 10) + "git commit -m x")
	f.Add(`bash "$DIR/run.sh"`)
	f.Add(`echo "$BODY" > s.sh && sudo bash s.sh`)
	f.Add(`bash -c 'git commit -m x && ('`)
	f.Add(`f() { git "${@:1}"; }; f commit`)
	f.Add("HOME=/x git zz")
	f.Add(`git -c alias.abcdefghijklmnopqrstuvwx='!git zz' abcdefghijklmnopqrstuvwx`)
	f.Add(`git $SECRET; gh pr $A; eval "$UNSET"`)
	f.Add(`git ${!x} "${X:-status}" "${arr[0]}"; X=$(echo commit); git $X`)
	f.Add(`sudo git $(echo commit) -m x; env $(cat .env) git "$(echo push)"`)
	f.Add(`git {commit,-m,x}; git c?mmit; git 'push '; git $'push\n'`)
	f.Add(`X=status; git $X; Y="commit -m x"; git $Y; eval "$Y"; eval 'git $X'`)
	f.Add(`f() { git "$1"; }; f $(echo commit); alias g=git; g $(echo push)`)
	f.Add(`x=status; printf -v x commit; read -a x; for x in a; do :; done; git $x`)
	f.Add(`x=status; f(){ x=commit; }; f; eval x=push; . <(echo x=add); git $x`)
	f.Add(`docker run img git $X; mise exec -- gh $Y; mise exec -- git-$Z`)

	f.Fuzz(func(t *testing.T, command string) {
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

			checkOpacities(t, result)
		}
	})
}

// checkOpacities fails when a truncated parse is not explained, or when an
// explanation carries text that is not a plain name.
func checkOpacities(t *testing.T, result *parser.ParseResult) {
	t.Helper()

	if result.Truncated != (len(result.Opacities) > 0) {
		t.Fatalf("truncated %v with %d opacities", result.Truncated, len(result.Opacities))
	}

	for _, o := range result.Opacities {
		names := strings.Fields(o.Operation)
		for _, entry := range o.Origin {
			names = append(names, strings.Fields(entry)...)
		}

		for _, name := range names {
			if len(name) > 32 || strings.ContainsAny(name, "$`'\"/\\;|&(){}") {
				t.Fatalf("unsafe name %q in %+v", name, o)
			}
		}
	}
}
