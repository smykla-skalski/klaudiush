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

		It("prefers payload hook_event_name over the configured hook type", func() {
			input := `{
				"hook_event_name": "PostToolUse",
				"tool_name": "Write",
				"tool_input": {
					"file_path": "README.md",
					"content": "# hello"
				}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderClaude,
				EventType: hook.EventTypePreToolUse,
				EventName: "PreToolUse",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.Provider).To(Equal(hook.ProviderClaude))
			Expect(ctx.Event).To(Equal(hook.CanonicalEventAfterTool))
			Expect(ctx.EventType).To(Equal(hook.EventTypePostToolUse))
			Expect(ctx.EventName()).To(Equal("PostToolUse"))
			Expect(ctx.AffectedPaths).To(ConsistOf("README.md"))
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

		It("parses legacy nested AfterToolUse payloads as PostToolUse", func() {
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
			Expect(ctx.EventName()).To(Equal("PostToolUse"))
			Expect(ctx.TurnID).To(Equal("turn-123"))
			Expect(ctx.ToolUseID).To(Equal("call-123"))
			Expect(ctx.ToolExecuted).To(BeTrue())
			Expect(ctx.ToolSucceeded).To(BeTrue())
			Expect(ctx.ToolMutating).To(BeTrue())
			Expect(ctx.ToolName).To(Equal(hook.ToolTypeEdit))
			Expect(ctx.ToolFamily).To(Equal(hook.ToolFamilyEdit))
			Expect(ctx.AffectedPaths).To(ConsistOf("docs/new.md", "go.mod"))
		})

		It("parses current PreToolUse Bash payloads", func() {
			input := `{
				"session_id": "sess-123",
				"turn_id": "turn-1",
				"cwd": "/tmp/project",
				"hook_event_name": "PreToolUse",
				"model": "gpt-5.5",
				"permission_mode": "default",
				"tool_name": "Bash",
				"tool_use_id": "call-1",
				"tool_input": {"command": "git push --force origin main"}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderCodex,
				EventName: "PreToolUse",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.Provider).To(Equal(hook.ProviderCodex))
			Expect(ctx.Event).To(Equal(hook.CanonicalEventBeforeTool))
			Expect(ctx.EventType).To(Equal(hook.EventTypePreToolUse))
			Expect(ctx.EventName()).To(Equal("PreToolUse"))
			Expect(ctx.ToolName).To(Equal(hook.ToolTypeBash))
			Expect(ctx.GetCommand()).To(Equal("git push --force origin main"))
			Expect(ctx.ToolUseID).To(Equal("call-1"))
		})

		It("reads apply_patch text from tool_input.command as a file edit", func() {
			input := `{
				"session_id": "sess-123",
				"cwd": "/tmp/project",
				"hook_event_name": "PreToolUse",
				"tool_name": "apply_patch",
				"tool_input": {
					"command": "*** Begin Patch\n*** Update File: docs/guide.md\n@@\n-old line\n+new line\n+second line\n*** End Patch\n"
				}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderCodex,
				EventName: "PreToolUse",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.ToolName).To(Equal(hook.ToolTypeEdit))
			Expect(ctx.IsBashTool()).To(BeFalse())
			Expect(ctx.GetCommand()).To(BeEmpty())
			Expect(ctx.AffectedPaths).To(ConsistOf("docs/guide.md"))
			Expect(ctx.GetFilePath()).To(Equal("docs/guide.md"))
			Expect(ctx.ToolInput.OldString).To(Equal("old line"))
			Expect(ctx.ToolInput.NewString).To(Equal("new line\nsecond line"))
		})

		It("treats an apply_patch that only adds one file as a Write", func() {
			input := `{
				"hook_event_name": "PreToolUse",
				"tool_name": "apply_patch",
				"tool_input": {
					"command": "*** Begin Patch\n*** Add File: run.sh\n+#!/bin/sh\n+echo $1\n*** End Patch\n"
				}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderCodex,
				EventName: "PreToolUse",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.ToolName).To(Equal(hook.ToolTypeWrite))
			Expect(ctx.RawToolName).To(Equal("apply_patch"))
			Expect(ctx.GetFilePath()).To(Equal("run.sh"))
			Expect(ctx.GetContent()).To(Equal("#!/bin/sh\necho $1"))
		})

		It("splits multi-file apply_patch payloads into per-file entries", func() {
			input := `{
				"hook_event_name": "PreToolUse",
				"tool_name": "apply_patch",
				"tool_input": {
					"command": "*** Begin Patch\n*** Add File: a.md\n+a\n*** Add File: b.md\n+b\n*** End Patch\n"
				}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderCodex,
				EventName: "PreToolUse",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.AffectedPaths).To(ConsistOf("a.md", "b.md"))
			Expect(ctx.ToolName).To(Equal(hook.ToolTypeEdit))
			Expect(ctx.ToolInput.NewString).To(BeEmpty())
			Expect(ctx.PatchFiles).To(HaveLen(2))
			Expect(ctx.PatchFiles[1].ToolName).To(Equal(hook.ToolTypeWrite))
			Expect(ctx.PatchFiles[1].Input.FilePath).To(Equal("b.md"))
			Expect(ctx.PatchFiles[1].Input.Content).To(Equal("b"))
		})

		It("covers both paths of an apply_patch move", func() {
			input := `{
				"hook_event_name": "PreToolUse",
				"tool_name": "apply_patch",
				"tool_input": {
					"command": "*** Begin Patch\n*** Update File: ok.txt\n*** Move to: protected/moved.txt\n@@\n-a\n+b\n*** End Patch\n"
				}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderCodex,
				EventName: "PreToolUse",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.AffectedPaths).To(ConsistOf("ok.txt", "protected/moved.txt"))
			Expect(ctx.PatchFiles).To(HaveLen(2))
			Expect(ctx.PatchFiles[1].Input.FilePath).To(Equal("protected/moved.txt"))
			Expect(ctx.PatchFiles[1].Input.NewString).To(Equal("b"))
		})

		DescribeTable("finds no patch paths without usable patch text",
			func(toolInput string) {
				input := `{"hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":` +
					toolInput + `}`

				p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
				ctx, err := p.ParseWithOptions(parser.ParseOptions{
					Provider:  hook.ProviderCodex,
					EventName: "PreToolUse",
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(ctx.AffectedPaths).To(BeEmpty())
				Expect(ctx.PatchFiles).To(BeEmpty())
			},
			Entry("no patch text", `{}`),
			Entry("non-string input", `{"input": 42}`),
		)

		It("keeps patch context lines in both old and new text", func() {
			input := `{
				"hook_event_name": "PreToolUse",
				"tool_name": "apply_patch",
				"tool_input": {
					"command": "*** Begin Patch\n*** Update File: a.go\n@@\n keep\n-old\n+new\n*** End Patch\n"
				}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderCodex,
				EventName: "PreToolUse",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.ToolInput.OldString).To(Equal("keep\nold"))
			Expect(ctx.ToolInput.NewString).To(Equal("keep\nnew"))
		})

		It("keeps MCP tool names and arguments on PreToolUse", func() {
			input := `{
				"hook_event_name": "PreToolUse",
				"tool_name": "mcp__github__create_pull_request",
				"tool_input": {"title": "feat(x): y", "body": "z"}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderCodex,
				EventName: "PreToolUse",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.Event).To(Equal(hook.CanonicalEventBeforeTool))
			Expect(ctx.RawToolName).To(Equal("mcp__github__create_pull_request"))
			Expect(ctx.ToolInput.Additional).To(HaveKey("title"))
		})

		It("keeps Codex PermissionRequest distinct from PreToolUse", func() {
			input := `{"hook_event_name": "PermissionRequest", "tool_name": "Bash", "tool_input": {"command": "ls"}}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{Provider: hook.ProviderCodex})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.EventName()).To(Equal("PermissionRequest"))
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

	Describe("Parse with Gemini input", func() {
		It("parses BeforeTool payloads with Gemini tool mappings", func() {
			input := `{
				"session_id": "sess-gemini",
				"cwd": "/tmp/project",
				"hook_event_name": "BeforeTool",
				"tool_name": "run_shell_command",
				"tool_input": {
					"command": "git status"
				}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderGemini,
				EventName: "BeforeTool",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.Provider).To(Equal(hook.ProviderGemini))
			Expect(ctx.Event).To(Equal(hook.CanonicalEventBeforeTool))
			Expect(ctx.EventName()).To(Equal("BeforeTool"))
			Expect(ctx.ToolName).To(Equal(hook.ToolTypeBash))
			Expect(ctx.ToolFamily).To(Equal(hook.ToolFamilyShell))
			Expect(ctx.GetCommand()).To(Equal("git status"))
		})

		It("parses AfterTool payloads and derives affected paths", func() {
			input := `{
				"session_id": "sess-gemini",
				"cwd": "/tmp/project",
				"hook_event_name": "AfterTool",
				"tool_name": "write_file",
				"tool_input": {
					"file_path": "README.md",
					"content": "# hello"
				},
				"tool_response": {
					"llmContent": "ok"
				}
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
			ctx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderGemini,
				EventName: "AfterTool",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(ctx.Provider).To(Equal(hook.ProviderGemini))
			Expect(ctx.Event).To(Equal(hook.CanonicalEventAfterTool))
			Expect(ctx.EventName()).To(Equal("AfterTool"))
			Expect(ctx.ToolName).To(Equal(hook.ToolTypeWrite))
			Expect(ctx.ToolFamily).To(Equal(hook.ToolFamilyWrite))
			Expect(ctx.AffectedPaths).To(ConsistOf("README.md"))
		})

		It("parses SessionEnd and PreCompress lifecycle payloads", func() {
			sessionEndInput := `{
				"session_id": "sess-gemini",
				"cwd": "/tmp/project",
				"reason": "exit"
			}`

			p := parser.NewJSONParser(bytes.NewReader([]byte(sessionEndInput)))
			sessionEndCtx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderGemini,
				EventName: "SessionEnd",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(sessionEndCtx.Provider).To(Equal(hook.ProviderGemini))
			Expect(sessionEndCtx.Event).To(Equal(hook.CanonicalEventTurnStop))
			Expect(sessionEndCtx.EventName()).To(Equal("SessionEnd"))

			preCompressInput := `{
				"session_id": "sess-gemini",
				"cwd": "/tmp/project",
				"trigger": "manual"
			}`

			p = parser.NewJSONParser(bytes.NewReader([]byte(preCompressInput)))
			preCompressCtx, err := p.ParseWithOptions(parser.ParseOptions{
				Provider:  hook.ProviderGemini,
				EventName: "PreCompress",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(preCompressCtx.Provider).To(Equal(hook.ProviderGemini))
			Expect(preCompressCtx.Event).To(Equal(hook.CanonicalEventPreCompress))
			Expect(preCompressCtx.EventName()).To(Equal("PreCompress"))
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

var _ = Describe("Parse with Elicitation input", func() {
	It("parses Elicitation event with all fields", func() {
		input := `{
			"hook_event_name": "Elicitation",
			"mcp_server_name": "my-mcp-server",
			"mode": "form",
			"url": "https://example.com/auth",
			"elicitation_id": "elic-abc-123",
			"requested_schema": {"type": "object", "properties": {"token": {"type": "string"}}},
			"message": "Please provide your credentials"
		}`

		p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
		ctx, err := p.ParseWithOptions(parser.ParseOptions{
			Provider:  hook.ProviderClaude,
			EventName: "Elicitation",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Event).To(Equal(hook.CanonicalEventElicitation))
		Expect(ctx.IsElicitationEvent()).To(BeTrue())
		Expect(ctx.Elicitation).NotTo(BeNil())
		Expect(ctx.Elicitation.MCPServerName).To(Equal("my-mcp-server"))
		Expect(ctx.Elicitation.Mode).To(Equal("form"))
		Expect(ctx.Elicitation.URL).To(Equal("https://example.com/auth"))
		Expect(ctx.Elicitation.ElicitationID).To(Equal("elic-abc-123"))
		Expect(ctx.Elicitation.Message).To(Equal("Please provide your credentials"))
		Expect(ctx.Elicitation.RequestedSchema).NotTo(BeEmpty())
		Expect(ctx.GetMCPServerName()).To(Equal("my-mcp-server"))
	})

	It("parses ElicitationResult event with action and content", func() {
		input := `{
			"hook_event_name": "ElicitationResult",
			"mcp_server_name": "my-mcp-server",
			"action": "approve",
			"content": {"token": "abc123"}
		}`

		p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
		ctx, err := p.ParseWithOptions(parser.ParseOptions{
			Provider:  hook.ProviderClaude,
			EventName: "ElicitationResult",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Event).To(Equal(hook.CanonicalEventElicitationResult))
		Expect(ctx.IsElicitationEvent()).To(BeTrue())
		Expect(ctx.Elicitation).NotTo(BeNil())
		Expect(ctx.Elicitation.MCPServerName).To(Equal("my-mcp-server"))
		Expect(ctx.Elicitation.Action).To(Equal("approve"))
		Expect(ctx.Elicitation.Content).NotTo(BeEmpty())
	})

	It("does not populate Elicitation field for non-elicitation events", func() {
		input := `{
			"hook_event_name": "PreToolUse",
			"tool_name": "Bash",
			"tool_input": {"command": "echo test"}
		}`

		p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
		ctx, err := p.ParseWithOptions(parser.ParseOptions{
			Provider:  hook.ProviderClaude,
			EventName: "PreToolUse",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(ctx.Elicitation).To(BeNil())
		Expect(ctx.IsElicitationEvent()).To(BeFalse())
	})
})

var _ = Describe("Parse with PostCompact input", func() {
	It("parses PostCompact event with summary and trigger", func() {
		input := `{
			"hook_event_name": "PostCompact",
			"compact_summary": "Removed 15 tool calls from context",
			"trigger": "auto"
		}`

		p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
		ctx, err := p.ParseWithOptions(parser.ParseOptions{
			Provider:  hook.ProviderClaude,
			EventName: "PostCompact",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Event).To(Equal(hook.CanonicalEventPostCompact))
		Expect(ctx.CompactSummary).To(Equal("Removed 15 tool calls from context"))
		Expect(ctx.CompactTrigger).To(Equal("auto"))
	})

	It("handles PostCompact with missing optional fields", func() {
		input := `{
			"hook_event_name": "PostCompact"
		}`

		p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
		ctx, err := p.ParseWithOptions(parser.ParseOptions{
			Provider:  hook.ProviderClaude,
			EventName: "PostCompact",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Event).To(Equal(hook.CanonicalEventPostCompact))
		Expect(ctx.CompactSummary).To(BeEmpty())
		Expect(ctx.CompactTrigger).To(BeEmpty())
	})

	It("does not populate compact fields for non-PostCompact events", func() {
		input := `{
			"hook_event_name": "PreToolUse",
			"tool_name": "Bash",
			"tool_input": {"command": "echo test"}
		}`

		p := parser.NewJSONParser(bytes.NewReader([]byte(input)))
		ctx, err := p.ParseWithOptions(parser.ParseOptions{
			Provider:  hook.ProviderClaude,
			EventName: "PreToolUse",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(ctx.CompactSummary).To(BeEmpty())
		Expect(ctx.CompactTrigger).To(BeEmpty())
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
