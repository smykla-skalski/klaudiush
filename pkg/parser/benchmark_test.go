package parser_test

import (
	"testing"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// BenchmarkBashParser benchmarks the bash command parser.
// The bash parser is the hottest code path - called from registry predicates,
// dispatcher file writes, and main.go working directory extraction.
func BenchmarkBashParser(b *testing.B) {
	b.Run("NewBashParser", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = parser.NewBashParser()
		}
	})

	b.Run("Parse/SimpleCommand", func(b *testing.B) {
		p := parser.NewBashParser()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = p.Parse("echo hello")
		}
	})

	b.Run("Parse/GitCommit", func(b *testing.B) {
		p := parser.NewBashParser()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = p.Parse(`git commit -sS -m "feat(auth): add OAuth2 support"`)
		}
	})

	b.Run("Parse/GitPush", func(b *testing.B) {
		p := parser.NewBashParser()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = p.Parse("git push upstream feature/auth")
		}
	})

	b.Run("Parse/ChainedGitCommands", func(b *testing.B) {
		p := parser.NewBashParser()
		cmd := `cd /repo && git add -A && git commit -sS -m "feat: add feature" && git push upstream main`

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = p.Parse(cmd)
		}
	})

	b.Run("Parse/FileWrites", func(b *testing.B) {
		p := parser.NewBashParser()
		cmd := `echo "package main" > main.go && echo "test" | tee output.txt`

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = p.Parse(cmd)
		}
	})

	b.Run("Parse/Heredoc", func(b *testing.B) {
		p := parser.NewBashParser()
		cmd := "cat <<'EOF'\nline 1\nline 2\nline 3\nEOF"

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = p.Parse(cmd)
		}
	})

	b.Run("Parse/ComplexPipeline", func(b *testing.B) {
		p := parser.NewBashParser()
		cmd := `(git log --oneline -10 | grep "feat" | wc -l) > /tmp/count.txt 2>&1`

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = p.Parse(cmd)
		}
	})

	b.Run("Parse/NonGitCommand", func(b *testing.B) {
		p := parser.NewBashParser()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = p.Parse("ls -la /usr/local/bin")
		}
	})

	b.Run("FindDoubleQuotedBackticks/NoBackticks", func(b *testing.B) {
		p := parser.NewBashParser()
		cmd := `echo "hello world"`

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = p.FindDoubleQuotedBackticks(cmd)
		}
	})

	b.Run("FindDoubleQuotedBackticks/WithBackticks", func(b *testing.B) {
		p := parser.NewBashParser()
		cmd := "echo \"Fix `parser` module\""

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = p.FindDoubleQuotedBackticks(cmd)
		}
	})
}
