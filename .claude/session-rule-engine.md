# Rule Engine Architecture

Dynamic validation configuration system allowing users to define custom validation rules via TOML without modifying code. Supports glob/regex patterns, priority-based evaluation, and first-match semantics.

## Core Design Philosophy

**Configuration Over Code**: Users can block/warn/allow operations via TOML rules instead of patching validators. Enables organization-specific policies without forking codebase.

**Pattern Auto-Detection**: Automatically distinguishes glob from regex patterns based on syntax. Users write natural patterns (`**/org/**` or `^main$`) without explicit type declaration.

**First-Match Semantics**: Rules evaluated in priority order, first match wins. Prevents ambiguous behavior when multiple rules could apply.

**Graceful Validator Integration**: Validators check rules first via `RuleValidatorAdapter.CheckRules()`. If rule matches, return early. Otherwise continue with built-in validation logic. Nil adapter is valid (allows gradual migration).

**Priority-Based Merge**: Project rules override global rules with same name. Different-named rules combine. Higher priority number = evaluated first.

## Core Types

### Rule Structure

```go
// internal/rules/types.go
type Rule struct {
    Name        string      // Unique identifier
    Description string      // Human-readable explanation
    Enabled     bool        // Enable/disable without deletion
    Priority    int         // Higher = evaluated first
    Match       *RuleMatch  // Conditions (AND logic)
    Action      *RuleAction // Block, Warn, or Allow
}

type RuleMatch struct {
    ValidatorType   string  // "git.push", "git.*", "*"
    RepoPattern     string  // Glob/regex for repository path
    Remote          string  // Exact remote name
    BranchPattern   string  // Branch name pattern
    FilePattern     string  // File path pattern
    ContentPattern  string  // File content (always regex)
    CommandPattern  string  // Bash command pattern
    ToolType        string  // "Bash", "Write", "Edit"
    EventType       string  // "PreToolUse", "PostToolUse"
}

type RuleAction struct {
    Type      ActionType  // Block, Warn, Allow
    Message   string      // User-facing message
    Reference string      // Error code (e.g., "GIT019")
}

type ActionType int
const (
    ActionAllow ActionType = iota  // Explicitly allow (overrides block)
    ActionWarn                      // Log warning, allow operation
    ActionBlock                     // Stop operation, exit 2
)
```

**Match Logic**: All specified match conditions must be true (AND logic). Empty/nil condition = match all.

**Action Priority**: Block > Warn > Allow. First matching rule determines action.

## Pattern System

Auto-detects pattern type from syntax:

### Regex Indicators

Patterns with these characters treated as regex: `^`, `$`, `(?`, `\\d`, `\\w`, `[`, `]`, `(`, `)`, `|`, `+`

```toml
branch_pattern = "^(main|master)$"  # Regex - has ^ and $
command_pattern = "git push.*--force"  # Regex - has .*
```

### Glob Patterns

Uses `gobwas/glob` for efficient glob matching:

- `*` - Matches single path component (not `/`)
- `**` - Matches multiple path components (including `/`)
- `{a,b}` - Brace expansion (matches `a` or `b`)
- `?` - Matches single character

```toml
repo_pattern = "**/myorg/**"      # Glob - matches /home/user/myorg/project
file_pattern = "*.{go,rs}"        # Glob - matches test.go or test.rs
branch_pattern = "feature/*"      # Glob - matches feature/foo but not feature/foo/bar
```

**Critical Gotcha**: Use `**` for multi-directory matching. Pattern `*/myorg/*` only matches `/myorg/project`, not `/home/user/myorg/project`.

### Pattern Cache

Compiled patterns cached for performance:

```go
// internal/rules/pattern.go
type PatternCache struct {
    mu    sync.RWMutex
    globs map[string]glob.Glob
    regex map[string]*regexp.Regexp
}
```

Cache hit avoids recompiling same pattern across multiple rule evaluations.

## Matcher System

Nine matcher types plus composites:

### Simple Matchers

```go
// Exact string match
type RemoteMatcher struct {
    Remote string
}

// Supports wildcards: "git.*" matches "git.push", "git.commit"
type ValidatorTypeMatcher struct {
    ValidatorType string
}

// Case-insensitive
type ToolTypeMatcher struct {
    ToolType string
}

// Case-insensitive
type EventTypeMatcher struct {
    EventType string
}
```

### Pattern Matchers

```go
// Repository root path
type RepoPatternMatcher struct {
    Pattern *CompiledPattern
}

// Branch name from git context
type BranchPatternMatcher struct {
    Pattern *CompiledPattern
}

// File path (from git or hook context)
type FilePatternMatcher struct {
    Pattern *CompiledPattern
}

// File content (always regex)
type ContentPatternMatcher struct {
    Pattern *regexp.Regexp
}

// Bash command
type CommandPatternMatcher struct {
    Pattern *CompiledPattern
}
```

### Composite Matchers

```go
type CompositeType int
const (
    AND CompositeType = iota
    OR
    NOT
)

type CompositeMatcher struct {
    Type     CompositeType
    Matchers []Matcher
}
```

**Usage**: `CompositeMatcher(AND, [matcher1, matcher2])` = both must match.

## Registry

Stores compiled rules sorted by priority:

```go
// internal/rules/registry.go
type Registry struct {
    mu    sync.RWMutex
    rules []*Rule  // Sorted by priority (descending)
}

func (r *Registry) AddRule(rule *Rule) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    // Merge: same name = replace, different name = add
    for i, existing := range r.rules {
        if existing.Name == rule.Name {
            r.rules[i] = rule
            r.sortRules()
            return nil
        }
    }

    r.rules = append(r.rules, rule)
    r.sortRules()
    return nil
}

func (r *Registry) sortRules() {
    slices.SortFunc(r.rules, func(a, b *Rule) int {
        return cmp.Compare(b.Priority, a.Priority)  // Descending
    })
}
```

**Merge Semantics**:

- Rules with same name: Project rule replaces global rule
- Rules with different names: Both rules active
- Priority sort: Higher number evaluated first

**Thread Safety**: RWMutex allows concurrent reads during evaluation while protecting writes during rule addition.

## Evaluator

Evaluates rules in priority order with first-match semantics:

```go
// internal/rules/evaluator.go
type Evaluator struct {
    registry      *Registry
    defaultAction ActionType
    stopOnFirst   bool
}

func (e *Evaluator) Evaluate(ctx *MatchContext) *RuleResult {
    rules := e.registry.GetRules()

    for _, rule := range rules {
        if !rule.Enabled {
            continue
        }

        if e.matchesRule(ctx, rule) {
            return &RuleResult{
                Matched: true,
                Rule:    rule,
                Action:  rule.Action,
            }
        }

        if e.stopOnFirst {
            break  // First match wins
        }
    }

    // No match - use default action
    return &RuleResult{
        Matched: false,
        Action: &RuleAction{
            Type: e.defaultAction,
        },
    }
}
```

**First-Match**: When `stopOnFirst=true` (default), evaluation stops at first match. Prevents ambiguous behavior when multiple rules could apply.

**Default Action**: When no rules match, uses default action (typically `ActionAllow`).

## Rule Engine

Main entry point for rule evaluation:

```go
// internal/rules/engine.go
type RuleEngine struct {
    evaluator *Evaluator
    logger    *slog.Logger
}

func NewRuleEngine(rules []*Rule, opts ...EngineOption) (*RuleEngine, error) {
    registry := NewRegistry()
    for _, rule := range rules {
        if err := registry.AddRule(rule); err != nil {
            return nil, err
        }
    }

    engine := &RuleEngine{
        evaluator: NewEvaluator(
            registry,
            WithDefaultAction(ActionAllow),
            WithStopOnFirst(true),
        ),
    }

    for _, opt := range opts {
        opt(engine)
    }

    return engine, nil
}

func (e *RuleEngine) Evaluate(ctx *MatchContext) *RuleResult {
    result := e.evaluator.Evaluate(ctx)

    if e.logger != nil {
        e.logger.Debug("Rule evaluation",
            "matched", result.Matched,
            "action", result.Action.Type,
            "rule", result.Rule.Name,
        )
    }

    return result
}
```

**Options Pattern**: Engine configured via functional options (`WithLogger`, `WithDefaultAction`).

## Validator Integration

### RuleValidatorAdapter

Bridges rule engine with validators:

```go
// internal/rules/adapter.go
type RuleValidatorAdapter struct {
    engine            *RuleEngine
    validatorType     ValidatorType
    gitContextProvider func() *GitContext  // Optional
}

func (a *RuleValidatorAdapter) CheckRules(ctx *hook.Context) *validator.Result {
    matchCtx := &MatchContext{
        ValidatorType: a.validatorType,
        HookContext:   ctx,
    }

    // Add git context if provider set
    if a.gitContextProvider != nil {
        matchCtx.GitContext = a.gitContextProvider()
    }

    result := a.engine.Evaluate(matchCtx)
    if !result.Matched {
        return nil  // No rule matched - continue with built-in validation
    }

    // Convert rule result to validator result
    return a.convertToValidatorResult(result)
}

func (a *RuleValidatorAdapter) convertToValidatorResult(ruleResult *RuleResult) *validator.Result {
    switch ruleResult.Action.Type {
    case ActionBlock:
        return &validator.Result{
            Passed:      false,
            ShouldBlock: true,
            Message:     ruleResult.Action.Message,
            Reference:   validator.Reference(ruleResult.Action.Reference),
        }
    case ActionWarn:
        return &validator.Result{
            Passed:      true,
            ShouldBlock: false,
            Message:     ruleResult.Action.Message,
        }
    case ActionAllow:
        return &validator.Result{
            Passed: true,
        }
    }
}
```

### Validator Integration Pattern

Standard pattern for adding rule support to validators:

```go
// 1. Validator struct: Add optional rule adapter field
type PushValidator struct {
    validator.BaseValidator
    gitRunner   git.Runner
    config      *config.GitPushValidatorConfig
    ruleAdapter *rules.RuleValidatorAdapter  // Can be nil
}

// 2. Constructor: Accept adapter as last param (nil = backward compatible)
func NewPushValidator(
    log *slog.Logger,
    runner git.Runner,
    cfg *config.GitPushValidatorConfig,
    ruleAdapter *rules.RuleValidatorAdapter,  // Last param, can be nil
) *PushValidator {
    return &PushValidator{
        BaseValidator: validator.NewBaseValidator("git.push", log),
        gitRunner:     runner,
        config:        cfg,
        ruleAdapter:   ruleAdapter,
    }
}

// 3. Validate: Check rules first, return early if matched
func (v *PushValidator) Validate(ctx context.Context, hookCtx *hook.Context) validator.Result {
    // Check rules if adapter present
    if v.ruleAdapter != nil {
        if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
            return *result  // Rule matched - use rule result
        }
    }

    // No rule matched - continue with built-in validation
    remote := v.extractRemote(hookCtx)
    if remote == "origin" {
        return validator.FailWithRef(
            validator.RefGitPushOrigin,
            "Pushing to origin is not allowed",
        )
    }

    return validator.Pass()
}
```

**Key Points**:

- **Nil adapter is valid**: Tests pass `nil`, production creates adapter when rules configured
- **Context parameter**: Must change from `_ context.Context` to `ctx context.Context` to pass to `CheckRules()`
- **Early return**: If rule matches, return immediately (rule overrides built-in validation)
- **Backward compatible**: Existing code without rule engine continues working (adapter is nil)

## Configuration Schema

Rules configured in TOML files (global and project):

```toml
[rules]
enabled = true                # Global enable/disable
stop_on_first_match = true    # First match wins (default)

[[rules.rules]]
name = "block-origin-push"
description = "Prevent accidental pushes to origin remote"
priority = 100                # Higher = evaluated first
enabled = true

[rules.rules.match]
validator_type = "git.push"
repo_pattern = "**/myorg/**"  # Glob pattern
remote = "origin"             # Exact match

[rules.rules.action]
type = "block"
message = "Don't push to origin. Use 'upstream' instead."
reference = "GIT019"          # Error code

[[rules.rules]]
name = "allow-main-push"
description = "Allow pushes to main if from CI"
priority = 50

[rules.rules.match]
validator_type = "git.push"
branch_pattern = "^main$"     # Regex pattern

[rules.rules.action]
type = "allow"                # Override other rules
```

### Configuration Precedence

1. CLI Flags (highest)
2. Environment Variables (`KLAUDIUSH_RULES_*`)
3. Project Config (`.klaudiush/config.toml`)
4. Global Config (`~/.klaudiush/config.toml`)
5. Defaults (lowest)

### Rule Merge Semantics

**Same name**: Project rule replaces global rule

```toml
# Global: ~/.klaudiush/config.toml
[[rules.rules]]
name = "block-force-push"
priority = 100

# Project: .klaudiush/config.toml
[[rules.rules]]
name = "block-force-push"  # Same name
priority = 50               # Different priority - project wins
```

**Different names**: Rules combine

```toml
# Global
[[rules.rules]]
name = "rule-A"

# Project
[[rules.rules]]
name = "rule-B"  # Different name - both active
```

## Configuration Factory

Creates rule engine from config:

```go
// internal/config/factory/rules_factory.go
type RulesFactory struct {
    config *config.RulesConfig
    logger *slog.Logger
}

func (f *RulesFactory) CreateRuleEngine() (*rules.RuleEngine, error) {
    if !f.config.Enabled {
        return nil, nil  // Rules disabled
    }

    var ruleList []*rules.Rule
    for _, ruleConfig := range f.config.Rules {
        rule, err := f.convertConfigToRule(ruleConfig)
        if err != nil {
            return nil, err
        }
        ruleList = append(ruleList, rule)
    }

    return rules.NewRuleEngine(ruleList,
        rules.WithLogger(f.logger),
        rules.WithDefaultAction(rules.ActionAllow),
        rules.WithStopOnFirst(f.config.StopOnFirstMatch),
    )
}
```

Factory integrated into main validator factory to create rule engine once and share across all validators.

## Common Pitfalls

1. **Using `*` for multi-directory glob**: `*/myorg/*` only matches one level deep. Use `**/myorg/**` for multi-level matching.

2. **Forgetting priority in rules**: Without explicit priority, rules get default priority 0. Higher-priority rules should have higher numbers.

3. **Not checking if adapter is nil**: Validators must check `if v.ruleAdapter != nil` before calling `CheckRules()`. Nil adapter is valid.

4. **Using underscore for context parameter**: Validators need `ctx context.Context`, not `_ context.Context`. Context passed to `CheckRules()`.

5. **Not returning early when rule matches**: If `CheckRules()` returns non-nil result, must return immediately. Don't continue with built-in validation.

6. **Mixing glob and regex without understanding**: Patterns auto-detected but mixing can be confusing. Use `^`/`$` anchors to force regex.

7. **Expecting OR logic in match conditions**: All match conditions are AND. Use multiple rules for OR logic.

8. **Not testing with project+global config merge**: Rules merge behavior only testable with both config sources. Test project overrides global.

9. **Assuming sequential rule evaluation**: Rules evaluated in priority order, not file order. Always set explicit priorities.

10. **Not handling disabled rules**: Check `rule.Enabled` before evaluation. Disabled rules should be skipped, not deleted.
