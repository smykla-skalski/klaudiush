package parser_test

import (
	"bytes"
	"testing"

	"github.com/smykla-skalski/klaudiush/internal/parser"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// BenchmarkJSONParser benchmarks the JSON input parser.
// Entry point for every invocation. Uses double json.Unmarshal
// (root + tool_input RawMessage).
func BenchmarkJSONParser(b *testing.B) {
	bashPayload := []byte(`{
		"tool_name": "Bash",
		"tool_input": {
			"command": "git commit -sS -m \"feat(auth): add OAuth2 support\""
		},
		"session_id": "sess_01JTEST1234567890",
		"tool_use_id": "toolu_01JTEST1234567890"
	}`)

	writePayload := []byte(`{
		"tool_name": "Write",
		"tool_input": {
			"file_path": "/home/user/project/src/main.go",
			"content": "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
		},
		"session_id": "sess_01JTEST1234567890",
		"tool_use_id": "toolu_01JTEST1234567890"
	}`)

	minimalPayload := []byte(`{"tool_name":"Bash","tool_input":{"command":"ls"}}`)

	b.Run("Parse/BashCommand", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			p := parser.NewJSONParser(bytes.NewReader(bashPayload))
			_, _ = p.Parse(hook.EventTypePreToolUse)
		}
	})

	b.Run("Parse/WriteCommand", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			p := parser.NewJSONParser(bytes.NewReader(writePayload))
			_, _ = p.Parse(hook.EventTypePreToolUse)
		}
	})

	b.Run("Parse/Minimal", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			p := parser.NewJSONParser(bytes.NewReader(minimalPayload))
			_, _ = p.Parse(hook.EventTypePreToolUse)
		}
	})
}
