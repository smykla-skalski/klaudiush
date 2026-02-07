package git_test

import (
	"testing"

	gitpkg "github.com/smykla-labs/klaudiush/internal/git"
	gitvalidators "github.com/smykla-labs/klaudiush/internal/validators/git"
)

// BenchmarkCLIRunner benchmarks the CLI-based GitRunner
func BenchmarkCLIRunner(b *testing.B) {
	runner := gitvalidators.NewCLIGitRunner()

	b.Run("IsInRepo", func(b *testing.B) {
		for range b.N {
			_ = runner.IsInRepo()
		}
	})

	b.Run("GetRepoRoot", func(b *testing.B) {
		for range b.N {
			_, _ = runner.GetRepoRoot()
		}
	})

	b.Run("GetStagedFiles", func(b *testing.B) {
		for range b.N {
			_, _ = runner.GetStagedFiles()
		}
	})

	b.Run("GetCurrentBranch", func(b *testing.B) {
		for range b.N {
			_, _ = runner.GetCurrentBranch()
		}
	})
}

// BenchmarkSDKRunner benchmarks the SDK-based GitRunner
func BenchmarkSDKRunner(b *testing.B) {
	runner, err := gitpkg.NewSDKRunner()
	if err != nil {
		b.Fatalf("Failed to create SDK runner: %v", err)
	}

	b.Run("IsInRepo", func(b *testing.B) {
		for range b.N {
			_ = runner.IsInRepo()
		}
	})

	b.Run("GetRepoRoot", func(b *testing.B) {
		for range b.N {
			_, _ = runner.GetRepoRoot()
		}
	})

	b.Run("GetStagedFiles", func(b *testing.B) {
		for range b.N {
			_, _ = runner.GetStagedFiles()
		}
	})

	b.Run("GetCurrentBranch", func(b *testing.B) {
		for range b.N {
			_, _ = runner.GetCurrentBranch()
		}
	})
}
