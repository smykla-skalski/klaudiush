package crashdump

import (
	"testing"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

func TestNewSanitizer(t *testing.T) {
	s := NewSanitizer()
	if s == nil {
		t.Error("expected non-nil sanitizer")
	}
}

func TestSanitizer_SanitizeConfig_Nil(t *testing.T) {
	s := NewSanitizer()
	result := s.SanitizeConfig(nil)

	if result != nil {
		t.Errorf("expected nil result for nil config, got %v", result)
	}
}

func TestSanitizer_SanitizeConfig_Empty(t *testing.T) {
	s := NewSanitizer()
	cfg := &config.Config{}

	result := s.SanitizeConfig(cfg)
	if result == nil {
		t.Error("expected non-nil result for empty config")
	}
}

func TestSanitizer_IsSensitiveKey(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		key      string
		expected bool
	}{
		{key: "api_token", expected: true},
		{key: "github_token", expected: true},
		{key: "secret_key", expected: true},
		{key: "password", expected: true},
		{key: "api_key", expected: true},
		{key: "api-key", expected: true},
		{key: "credential", expected: true},
		{key: "auth_header", expected: true},
		{key: "normal_field", expected: false},
		{key: "enabled", expected: false},
		{key: "max_age", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := s.isSensitiveKey(tt.key)
			if result != tt.expected {
				t.Errorf("isSensitiveKey(%q) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}

func TestSanitizer_IsSensitiveValue(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{
			name:     "short value",
			value:    "short",
			expected: false,
		},
		{
			name:     "empty value",
			value:    "",
			expected: false,
		},
		{
			name:     "OpenAI API key",
			value:    "sk-1234567890abcdef1234567890abcdef",
			expected: true,
		},
		{
			name:     "GitHub PAT",
			value:    "ghp_1234567890abcdef1234567890abcdef12345678",
			expected: true,
		},
		{
			name:     "GitHub OAuth",
			value:    "gho_1234567890abcdef1234567890abcdef12345678",
			expected: true,
		},
		{
			name:     "AWS Access Key",
			value:    "AKIAIOSFODNN7EXAMPLE",
			expected: true,
		},
		{
			name:     "Slack Bot Token",
			value:    "xoxb-1234567890-1234567890-abcdefghijklmnop",
			expected: true,
		},
		{
			name:     "Bearer token",
			value:    "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected: true,
		},
		{
			name:     "normal long value",
			value:    "this is a long normal value that is not a secret",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.isSensitiveValue(tt.value)
			if result != tt.expected {
				t.Errorf("isSensitiveValue(%q) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestSanitizer_SanitizeMap(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name: "simple sensitive key",
			input: map[string]any{
				"api_token": "secret-value",
				"username":  "test-user",
			},
			expected: map[string]any{
				"api_token": redactedValue,
				"username":  "test-user",
			},
		},
		{
			name: "nested map",
			input: map[string]any{
				"config": map[string]any{
					"api_key": "sk-test123456789012",
					"timeout": 30,
				},
			},
			expected: map[string]any{
				"config": map[string]any{
					"api_key": redactedValue,
					"timeout": 30,
				},
			},
		},
		{
			name: "sensitive value detection",
			input: map[string]any{
				"header": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			},
			expected: map[string]any{
				"header": redactedValue,
			},
		},
		{
			name: "mixed sensitive and normal",
			input: map[string]any{
				"password":  "super-secret-password-value",
				"enabled":   true,
				"max_dumps": 10,
			},
			expected: map[string]any{
				"password":  redactedValue,
				"enabled":   true,
				"max_dumps": 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.sanitizeMap(tt.input)

			for key, expectedVal := range tt.expected {
				actualVal, ok := tt.input[key]
				if !ok {
					t.Errorf("key %q not found in result", key)

					continue
				}

				// Handle nested maps
				if expectedMap, ok := expectedVal.(map[string]any); ok {
					actualMap, ok := actualVal.(map[string]any)
					if !ok {
						t.Errorf("expected map for key %q, got %T", key, actualVal)

						continue
					}

					for nestedKey, nestedExpected := range expectedMap {
						if actualMap[nestedKey] != nestedExpected {
							t.Errorf(
								"nested key %q.%q = %v, want %v",
								key,
								nestedKey,
								actualMap[nestedKey],
								nestedExpected,
							)
						}
					}

					continue
				}

				if actualVal != expectedVal {
					t.Errorf("key %q = %v, want %v", key, actualVal, expectedVal)
				}
			}
		})
	}
}

func TestSanitizer_SanitizeSlice(t *testing.T) {
	s := NewSanitizer()

	tests := []struct {
		name     string
		input    []any
		expected []any
	}{
		{
			name: "slice with sensitive strings",
			input: []any{
				"normal-value",
				"sk-1234567890abcdef1234567890abcdef",
				"another-normal-value",
			},
			expected: []any{
				"normal-value",
				redactedValue,
				"another-normal-value",
			},
		},
		{
			name: "slice with nested maps",
			input: []any{
				map[string]any{
					"api_key":  "secret",
					"username": "test",
				},
			},
			expected: []any{
				map[string]any{
					"api_key":  redactedValue,
					"username": "test",
				},
			},
		},
		{
			name: "slice with nested slices",
			input: []any{
				[]any{
					"ghp_1234567890abcdef1234567890abcdef12345678",
					"normal-string",
				},
			},
			expected: []any{
				[]any{
					redactedValue,
					"normal-string",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.sanitizeSlice(tt.input)

			if len(tt.input) != len(tt.expected) {
				t.Fatalf("length mismatch: got %d, want %d", len(tt.input), len(tt.expected))
			}

			for i := range tt.expected {
				// Handle different types
				switch expected := tt.expected[i].(type) {
				case string:
					actual, ok := tt.input[i].(string)
					if !ok {
						t.Errorf("input[%d] is %T, want string", i, tt.input[i])

						continue
					}

					if actual != expected {
						t.Errorf("input[%d] = %q, want %q", i, actual, expected)
					}
				case map[string]any:
					actual, ok := tt.input[i].(map[string]any)
					if !ok {
						t.Errorf("input[%d] is %T, want map", i, tt.input[i])

						continue
					}

					for key, expectedVal := range expected {
						if actual[key] != expectedVal {
							t.Errorf("input[%d][%q] = %v, want %v", i, key, actual[key], expectedVal)
						}
					}
				case []any:
					actual, ok := tt.input[i].([]any)
					if !ok {
						t.Errorf("input[%d] is %T, want slice", i, tt.input[i])

						continue
					}

					for j := range expected {
						if actual[j] != expected[j] {
							t.Errorf("input[%d][%d] = %v, want %v", i, j, actual[j], expected[j])
						}
					}
				}
			}
		})
	}
}

func TestSanitizer_SanitizeConfig_Complex(t *testing.T) {
	s := NewSanitizer()

	// Create a realistic config structure
	// Note: We're using a generic map since we can't easily create
	// a fully populated config.Config in tests
	testData := map[string]any{
		"validators": map[string]any{
			"git": map[string]any{
				"enabled":      true,
				"github_token": "ghp_secret123456789012345678901234567890",
			},
			"file": map[string]any{
				"enabled":  true,
				"patterns": []any{"*.go", "*.md"},
			},
		},
		"crash_dump": map[string]any{
			"enabled":  true,
			"dump_dir": "~/.klaudiush/crash_dumps",
		},
	}

	// We need to test via SanitizeConfig which takes *config.Config
	// For this test, we'll directly test sanitizeMap instead
	s.sanitizeMap(testData)

	// Verify github_token was redacted
	gitConfig := testData["validators"].(map[string]any)["git"].(map[string]any)
	if gitConfig["github_token"] != redactedValue {
		t.Errorf("github_token not redacted: %v", gitConfig["github_token"])
	}

	// Verify enabled flags remain
	if gitConfig["enabled"] != true {
		t.Error("enabled flag was modified")
	}

	// Verify normal nested fields remain
	fileConfig := testData["validators"].(map[string]any)["file"].(map[string]any)
	if fileConfig["enabled"] != true {
		t.Error("file enabled flag was modified")
	}

	patterns := fileConfig["patterns"].([]any)
	if patterns[0] != "*.go" {
		t.Errorf("patterns[0] was modified: %v", patterns[0])
	}

	// Verify non-sensitive fields remain
	crashConfig := testData["crash_dump"].(map[string]any)
	if crashConfig["dump_dir"] != "~/.klaudiush/crash_dumps" {
		t.Error("dump_dir was modified")
	}
}
