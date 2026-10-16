package file_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("AICommentValidator in Lua files", func() {
	var strict, filler *file.AICommentValidator

	BeforeEach(func() {
		strict = file.NewAICommentValidator(logger.NewNoOpLogger(),
			&config.AICommentValidatorConfig{Mode: config.AICommentModeStrict}, nil)
		filler = file.NewAICommentValidator(logger.NewNoOpLogger(), nil, nil)
	})

	writeCtx := func(content string) *hook.Context {
		return &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{FilePath: "/repo/HandlerPreview.LUA", Content: content},
		}
	}

	DescribeTable("allows code and comment-like string contents in strict mode",
		func(content string) {
			result := strict.Validate(context.Background(), writeCtx(content))
			Expect(result.Passed).To(BeTrue(), result.Message)
		},
		Entry("length operators", "while #items > 0 and n > 1 do\n  n = #items\nend"),
		Entry("Busted count assertion", "assert.are.equal(2, #list)"),
		Entry("floor division", "local n = 10 // 3"),
		Entry("quoted markers", `local s = " -- Set x #items // Set y"`),
		Entry("escaped quotes", `local s = "\" -- Set x"`),
		Entry("long string", "local s = [[\n-- Set x\n#items // Set y\n]]"),
		Entry("long string with equals", "local s = [==[\n]=] -- Set x\n]==]"),
		Entry("escaped newline in string", "local s = \"text\\\n-- Set x\""),
		Entry("whitespace escape in string", "local s = \"text\\z\n \n-- Set x\""),
		Entry("task marker", "-- TODO: remove legacy fallback\nlocal x = 1"),
		Entry("block task marker", "--[=[TODO: remove legacy fallback\n]=]\nlocal x = 1"),
	)

	DescribeTable(
		"reports real comments in strict mode",
		func(content, reported string) {
			result := strict.Validate(context.Background(), writeCtx(content))
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring(reported))
		},
		Entry("line comment", "-- holds the total\nlocal x = 1", "Line 1: -- holds the total"),
		Entry(
			"trailing comment",
			"local n = #items -- holds the total",
			"Line 1: -- holds the total",
		),
		Entry("unspaced comment", "local n = #items--holds the total", "Line 1: --holds the total"),
		Entry(
			"second comment after exempt block",
			"--[[TODO: revisit]] -- holds the total",
			"Line 1: -- holds the total",
		),
		Entry(
			"block comment",
			"--[[holds the total\n]]\nlocal x = 1",
			"Line 1: --[[holds the total",
		),
		Entry("long block comment", "--[==[holds the total\n]==]", "Line 1: --[==[holds the total"),
		Entry(
			"comment after long string",
			"local s = [[-- Set x]] -- holds the total",
			"Line 1: -- holds the total",
		),
		Entry(
			"comment after block closes",
			"--[[TODO: revisit\n' `\n]]\n-- holds the total",
			"Line 4: -- holds the total",
		),
	)

	DescribeTable(
		"reports filler in Lua comments",
		func(content, reported string) {
			result := filler.Validate(context.Background(), writeCtx(content))
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring(reported))
		},
		Entry("action narration", "-- Set the value\nlocal x = 1", "Line 1: -- Set"),
		Entry(
			"generic narration",
			"-- This function returns a value\nlocal x = 1",
			"Line 1: -- This function returns",
		),
		Entry("block narration", "--[[Set the value\n]]\nlocal x = 1", "Line 1: --[[Set"),
		Entry("equals block narration", "--[=[Set the value]=]", "Line 1: --[=[Set"),
		Entry("unspaced narration", "local x=1-- Set the value", "Line 1: -- Set"),
	)

	DescribeTable("keeps filler matching inside comment spans",
		func(content string) {
			result := filler.Validate(context.Background(), writeCtx(content))
			Expect(result.Passed).To(BeTrue(), result.Message)
		},
		Entry("quoted narration after block", `--[[rationale]] print(" -- Set the value")`),
		Entry("long string after block", `--[[rationale]] print([[ -- Set the value]])`),
	)

	DescribeTable("uses the file context for edits",
		func(original, old, replacement string, passed bool) {
			path := filepath.Join(GinkgoT().TempDir(), "preview.lua")
			Expect(os.WriteFile(path, []byte(original), 0o600)).To(Succeed())
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeEdit,
				ToolInput: hook.ToolInput{FilePath: path, OldString: old, NewString: replacement},
			}

			result := strict.Validate(context.Background(), ctx)
			Expect(result.Passed).To(Equal(passed), result.Message)
		},
		Entry("length operator", "local n = 1\n", "local n = 1", "local n = #items", true),
		Entry("inside long string", "local s = [=[\nold\n]=]", "old", "-- Set x", true),
		Entry("inside block comment", "--[[TODO: revisit\nold\n]]", "old", "-- Set x", true),
		Entry("after long string closes", "local s = [=[\nold\n", "old", "]=]\n-- Set x", false),
		Entry("inside quoted line prefix", "local s = \"old\"", "old", "-- Set x", true),
		Entry("inside line comment", "-- TODO: old", "old", "-- Set x", true),
	)
})
