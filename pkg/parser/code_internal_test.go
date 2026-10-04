package parser

import (
	"strings"
	"testing"
)

// unsafeSamples hit every proseUnsafe alternative, each near-misses and the
// case and word-boundary rules.
var unsafeSamples = []string{
	"os.system(c)", "OS.SYSTEM(c)", "os.popen(c)", "run(c, shell=True)", "{shell: true}",
	"getoutput(c)", "execSync(c)", "os.execvp(c)", "exec(c)", "re.exec (c)", "_exec(c)",
	"myexec(c)", "eval(c)", "literal_eval(c)", "import shlex", "spawn(c)", "setattr(a)",
	"builtins", "globals()", "myglobals ()", "locals()", "x.__dict__", "vars(x)", "vars_(x)",
	"__import__", "importlib", "os.putenv", "environ.setdefault", "create_subprocess_shell",
	"alias.x", "[alias]", "[include]", "include.path", "includeIf.x", "~/.gitconfig",
	".git/config", "GIT_CONFIG", "git_dir", "GIT_COMMON_DIR", "GIT_WORK_TREE",
	"GIT_EXEC_PATH", "XDG_CONFIG_HOME", "os.chdir(d)", "open3", "$stdout = x", "$STDOUT=x",
	"sys.stdout = x", "sys.stdout == x", "xsys.stdout = x", "sys.stderr.write = f",
	"stdout.write == f", "os.dup2", "os.fork()", "forks", "fork_x", "os.pipe()", "mypipe()",
	"fdopen", "redirect_stdout", "io.StringIO()", "BytesIO", "print(x, file=f)",
	"print(x, file=sys.stderr)", "print(x, file = st)", "print(x, file=stdout)",
	"log(stream=s)", "log(stream=sy)", "log(stream=x)", "profile=x", "upstream=x",
	`"a" | sh`, `x |  "a"`, "a | b", "a || b", "'x'|", "|`x`", "HOME = x", `env["HOME"]= x`,
	"HOME == x", "MYHOME = x", "home = x", "PATH: x", "os.environ.PATH = x", "x.PATH == y",
	"{PATH}", "{path}", "PATHS = x", "print('not found on PATH')", "0PATH=PATH=0",
}

func TestCodeUnsafeMatchesProseUnsafe(t *testing.T) {
	for _, sample := range unsafeSamples {
		for _, code := range []string{sample, "x " + sample + " y", "a\n" + sample + "\nb"} {
			if got, want := codeUnsafe(code), proseUnsafe.MatchString(code); got != want {
				t.Errorf("codeUnsafe(%q) = %v, proseUnsafe = %v", code, got, want)
			}
		}
	}
}

func FuzzCodeUnsafe(f *testing.F) {
	for _, sample := range unsafeSamples {
		f.Add(sample)
	}

	f.Add(strings.Repeat("print(\"see gh\")\n", 4))

	f.Fuzz(func(t *testing.T, code string) {
		if got, want := codeUnsafe(code), proseUnsafe.MatchString(code); got != want {
			t.Errorf("codeUnsafe(%q) = %v, proseUnsafe = %v", code, got, want)
		}
	})
}
