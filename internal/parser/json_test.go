package parser_test

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/parser"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
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

	Describe("Parse with Codex input", func() {
		It("parses SessionStart payloads with provider-aware metadata", func() {
			input := `{
				"session_id": "sess-123",
				"cwd": "/tmp/project",
				"permission_mode": "workspace-write",
				"model": "gpt-5.4",
				"source": "cli"
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderCodex,
				EventName: "SessionStart",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.Provider).To(Equal(hook.ProviderCodex))
			Expect(ctx.Event).To(Equal(hook.CanonicalEventSessionStart))
			Expect(ctx.EventName()).To(Equal("SessionStart"))
			Expect(ctx.WorkingDir).To(Equal("/tmp/project"))
			Expect(ctx.PermissionMode).To(Equal("workspace-write"))
			Expect(ctx.Model).To(Equal("gpt-5.4"))
			Expect(ctx.Source).To(Equal("cli"))
		})

		It("parses AfterToolUse payloads and derives affected paths", func() {
			input := `{
				"session_id": "sess-123",
				"cwd": "/tmp/project",
				"hook_event": {
					"event_type": "AfterToolUse",
					"turn_id": "turn-123",
					"call_id": "call-123",
					"tool_executed": true,
					"tool_succeeded": true,
					"tool_mutating": true,
					"tool_name": "apply_patch",
					"tool_input": {
						"input": "*** Begin Patch\n*** Add File: docs/new.md\n+hello\n*** Update File: go.mod\n@@\n-go 1.25\n+go 1.26\n*** End Patch\n"
					}
				}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider: hook.ProviderCodex,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.Provider).To(Equal(hook.ProviderCodex))
			Expect(ctx.Event).To(Equal(hook.CanonicalEventAfterTool))
			Expect(ctx.EventName()).To(Equal("AfterToolUse"))
			Expect(ctx.TurnID).To(Equal("turn-123"))
			Expect(ctx.ToolUseID).To(Equal("call-123"))
			Expect(ctx.ToolExecuted).To(BeTrue())
			Expect(ctx.ToolSucceeded).To(BeTrue())
			Expect(ctx.ToolMutating).To(BeTrue())
			Expect(ctx.ToolName).To(Equal(hook.ToolTypeEdit))
			Expect(ctx.ToolFamily).To(Equal(hook.ToolFamilyEdit))
			Expect(ctx.AffectedPaths).To(ConsistOf("docs/new.md", "go.mod"))
		})

		It("parses Stop payloads with stop-hook fields", func() {
			input := `{
				"session_id": "sess-123",
				"cwd": "/tmp/project",
				"last_assistant_message": "done",
				"stop_hook_active": true
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderCodex,
				EventName: "Stop",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.Provider).To(Equal(hook.ProviderCodex))
			Expect(ctx.Event).To(Equal(hook.CanonicalEventTurnStop))
			Expect(ctx.EventName()).To(Equal("Stop"))
			Expect(ctx.LastAssistantMessage).To(Equal("done"))
			Expect(ctx.StopHookActive).To(BeTrue())
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
