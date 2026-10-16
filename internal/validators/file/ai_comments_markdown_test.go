package file_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// headingTexts open with words the default filler patterns match.
var headingTexts = []string{
	"Getting started",
	"Adding a module",
	"Setting up",
	"Creating a release",
	"Returns",
	"Create",
	"Initialize",
	"Configure",
	"Install",
}

var _ = Describe("AICommentValidator in Markdown files", func() {
	var (
		filler *file.AICommentValidator
		strict *file.AICommentValidator
	)

	BeforeEach(func() {
		filler = file.NewAICommentValidator(logger.NewNoOpLogger(), nil, nil)
		strict = file.NewAICommentValidator(
			logger.NewNoOpLogger(),
			&config.AICommentValidatorConfig{Mode: config.AICommentModeStrict},
			nil,
		)
	})

	writeCtx := func(path, content string) *hook.Context {
		return &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{FilePath: path, Content: content},
		}
	}

	passes := func(path, content string) {
		GinkgoHelper()

		for name, v := range map[string]*file.AICommentValidator{
			"filler": filler, "strict": strict,
		} {
			result := v.Validate(context.Background(), writeCtx(path, content))
			Expect(result.Passed).To(BeTrue(), "%s mode: %s", name, result.Message)
		}
	}

	headingEntries := func() []TableEntry {
		var entries []TableEntry

		for _, text := range headingTexts {
			for level := 1; level <= 6; level++ {
				heading := strings.Repeat("#", level) + " " + text
				entries = append(entries, Entry(
					fmt.Sprintf("level %d: %s", level, text),
					"/repo/docs/guide.md", heading+"\n\nBody text.\n",
				))
			}
		}

		return entries
	}

	DescribeTable(
		"does not report ATX headings as comments",
		passes,
		headingEntries(),
	)

	DescribeTable(
		"does not report headings in any Markdown extension",
		passes,
		Entry(".md", "/repo/docs/getting-started.md", "# Getting started\n"),
		Entry(".markdown", "/repo/docs/getting-started.markdown", "# Getting started\n"),
		Entry(".mdx", "/repo/docs/getting-started.mdx", "# Getting started\n"),
		Entry("upper case extension", "/repo/README.MD", "# Getting started\n"),
		Entry("heading without a space", "/repo/docs/a.md", "#Getting started\n"),
		Entry("heading with closing hashes", "/repo/docs/a.md", "## Creating a module ##\n"),
		Entry("indented heading", "/repo/docs/a.md", "   ### Setting up\n"),
		Entry("heading after front matter", "/repo/docs/a.mdx",
			"---\ntitle: Guide\n---\n\n# Getting started\n\n## Adding a module\n"),
		Entry("heading in a list item continuation", "/repo/docs/a.md",
			"- item\n\n  # Create the thing\n"),
	)

	DescribeTable(
		"does not report setext headings or inline code containing #",
		passes,
		Entry("setext level 1", "/repo/docs/a.md", "Getting started\n===============\n"),
		Entry("setext level 2", "/repo/docs/a.md", "Adding a module\n---\n\nText.\n"),
		Entry("hash in a code span", "/repo/docs/a.md",
			"Write `# Create the file` at the top of the script.\n"),
		Entry("hash in a double-backtick code span", "/repo/docs/a.md",
			"Write ``# Set the `value` first`` in the shell.\n"),
		Entry("hash after prose", "/repo/docs/a.md", "See issue # Create and fix.\n"),
		Entry("unbalanced backtick does not leak to later lines", "/repo/docs/a.md",
			"A stray ` backtick.\n\n# Getting started\n"),
	)

	DescribeTable(
		"does not check the contents of fenced code blocks",
		passes,
		Entry("heading inside a markdown fence", "/repo/docs/a.md",
			"```markdown\n# Getting started\n\n## Adding a module\n```\n"),
		Entry("shell comment inside a fence", "/repo/docs/a.md",
			"```bash\n# Install the tool\nmise install\n```\n"),
		Entry("trailing shell comment inside a fence", "/repo/docs/a.md",
			"```sh\nmise install  # Install the tool\n```\n"),
		Entry("slash comment inside a fence", "/repo/docs/a.md",
			"```go\n// Initialize the counter\ncount := 0\n```\n"),
		Entry("tilde fence", "/repo/docs/a.md",
			"~~~go\n// Initialize the counter\ncount := 0\n~~~\n"),
		Entry("fence closed by a longer fence", "/repo/docs/a.md",
			"```go\n// Initialize the counter\n`````\n"),
		Entry("shorter inner fence does not close", "/repo/docs/a.md",
			"````markdown\n```go\n```\n// Initialize the counter\n````\n"),
		Entry("other fence character does not close", "/repo/docs/a.md",
			"```go\n~~~\n// Initialize the counter\n```\n"),
		Entry("unterminated fence runs to the end", "/repo/docs/a.md",
			"```go\n// Initialize the counter\n"),
		Entry("indented fence in a list", "/repo/docs/a.mdx",
			"1. Step\n\n   ```go\n   // Initialize the counter\n   ```\n"),
		Entry("fence with CRLF line endings", "/repo/docs/a.md",
			"```go\r\n// Initialize the counter\r\n```\r\n"),
	)

	DescribeTable(
		"keeps checking prose outside fenced code blocks",
		func(content string, wantLine string) {
			result := filler.Validate(
				context.Background(), writeCtx("/repo/docs/a.md", content),
			)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring(wantLine))
		},
		Entry("after a closed fence",
			"```go\n// Initialize the counter\n```\n// Initialize the counter\n",
			"Line 4: // Initialize"),
		Entry("after a closed tilde fence",
			"~~~\nx\n~~~\n// Initialize the counter\n", "Line 4:"),
		Entry("a fence-looking line with a backtick in its info string is prose",
			"``` not a fence ` here\n// Initialize the counter\n", "Line 2:"),
		Entry("a heading does not hide a later slash comment",
			"# Getting started\n// Initialize the counter\n", "Line 2:"),
	)

	DescribeTable(
		"keeps reporting # filler comments outside Markdown",
		func(path, content string) {
			result := filler.Validate(context.Background(), writeCtx(path, content))
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Message).To(ContainSubstring("Line 1: # Getting"))
		},
		Entry("yaml", "/repo/config.yaml", "# Getting the value\nkey: value\n"),
		Entry("yml", "/repo/config.yml", "# Getting the value\nkey: value\n"),
		Entry("toml", "/repo/config.toml", "# Getting the value\nkey = 1\n"),
		Entry("shell", "/repo/run.sh", "# Getting the value\nx=1\n"),
		Entry("python", "/repo/run.py", "# Getting the value\nx = 1\n"),
		Entry("ruby", "/repo/run.rb", "# Getting the value\nx = 1\n"),
		Entry("plain text", "/repo/notes.txt", "# Getting the value\nx\n"),
		Entry("no extension", "/repo/Makefile", "# Getting the value\nall:\n"),
		Entry("md in the directory name", "/repo/docs.md/run.sh", "# Getting the value\nx=1\n"),
		Entry("md as a middle extension", "/repo/notes.md.txt", "# Getting the value\n"),
		Entry("no path", "", "# Getting the value\nx=1\n"),
	)

	Context("Edit payloads", func() {
		var dir string

		BeforeEach(func() {
			dir = GinkgoT().TempDir()
		})

		editCtx := func(path, oldString, newString string) *hook.Context {
			return &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeEdit,
				ToolInput: hook.ToolInput{
					FilePath: path, OldString: oldString, NewString: newString,
				},
			}
		}

		writeDoc := func(name, content string) string {
			path := filepath.Join(dir, name)
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

			return path
		}

		It("allows a heading in new_string of a file that does not exist", func() {
			ctx := editCtx(filepath.Join(dir, "missing.md"), "Old", "# Getting started")
			Expect(filler.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("allows headings in new_string of an existing file", func() {
			path := writeDoc("guide.md", "# Old\n\nText.\n")
			ctx := editCtx(path, "# Old", "# Getting started\n\n## Adding a module")
			Expect(filler.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("allows a heading in an Edit with no old_string", func() {
			path := writeDoc("guide.md", "# Old\n")
			ctx := editCtx(path, "", "# Getting started\n\n## Setting up")
			Expect(filler.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("allows a shell comment in a fragment inside an existing fence", func() {
			path := writeDoc("guide.md", "Intro.\n\n```bash\nold-command\n```\n")
			ctx := editCtx(path, "old-command", "# Install the tool\nmise install")
			Expect(filler.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("allows a slash comment in a fragment inside an existing fence", func() {
			path := writeDoc("guide.md", "Intro.\n\n```go\nold()\n```\n")
			ctx := editCtx(path, "old()", "// Initialize the counter\ncount := 0")
			Expect(filler.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("checks a fragment in prose after an earlier fence closed", func() {
			path := writeDoc("guide.md", "```go\nx()\n```\n\nold prose\n")
			ctx := editCtx(path, "old prose", "// Initialize the counter")
			Expect(filler.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("closes a fence the fragment opens", func() {
			path := writeDoc("guide.md", "old prose\n")
			ctx := editCtx(path, "old prose", "```go\n// Initialize the counter\n```")
			Expect(filler.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("keeps reporting a # filler comment when editing a YAML file", func() {
			path := writeDoc("config.yaml", "key: old\n")
			ctx := editCtx(path, "key: old", "# Getting the value\nkey: new")
			result := filler.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Line 1: # Getting"))
		})

		It("keeps reporting a # filler comment when editing a shell script", func() {
			path := writeDoc("run.sh", "x=old\n")
			ctx := editCtx(path, "x=old", "# Getting the value\nx=new")
			Expect(filler.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})
	})
})
