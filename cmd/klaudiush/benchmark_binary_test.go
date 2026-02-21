package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// benchBinaryPath returns the absolute path to the pre-built klaudiush binary.
// Skips the benchmark if the binary doesn't exist (run "task build" first).
func benchBinaryPath(b *testing.B) string {
	b.Helper()

	path := filepath.Join("..", "..", "bin", "klaudiush")

	if _, err := os.Stat(path); err != nil {
		b.Skip("binary not found at bin/klaudiush - run 'task build' first")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		b.Fatal("failed to resolve binary path:", err)
	}

	return absPath
}

// benchPayload reads a benchmark payload file from testdata.
func benchPayload(b *testing.B, name string) []byte {
	b.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", "bench-payloads", name))
	if err != nil {
		b.Fatal("failed to read payload:", err)
	}

	return data
}

// setupGitRepo creates a minimal git repo for git-related benchmarks.
// Returns the temp directory path (used as HOME).
func setupGitRepo(b *testing.B) string {
	b.Helper()

	dir := b.TempDir()

	commands := [][]string{
		{"git", "init", "-b", "main", dir},
		{"git", "-C", dir, "config", "user.email", "bench@test.local"},
		{"git", "-C", dir, "config", "user.name", "Bench"},
	}

	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("git setup %v failed: %v\n%s", args, err, out)
		}
	}

	// Create and stage a file so git commit validators have staged files.
	mainGo := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n"), 0o644); err != nil {
		b.Fatal("failed to create main.go:", err)
	}

	cmd := exec.Command("git", "-C", dir, "add", "main.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("git add failed: %v\n%s", err, out)
	}

	return dir
}

// BenchmarkBinaryE2E measures end-to-end execution time of the klaudiush binary
// with various payload types. Each iteration spawns the binary as a subprocess.
func BenchmarkBinaryE2E(b *testing.B) {
	binary := benchBinaryPath(b)
	gitHome := setupGitRepo(b)

	// Base environment: isolated HOME, disable SDK git (no go-git overhead),
	// disable debug logging for cleaner benchmarks.
	baseEnv := append(os.Environ(),
		"HOME="+gitHome,
		"KLAUDIUSH_USE_SDK_GIT=false",
	)

	cases := []struct {
		name    string
		payload string
	}{
		{"EmptyStdin", "empty.json"},
		{"NonGitBash", "non-git-bash.json"},
		{"WriteTool", "write-tool.json"},
		{"GitCommitPass", "git-commit-pass.json"},
		{"GitCommitFail", "git-commit-fail.json"},
		{"GitPush", "git-push.json"},
	}

	for _, tc := range cases {
		payload := benchPayload(b, tc.payload)

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()

			for range b.N {
				cmd := exec.Command(binary, "--hook-type", "PreToolUse")
				cmd.Stdin = bytes.NewReader(payload)
				cmd.Env = baseEnv
				cmd.Dir = gitHome

				if _, err := cmd.CombinedOutput(); err != nil {
					// Exit code 0 is expected even for deny responses.
					// Only fail on actual execution errors.
					if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 0 {
						b.Fatal("binary execution failed:", err)
					}
				}
			}
		})
	}
}
