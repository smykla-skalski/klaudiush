package suggest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"
	"sort"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/patterns"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// hashRegex matches the hash comment in a KLAUDIUSH.md file.
var hashRegex = regexp.MustCompile(`<!-- klaudiush:hash:([0-9a-f]+) -->`)

// seedPair is a minimal representation of a seed pattern for hashing.
type seedPair struct {
	Source string `json:"s"`
	Target string `json:"t"`
}

// hashableData is a struct derived from SuggestData for deterministic hashing.
// Uses the collected data (not raw config) to ensure the hash reflects
// what actually gets rendered. This avoids issues with config mutation
// from nil-safe getters like GetValidators().
type hashableData struct {
	Commit  *CommitRulesData  `json:"commit,omitempty"`
	Push    *PushRulesData    `json:"push,omitempty"`
	Branch  *BranchRulesData  `json:"branch,omitempty"`
	PR      *PRRulesData      `json:"pr,omitempty"`
	Linters []FileLinterData  `json:"linters,omitempty"`
	Secrets *SecretsRulesData `json:"secrets,omitempty"`
	Shell   *ShellRulesData   `json:"shell,omitempty"`
	Rules   []CustomRuleData  `json:"rules,omitempty"`
	Seeds   []seedPair        `json:"seeds"`
}

// ComputeHash computes a SHA-256 hash of the effective config.
// It collects the data first (same as rendering), then hashes the result.
// Returns the first 16 hex chars of the hash.
func ComputeHash(cfg *config.Config) (string, error) {
	collected := Collect(cfg, "")

	seeds := collectSeedPairs()

	hd := hashableData{
		Commit:  collected.Commit,
		Push:    collected.Push,
		Branch:  collected.Branch,
		PR:      collected.PR,
		Linters: collected.Linters,
		Secrets: collected.Secrets,
		Shell:   collected.Shell,
		Rules:   collected.Rules,
		Seeds:   seeds,
	}

	data, err := json.Marshal(hd)
	if err != nil {
		return "", errors.Wrap(err, "marshaling data for hash")
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:8]), nil
}

// ExtractHash reads a file and extracts the klaudiush hash comment.
// Returns empty string if no hash is found.
func ExtractHash(filePath string) (string, error) {
	//nolint:gosec // G304: filePath comes from CLI --output flag or default path
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", errors.Wrap(err, "reading file for hash extraction")
	}

	matches := hashRegex.FindSubmatch(content)
	if len(matches) < 2 {
		return "", nil
	}

	return string(matches[1]), nil
}

// collectSeedPairs extracts seed patterns into a sorted slice for deterministic hashing.
func collectSeedPairs() []seedPair {
	seedData := patterns.SeedPatterns()

	pairs := make([]seedPair, 0, len(seedData.Patterns))
	for _, p := range seedData.Patterns {
		pairs = append(pairs, seedPair{Source: p.SourceCode, Target: p.TargetCode})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Source != pairs[j].Source {
			return pairs[i].Source < pairs[j].Source
		}

		return pairs[i].Target < pairs[j].Target
	})

	return pairs
}
