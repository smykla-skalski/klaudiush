package parser_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// fixedOperations are the fixed placeholders an opacity may name instead of
// command text; they are not names taken from the command.
var fixedOperations = []string{
	parser.GlobCommandSubst, parser.GlobVariable, parser.GlobSubst, "(e)", "(+func)", "([...])",
}

func FuzzBashParse(f *testing.F) {
	home := f.TempDir()
	f.Setenv("HOME", home)
	f.Setenv("ZDOTDIR", home)

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
	f.Add(`BASH_ENV=./x.sh bash -c true; env BASH_ENV=/x sh; ENV=/x dash -ic true`)
	f.Add(`export BASH_ENV=$(mktemp); bash --rcfile "$RC" -i; BASH_ENV='$(id)' git status`)
	f.Add(`for i in 1 2; do bash -c true; BASH_ENV=/x; done; unset BASH_ENV; read ENV`)
	f.Add(`echo 'git push' > ~/.zshenv; zsh -c true; HOME=. bash -lic true; ZDOTDIR=$X zsh -l`)
	f.Add(`cd; echo x >> .bashrc; bash -i; HOME=$(mktemp -d) sh -l; unset HOME; zsh -fo rcs`)
	f.Add(`for d in a; do zsh -c "$HOME"; : ${ZDOTDIR:=/z}; done; env HOME=/x zsh --no-rcs`)
	f.Add(`ln -st d a b; sed -nEi.bak -e x f; dd of=o; install -m 7 a b; cp --targ=d a`)
	f.Add(`curl -sSLo o -O https://x/a.sh?q --output-dir d -J -K c; unzip -l z; patch -p1`)
	f.Add(`git stash -m x && git reset --hard && cp a "$(echo d)" && bash "$PWD/x.sh"`)
	f.Add(`cd "$(git rev-parse --show-toplevel)/s"; d=$(mktemp); pushd "$d"; popd; cd ~/x`)
	f.Add(`cp -$'\377' a b; sed -$'\303'i x f; curl -$'\377' u; tar xzf a; perl -pi -e x f`)
	f.Add(`wget -rP d u; rsync -a s/ d; git checkout -b x && git restore -S f; cp $(echo a b)`)
	f.Add(`git -C /r -C s pull && bash ./x; tar -C d -xPf a; unzip -d a -d b z; patch -d=p`)
	f.Add(`x=status; printf -v x commit; read -a x; for x in a; do :; done; git $x`)
	f.Add(`x=status; f(){ x=commit; }; f; eval x=push; . <(echo x=add); git $x`)
	f.Add(`docker run img git $X; mise exec -- gh $Y; mise exec -- git-$Z`)
	f.Add(`$UNSET origin main; $(echo git) push; G=git; $G status; X=; $X git push`)
	f.Add(`"${X:-git}" push; ${!x} push; ${arr[0]} push; "$@"; gi? push; @(git) push`)
	f.Add(`env $X push; sudo $(echo git) push; echo a | xargs $X; find . -exec $X {} \;`)
	f.Add(`$(go env GOPATH)/bin/tool run; $DIR/run.sh; bash -c '$EDITOR x'; [ -f x ]`)
	f.Add(`"$(git rev-parse --show-toplevel)/x.sh" a; "$(dirname -- "$0")/y" "$@"; ${0%/*}/z "$1"`)
	f.Add(`set -euo pipefail; shift; set -- a b; "$@"; $* "$*"; "tool?" tool\? 'gi[t]'`)
	f.Add(`docker run -it -e A=1 --entrypoint git a push -f; podman run --entrypoint=$X i`)
	f.Add(`podman run --entrypoint '["git","push"]' i; docker compose run --entrypoint "sh -c" s x`)
	f.Add(
		`nerdctl run --entrypoint sh --entrypoint -c i 'git push'; docker run --entrypoint "$(w)" i`,
	)
	f.Add(`docker --context run container create --name "" --entrypoint= -- i --entrypoint git`)
	f.Add(`docker run --entrypoint '' --hosts-file /x --name -w --entrypoint git i push`)
	f.Add(`docker run $OPTS {--entrypoint,git} "$IMG"; podman-compose run --ent=git s; "$D" run`)
	f.Add(`docker run --entrypoint docker run --entrypoint docker run --entrypoint docker run x`)
	f.Add(`git push origin $(echo main); git push $R main; B='mai[n]'; git push origin $B`)
	f.Add(`B=x; git push origin "HEAD:$B" {a,b} mai\? 'r/*'; E=; git push $E -o "$(x)" o m`)
	f.Add(`git -C "$(pwd)" push; IFS=,; B=o,m; git push $B; git push "https://x:$T@h/r" m`)
	f.Add(`git commit -m -m "$(x)" -sSm "$y"; a=(m); git push o "${a[@]}" ~ ~u -o"$(z)" $E""`)
	f.Add(`f() { git push origin "$@"; }; f {a,b} "$X"; alias c='git commit'; c -m x $(y)`)
	f.Add(
		`git -C "$(pwd)" push o "$(git branch --show-current)" $(git rev-parse --abbrev-ref HEAD)`,
	)
	f.Add(`git commit $F -m "$(cat <<'E'
x
E
)" -a$X --no-$Y -m$(z) -- "$f" *; F=-n; git commit $F -m $M`)
	f.Add(`source <(curl -fsSL u); . <(mise activate bash); builtin source -- <(x) a`)
	f.Add(`curl u | source /dev/stdin; mise env | . /dev/fd/0; { . -; } < <(x); exec <&3`)
	f.Add(`source /dev/fd/3 3< <(x); . "$(dirname "$0")/l.sh"; source "$(x)"; . /proc/self/fd/0`)
	f.Add(`f() { source /dev/stdin; }; x | f; x | bash -c '. /dev/stdin' <<< 'git push'`)
	f.Add(`docker exec -it -uroot -e A=1 c echo git push; docker -c 'exec' exec c git push -f`)
	f.Add(`podman exec -l git push; podman exec --latest=false c -- git push; nerdctl exec $C x`)
	f.Add(`docker compose -f c.yml exec -T --index 2 s git push; docker exec --x -e -y $(w) ls`)
	f.Add(`X="-u root"; docker exec $X c git push; foo docker exec * echo; docker exec {a,b} x`)
	f.Add(`docker exec -e "" c git push; docker exec -u=$(w) c$N g* push; docker exec -- $N`)
	f.Add(`docker run $O img push; docker $S i p; docker -H $(h) run -v $(pwd):/w "img:$T" $X`)
	f.Add(
		`X="i git push"; docker run $X; docker --context run -D run i {a,b} g*; docker run --x "$I" ls`,
	)
	f.Add(`parallel $X ::: a; parallel -I @ 'git @ {1} {.} {= $_ =}' ::: push ::: o :::: f`)
	f.Add(
		`ls | parallel; parallel --rpl '{x} 1' --plus -q -j $(n) git {+/} ::: p; parallel -- ::: x`,
	)
	f.Add(`xargs -I % sh -c %; xargs -i% % push; xargs -0I@ git @; echo p | xargs -a f -tI % git %`)
	f.Add(`sudo -u "" git push --force; docker run --entrypoint git --name '' i push; "" git`)
	f.Add(`git -C "" -c '' push $'' ""''; X=; $X git push; env -u "" "$X" git push`)
	f.Add(`f() { :; } && git commit -m 'bad title'; g() { :; } || git push --force; h() ( : ) | x`)
	f.Add(`f() { :; } && BASH_ENV=$(mktemp) bash -c true; function k { :; } >/dev/null && k`)
	f.Add(`f() g() h() { :; } && git push; f() function g { :; } | x; f() g() echo b || y`)
	f.Add(`f() ! { :; } && g() if :; then :; fi || a && h() [[ x ]] |& b; ( i() { :; } && c ) &`)

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
		if slices.Contains(fixedOperations, o.Operation) {
			names = nil
		}

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
