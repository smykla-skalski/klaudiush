package secrets_test

import (
	"strings"
	"testing"

	"github.com/smykla-skalski/klaudiush/internal/validators/secrets"
)

// BenchmarkSecretsDetector benchmarks the secrets detection engine.
// 28+ compiled regexes scanned against content. strings.Split on every Detect call.
func BenchmarkSecretsDetector(b *testing.B) {
	cleanGoFile := `package main

import "fmt"

func main() {
	fmt.Println("hello world")
}
`

	// ~100 lines of clean Go code
	var largeBuilder strings.Builder
	largeBuilder.WriteString("package main\n\nimport \"fmt\"\n\n")

	for i := range 100 {
		largeBuilder.WriteString("// Line ")
		largeBuilder.WriteString(strings.Repeat("x", 60))

		_ = i

		largeBuilder.WriteString("\n")
	}

	cleanLargeFile := largeBuilder.String()

	fileWithAWSKey := `package config

const (
	region = "us-east-1"
	accessKey = "AKIAIOSFODNN7EXAMPLE"
)
`

	fileWithMultipleSecrets := `package config

const (
	awsKey = "AKIAIOSFODNN7EXAMPLE"
	ghToken = "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef12"
	dbConn = "postgres://user:password@localhost:5432/mydb"
)
`

	b.Run("Detect/CleanGoFile", func(b *testing.B) {
		d := secrets.NewDefaultPatternDetector()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = d.Detect(cleanGoFile)
		}
	})

	b.Run("Detect/CleanLargeFile", func(b *testing.B) {
		d := secrets.NewDefaultPatternDetector()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = d.Detect(cleanLargeFile)
		}
	})

	b.Run("Detect/WithAWSKey", func(b *testing.B) {
		d := secrets.NewDefaultPatternDetector()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = d.Detect(fileWithAWSKey)
		}
	})

	b.Run("Detect/MultipleSecrets", func(b *testing.B) {
		d := secrets.NewDefaultPatternDetector()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = d.Detect(fileWithMultipleSecrets)
		}
	})

	b.Run("NewDefaultPatternDetector", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_ = secrets.NewDefaultPatternDetector()
		}
	})
}
