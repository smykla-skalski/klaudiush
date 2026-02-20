package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
)

// checksumParts is the expected number of parts in a checksum line (hash + filename).
const checksumParts = 2

// ParseChecksums parses a checksums.txt file content into a map of filename -> hex hash.
// Expected format: "hash  filename" (two spaces between hash and filename).
func ParseChecksums(content string) map[string]string {
	result := make(map[string]string)

	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Format: "<hash>  <filename>" (two spaces)
		parts := strings.SplitN(line, "  ", checksumParts)
		if len(parts) != checksumParts {
			continue
		}

		hash := strings.TrimSpace(parts[0])
		filename := strings.TrimSpace(parts[1])

		if hash != "" && filename != "" {
			result[filename] = hash
		}
	}

	return result
}

// VerifyFileChecksum computes the SHA256 of a file and compares it to the expected hex digest.
//
//nolint:gosec // G304: filePath is the archive we just downloaded, not user-controlled
func VerifyFileChecksum(filePath, expectedHex string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(err, "opening file for checksum")
	}
	defer f.Close() //nolint:errcheck // read-only file

	h := sha256.New()

	if _, err := io.Copy(h, f); err != nil {
		return errors.Wrap(err, "computing checksum")
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expectedHex) {
		return errors.Errorf(
			"checksum mismatch: expected %s, got %s",
			expectedHex, actual,
		)
	}

	return nil
}
