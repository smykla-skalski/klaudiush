package file_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("AICommentValidator broadened coverage", func() {
	var (
		v   *file.AICommentValidator
		ctx *hook.Context
	)

	BeforeEach(func() {
		v = file.NewAICommentValidator(logger.NewNoOpLogger(), nil, nil)
		ctx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
		}
	})

	DescribeTable(
		"flags comments that open with a restating verb",
		func(content string) {
			ctx.ToolInput.Content = content
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
		},
		Entry("configure", "// Configure the http client\ncli := newClient()"),
		Entry("handle", "// Handle the incoming request\nserve(r)"),
		Entry("parse", "// Parse the json payload\np := parse(b)"),
		Entry("increment without article", "// Increment counter\ncounter++"),
		Entry("append", "// Append the item\nlist = append(list, x)"),
		Entry("validate", "// Validate the input\ncheckInput(in)"),
		Entry("iterate", "// Iterate over rows\nfor range rows {\n}"),
		Entry("send", "// Send the response\nwrite(b)"),
	)

	DescribeTable(
		"allows markers, directives and doc comments",
		func(content string) {
			ctx.ToolInput.Content = content
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		},
		Entry("TODO marker", "// TODO: return the cached value\nreturn nil"),
		Entry("FIXME marker", "// FIXME handle the retry path\nx := 1"),
		Entry("NOTE marker", "// NOTE: keep in sync with the proto\nx := 1"),
		Entry("exported Go func doc",
			"// Returns the user for the given id.\nfunc GetUser(id string) *User { return nil }"),
		Entry("unexported Go func doc",
			"// maybeReportWindowDepletion emits the warning log and increments the\n"+
				"// counter when the most recent send on a now-closed stream matches the\n"+
				"// receive-window-depletion signature.\n"+
				"func (s *statsCallbacks) maybeReportWindowDepletion(last lastSend) {}"),
		Entry("Go package doc",
			"// Package dispatcher validates hook events.\npackage dispatcher"),
		Entry("Go type block doc",
			"// Returns known validation states.\ntype (\n\tvalidationState string\n)"),
		Entry("private Python def doc",
			"# Returns the configured value.\ndef _get_value():\n    return value"),
		Entry("exported JS function",
			"// Creates a new widget instance.\nexport function makeWidget() {}"),
		Entry("go:generate directive", "//go:generate mockgen -source=x.go\nx := 1"),
		Entry("go:build constraint", "//go:build linux\npackage foo"),
		Entry("exception token escape hatch",
			"// EXC:FILE011:documents-a-load-bearing-invariant here\nreturn true"),
	)

	DescribeTable(
		"default mode allows why-comments that no filler pattern catches",
		func(content string) {
			ctx.ToolInput.Content = content
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		},
		Entry("why comment", "// Guard against nil to avoid a shutdown panic.\nif cli == nil {\n}"),
		Entry("multi-line rationalization",
			"return false\n"+
				"// Unreadable store: assume a tracker exists. Claiming this stub as\n"+
				"// the only record would mint a second task for the same issue — the\n"+
				"// exact duplication this helper prevents; a mislabeled stub is cheaper.\n"+
				"s.logger.Error(\"scan\")"),
		Entry("preview build explanation",
			"// Preview builds report version 0.0.0-preview.* which sorts below every\n"+
				"// released version; treat them as latest rather than legacy.\n"+
				"latest := embeddedDNS"),
		Entry("prose starting with a slash is not a Rust doc",
			"x := 1\n// /tmp is where we stash it for now"),
	)

	DescribeTable(
		"explicit strict mode blocks in-body prose that no pattern catches",
		func(content string) {
			sv := file.NewAICommentValidator(
				logger.NewNoOpLogger(),
				&config.AICommentValidatorConfig{Mode: config.AICommentModeStrict},
				nil,
			)
			ctx.ToolInput.Content = content
			result := sv.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
		},
		Entry("why comment", "// Guard against nil to avoid a shutdown panic.\nif cli == nil {\n}"),
		Entry("trailing narration", "x := compute()  // holds the running total"),
		Entry("prose starting with a slash is not a Rust doc",
			"x := 1\n// /tmp is where we stash it for now"),
	)

	DescribeTable(
		"does not mistake markers inside multi-line backtick strings for comments",
		func(content string) {
			ctx.ToolInput.Content = content
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		},
		Entry("go raw string spanning lines with // inside",
			"q := `SELECT 1\nFROM t -- note\nWHERE u // not a comment`\nreturn q"),
		Entry("go raw string with # inside",
			"tmpl := `line one\n# not a comment here\nline three`\nreturn tmpl"),
	)

	It("allows Rust doc comments (///)", func() {
		ctx.ToolInput.FilePath = "/repo/lib.rs"
		ctx.ToolInput.Content = "/// Widget models a thing.\npub struct Widget;"
		result := v.Validate(context.Background(), ctx)
		Expect(result.Passed).To(BeTrue())
	})

	It("keeps .env dotfiles on pattern-based behaviour", func() {
		ctx.ToolInput.FilePath = "/repo/.env"
		ctx.ToolInput.Content = "# API host for the beta cohort\nHOST=localhost"
		result := v.Validate(context.Background(), ctx)
		Expect(result.Passed).To(BeTrue())
	})

	DescribeTable(
		"still flags filler that is not a doc comment",
		func(content string) {
			ctx.ToolInput.Content = content
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
		},
		Entry(
			"in-body return narration",
			"if id == \"\" {\n\treturn nil\n}\n// Returns the user for the given id.\nreturn getUser(id)",
		),
		Entry("blank line breaks doc association",
			"// Returns the user.\n\nfunc GetUser() {\n}"),
		Entry("generic package doc",
			"// This package provides validation helpers.\npackage dispatcher"),
	)
	DescribeTable(
		"allows // or # inside string and URL literals",
		func(content string) {
			ctx.ToolInput.Content = content
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		},
		Entry("https url with verb after slashes",
			`endpoint := "https://set.example.com/v1"`),
		Entry("http url with verb",
			`u := "http://config.example.com/handle"`),
		Entry("hash inside string literal",
			`color := "#set0aa"`),
		Entry("double-slash inside string literal",
			`msg := "before // after"`),
	)

	DescribeTable(
		"config/markup/data/shell files keep pattern-based behaviour",
		func(path string) {
			ctx.ToolInput.FilePath = path
			ctx.ToolInput.Content = "# Feature flag for the beta cohort\nfoo = true"
			result := v.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		},
		Entry("toml config", "/repo/config.toml"),
		Entry("yaml config", "/repo/values.yaml"),
		Entry("shell script", "/repo/setup.sh"),
		Entry("Makefile", "/repo/Makefile"),
	)

	It("allows BDD phase markers in Go tests", func() {
		ctx.ToolInput.FilePath = "/repo/svc_test.go"
		ctx.ToolInput.Content = "func TestFlow(t *testing.T) {\n// given a cached response\nseed()\n// when loading data\nload()\n// then status is fresh\nassertFresh()\n}"
		result := v.Validate(context.Background(), ctx)
		Expect(result.Passed).To(BeTrue())
	})

	It("does not allow BDD phase markers as trailing comments in strict mode", func() {
		sv := file.NewAICommentValidator(
			logger.NewNoOpLogger(),
			&config.AICommentValidatorConfig{Mode: config.AICommentModeStrict},
			nil,
		)
		ctx.ToolInput.FilePath = "/repo/svc_test.go"
		ctx.ToolInput.Content = "seed() // given a cached response"
		result := sv.Validate(context.Background(), ctx)
		Expect(result.Passed).To(BeFalse())
	})

	It("explicit strict mode blocks in-body comments in a .go file", func() {
		sv := file.NewAICommentValidator(
			logger.NewNoOpLogger(),
			&config.AICommentValidatorConfig{Mode: config.AICommentModeStrict},
			nil,
		)
		ctx.ToolInput.FilePath = "/repo/svc.go"
		ctx.ToolInput.Content = "x := f()\n// holds the running total"
		result := sv.Validate(context.Background(), ctx)
		Expect(result.Passed).To(BeFalse())
	})

	It("filler mode opt-out allows a plain why comment", func() {
		fv := file.NewAICommentValidator(
			logger.NewNoOpLogger(),
			&config.AICommentValidatorConfig{Mode: config.AICommentModeFiller},
			nil,
		)
		fctx := &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{
				FilePath: "/repo/svc.go",
				Content:  "// Guard against nil to avoid a panic.\nif c == nil {\n}",
			},
		}
		result := fv.Validate(context.Background(), fctx)
		Expect(result.Passed).To(BeTrue())
	})
})

func documentedField(doc, decl string) string {
	return "type T struct {\n\t// " + doc + "\n\t" + decl + "\n}"
}

var _ = Describe("AICommentValidator struct field docs", func() {
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
		ctx.ToolInput.FilePath = "/repo/api.go"
	})

	DescribeTable(
		"allows a comment documenting a tagged struct field",
		func(content string) {
			ctx.ToolInput.Content = content
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
		},
		Entry("scalar field", documentedField(
			"Version number of the control plane.",
			"Version string `json:\"version,omitempty\"`",
		)),
		Entry("pointer to named type", documentedField(
			"KumaCP is the version of the zone control plane.",
			"KumaCP *KumaCpVersion `json:\"kumaCp,omitempty\"`",
		)),
		Entry("slice of pointers", documentedField(
			"Subscriptions created by a given zone.",
			"Subscriptions []*KDSSubscription `json:\"subscriptions,omitempty\"`",
		)),
		Entry("map field", documentedField(
			"Stat holds the per service stats.",
			"Stat map[string]*KDSServiceStats `json:\"stat,omitempty\"`",
		)),
		Entry("embedded field", documentedField(
			"Time is pinned to UTC so the bytes do not vary by host.",
			"time.Time `json:\"-\"`",
		)),
	)

	DescribeTable(
		"still blocks prose above code that does not declare anything",
		func(content string) {
			ctx.ToolInput.Content = content
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		},
		Entry("assignment of a raw string",
			"// Build the query we send upstream.\nq := fmt.Sprintf(`select %d`, id)"),
		Entry("return of a map literal containing a raw string",
			"// Hand back the defaults.\nreturn map[string]string{\"a\": `b`}"),
		Entry("plain statement",
			"// Guard against nil to avoid a shutdown panic.\nif cli == nil {\n}"),
	)
})

var _ = Describe("AICommentValidator multi-line string literals", func() {
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
		"does not treat markers inside triple-quoted strings as comments",
		func(path, content string) {
			ctx.ToolInput.FilePath = path
			ctx.ToolInput.Content = content
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
		},
		Entry("markdown heading in a python triple-double string", "/repo/gen.py",
			"BODY = \"\"\"\n## Problem\n\nText.\n\"\"\"\nprint(BODY)"),
		Entry("heading in a python triple-single string", "/repo/gen.py",
			"BODY = '''\n# Steps\n// not code\n'''"),
		Entry("hash line in a python docstring", "/repo/cli.py",
			"def run():\n    \"\"\"Run the tool.\n\n    # example\n    \"\"\"\n    return 1"),
		Entry("f-string prefix", "/repo/gen.py",
			"msg = f\"\"\"\n## {title}\n\"\"\""),
		Entry("escaped quote does not close the string", "/repo/gen.py",
			"X = \"\"\"a \\\"\"\" ## b\n# still inside\n\"\"\""),
		Entry("apostrophe inside a triple-double string", "/repo/gen.py",
			"X = \"\"\"\nIt's here\n# heading\n\"\"\""),
		Entry("hash in a regular python string", "/repo/gen.py",
			"x = '# not a comment'\ny = \"## also not\""),
		Entry("toml multi-line string", "/repo/config.toml",
			"Q = '''\n# Set the value\n'''"),
	)

	DescribeTable(
		"keeps detecting real comments around triple-quoted strings",
		func(path, content string) {
			ctx.ToolInput.FilePath = path
			ctx.ToolInput.Content = content
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		},
		Entry("comment after a string closes on a later line", "/repo/gen.py",
			"BODY = \"\"\"\n## Problem\n\"\"\"\n# holds the body\nprint(BODY)"),
		Entry("trailing comment after a one-line triple string", "/repo/gen.py",
			"x = \"\"\"a\"\"\"  # holds the text"),
		Entry("comment before the string opens", "/repo/gen.py",
			"# holds the body\nBODY = \"\"\"\n## Problem\n\"\"\""),
		Entry("python comment with an apostrophe", "/repo/gen.py",
			"x = 1\n# it's the running total"),
	)

	It("filler mode ignores verb-first text inside a python triple string", func() {
		v := file.NewAICommentValidator(logger.NewNoOpLogger(), nil, nil)
		ctx.ToolInput.FilePath = "/repo/gen.py"
		ctx.ToolInput.Content = "HELP = \"\"\"\n# Set the value first\n\"\"\""
		Expect(v.Validate(context.Background(), ctx).Passed).To(BeTrue())
	})

	DescribeTable(
		"does not treat a backslash as an escape in raw triple-quoted strings",
		func(path, content string) {
			ctx.ToolInput.FilePath = path
			ctx.ToolInput.Content = content
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		},
		Entry("toml literal string ending in a backslash", "/repo/config.toml",
			"p = '''C:\\dir\\'''\n# Set the value"),
	)

	It("still honours escapes in python triple-quoted strings", func() {
		ctx.ToolInput.FilePath = "/repo/gen.py"
		ctx.ToolInput.Content = "p = \"\"\"C:\\dir\\\"\"\"\n# still inside\n\"\"\""
		Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
	})

	Context("Edit fragments", func() {
		var dir string

		BeforeEach(func() {
			dir = GinkgoT().TempDir()
			ctx.ToolName = hook.ToolTypeEdit
		})

		writeSource := func(content string) string {
			path := filepath.Join(dir, "gen.py")
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

			return path
		}

		It("allows a heading in a fragment inside an existing triple string", func() {
			ctx.ToolInput.FilePath = writeSource("BODY = \"\"\"\n## Old\n\nText.\n\"\"\"\n")
			ctx.ToolInput.OldString = "## Old"
			ctx.ToolInput.NewString = "## Problem\n\n## Steps"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("flags a comment after a fragment closes an existing docstring", func() {
			ctx.ToolInput.FilePath = writeSource(
				"def total():\n    \"\"\"Compute.\n\n    Returns the sum.\n    \"\"\"\n    return 1\n",
			)
			ctx.ToolInput.OldString = "    Returns the sum.\n    \"\"\"\n    return 1"
			ctx.ToolInput.NewString = "    Returns the total.\n    \"\"\"\n    # add tax before rounding\n    return 1"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("does not carry string state between joined patch hunks", func() {
			ctx.ToolInput.FilePath = writeSource(
				"def total():\n    \"\"\"Old summary.\"\"\"\n    x = 1\n    return x\n",
			)
			ctx.ToolInput.NewString = "    \"\"\"New summary.\n    # add tax before rounding"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("does not pair openers from different patch hunks", func() {
			ctx.ToolInput.FilePath = writeSource("def total():\n    return 1\n")
			ctx.ToolInput.NewString = "    \"\"\"Compute the total.\n" +
				"    # add tax before rounding\n" +
				"    \"\"\"Compute the average."
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("falls back to code state when replaced occurrences disagree", func() {
			ctx.ToolInput.FilePath = writeSource(
				"DOC = \"\"\"\nfoo\n\"\"\"\nfoo\n",
			)
			ctx.ToolInput.OldString = "foo"
			ctx.ToolInput.NewString = "# holds the total\nfoo"
			ctx.ToolInput.Additional = map[string]json.RawMessage{
				"replace_all": json.RawMessage("true"),
			}
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("does not open a string from quotes in an edited unspaced comment", func() {
			ctx.ToolInput.FilePath = writeSource("x = 1#note\ny = 2\n")
			ctx.ToolInput.OldString = "note\ny = 2"
			ctx.ToolInput.NewString = "note \"\"\"\n# add tax before rounding\ny = 2"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("checks a bounded number of old_string matches", func() {
			ctx.ToolInput.FilePath = writeSource(
				"DATA = [" + strings.Repeat("foo, ", 20000) + "]\n",
			)
			ctx.ToolInput.OldString = "foo"
			ctx.ToolInput.NewString = "bar"
			ctx.ToolInput.Additional = map[string]json.RawMessage{
				"replace_all": json.RawMessage("true"),
			}

			start := time.Now()
			passed := sv.Validate(context.Background(), ctx).Passed

			Expect(passed).To(BeTrue())
			Expect(time.Since(start)).To(BeNumerically("<", 2*time.Second))
		})

		It("scans joined patch hunks line by line for a new file", func() {
			ctx.ToolInput.FilePath = filepath.Join(dir, "new.py")
			ctx.ToolInput.NewString = "    \"\"\"New summary.\nx = compute()  # bump the retry count"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("flags a comment added after a python 3.12 f-string field", func() {
			ctx.ToolInput.FilePath = writeSource("print(f\"{d[\"#\"]}\", value)\n")
			ctx.ToolInput.OldString = "value)"
			ctx.ToolInput.NewString = "total)  # log the total"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("matches an LF old_string in a CRLF file", func() {
			ctx.ToolInput.FilePath = writeSource("BODY = \"\"\"\r\n## Old\r\nText.\r\n\"\"\"\r\n")
			ctx.ToolInput.OldString = "## Old\nText."
			ctx.ToolInput.NewString = "## Problem\nText."
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("recognises a python shebang in an extension-less file", func() {
			path := filepath.Join(dir, "gen")
			Expect(
				os.WriteFile(
					path,
					[]byte("#!/usr/bin/env python3\nBODY = \"\"\"\n## Old\n\"\"\"\n"),
					0o600,
				),
			).
				To(Succeed())

			ctx.ToolInput.FilePath = path
			ctx.ToolInput.OldString = "## Old"
			ctx.ToolInput.NewString = "## Problem"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("resolves a relative path against the hook working directory", func() {
			writeSource("BODY = \"\"\"\n## Old\n\nText.\n\"\"\"\n")

			ctx.WorkingDir = dir
			ctx.ToolInput.FilePath = "gen.py"
			ctx.ToolInput.OldString = "## Old"
			ctx.ToolInput.NewString = "## Problem"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("does not open a string from quotes in an edited comment", func() {
			ctx.ToolInput.FilePath = writeSource("def f():\n    # use docstrings\n    return 1\n")
			ctx.ToolInput.OldString = "use docstrings\n    return 1"
			ctx.ToolInput.NewString = "use \"\"\" for docstrings\n" +
				"    # add tax before rounding\n    return 1"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("flags new text written into an existing comment", func() {
			ctx.ToolInput.FilePath = writeSource("x = 0  # old wording\n")
			ctx.ToolInput.OldString = "old wording"
			ctx.ToolInput.NewString = "Initialize the counter"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("keeps the exemption of the edited comment's marker", func() {
			ctx.ToolInput.FilePath = writeSource("x = 0  # TODO: fix foo\n")
			ctx.ToolInput.OldString = "fix foo"
			ctx.ToolInput.NewString = "fix bar"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("keeps the doc comment exemption of an edited comment", func() {
			ctx.ToolInput.FilePath = writeSource(
				"# Returns configured value.\ndef get_value():\n    return 1\n",
			)
			ctx.ToolInput.OldString = "configured"
			ctx.ToolInput.NewString = "cached"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("flags an edited comment that documents no declaration", func() {
			ctx.ToolInput.FilePath = writeSource(
				"# Returns configured value.\n\ndef get_value():\n    return 1\n",
			)
			ctx.ToolInput.OldString = "configured"
			ctx.ToolInput.NewString = "cached"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("does not check comments after the edited text", func() {
			ctx.ToolInput.FilePath = writeSource("x = 1  # holds the total\n")
			ctx.ToolInput.OldString = "x = 1"
			ctx.ToolInput.NewString = "x = 2"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("starts in the shared string state of matches on different lines", func() {
			ctx.ToolInput.FilePath = writeSource(
				"A = \"\"\"\nfoo\n\"\"\"\nB = \"\"\"\n  foo\n\"\"\"\n",
			)
			ctx.ToolInput.OldString = "foo"
			ctx.ToolInput.NewString = "## Problem"
			ctx.ToolInput.Additional = map[string]json.RawMessage{
				"replace_all": json.RawMessage("true"),
			}
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
		})

		It("seeds no string state from a TOML literal string ending in a backslash", func() {
			path := filepath.Join(dir, "config.toml")
			Expect(
				os.WriteFile(path, []byte("dir = 'C:\\'  # don't wrap in '''\nkey = 1\n"), 0o600),
			).
				To(Succeed())

			ctx.ToolInput.FilePath = path
			ctx.ToolInput.OldString = "key = 1"
			ctx.ToolInput.NewString = "key = 1\n# Set the value to one"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})

		It("scans from code state when the file cannot be read", func() {
			ctx.ToolInput.FilePath = filepath.Join(dir, "missing.py")
			ctx.ToolInput.OldString = "x = 1"
			ctx.ToolInput.NewString = "x = 1\n# holds the total"
			Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
		})
	})

	DescribeTable(
		"uses the comment marker of the file's language",
		func(path, content string, passes bool) {
			ctx.ToolInput.FilePath = path
			ctx.ToolInput.Content = content
			Expect(sv.Validate(context.Background(), ctx).Passed).To(Equal(passes))
		},
		Entry("triple quote after an unspaced hash does not open a string", "/repo/gen.py",
			"x = 1#\"\"\"\ny = 2\n# add tax before rounding", false),
		Entry("hash in a python 3.12 f-string field reusing the quote", "/repo/gen.py",
			"line = f\"{\"#\" * depth} {title}\"", true),
		Entry("comment after a python 3.12 f-string field reusing the quote", "/repo/gen.py",
			"x = f\"{d[\"#\"]}\"  # holds the value", false),
		Entry("toml basic string closing on a quote run", "/repo/config.toml",
			"s = \"\"\"\"hi\"\"\"\"  # Set the value", false),
		Entry("triple quote inside a comment does not open a string", "/repo/gen.py",
			"x = 1  # EXC:FILE011:keep \"\"\"\ny = 2\n# add tax before rounding", false),
		Entry("python floor division is not a comment", "/repo/calc.py",
			"half = total // 2", true),
		Entry("toml literal string ending in a backslash", "/repo/config.toml",
			"dir = 'C:\\'  # don't wrap in '''\n# Set the value to one", false),
		Entry("triple quote nested in a python 3.12 f-string field", "/repo/gen.py",
			"s = f\"{\"\"\"nested\"\"\"}\"\n# Set the value to one", false),
		Entry("comment in a multi-line f-string field", "/repo/gen.py",
			"s = f\"\"\"{\n    total  # add tax\n}\"\"\"", false),
		Entry("heading in a triple f-string around a field", "/repo/gen.py",
			"s = f\"\"\"\n## {title}\n\n{body:>{width}}\n\"\"\"", true),
		Entry("quote as a format spec fill character", "/repo/gen.py",
			"s = f\"{x:'>10}\"\n# Set the value to one", false),
		Entry("escaped braces in a triple f-string", "/repo/gen.py",
			"s = f\"\"\"{{\n## Heading\n}}\"\"\"", true),
		Entry("backtick in an unspaced python comment", "/repo/gen.py",
			"x = 1#see `\n# Set the value to one", false),
		Entry("backtick in an unspaced toml comment", "/repo/config.toml",
			"x = 1#see `\n# Set the value to one", false),
		Entry("keyword before a string is not an f-string prefix", "/repo/gen.py",
			"if\"{\" in s:\n    # Set the value to one\n    pass", false),
		Entry("triple quote in a backslash-continued string", "/repo/gen.py",
			"s = 'abc\\\n\"\"\" '\nx = 1  # hidden", false),
		Entry("comment after a backslash-continued string", "/repo/gen.py",
			"s = 'abc\\\ndef'  # explain", false),
		Entry("comment after a continued string in CRLF content", "/repo/gen.py",
			"msg = \"a \\\r\nb\"  # explain\r\n", false),
		Entry("escaped backslash at the end of a closed string", "/repo/gen.py",
			"s = 'abc\\\\'\nx = 1  # explain", false),
		Entry("triple quote in javadoc does not open a string", "/repo/Main.java",
			" * Text blocks start with {@code \"\"\"}.\n// add tax before rounding", false),
		Entry("triple quote in kdoc does not open a string", "/repo/Main.kt",
			"/** Wraps the value in \"\"\" quotes. */\n// add tax before rounding", false),
	)

	It("recognises a python shebang in an extension-less Write", func() {
		ctx.ToolInput.FilePath = "/repo/bin/gen"
		ctx.ToolInput.Content = "#!/usr/bin/env python3\nBODY = \"\"\"\n## Problem\n\"\"\""
		Expect(sv.Validate(context.Background(), ctx).Passed).To(BeTrue())
	})

	It("treats triple quotes as plain quotes in languages without them", func() {
		ctx.ToolInput.FilePath = "/repo/main.go"
		ctx.ToolInput.Content = "s := \"\"\"\n// holds the running total"
		Expect(sv.Validate(context.Background(), ctx).Passed).To(BeFalse())
	})
})
