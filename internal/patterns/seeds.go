package patterns

import "time"

// seedCount is the initial count for seed patterns, above the default min_count of 3.
const seedCount = 5

// seedPairs defines source->target failure cascades.
// Each pair represents a commonly observed sequence where fixing the source error
// causes the target error to appear.
var seedPairs = [][2]string{
	// Git commit cascades
	{"GIT013", "GIT004"}, // conventional format fix -> title too long
	{"GIT004", "GIT005"}, // shorten title -> body line too long
	{"GIT005", "GIT016"}, // body line fix -> list format issue
	{"GIT013", "GIT006"}, // conventional format fix -> infra scope misuse
	{"GIT010", "GIT013"}, // adding missing flags -> conventional format
	{"GIT010", "GIT004"}, // adding flags pushes title over 50 chars
	// File linter cascades
	{"FILE006", "FILE005"}, // gofumpt reformats doc comments -> markdown lint
	{"FILE002", "FILE003"}, // terraform fmt passes -> tflint catches issues
	{"FILE010", "FILE007"}, // removing noqa directive -> ruff error
	{"FILE010", "FILE008"}, // removing eslint-disable -> oxlint error
	// Cross-category: secrets to shell
	{"SEC001", "SHELL001"}, // moving API key to env var -> command substitution
	{"SEC004", "SHELL001"}, // moving token to env var -> command substitution
	// Cross-category: PR then markdown
	{"GIT023", "FILE005"}, // fixing PR body formatting -> markdown lint
}

// SeedPatterns returns the built-in seed patterns.
// These represent commonly observed failure cascades.
func SeedPatterns() *PatternData {
	now := time.Now()
	patterns := make(map[string]*FailurePattern, len(seedPairs))

	for _, pair := range seedPairs {
		key := pair[0] + "->" + pair[1]
		patterns[key] = &FailurePattern{
			SourceCode: pair[0],
			TargetCode: pair[1],
			Count:      seedCount,
			FirstSeen:  now,
			LastSeen:   now,
			Seed:       true,
		}
	}

	return &PatternData{
		Patterns:    patterns,
		LastUpdated: now,
		Version:     patternDataVersion,
	}
}

// EnsureSeedData writes seed patterns to the project store if not already present.
func EnsureSeedData(store *FilePatternStore) error {
	if store.HasProjectData() {
		return nil
	}

	store.SetProjectData(SeedPatterns())

	return store.SaveProject()
}
