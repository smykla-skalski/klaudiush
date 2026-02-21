package parser_test

import (
	"testing"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// BenchmarkGitParser benchmarks git command parsing.
// ParseGitCommand allocates 4 maps/slices per call. HasFlag is O(n*m) with combined flags.
func BenchmarkGitParser(b *testing.B) {
	b.Run("ParseGitCommand/Simple", func(b *testing.B) {
		cmd := parser.Command{Name: "git", Args: []string{"push", "origin", "main"}}

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = parser.ParseGitCommand(cmd)
		}
	})

	b.Run("ParseGitCommand/CombinedFlags", func(b *testing.B) {
		cmd := parser.Command{Name: "git", Args: []string{"commit", "-sSm", "feat: message"}}

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = parser.ParseGitCommand(cmd)
		}
	})

	b.Run("ParseGitCommand/GlobalOptions", func(b *testing.B) {
		cmd := parser.Command{
			Name: "git",
			Args: []string{
				"-C",
				"/path/to/repo",
				"--git-dir=/path/.git",
				"commit",
				"-sS",
				"-m",
				"msg",
			},
		}

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = parser.ParseGitCommand(cmd)
		}
	})

	b.Run("ParseGitCommand/ManyFlags", func(b *testing.B) {
		cmd := parser.Command{
			Name: "git",
			Args: []string{
				"commit", "--signoff", "--gpg-sign", "--message", "msg",
				"--allow-empty", "--verbose",
			},
		}

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = parser.ParseGitCommand(cmd)
		}
	})

	b.Run("HasFlag/DirectMatch", func(b *testing.B) {
		cmd := parser.Command{Name: "git", Args: []string{"commit", "-s", "-S", "-m", "msg"}}
		gitCmd, _ := parser.ParseGitCommand(cmd)

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = gitCmd.HasFlag("-s")
		}
	})

	b.Run("HasFlag/CombinedFlagMatch", func(b *testing.B) {
		cmd := parser.Command{Name: "git", Args: []string{"commit", "-sS", "-m", "msg"}}
		gitCmd, _ := parser.ParseGitCommand(cmd)

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = gitCmd.HasFlag("-S")
		}
	})

	b.Run("HasFlag/NotFound", func(b *testing.B) {
		cmd := parser.Command{
			Name: "git",
			Args: []string{
				"commit", "--signoff", "--gpg-sign", "--message", "msg",
				"--allow-empty", "--verbose",
			},
		}
		gitCmd, _ := parser.ParseGitCommand(cmd)

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = gitCmd.HasFlag("--no-verify")
		}
	})

	b.Run("EndToEnd/BashToGitCommand", func(b *testing.B) {
		// This is the actual hot path in predicates: bash parse + git parse.
		p := parser.NewBashParser()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			result, err := p.Parse(`git commit -sS -m "feat(auth): add support"`)
			if err != nil {
				b.Fatal(err)
			}

			for _, cmd := range result.Commands {
				if cmd.Name == "git" {
					_, _ = parser.ParseGitCommand(cmd)
				}
			}
		}
	})
}
