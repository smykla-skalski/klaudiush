package rules_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/rules"
)

func TestRules(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rules Suite")
}

var _ = Describe("Pattern", func() {
	Describe("DetectPatternType", func() {
		DescribeTable("should detect pattern type correctly",
			func(pattern string, expected rules.PatternType) {
				result := rules.DetectPatternType(pattern)
				Expect(result).To(Equal(expected))
			},
			// Glob patterns
			Entry("simple glob with *", "*/kong/*", rules.PatternTypeGlob),
			Entry("glob with **", "**/test/**", rules.PatternTypeGlob),
			Entry("glob with ?", "file?.txt", rules.PatternTypeGlob),
			Entry("glob with braces", "{main,master}", rules.PatternTypeGlob),
			Entry("simple path", "path/to/file", rules.PatternTypeGlob),

			// Regex patterns
			Entry("regex with ^", "^start", rules.PatternTypeRegex),
			Entry("regex with $", "end$", rules.PatternTypeRegex),
			Entry("regex with both anchors", "^exact$", rules.PatternTypeRegex),
			Entry("regex with group", "(?i)case-insensitive", rules.PatternTypeRegex),
			Entry("regex with \\d", "file\\d+", rules.PatternTypeRegex),
			Entry("regex with \\w", "\\w+", rules.PatternTypeRegex),
			Entry("regex with character class", "[a-z]+", rules.PatternTypeRegex),
			Entry("regex with alternation", "foo|bar", rules.PatternTypeRegex),
			Entry("regex with +", "a+", rules.PatternTypeRegex),
			Entry("regex with .*", "prefix.*suffix", rules.PatternTypeRegex),
			Entry("regex with .+", ".+test", rules.PatternTypeRegex),
		)
	})

	Describe("GlobPattern", func() {
		It("should match simple glob patterns", func() {
			pattern, err := rules.NewGlobPattern("**/myorg/**")
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("/home/user/myorg/project")).To(BeTrue())
			Expect(pattern.Match("/home/user/other/project")).To(BeFalse())
			Expect(pattern.String()).To(Equal("**/myorg/**"))
		})

		It("should match double star patterns", func() {
			pattern, err := rules.NewGlobPattern("**/test/**")
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("path/to/test/file.go")).To(BeTrue())
			Expect(pattern.Match("/test/file.go")).To(BeTrue())
			Expect(pattern.Match("path/other/file.go")).To(BeFalse())
		})

		It("should match brace expansion patterns", func() {
			pattern, err := rules.NewGlobPattern("*.{go,ts}")
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("file.go")).To(BeTrue())
			Expect(pattern.Match("file.ts")).To(BeTrue())
			Expect(pattern.Match("file.js")).To(BeFalse())
		})

		It("should return error for invalid patterns", func() {
			_, err := rules.NewGlobPattern("[invalid")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("RegexPattern", func() {
		It("should match regex patterns", func() {
			pattern, err := rules.NewRegexPattern("^.*/kong/.*$")
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("/home/user/kong/project")).To(BeTrue())
			Expect(pattern.Match("/home/user/other/project")).To(BeFalse())
			Expect(pattern.String()).To(Equal("^.*/kong/.*$"))
		})

		It("should match case-insensitive patterns", func() {
			pattern, err := rules.NewRegexPattern("(?i)kong")
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("kong")).To(BeTrue())
			Expect(pattern.Match("Kong")).To(BeTrue())
			Expect(pattern.Match("KONG")).To(BeTrue())
		})

		It("should return error for invalid regex", func() {
			_, err := rules.NewRegexPattern("[invalid")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CompilePattern", func() {
		It("should auto-detect and compile glob patterns", func() {
			pattern, err := rules.CompilePattern("**/myorg/**")
			Expect(err).NotTo(HaveOccurred())
			Expect(pattern.Match("/home/myorg/project")).To(BeTrue())
		})

		It("should auto-detect and compile regex patterns", func() {
			pattern, err := rules.CompilePattern("^start")
			Expect(err).NotTo(HaveOccurred())
			Expect(pattern.Match("start of string")).To(BeTrue())
			Expect(pattern.Match("middle start")).To(BeFalse())
		})
	})

	Describe("PatternCache", func() {
		var cache *rules.PatternCache

		BeforeEach(func() {
			cache = rules.NewPatternCache()
		})

		It("should cache compiled patterns", func() {
			pattern1, err1 := cache.Get("*/test/*")
			Expect(err1).NotTo(HaveOccurred())

			pattern2, err2 := cache.Get("*/test/*")
			Expect(err2).NotTo(HaveOccurred())

			// Should be the same instance.
			Expect(pattern1).To(BeIdenticalTo(pattern2))
		})

		It("should cache compilation errors", func() {
			_, err1 := cache.Get("[invalid")
			Expect(err1).To(HaveOccurred())

			_, err2 := cache.Get("[invalid")
			Expect(err2).To(HaveOccurred())
			Expect(err2).To(Equal(err1))
		})

		It("should track cache size", func() {
			Expect(cache.Size()).To(Equal(0))

			_, _ = cache.Get("pattern1")
			Expect(cache.Size()).To(Equal(1))

			_, _ = cache.Get("pattern2")
			Expect(cache.Size()).To(Equal(2))

			// Same pattern shouldn't increase size.
			_, _ = cache.Get("pattern1")
			Expect(cache.Size()).To(Equal(2))
		})

		It("should clear cache", func() {
			_, _ = cache.Get("pattern1")
			_, _ = cache.Get("pattern2")
			Expect(cache.Size()).To(Equal(2))

			cache.Clear()
			Expect(cache.Size()).To(Equal(0))
		})
	})

	Describe("NegatedPattern", func() {
		It("should invert glob pattern matches", func() {
			pattern, err := rules.CompilePattern("!*.tmp")
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("file.txt")).To(BeTrue())
			Expect(pattern.Match("file.tmp")).To(BeFalse())
			Expect(pattern.String()).To(Equal("!*.tmp"))
		})

		It("should invert regex pattern matches", func() {
			pattern, err := rules.CompilePattern("!^test.*")
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("production")).To(BeTrue())
			Expect(pattern.Match("testing")).To(BeFalse())
		})

		It("should handle negated path patterns", func() {
			pattern, err := rules.CompilePattern("!*/vendor/*")
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("src/main.go")).To(BeTrue())
			Expect(pattern.Match("project/vendor/lib.go")).To(BeFalse())
		})
	})

	Describe("IsNegated and StripNegation", func() {
		It("should detect negated patterns", func() {
			Expect(rules.IsNegated("!pattern")).To(BeTrue())
			Expect(rules.IsNegated("pattern")).To(BeFalse())
			Expect(rules.IsNegated("")).To(BeFalse())
		})

		It("should strip negation prefix", func() {
			Expect(rules.StripNegation("!pattern")).To(Equal("pattern"))
			Expect(rules.StripNegation("pattern")).To(Equal("pattern"))
			Expect(rules.StripNegation("!!double")).To(Equal("!double"))
		})
	})

	Describe("CaseInsensitivePattern", func() {
		It("should match case-insensitively with glob", func() {
			pattern, err := rules.NewCaseInsensitiveGlobPattern("*.Md")
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("README.md")).To(BeTrue())
			Expect(pattern.Match("README.MD")).To(BeTrue())
			Expect(pattern.Match("readme.Md")).To(BeTrue())
			Expect(pattern.Match("file.txt")).To(BeFalse())
		})

		It("should match case-insensitively with paths", func() {
			pattern, err := rules.NewCaseInsensitiveGlobPattern("**/Docs/**")
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("project/docs/readme.md")).To(BeTrue())
			Expect(pattern.Match("project/DOCS/README.md")).To(BeTrue())
			Expect(pattern.Match("project/DoCs/file.txt")).To(BeTrue())
			Expect(pattern.Match("project/src/file.go")).To(BeFalse())
		})
	})

	Describe("CompilePatternWithOptions", func() {
		It("should compile case-insensitive glob patterns", func() {
			opts := rules.PatternOptions{CaseInsensitive: true}
			pattern, err := rules.CompilePatternWithOptions("*.Go", opts)
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("main.go")).To(BeTrue())
			Expect(pattern.Match("main.GO")).To(BeTrue())
			Expect(pattern.Match("main.Go")).To(BeTrue())
		})

		It("should compile case-insensitive regex patterns", func() {
			opts := rules.PatternOptions{CaseInsensitive: true}
			pattern, err := rules.CompilePatternWithOptions("^hello", opts)
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("hello world")).To(BeTrue())
			Expect(pattern.Match("Hello World")).To(BeTrue())
			Expect(pattern.Match("HELLO WORLD")).To(BeTrue())
			Expect(pattern.Match("world hello")).To(BeFalse())
		})

		It("should compile negated patterns via options", func() {
			opts := rules.PatternOptions{Negate: true}
			pattern, err := rules.CompilePatternWithOptions("*.tmp", opts)
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("file.txt")).To(BeTrue())
			Expect(pattern.Match("file.tmp")).To(BeFalse())
		})

		It("should combine negation and case-insensitivity", func() {
			opts := rules.PatternOptions{CaseInsensitive: true, Negate: true}
			pattern, err := rules.CompilePatternWithOptions("*.Tmp", opts)
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("file.txt")).To(BeTrue())
			Expect(pattern.Match("file.tmp")).To(BeFalse())
			Expect(pattern.Match("file.TMP")).To(BeFalse())
		})

		It("should not duplicate (?i) flag for regex", func() {
			opts := rules.PatternOptions{CaseInsensitive: true}
			pattern, err := rules.CompilePatternWithOptions("(?i)^test", opts)
			Expect(err).NotTo(HaveOccurred())

			// Should still work correctly without adding duplicate flag.
			Expect(pattern.Match("test")).To(BeTrue())
			Expect(pattern.Match("TEST")).To(BeTrue())
		})
	})

	Describe("MultiPattern", func() {
		It("should match any pattern (OR logic)", func() {
			patterns := []string{"*.go", "*.ts"}
			pattern, err := rules.CompileMultiPattern(
				patterns,
				rules.MultiPatternAny,
				rules.PatternOptions{},
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("main.go")).To(BeTrue())
			Expect(pattern.Match("index.ts")).To(BeTrue())
			Expect(pattern.Match("style.css")).To(BeFalse())
		})

		It("should match all patterns (AND logic)", func() {
			// File must contain both "test" AND end with ".go".
			patterns := []string{"*test*", "*.go"}
			pattern, err := rules.CompileMultiPattern(
				patterns,
				rules.MultiPatternAll,
				rules.PatternOptions{},
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("main_test.go")).To(BeTrue())
			Expect(pattern.Match("test_utils.go")).To(BeTrue())
			Expect(pattern.Match("main.go")).To(BeFalse())      // No "test".
			Expect(pattern.Match("test.js")).To(BeFalse())      // Not .go.
			Expect(pattern.Match("main_test.js")).To(BeFalse()) // Not .go.
		})

		It("should return single pattern when only one provided", func() {
			patterns := []string{"*.go"}
			pattern, err := rules.CompileMultiPattern(
				patterns,
				rules.MultiPatternAny,
				rules.PatternOptions{},
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("main.go")).To(BeTrue())
			Expect(pattern.Match("main.js")).To(BeFalse())
		})

		It("should return nil for empty patterns", func() {
			pattern, err := rules.CompileMultiPattern(
				[]string{},
				rules.MultiPatternAny,
				rules.PatternOptions{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(pattern).To(BeNil())
		})

		It("should apply case-insensitivity to all patterns", func() {
			patterns := []string{"*.Go", "*.Ts"}
			pattern, err := rules.CompileMultiPattern(
				patterns,
				rules.MultiPatternAny,
				rules.PatternOptions{CaseInsensitive: true},
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("main.go")).To(BeTrue())
			Expect(pattern.Match("main.GO")).To(BeTrue())
			Expect(pattern.Match("index.ts")).To(BeTrue())
			Expect(pattern.Match("index.TS")).To(BeTrue())
		})

		It("should support negated patterns in multi-pattern", func() {
			// Match anything except .tmp files.
			patterns := []string{"*", "!*.tmp"}
			pattern, err := rules.CompileMultiPattern(
				patterns,
				rules.MultiPatternAll,
				rules.PatternOptions{},
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(pattern.Match("main.go")).To(BeTrue())
			Expect(pattern.Match("lib.go")).To(BeTrue())
			Expect(pattern.Match("cache.tmp")).To(BeFalse())
		})

		It("should have correct string representation", func() {
			patterns := []string{"*.go", "*.ts"}
			pattern, err := rules.CompileMultiPattern(
				patterns,
				rules.MultiPatternAny,
				rules.PatternOptions{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(pattern.String()).To(Equal("any(*.go, *.ts)"))

			patternAll, err := rules.CompileMultiPattern(
				patterns,
				rules.MultiPatternAll,
				rules.PatternOptions{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(patternAll.String()).To(Equal("all(*.go, *.ts)"))
		})

		It("should propagate compilation errors", func() {
			patterns := []string{"*.go", "[invalid"}
			_, err := rules.CompileMultiPattern(
				patterns,
				rules.MultiPatternAny,
				rules.PatternOptions{},
			)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Pattern Mode Constants", func() {
		It("should have correct string values", func() {
			Expect(rules.PatternModeAny).To(Equal("any"))
			Expect(rules.PatternModeAll).To(Equal("all"))
		})
	})

	Describe("GetCachedPattern", func() {
		It("should return cached pattern", func() {
			// Clear cache first to ensure clean state.
			rules.ClearPatternCache()

			pattern1, err := rules.GetCachedPattern("*.go")
			Expect(err).NotTo(HaveOccurred())

			pattern2, err := rules.GetCachedPattern("*.go")
			Expect(err).NotTo(HaveOccurred())

			// Should be the same instance from cache.
			Expect(pattern1).To(BeIdenticalTo(pattern2))
		})

		It("should return error for invalid pattern", func() {
			rules.ClearPatternCache()

			_, err := rules.GetCachedPattern("[invalid")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ClearPatternCache", func() {
		It("should clear the default cache", func() {
			// Add a pattern to cache.
			_, _ = rules.GetCachedPattern("*.test")

			// Clear cache.
			rules.ClearPatternCache()

			// Pattern should be recompiled (can't easily verify instance change,
			// but this exercises the code path).
			pattern, err := rules.GetCachedPattern("*.test")
			Expect(err).NotTo(HaveOccurred())
			Expect(pattern).NotTo(BeNil())
		})
	})

	Describe("MultiPattern edge cases", func() {
		It("should handle unknown mode in Match", func() {
			patterns := []rules.Pattern{
				mustCompilePattern("*.go"),
				mustCompilePattern("*.ts"),
			}

			// Create MultiPattern with invalid mode value to test default case.
			mp := rules.NewMultiPattern(patterns, rules.MultiPatternMode(99), "test")

			// Unknown mode should return false.
			Expect(mp.Match("main.go")).To(BeFalse())
		})
	})

	Describe("NegatedPattern", func() {
		It("should create and invert pattern", func() {
			inner := mustCompilePattern("*.tmp")
			negated := rules.NewNegatedPattern(inner)

			Expect(negated.Match("file.go")).To(BeTrue())
			Expect(negated.Match("file.tmp")).To(BeFalse())
			Expect(negated.String()).To(Equal("!*.tmp"))
		})
	})

	Describe("CaseInsensitivePattern", func() {
		It("should return original pattern string", func() {
			pattern, err := rules.NewCaseInsensitiveGlobPattern("*.Md")
			Expect(err).NotTo(HaveOccurred())
			Expect(pattern.String()).To(Equal("*.Md"))
		})

		It("should return error for invalid pattern", func() {
			_, err := rules.NewCaseInsensitiveGlobPattern("[invalid")
			Expect(err).To(HaveOccurred())
		})
	})
})

// Helper function to compile a pattern without error handling.
//
//nolint:ireturn // test helper for polymorphic patterns
func mustCompilePattern(s string) rules.Pattern {
	p, err := rules.CompilePattern(s)
	if err != nil {
		panic(err)
	}

	return p
}
