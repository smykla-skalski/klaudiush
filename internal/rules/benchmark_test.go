package rules_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/pkg/hook"
)

// BenchmarkPatternMatching benchmarks pattern matching performance.
// Target: < 100μs per match.
func BenchmarkPatternMatching(b *testing.B) {
	b.Run("GlobPattern/Simple", func(b *testing.B) {
		pattern, err := rules.NewGlobPattern("*/kong/*")
		if err != nil {
			b.Fatalf("failed to compile glob: %v", err)
		}

		b.ResetTimer()

		for range b.N {
			pattern.Match("/home/user/kong/repo")
		}
	})

	b.Run("GlobPattern/Complex", func(b *testing.B) {
		pattern, err := rules.NewGlobPattern("**/github.com/**/kong-mesh/**")
		if err != nil {
			b.Fatalf("failed to compile glob: %v", err)
		}

		b.ResetTimer()

		for range b.N {
			pattern.Match("/home/user/Projects/github.com/company/kong-mesh/src/main.go")
		}
	})

	b.Run("RegexPattern/Simple", func(b *testing.B) {
		pattern, err := rules.NewRegexPattern(`^.*/kong/.*$`)
		if err != nil {
			b.Fatalf("failed to compile regex: %v", err)
		}

		b.ResetTimer()

		for range b.N {
			pattern.Match("/home/user/kong/repo")
		}
	})

	b.Run("RegexPattern/Complex", func(b *testing.B) {
		pattern, err := rules.NewRegexPattern(`^.*/github\.com/.*/kong-mesh/.*\.go$`)
		if err != nil {
			b.Fatalf("failed to compile regex: %v", err)
		}

		b.ResetTimer()

		for range b.N {
			pattern.Match("/home/user/Projects/github.com/company/kong-mesh/src/main.go")
		}
	})

	b.Run("PatternCache/Hit", func(b *testing.B) {
		cache := rules.NewPatternCache()

		// Warm up the cache.
		_, _ = cache.Get("*/kong/*")

		b.ResetTimer()

		for range b.N {
			_, _ = cache.Get("*/kong/*")
		}
	})

	b.Run("PatternCache/Miss", func(b *testing.B) {
		for i := range b.N {
			cache := rules.NewPatternCache()
			_, _ = cache.Get(fmt.Sprintf("*/pattern%d/*", i))
		}
	})

	b.Run("DetectPatternType/Glob", func(b *testing.B) {
		for range b.N {
			rules.DetectPatternType("*/kong/*")
		}
	})

	b.Run("DetectPatternType/Regex", func(b *testing.B) {
		for range b.N {
			rules.DetectPatternType(`^.*/kong/.*$`)
		}
	})
}

// BenchmarkMatcherEvaluation benchmarks matcher evaluation performance.
// Target: < 1ms per evaluation.
func BenchmarkMatcherEvaluation(b *testing.B) {
	ctx := &rules.MatchContext{
		ValidatorType: rules.ValidatorGitPush,
		Command:       "git push origin main",
		GitContext: &rules.GitContext{
			RepoRoot: "/home/user/Projects/github.com/company/kong-mesh",
			Remote:   "origin",
			Branch:   "feat/new-feature",
			IsInRepo: true,
		},
		FileContext: &rules.FileContext{
			Path:    "/home/user/Projects/github.com/company/kong-mesh/src/main.go",
			Content: "package main\n\nfunc main() {}\n",
		},
	}

	b.Run("ValidatorTypeMatcher", func(b *testing.B) {
		matcher := rules.NewValidatorTypeMatcher(rules.ValidatorGitPush)

		b.ResetTimer()

		for range b.N {
			matcher.Match(ctx)
		}
	})

	b.Run("ValidatorTypeMatcher/Wildcard", func(b *testing.B) {
		matcher := rules.NewValidatorTypeMatcher(rules.ValidatorGitAll)

		b.ResetTimer()

		for range b.N {
			matcher.Match(ctx)
		}
	})

	b.Run("RemoteMatcher", func(b *testing.B) {
		matcher := rules.NewRemoteMatcher("origin")

		b.ResetTimer()

		for range b.N {
			matcher.Match(ctx)
		}
	})

	b.Run("RepoPatternMatcher/Glob", func(b *testing.B) {
		matcher, err := rules.NewRepoPatternMatcher("**/kong-mesh")
		if err != nil {
			b.Fatalf("failed to create matcher: %v", err)
		}

		b.ResetTimer()

		for range b.N {
			matcher.Match(ctx)
		}
	})

	b.Run("RepoPatternMatcher/Regex", func(b *testing.B) {
		matcher, err := rules.NewRepoPatternMatcher(`^.*/kong-mesh$`)
		if err != nil {
			b.Fatalf("failed to create matcher: %v", err)
		}

		b.ResetTimer()

		for range b.N {
			matcher.Match(ctx)
		}
	})

	b.Run("BranchPatternMatcher", func(b *testing.B) {
		matcher, err := rules.NewBranchPatternMatcher("feat/*")
		if err != nil {
			b.Fatalf("failed to create matcher: %v", err)
		}

		b.ResetTimer()

		for range b.N {
			matcher.Match(ctx)
		}
	})

	b.Run("FilePatternMatcher", func(b *testing.B) {
		matcher, err := rules.NewFilePatternMatcher("**/*.go")
		if err != nil {
			b.Fatalf("failed to create matcher: %v", err)
		}

		b.ResetTimer()

		for range b.N {
			matcher.Match(ctx)
		}
	})

	b.Run("ContentPatternMatcher", func(b *testing.B) {
		matcher, err := rules.NewContentPatternMatcher(`package\s+main`)
		if err != nil {
			b.Fatalf("failed to create matcher: %v", err)
		}

		b.ResetTimer()

		for range b.N {
			matcher.Match(ctx)
		}
	})

	b.Run("CommandPatternMatcher", func(b *testing.B) {
		matcher, err := rules.NewCommandPatternMatcher("git push*")
		if err != nil {
			b.Fatalf("failed to create matcher: %v", err)
		}

		b.ResetTimer()

		for range b.N {
			matcher.Match(ctx)
		}
	})

	b.Run("CompositeMatcher/AND/3", func(b *testing.B) {
		m1 := rules.NewValidatorTypeMatcher(rules.ValidatorGitPush)
		m2 := rules.NewRemoteMatcher("origin")
		m3, _ := rules.NewBranchPatternMatcher("feat/*")

		composite := rules.NewAndMatcher(m1, m2, m3)

		b.ResetTimer()

		for range b.N {
			composite.Match(ctx)
		}
	})

	b.Run("CompositeMatcher/OR/3", func(b *testing.B) {
		m1, _ := rules.NewRepoPatternMatcher("**/kong")
		m2, _ := rules.NewRepoPatternMatcher("**/kuma")
		m3, _ := rules.NewRepoPatternMatcher("**/kong-mesh")

		composite := rules.NewOrMatcher(m1, m2, m3)

		b.ResetTimer()

		for range b.N {
			composite.Match(ctx)
		}
	})
}

// BenchmarkRuleEvaluation benchmarks rule engine evaluation performance.
// Target: < 1ms per evaluation.
func BenchmarkRuleEvaluation(b *testing.B) {
	createRules := func(count int) []*rules.Rule {
		result := make([]*rules.Rule, count)

		for i := range count {
			result[i] = &rules.Rule{
				Name:        fmt.Sprintf("rule-%d", i),
				Description: fmt.Sprintf("Test rule %d", i),
				Enabled:     true,
				Priority:    count - i,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitPush,
					RepoPattern:   fmt.Sprintf("**/org%d/*", i),
				},
				Action: &rules.RuleAction{
					Type:    rules.ActionBlock,
					Message: fmt.Sprintf("Blocked by rule %d", i),
				},
			}
		}

		return result
	}

	hookCtx := &hook.Context{
		EventType: hook.EventTypePreToolUse,
		ToolName:  hook.ToolTypeBash,
		ToolInput: hook.ToolInput{
			Command: "git push origin main",
		},
	}

	matchCtx := &rules.MatchContext{
		HookContext:   hookCtx,
		ValidatorType: rules.ValidatorGitPush,
		Command:       "git push origin main",
		GitContext: &rules.GitContext{
			RepoRoot: "/home/user/Projects/github.com/company/kong-mesh",
			Remote:   "origin",
			Branch:   "main",
			IsInRepo: true,
		},
	}

	b.Run("Engine/1Rule", func(b *testing.B) {
		engine, _ := rules.NewRuleEngine(createRules(1))
		ctx := context.Background()

		b.ResetTimer()

		for range b.N {
			engine.Evaluate(ctx, matchCtx)
		}
	})

	b.Run("Engine/10Rules", func(b *testing.B) {
		engine, _ := rules.NewRuleEngine(createRules(10))
		ctx := context.Background()

		b.ResetTimer()

		for range b.N {
			engine.Evaluate(ctx, matchCtx)
		}
	})

	b.Run("Engine/50Rules", func(b *testing.B) {
		engine, _ := rules.NewRuleEngine(createRules(50))
		ctx := context.Background()

		b.ResetTimer()

		for range b.N {
			engine.Evaluate(ctx, matchCtx)
		}
	})

	b.Run("Engine/100Rules", func(b *testing.B) {
		engine, _ := rules.NewRuleEngine(createRules(100))
		ctx := context.Background()

		b.ResetTimer()

		for range b.N {
			engine.Evaluate(ctx, matchCtx)
		}
	})

	// Benchmark with matching rule at different positions.
	b.Run("Engine/MatchFirst", func(b *testing.B) {
		testRules := createRules(50)

		// First rule matches.
		testRules[0].Match.RepoPattern = "**/kong-mesh"

		engine, _ := rules.NewRuleEngine(testRules)
		ctx := context.Background()

		b.ResetTimer()

		for range b.N {
			engine.Evaluate(ctx, matchCtx)
		}
	})

	b.Run("Engine/MatchMiddle", func(b *testing.B) {
		testRules := createRules(50)

		// Middle rule matches.
		testRules[25].Match.RepoPattern = "**/kong-mesh"

		engine, _ := rules.NewRuleEngine(testRules)
		ctx := context.Background()

		b.ResetTimer()

		for range b.N {
			engine.Evaluate(ctx, matchCtx)
		}
	})

	b.Run("Engine/MatchLast", func(b *testing.B) {
		testRules := createRules(50)

		// Last rule matches.
		testRules[49].Match.RepoPattern = "**/kong-mesh"

		engine, _ := rules.NewRuleEngine(testRules)
		ctx := context.Background()

		b.ResetTimer()

		for range b.N {
			engine.Evaluate(ctx, matchCtx)
		}
	})

	b.Run("Engine/NoMatch", func(b *testing.B) {
		engine, _ := rules.NewRuleEngine(createRules(50))
		ctx := context.Background()

		b.ResetTimer()

		for range b.N {
			engine.Evaluate(ctx, matchCtx)
		}
	})
}

// BenchmarkBuildMatcher benchmarks matcher construction.
func BenchmarkBuildMatcher(b *testing.B) {
	b.Run("Simple", func(b *testing.B) {
		match := &rules.RuleMatch{
			ValidatorType: rules.ValidatorGitPush,
			Remote:        "origin",
		}

		b.ResetTimer()

		for range b.N {
			_, _ = rules.BuildMatcher(match)
		}
	})

	b.Run("Complex", func(b *testing.B) {
		match := &rules.RuleMatch{
			ValidatorType:  rules.ValidatorGitPush,
			RepoPattern:    "**/kong-mesh",
			Remote:         "origin",
			BranchPattern:  "main",
			CommandPattern: "git push*",
		}

		b.ResetTimer()

		for range b.N {
			// Clear pattern cache to simulate fresh compilation.
			rules.ClearPatternCache()

			_, _ = rules.BuildMatcher(match)
		}
	})

	b.Run("Complex/Cached", func(b *testing.B) {
		match := &rules.RuleMatch{
			ValidatorType:  rules.ValidatorGitPush,
			RepoPattern:    "**/kong-mesh",
			Remote:         "origin",
			BranchPattern:  "main",
			CommandPattern: "git push*",
		}

		// Warm up cache.
		_, _ = rules.BuildMatcher(match)

		b.ResetTimer()

		for range b.N {
			_, _ = rules.BuildMatcher(match)
		}
	})
}

// BenchmarkRegistry benchmarks registry operations.
func BenchmarkRegistry(b *testing.B) {
	createRule := func(i int) *rules.Rule {
		return &rules.Rule{
			Name:     fmt.Sprintf("rule-%d", i),
			Enabled:  true,
			Priority: i,
			Match: &rules.RuleMatch{
				ValidatorType: rules.ValidatorGitPush,
			},
			Action: &rules.RuleAction{
				Type:    rules.ActionBlock,
				Message: "blocked",
			},
		}
	}

	b.Run("Add", func(b *testing.B) {
		for range b.N {
			registry := rules.NewRegistry()
			_ = registry.Add(createRule(0))
		}
	})

	b.Run("AddAll/10", func(b *testing.B) {
		testRules := make([]*rules.Rule, 10)
		for i := range 10 {
			testRules[i] = createRule(i)
		}

		b.ResetTimer()

		for range b.N {
			registry := rules.NewRegistry()
			_ = registry.AddAll(testRules)
		}
	})

	b.Run("Get", func(b *testing.B) {
		registry := rules.NewRegistry()
		for i := range 100 {
			_ = registry.Add(createRule(i))
		}

		b.ResetTimer()

		for range b.N {
			registry.Get("rule-50")
		}
	})

	b.Run("GetEnabled/100Rules", func(b *testing.B) {
		registry := rules.NewRegistry()
		for i := range 100 {
			_ = registry.Add(createRule(i))
		}

		b.ResetTimer()

		for range b.N {
			registry.GetEnabled()
		}
	})
}

// BenchmarkMemoryAllocation benchmarks memory allocation patterns.
func BenchmarkMemoryAllocation(b *testing.B) {
	b.Run("RuleEngine/Creation/10Rules", func(b *testing.B) {
		testRules := make([]*rules.Rule, 10)

		for i := range 10 {
			testRules[i] = &rules.Rule{
				Name:     fmt.Sprintf("rule-%d", i),
				Enabled:  true,
				Priority: i,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitPush,
					RepoPattern:   fmt.Sprintf("**/org%d/*", i),
				},
				Action: &rules.RuleAction{
					Type:    rules.ActionBlock,
					Message: "blocked",
				},
			}
		}

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			_, _ = rules.NewRuleEngine(testRules)
		}
	})

	b.Run("Evaluation/NoAlloc", func(b *testing.B) {
		testRules := []*rules.Rule{{
			Name:     "test",
			Enabled:  true,
			Priority: 1,
			Match: &rules.RuleMatch{
				ValidatorType: rules.ValidatorGitPush,
			},
			Action: &rules.RuleAction{
				Type:    rules.ActionAllow,
				Message: "allowed",
			},
		}}

		engine, _ := rules.NewRuleEngine(testRules)

		matchCtx := &rules.MatchContext{
			ValidatorType: rules.ValidatorGitPush,
		}

		ctx := context.Background()

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			engine.Evaluate(ctx, matchCtx)
		}
	})
}
