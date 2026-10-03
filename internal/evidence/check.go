// Package evidence decides whether required checks (tests, reviews) passed
// against the content an agent is about to hand back. Results are tied to a
// digest of the files a check covers, so any later change to those files
// makes them stale.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/cockroachdb/errors"
	"mvdan.cc/sh/v3/syntax"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// ErrInvalidCheck marks a check configuration that cannot be used.
var ErrInvalidCheck = errors.New("invalid evidence check")

// idHashLength is how many hex digits of the definition hash a check ID keeps.
const idHashLength = 12

// Check is a compiled required check.
type Check struct {
	Name     string
	Kind     string
	Commands [][]string
	Lines    []string
	Paths    []string
	Exclude  []string
	Base     string
	Timeout  time.Duration

	id string
}

// ID identifies the check definition. Results and baselines are stored by
// ID, so changing what a check runs or covers invalidates them.
func (c *Check) ID() string {
	return c.id
}

// IsReview reports whether results must identify the exact reviewed diff.
func (c *Check) IsReview() bool {
	return c.Kind == config.EvidenceKindReview
}

// RunCommand is the command line the verifier runs.
func (c *Check) RunCommand() string {
	return c.Lines[0]
}

// stateDir holds klaudiush's own project files. Hooks write state there
// (patterns.json) and a configuration change already invalidates results
// through the check ID, so it is never fingerprinted.
const stateDir = ".klaudiush/"

// Covers reports whether a repository-relative slash path belongs to the check.
func (c *Check) Covers(path string) bool {
	if strings.HasPrefix(path, stateDir) || matchAny(c.Exclude, path) {
		return false
	}

	return len(c.Paths) == 0 || matchAny(c.Paths, path)
}

func matchAny(patterns []string, path string) bool {
	return slices.ContainsFunc(patterns, func(pattern string) bool {
		matched, err := doublestar.Match(pattern, path)

		return err == nil && matched
	})
}

// Compile validates and compiles the configured checks. A disabled or empty
// configuration compiles to no checks.
func Compile(cfg *config.EvidenceConfig) ([]*Check, error) {
	if cfg == nil {
		return nil, nil
	}

	checks := make([]*Check, 0, len(cfg.Checks))
	seen := make(map[string]bool, len(cfg.Checks))

	var errs []error

	for i, item := range cfg.Checks {
		check, err := compileCheck(item)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "checks[%d]", i))

			continue
		}

		if seen[check.Name] {
			errs = append(errs, errors.Wrapf(ErrInvalidCheck,
				"checks[%d]: duplicate name %q", i, check.Name))

			continue
		}

		seen[check.Name] = true

		checks = append(checks, check)
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return checks, nil
}

func compileCheck(item *config.EvidenceCheckConfig) (*Check, error) {
	if item == nil {
		return nil, errors.Wrap(ErrInvalidCheck, "empty check")
	}

	name := strings.TrimSpace(item.Name)
	if name == "" || strings.ContainsAny(name, " \t\n/") {
		return nil, errors.Wrapf(ErrInvalidCheck,
			"name must be a non-empty word without spaces or slashes, got %q", item.Name)
	}

	kind := item.GetKind()
	if kind != config.EvidenceKindTest && kind != config.EvidenceKindReview {
		return nil, errors.Wrapf(ErrInvalidCheck,
			"%s: kind must be %q or %q, got %q",
			name, config.EvidenceKindTest, config.EvidenceKindReview, item.Kind)
	}

	if len(item.Commands) == 0 {
		return nil, errors.Wrapf(ErrInvalidCheck, "%s: commands must not be empty", name)
	}

	check := &Check{
		Name:    name,
		Kind:    kind,
		Paths:   slices.Clone(item.Paths),
		Exclude: slices.Clone(item.Exclude),
		Timeout: item.GetTimeout(),
	}

	if check.IsReview() {
		check.Base = strings.TrimSpace(item.Base)
		if check.Base == "" {
			return nil, errors.Wrapf(ErrInvalidCheck,
				"%s: a review needs a base branch, such as \"origin/main\"", name)
		}
	}

	for _, line := range item.Commands {
		argv, err := literalArgv(line)
		if err != nil {
			return nil, errors.Wrapf(err, "%s: command %q", name, line)
		}

		check.Commands = append(check.Commands, argv)
		check.Lines = append(check.Lines, shellLine(argv))
	}

	for _, pattern := range slices.Concat(check.Paths, check.Exclude) {
		if pattern == "" || !doublestar.ValidatePattern(pattern) {
			return nil, errors.Wrapf(ErrInvalidCheck, "%s: invalid path pattern %q", name, pattern)
		}
	}

	check.id = name + "@" + definitionHash(check)

	return check, nil
}

// shellLine renders argv as a command line the shell splits back into the
// same words, quoting only the words that need it. Quote fails only on null
// bytes, which a parsed command line cannot hold.
func shellLine(argv []string) string {
	words := make([]string, 0, len(argv))

	for _, arg := range argv {
		quoted, err := syntax.Quote(arg, syntax.LangBash)
		if err != nil {
			quoted = arg
		}

		words = append(words, quoted)
	}

	return strings.Join(words, " ")
}

func definitionHash(check *Check) string {
	hash := sha256.New()

	write := func(parts ...string) {
		for _, part := range parts {
			hash.Write([]byte(part))
			hash.Write([]byte{0})
		}

		hash.Write([]byte{'\n'})
	}

	write(check.Kind, check.Base)

	for _, argv := range check.Commands {
		write(argv...)
	}

	write(check.Paths...)
	write(check.Exclude...)

	return hex.EncodeToString(hash.Sum(nil))[:idHashLength]
}

// Find returns the check with the given name.
func Find(checks []*Check, name string) *Check {
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}

	return nil
}
