package parser_test

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/parser"
	"github.com/smykla-labs/klaudiush/pkg/hook"
)

var _ = Describe("JSONParser", func() {
	Describe("Parse with session fields", func() {
		It("parses all session fields when present", func() {
			input := `{
				"session_id": "d267099c-6c3a-45ed-997c-2fa4c8ec9b39",
				"tool_use_id": "toolu_012EzpTqLzKXw5C4XP5E733v",
				"transcript_path": "/Users/test/.claude/transcripts/session.jsonl",
				"tool_name": "Bash",
				"tool_input": {"command": "echo test"}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.Parse(hook.EventTypePreToolUse)

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.SessionID).To(Equal("d267099c-6c3a-45ed-997c-2fa4c8ec9b39"))
			Expect(ctx.ToolUseID).To(Equal("toolu_012EzpTqLzKXw5C4XP5E733v"))
			Expect(ctx.TranscriptPath).To(Equal("/Users/test/.claude/transcripts/session.jsonl"))
		})

		It("handles missing session fields gracefully", func() {
			input := `{
				"tool_name": "Bash",
				"tool_input": {"command": "echo test"}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.Parse(hook.EventTypePreToolUse)

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.SessionID).To(BeEmpty())
			Expect(ctx.ToolUseID).To(BeEmpty())
			Expect(ctx.TranscriptPath).To(BeEmpty())
		})

		It("handles partial session fields", func() {
			input := `{
				"session_id": "abc-123",
				"tool_name": "Write",
				"tool_input": {"file_path": "/tmp/test.txt"}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.Parse(hook.EventTypePreToolUse)

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.SessionID).To(Equal("abc-123"))
			Expect(ctx.ToolUseID).To(BeEmpty())
			Expect(ctx.TranscriptPath).To(BeEmpty())
		})
	})

	Describe("Parse with full Claude Code input", func() {
		It("parses real-world Claude Code hook JSON", func() {
			input := `{
				"session_id": "d267099c-6c3a-45ed-997c-2fa4c8ec9b39",
				"tool_use_id": "toolu_012EzpTqLzKXw5C4XP5E733v",
				"transcript_path": "/Users/test/projects/klaudiush/d267099c-6c3a-45ed-997c-2fa4c8ec9b39.jsonl",
				"cwd": "/Users/test/projects/klaudiush",
				"permission_mode": "acceptEdits",
				"hook_event_name": "PreToolUse",
				"tool_name": "Bash",
				"tool_input": {
					"command": "git status",
					"description": "Check git status"
				}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.Parse(hook.EventTypePreToolUse)

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx).NotTo(BeNil())

			// Verify session fields
			Expect(ctx.SessionID).To(Equal("d267099c-6c3a-45ed-997c-2fa4c8ec9b39"))
			Expect(ctx.ToolUseID).To(Equal("toolu_012EzpTqLzKXw5C4XP5E733v"))
			Expect(
				ctx.TranscriptPath,
			).To(Equal("/Users/test/projects/klaudiush/d267099c-6c3a-45ed-997c-2fa4c8ec9b39.jsonl"))
			Expect(ctx.HasSessionID()).To(BeTrue())
			Expect(ctx.SessionID).To(Equal("d267099c-6c3a-45ed-997c-2fa4c8ec9b39"))

			// Verify existing fields still work
			Expect(ctx.EventType).To(Equal(hook.EventTypePreToolUse))
			Expect(ctx.ToolName).To(Equal(hook.ToolTypeBash))
			Expect(ctx.GetCommand()).To(Equal("git status"))
		})
	})

	Describe("Backward compatibility", func() {
		It("works with inputs without session fields", func() {
			input := `{
				"tool_name": "Bash",
				"tool_input": {"command": "ls -la"}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.Parse(hook.EventTypePreToolUse)

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx).NotTo(BeNil())

			// Session fields should be empty
			Expect(ctx.SessionID).To(BeEmpty())
			Expect(ctx.ToolUseID).To(BeEmpty())
			Expect(ctx.TranscriptPath).To(BeEmpty())
			Expect(ctx.HasSessionID()).To(BeFalse())

			// Existing functionality should work
			Expect(ctx.ToolName).To(Equal(hook.ToolTypeBash))
			Expect(ctx.GetCommand()).To(Equal("ls -la"))
		})
	})
})

var _ = Describe("Context session helpers", func() {
	Describe("HasSessionID", func() {
		It("returns true when session ID is present", func() {
			ctx := &hook.Context{
				SessionID: "d267099c-6c3a-45ed-997c-2fa4c8ec9b39",
			}

			Expect(ctx.HasSessionID()).To(BeTrue())
		})

		It("returns false when session ID is empty", func() {
			ctx := &hook.Context{
				SessionID: "",
			}

			Expect(ctx.HasSessionID()).To(BeFalse())
		})

		It("returns false when session ID is not set", func() {
			ctx := &hook.Context{}

			Expect(ctx.HasSessionID()).To(BeFalse())
		})
	})
})
