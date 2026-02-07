package parser

import (
	"bytes"
	"testing"

	"github.com/smykla-labs/klaudiush/pkg/hook"
)

func FuzzJSONParse(f *testing.F) {
	// Seed corpus with various JSON inputs
	f.Add([]byte(`{"tool_name":"Bash","tool_input":{"command":"git status"}}`))
	f.Add([]byte(`{"tool":"Write","tool_input":{"file_path":"/tmp/test.txt"}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"tool_name":"","tool_input":null}`))
	f.Add([]byte(`{invalid json`))
	f.Add([]byte(`{"tool_name":"Read","tool_input":{"file_path":"/etc/passwd"}}`))
	f.Add([]byte(`{"tool_name":"Edit","tool_input":{"file_path":"test.go"}}`))
	f.Add([]byte(`{"tool_name":"Edit","tool_input":{"old_string":"foo","new_string":"bar"}}`))
	f.Add([]byte(`{"notification_type":"bell"}`))
	f.Add([]byte(`{"tool_name":"Grep","tool_input":{"pattern":"TODO"}}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"string"`))
	f.Add([]byte{})

	f.Fuzz(func(_ *testing.T, data []byte) {
		p := NewJSONParser(bytes.NewReader(data))

		// Test with different event types
		for _, eventType := range []hook.EventType{
			hook.EventTypePreToolUse,
			hook.EventTypePostToolUse,
			hook.EventTypeNotification,
		} {
			ctx, err := p.Parse(eventType)
			if err == nil && ctx != nil {
				// Access all fields - should not panic
				_ = ctx.EventType
				_ = ctx.ToolName
				_ = ctx.ToolInput.Command
				_ = ctx.ToolInput.FilePath
				_ = ctx.ToolInput.Content
				_ = ctx.NotificationType
				_ = ctx.RawJSON
				_ = ctx.SessionID
				_ = ctx.ToolUseID
				_ = ctx.TranscriptPath
				_ = ctx.HasSessionID()
			}

			// Reset reader for next iteration
			p = NewJSONParser(bytes.NewReader(data))
		}
	})
}
