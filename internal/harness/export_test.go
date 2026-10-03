package harness

import "time"

// SetCodexTrustLimit shortens the Codex hook listing timeout for a test and
// returns a function that restores it.
func SetCodexTrustLimit(limit time.Duration) func() {
	previous := codexTrustLimit
	codexTrustLimit = limit

	return func() { codexTrustLimit = previous }
}

// CodexCatalog is the model catalog the Codex driver writes.
const CodexCatalog = codexCatalog
