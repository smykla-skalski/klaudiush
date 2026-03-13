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

const (
	hashSubmatchMinCount = 2
	hashSubmatchValueIdx = 1
)

// hashRegex matches the hash comment in a KLAUDIUSH.md file.
var hashRegex = regexp.MustCompile(`<!-- klaudiush:hash:([0-9a-f]+) -->`)

// seedPair is a minimal representation of a seed pattern for hashing.
type seedPair struct {
	Source string `json:"s"`
	Target string `json:"t"`
}

// ComputeHash computes a SHA-256 hash of the effective config.
// It collects the data first (same as rendering), then hashes the result.
// Returns the first 16 hex chars of the hash.
func ComputeHash(cfg *config.Config) (string, error) {
	collected := Collect(cfg, "")

	seeds := collectSeedPairs()

	payload := map[string]any{
		"commit":  collected.Commit,
		"push":    collected.Push,
		"branch":  collected.Branch,
		"pr":      collected.PR,
		"linters": collected.Linters,
		"secrets": collected.Secrets,
		"shell":   collected.Shell,
		"rules":   collected.Rules,
		"seeds":   seeds,
	}

	data, err := json.Marshal(payload)
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
	if len(matches) < hashSubmatchMinCount {
		return "", nil
	}

	return string(matches[hashSubmatchValueIdx]), nil
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
