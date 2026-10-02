package hook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalFilePath(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}

	realPath := filepath.Join(dir, "real.md")
	if err := os.WriteFile(realPath, nil, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(realPath, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	cases := []struct {
		name, workingDir, path, want string
	}{
		{"empty", dir, "", ""},
		{"relative", dir, "real.md", realPath},
		{"symlink", "", link, realPath},
		{"missing file", dir, "sub/../missing.md", filepath.Join(dir, "missing.md")},
		{"relative without directory", "", "a/b.md", "a/b.md"},
	}

	for _, tc := range cases {
		if got := CanonicalFilePath(tc.workingDir, tc.path); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestContextResource(t *testing.T) {
	cases := []struct {
		name string
		ctx  *Context
		want string
	}{
		{"nil", nil, ""},
		{
			"file tool",
			&Context{
				ToolName:   ToolTypeWrite,
				WorkingDir: "/repo",
				ToolInput:  ToolInput{FilePath: "a.md"},
			},
			"file:/repo/a.md",
		},
		{
			"derived write",
			&Context{Derived: true, ToolInput: ToolInput{FilePath: "/repo/b.md"}},
			"file:/repo/b.md",
		},
		{
			"shell command",
			&Context{ToolName: ToolTypeBash, ToolInput: ToolInput{Command: "ls"}},
			"command:ls",
		},
		{
			"read path",
			&Context{ToolName: ToolTypeRead, ToolInput: ToolInput{FilePath: "/x"}},
			"file:/x",
		},
		{"tool only", &Context{ToolName: ToolTypeGrep}, "tool:Grep"},
	}

	for _, tc := range cases {
		if got := tc.ctx.Resource(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestContextNeedsRecheck(t *testing.T) {
	ctx := &Context{WorkingDir: "/repo", RecheckFiles: []string{"/repo/a.md"}}

	if !ctx.NeedsRecheck("a.md") {
		t.Error("relative path of an unresolved file should need a recheck")
	}

	if ctx.NeedsRecheck("b.md") || ctx.NeedsRecheck("") {
		t.Error("other paths should not need a recheck")
	}

	var empty *Context
	if empty.NeedsRecheck("a.md") {
		t.Error("nil context should not need a recheck")
	}
}
