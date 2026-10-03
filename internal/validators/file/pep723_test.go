package file_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const pep723Script = "#!/usr/bin/env -S uv run --quiet --script\n" +
	"# /// script\n" +
	"# requires-python = \">=3.10\"\n" +
	"# dependencies = [\"requests>=2.31\"]\n" +
	"# ///\n" +
	"import requests\n"

var reportedLine = regexp.MustCompile(`(?m)^Line (\d+):`)

// flaggedLines returns the line numbers a FILE011 result reports, or nil.
func flaggedLines(result *validator.Result) []int {
	lines := []int{}
	if result.Passed {
		return lines
	}

	for _, m := range reportedLine.FindAllStringSubmatch(result.Message, -1) {
		n, err := strconv.Atoi(m[1])
		Expect(err).NotTo(HaveOccurred())

		lines = append(lines, n)
	}

	return lines
}

var _ = Describe("AICommentValidator PEP 723 metadata", func() {
	var (
		sv  *file.AICommentValidator
		ctx *hook.Context
	)

	BeforeEach(func() {
		sv = file.NewAICommentValidator(
			logger.NewNoOpLogger(),
			&config.AICommentValidatorConfig{Mode: config.AICommentModeStrict},
			nil,
		)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
		}
	})

	DescribeTable(
		"Write",
		func(path, content string, flagged ...int) {
			ctx.ToolInput.FilePath = path
			ctx.ToolInput.Content = content
			Expect(flaggedLines(sv.Validate(context.Background(), ctx))).To(Equal(flagged))
		},
		Entry("block after a shebang", "/repo/fetch.py", pep723Script),
		Entry("block without a shebang", "/repo/fetch.py",
			"# /// script\n# dependencies = []\n# ///\nimport sys\n"),
		Entry("block lower in the file", "/repo/fetch.py",
			"import sys\n\n# /// script\n# dependencies = [\n#   \"rich\",\n# ]\n# ///\n"),
		Entry("block with a bare # line", "/repo/fetch.py",
			"# /// script\n# [tool.uv]\n#\n# key = 1\n# ///\n"),
		Entry("block of a type other than script", "/repo/fetch.py",
			"def f(xs):\n# /// note\n# sum the values\n# ///\n    return sum(xs)\n", 2, 3, 4),
		Entry(
			"second script block",
			"/repo/fetch.py",
			"# /// script\n# a = 1\n# ///\nimport sys\n# /// script\n# sum the values\n# ///\n",
			5,
			6,
			7,
		),
		Entry("block in CRLF content", "/repo/fetch.py",
			"# /// script\r\n# dependencies = []\r\n# ///\r\nimport sys\r\n"),
		Entry("closes on the last # /// of the run", "/repo/fetch.py",
			"# /// script\n# a = 1\n# ///\n# b = 2\n# ///\nimport sys\n"),
		Entry("comment after the block is flagged", "/repo/fetch.py",
			pep723Script+"# fetch the page before parsing\nrequests.get(u)\n", 7),
		Entry("comment right after the closing line is flagged", "/repo/fetch.py",
			"# /// script\n# dependencies = []\n# ///\n# fetch the page\nimport sys\n", 4),
		Entry("unterminated block hides nothing", "/repo/fetch.py",
			"# /// script\n# dependencies = []\nimport sys\n# fetch the page\n# ///\n", 1, 2, 4, 5),
		Entry("unterminated block at end of file", "/repo/fetch.py",
			"# /// script\n# dependencies = []\n", 1, 2),
		Entry("block without content lines", "/repo/fetch.py",
			"# /// script\n# ///\nimport sys\n", 1, 2),
		Entry("unspaced content line breaks the block", "/repo/fetch.py",
			"# /// script\n#dependencies = []\n# ///\n", 1, 2, 3),
		Entry("indented block is not top level", "/repo/fetch.py",
			"if x:\n    # /// script\n    # a = 1\n    # ///\n    pass\n", 2, 3, 4),
		Entry("start line with trailing text", "/repo/fetch.py",
			"# /// script extra\n# a = 1\n# ///\n", 1, 2, 3),
		Entry("start line without a type", "/repo/fetch.py",
			"# ///\n# a = 1\n# ///\n", 1, 2, 3),
		Entry("block inside a docstring is not a comment", "/repo/fetch.py",
			"DOC = \"\"\"\n# /// script\n# a = 1\n# ///\n\"\"\"\n"),
		Entry("extension-less uv script", "/repo/bin/fetch", pep723Script),
		Entry("comment in an extension-less uv script is flagged", "/repo/bin/fetch",
			pep723Script+"# fetch the page\n", 7),
		Entry("extension-less file without a python shebang", "/repo/bin/fetch",
			"#!/bin/sh\n# /// script\n# a = 1\n# ///\n", 2, 3, 4),
		Entry("only python files are exempt", "/repo/main.rb",
			"# /// script\n# a = 1\n# ///\n", 1, 2, 3),
	)

	Context("Edit", func() {
		var dir string

		BeforeEach(func() {
			dir = GinkgoT().TempDir()
			ctx.ToolName = hook.ToolTypeEdit
		})

		writeSource := func(content string) string {
			path := filepath.Join(dir, "fetch.py")
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

			return path
		}

		DescribeTable(
			"against the file on disk",
			func(source, oldString, newString string, flagged ...int) {
				ctx.ToolInput.FilePath = writeSource(source)
				ctx.ToolInput.OldString = oldString
				ctx.ToolInput.NewString = newString
				Expect(flaggedLines(sv.Validate(context.Background(), ctx))).To(Equal(flagged))
			},
			Entry("adds a whole block", "import requests\n",
				"import requests", pep723Script),
			Entry("changes a line inside an existing block", pep723Script,
				"# dependencies = [\"requests>=2.31\"]",
				"# dependencies = [\"requests>=2.31\", \"rich\"]"),
			Entry("adds lines inside an existing block", pep723Script,
				"# requires-python = \">=3.10\"",
				"# requires-python = \">=3.11\"\n#\n# [tool.uv]\n# exclude-newer = \"2026-01-01\""),
			Entry("comment added after an existing block", pep723Script,
				"import requests", "# fetch the page\nimport requests", 1),
			Entry("edit that breaks the block", pep723Script,
				"# dependencies = [\"requests>=2.31\"]",
				"DEPS = 1\n# dependencies = []", 2),
			Entry("block-like lines in an unterminated block", "# /// script\nimport sys\n",
				"import sys", "# dependencies = []\nimport sys", 1),
		)

		It("exempts a line edited deep inside a long block", func() {
			ctx.ToolInput.FilePath = writeSource("# /// script\n# dependencies = [\n" +
				strings.Repeat("#   \"pkg\",\n", 300) + "#   \"target\",\n# ]\n# ///\nimport sys\n")
			ctx.ToolInput.OldString = "#   \"target\","
			ctx.ToolInput.NewString = "#   \"target2\","
			Expect(flaggedLines(sv.Validate(context.Background(), ctx))).To(BeEmpty())
		})

		It("exempts a line edited far above the closing line of a long block", func() {
			ctx.ToolInput.FilePath = writeSource(
				"# /// script\n# dependencies = [\n#   \"target\",\n" +
					strings.Repeat("#   \"pkg\",\n", 300) + "# ]\n# ///\nimport sys\n",
			)
			ctx.ToolInput.OldString = "#   \"target\","
			ctx.ToolInput.NewString = "#   \"target2\","
			Expect(flaggedLines(sv.Validate(context.Background(), ctx))).To(BeEmpty())
		})

		It("exempts a line edited inside an extension-less uv script", func() {
			path := filepath.Join(dir, "fetch")
			Expect(os.WriteFile(path, []byte(pep723Script), 0o600)).To(Succeed())
			ctx.ToolInput.FilePath = path
			ctx.ToolInput.OldString = "# dependencies = [\"requests>=2.31\"]"
			ctx.ToolInput.NewString = "# dependencies = [\"requests>=2.32\"]"
			Expect(flaggedLines(sv.Validate(context.Background(), ctx))).To(BeEmpty())
		})

		It("flags a second block added far below the first", func() {
			ctx.ToolInput.FilePath = writeSource(
				pep723Script + strings.Repeat("x = 1\n", 300) + "y = 2\n",
			)
			ctx.ToolInput.OldString = "y = 2"
			ctx.ToolInput.NewString = "# /// script\n# sum the values\n# ///\ny = 2"
			Expect(flaggedLines(sv.Validate(context.Background(), ctx))).To(Equal([]int{1, 2, 3}))
		})

		It("flags a block in a patch-style Edit to a file that has one", func() {
			ctx.ToolInput.FilePath = writeSource(pep723Script)
			ctx.ToolInput.NewString = "# /// script\n# sum the values\n# ///"
			Expect(flaggedLines(sv.Validate(context.Background(), ctx))).To(Equal([]int{1, 2, 3}))
		})

		It("exempts a block in a patch-style Edit with no old_string", func() {
			ctx.ToolInput.FilePath = writeSource("import sys\n")
			ctx.ToolInput.NewString = "# /// script\n# dependencies = []\n# ///"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})
	})
})
