package suggest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"regexp"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// SeedVersion tracks the seed pattern data version.
// Bump this when seed patterns change to invalidate cached files.
const SeedVersion = 1

// hashRegex matches the hash comment in a KLAUDIUSH.md file.
var hashRegex = regexp.MustCompile(`<!-- klaudiush:hash:([0-9a-f]+) -->`)

// hashableData is a struct derived from SuggestData for deterministic hashing.
// Uses the collected data (not raw config) to ensure the hash reflects
// what actually gets rendered. This avoids issues with config mutation
// from nil-safe getters like GetValidators().
type hashableData struct {
	Commit      *CommitRulesData  `json:"commit,omitempty"`
	Push        *PushRulesData    `json:"push,omitempty"`
	Branch      *BranchRulesData  `json:"branch,omitempty"`
	PR          *PRRulesData      `json:"pr,omitempty"`
	Linters     []FileLinterData  `json:"linters,omitempty"`
	Secrets     *SecretsRulesData `json:"secrets,omitempty"`
	Shell       *ShellRulesData   `json:"shell,omitempty"`
	Rules       []CustomRuleData  `json:"rules,omitempty"`
	SeedVersion int               `json:"seed_version"`
}

// ComputeHash computes a SHA-256 hash of the effective config.
// It collects the data first (same as rendering), then hashes the result.
// Returns the first 16 hex chars of the hash.
func ComputeHash(cfg *config.Config) (string, error) {
	collected := Collect(cfg, "")

	hd := hashableData{
		Commit:      collected.Commit,
		Push:        collected.Push,
		Branch:      collected.Branch,
		PR:          collected.PR,
		Linters:     collected.Linters,
		Secrets:     collected.Secrets,
		Shell:       collected.Shell,
		Rules:       collected.Rules,
		SeedVersion: SeedVersion,
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
