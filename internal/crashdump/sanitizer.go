package crashdump

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

// minSecretLength is the minimum length for a value to be considered a potential secret.
const minSecretLength = 16

// sensitivePatterns contains patterns for fields that should be sanitized.
var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)token`),
	regexp.MustCompile(`(?i)secret`),
	regexp.MustCompile(`(?i)password`),
	regexp.MustCompile(`(?i)key`),
	regexp.MustCompile(`(?i)credential`),
	regexp.MustCompile(`(?i)auth`),
	regexp.MustCompile(`(?i)api[-_]?key`),
}

const redactedValue = "[REDACTED]"

// Sanitizer handles config sanitization for crash dumps.
type Sanitizer struct{}

// NewSanitizer creates a new config sanitizer.
func NewSanitizer() *Sanitizer {
	return &Sanitizer{}
}

// SanitizeConfig converts config to a map and removes sensitive values.
func (s *Sanitizer) SanitizeConfig(cfg *config.Config) map[string]any {
	if cfg == nil {
		return nil
	}

	// Marshal config to JSON, then unmarshal to generic map
	data, err := json.Marshal(cfg)
	if err != nil {
		return map[string]any{"error": "failed to serialize config"}
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return map[string]any{"error": "failed to deserialize config"}
	}

	// Recursively sanitize sensitive fields
	s.sanitizeMap(result)

	return result
}

// sanitizeMap recursively sanitizes a map, redacting sensitive values.
func (s *Sanitizer) sanitizeMap(m map[string]any) {
	for key, value := range m {
		if s.isSensitiveKey(key) {
			m[key] = redactedValue

			continue
		}

		switch v := value.(type) {
		case map[string]any:
			s.sanitizeMap(v)
		case []any:
			s.sanitizeSlice(v)
		case string:
			if s.isSensitiveValue(v) {
				m[key] = redactedValue
			}
		}
	}
}

// sanitizeSlice recursively sanitizes a slice.
func (s *Sanitizer) sanitizeSlice(slice []any) {
	for i, value := range slice {
		switch v := value.(type) {
		case map[string]any:
			s.sanitizeMap(v)
		case []any:
			s.sanitizeSlice(v)
		case string:
			if s.isSensitiveValue(v) {
				slice[i] = redactedValue
			}
		}
	}
}

// isSensitiveKey checks if a key name matches sensitive patterns.
func (*Sanitizer) isSensitiveKey(key string) bool {
	for _, pattern := range sensitivePatterns {
		if pattern.MatchString(key) {
			return true
		}
	}

	return false
}

// isSensitiveValue checks if a value looks like a secret.
func (*Sanitizer) isSensitiveValue(value string) bool {
	// Skip empty or very short values
	if len(value) < minSecretLength {
		return false
	}

	// Check for common secret prefixes
	secretPrefixes := []string{
		"sk-",     // OpenAI/Anthropic API keys
		"ghp_",    // GitHub PAT
		"gho_",    // GitHub OAuth
		"ghs_",    // GitHub App
		"ghr_",    // GitHub Refresh
		"AKIA",    // AWS Access Key ID
		"xoxb-",   // Slack Bot Token
		"xoxp-",   // Slack User Token
		"Bearer ", // Bearer tokens
	}

	for _, prefix := range secretPrefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}

	return false
}
