package git

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
)

const (
	// detectSampleSize is the number of recent commits to analyze for style detection.
	detectSampleSize = 20

	// detectScopeOnlyThreshold is the minimum ratio of scope-only commits to declare
	// the repo as "scope-only". Without a clear majority, we default to "conventional".
	detectScopeOnlyThreshold = 0.5

	// detectMinCommits is the minimum number of commits required to make a detection
	// decision. Repos with fewer commits fall back to "conventional".
	detectMinCommits = 3

	detectTimeout = 5 * time.Second
)

// conventionalTypeSet is the set of standard conventional commit types.
// A commit matching "type: desc" or "type(scope): desc" where type is in
// this set is treated as conventional.
var conventionalTypeSet = map[string]bool{
	"build":    true,
	"chore":    true,
	"ci":       true,
	"docs":     true,
	"feat":     true,
	"fix":      true,
	"perf":     true,
	"refactor": true,
	"revert":   true,
	"style":    true,
	"test":     true,
}

// conventionalSubjectRegex matches "type: desc" or "type(scope): desc" (conventional).
var conventionalSubjectRegex = regexp.MustCompile(
	`^(\w+)(?:\([^)]+\))?!?: .+`,
)

// scopeOnlySubjectRegex matches "scope: desc" where scope contains path chars.
var scopeOnlySubjectRegex = regexp.MustCompile(`^[a-z][a-z0-9./_-]*: .+`)

// CommitStyleDetector analyzes git history to detect the commit style convention.
type CommitStyleDetector struct {
	runner execpkg.CommandRunner
}

// NewCommitStyleDetector creates a detector that samples recent commits.
func NewCommitStyleDetector() *CommitStyleDetector {
	return &CommitStyleDetector{
		runner: execpkg.NewCommandRunner(detectTimeout),
	}
}

// Detect returns the detected commit style: "conventional" or "scope-only".
// Falls back to "conventional" when detection is inconclusive.
func (d *CommitStyleDetector) Detect(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, detectTimeout)
	defer cancel()

	result := d.runner.Run(ctx, "git", "log", "--format=%s",
		"--max-count="+strconv.Itoa(detectSampleSize))
	if result.Err != nil {
		return commitStyleConventional
	}

	subjects := parseLines(result.Stdout)
	if len(subjects) < detectMinCommits {
		return commitStyleConventional
	}

	conventional, scopeOnly := 0, 0

	for _, subj := range subjects {
		subj = strings.TrimSpace(subj)
		if subj == "" {
			continue
		}

		if isConventionalSubject(subj) {
			conventional++
		} else if scopeOnlySubjectRegex.MatchString(subj) {
			scopeOnly++
		}
	}

	total := conventional + scopeOnly
	if total == 0 {
		return commitStyleConventional
	}

	if float64(scopeOnly)/float64(total) > detectScopeOnlyThreshold && scopeOnly > conventional {
		return commitStyleScopeOnly
	}

	return commitStyleConventional
}

// isConventionalSubject returns true when the subject has a known conventional type prefix.
func isConventionalSubject(subject string) bool {
	matches := conventionalSubjectRegex.FindStringSubmatch(subject)
	if len(matches) < 2 { //nolint:mnd // 2 = full match + type group
		return false
	}

	return conventionalTypeSet[matches[1]]
}
