package patterns

import "time"

// seedCount is the initial count for seed patterns, above the default min_count of 3.
const seedCount = 5

// SeedPatterns returns the built-in seed patterns.
// These represent commonly observed failure cascades.
func SeedPatterns() *PatternData {
	now := time.Now()

	return &PatternData{
		Patterns: map[string]*FailurePattern{
			"GIT013->GIT004": {
				SourceCode: "GIT013",
				TargetCode: "GIT004",
				Count:      seedCount,
				FirstSeen:  now,
				LastSeen:   now,
				Seed:       true,
			},
			"GIT004->GIT005": {
				SourceCode: "GIT004",
				TargetCode: "GIT005",
				Count:      seedCount,
				FirstSeen:  now,
				LastSeen:   now,
				Seed:       true,
			},
			"GIT005->GIT016": {
				SourceCode: "GIT005",
				TargetCode: "GIT016",
				Count:      seedCount,
				FirstSeen:  now,
				LastSeen:   now,
				Seed:       true,
			},
			"GIT013->GIT006": {
				SourceCode: "GIT013",
				TargetCode: "GIT006",
				Count:      seedCount,
				FirstSeen:  now,
				LastSeen:   now,
				Seed:       true,
			},
		},
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
