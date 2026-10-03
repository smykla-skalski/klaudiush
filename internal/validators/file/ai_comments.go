package file

import (
	"cmp"
	"context"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// defaultAICommentPatterns flag a comment that opens with an action verb that
// merely restates the adjacent code — the dominant tell of LLM-generated
// filler. Idiomatic doc comments open with the identifier name and useful
// comments open with a reason ("why"), so neither trips these patterns; only
// the "verb-first narration of what the next line does" style does.
var defaultAICommentPatterns = []string{
	// A comment opening (optionally after "the"/"a") with a verb whose only
	// job is to describe the mechanics of the following statement.
	`(?i)(^\s*|\s)(//|#)\s*(the\s+|a\s+|an\s+)?` +
		`(initiali[sz]|set|reset|get|loop|iterate|check|return|` +
		`creat|mak|build|construct|instantiat|` +
		`call|invok|execut|run|increment|decrement|` +
		`add|append|prepend|insert|push|` +
		`remov|delet|clear|pop|drop|updat|modif|chang|` +
		`handl|process|pars|format|convert|encod|decod|` +
		`serializ|deserializ|marshal|unmarshal|comput|calculat|` +
		`assign|declar|defin|configur|register|setup|` +
		`open|clos|read|writ|load|sav|stor|fetch|send|receiv|` +
		`wait|start|stop|begin|print|log|output|emit|dispatch|` +
		`render|draw|filter|sort|find|search|count|` +
		`validat|verif|ensur|sanitiz|normaliz|` +
		`copy|mov|renam|split|join|trim|replac|compar|` +
		`toggl|enabl|disabl|mark|lock|bind|connect)` +
		`(e|es|ed|d|s|ing|ping|ting|ning|ling|ging|ies|ied|y)?\b`,
	// "This function/method/... does/is/handles/..." restatements.
	`(?i)(^\s*|\s)(//|#)\s*this\s+(function|method|class|struct|interface|type|` +
		`variable|field|constant|value|helper|wrapper|package)\s+` +
		`(does|is|are|will|handles?|returns?|sets?|gets?|` +
		`represents?|holds?|stores?|contains?|provides?)\b`,
}

// aiGenericDocComment matches doc comments that avoid the declaration name and
// instead narrate the declaration kind. These are still filler comments.
var aiGenericDocComment = regexp.MustCompile(
	`(?i)^\s*this\s+(function|method|class|struct|interface|type|` +
		`variable|field|constant|value|helper|wrapper|package)\b`,
)

// aiTodoMarker matches a comment that opens with a task or annotation marker.
// These carry intent ("what still needs doing") rather than restating code and
// are always allowed.
var aiTodoMarker = regexp.MustCompile(
	`(?i)^\s*(TODO|FIXME|HACK|XXX|BUG|WARNING|NOTE|` +
		`OPTIMI[SZ]E|REVIEW|DEPRECATED)\b|^\s*@\w+`,
)

// aiTestPhaseMarker matches BDD-style phase markers commonly used in tests.
// They structure test intent rather than narrating implementation details.
var aiTestPhaseMarker = regexp.MustCompile(
	`(?i)^\s*(given|when|then|arrange|act|assert)\b`,
)

// aiDirectiveMarker matches machine-readable directives and interpreter hints
// that must never be treated as prose: shebangs, build constraints, codegen and
// linter pragmas, and character-encoding cookies. These are load-bearing
// (flagging them would break compilation or tooling), so they are always
// allowed regardless of policy. Matched against the comment body with leading
// whitespace trimmed.
var aiDirectiveMarker = regexp.MustCompile(
	`(?i)^(` +
		`go:` + // Go compiler directives
		`|line\b|export\b` + // cgo //line, //export
		`|\+build\b` + // legacy build tags
		`|nolint\b` + // linter pragma
		`|-\*-|coding[:=]` + // character-encoding cookie
		`|type:|noqa\b|pragma\b` + // Python type/coverage pragmas
		`|(py(lint|right)|mypy|flake8|ruff|isort|fmt):` +
		`|shellcheck\b|swiftlint:` +
		`|eslint-|@ts-|prettier-ignore|biome-ignore|oxlint-|istanbul\b|c8\b|@flow\b` +
		`)`,
)

// aiExceptionToken matches an inline EXC:<CODE>:<reason> escape token, letting a
// genuinely load-bearing comment opt out of the block. Mirrors the exception
// token format used elsewhere (see internal/exceptions).
var aiExceptionToken = regexp.MustCompile(`(?:^|\s)EXC:[A-Z]{2,10}[0-9]{1,5}:\S`)

// nonStrictExtensions are file types whose comments are ordinarily
// human-authored documentation (config, markup, data, shell) rather than inline
// code narration. For these the validator keeps pattern-based behaviour instead
// of the strict block-all policy.
var nonStrictExtensions = map[string]bool{
	".toml": true, ".yaml": true, ".yml": true, ".json": true, ".jsonc": true,
	".json5": true, ".md": true, ".markdown": true, ".mdx": true, ".txt": true,
	".rst": true, ".ini": true, ".cfg": true, ".conf": true, ".config": true,
	".env": true, ".properties": true, ".lock": true, ".csv": true, ".tsv": true,
	".xml": true, ".html": true, ".htm": true, ".svg": true, ".sql": true,
	".mk": true, ".sh": true, ".bash": true, ".zsh": true, ".fish": true,
	".ksh": true, ".ps1": true,
}

// nonStrictBasenames are extension-less files whose comments are documentation.
var nonStrictBasenames = map[string]bool{
	"makefile": true, "gnumakefile": true, "dockerfile": true,
	".gitignore": true, ".dockerignore": true, ".gitattributes": true,
}

// aiDocDecl matches a source line that declares a symbol or package.
// A leading comment block directly above such a line is its documentation and
// is allowed even when it opens with a verb. A tagged struct field counts: code
// generators publish those comments as API documentation, so removing them
// removes the description from the generated schema.
var aiDocDecl = regexp.MustCompile(
	`^\s*(` +
		`package\s+[A-Za-z_]` + // Go package doc
		`|func\s+(\([^)]*\)\s*)?[A-Za-z_]` + // Go func or method
		`|type\s+(\(|[A-Za-z_])` + // Go type or block
		`|(const|var)\s+(\(|[A-Za-z_])` + // Go const/var or block
		`|export\b` + // JS/TS export
		`|(async\s+)?(def|class)\s+[A-Za-z_]` + // Python def/class
		`|[A-Za-z_][\w.]*(\s+[\w*\[\]./]+)?\s+\x60` + // Go struct field with a tag
		`)`,
)

// isShebangOrDocMarker reports whether the comment body (marker stripped, not
// trimmed) is a shebang (#!), a Rust doc comment (///) or a Rust inner doc
// comment (//!). These sit flush against the marker, so a leading space (an
// ordinary comment like "// /tmp/foo") is intentionally not matched.
func isShebangOrDocMarker(body string) bool {
	return len(body) > 0 && (body[0] == '/' || body[0] == '!')
}

// commentBody returns the comment text after the marker at idx (the "//" or "#"
// characters stripped), preserving any leading whitespace.
func commentBody(line string, idx int) string {
	if line[idx] == '#' {
		return line[idx+1:]
	}

	return line[idx+2:]
}

// AICommentValidator flags in-body comments. In strict mode it blocks every
// comment in a source file except task markers, machine directives, doc
// comments, Go test phase markers, and comments carrying an EXC: token. In filler
// mode it blocks only comments matching the configured patterns.
type AICommentValidator struct {
	validator.BaseValidator
	config   *config.AICommentValidatorConfig
	patterns []*regexp.Regexp
}

// NewAICommentValidator creates a new AICommentValidator.
func NewAICommentValidator(
	log logger.Logger,
	cfg *config.AICommentValidatorConfig,
	ruleAdapter validator.RuleChecker,
) *AICommentValidator {
	v := &AICommentValidator{
		BaseValidator: *validator.NewBaseValidatorWithRules("validate-ai-comments", log, ruleAdapter),
		config:        cfg,
	}

	v.patterns = compilePatterns(log, "AI comment", v.getPatterns())

	return v
}

// aiCommentHeader is the message header shown when a filler comment is blocked.
const aiCommentHeader = "Filler comments that only restate the code are not allowed"

// aiCommentStrictHeader is shown when strict mode blocks an in-body comment.
const aiCommentStrictHeader = "Inline comments are not allowed — write self-explanatory code instead.\n" +
	"Allowed: task/annotation markers, non-generic declaration doc comments,\n" +
	"standalone Go *_test.go phase markers, and machine directives. To keep a\n" +
	"comment, append an exception token, e.g.\n" +
	"// EXC:FILE011:documents-a-non-obvious-invariant."

// Validate blocks in-body comments per the configured mode, exempting task
// markers, machine directives, doc comments, Go test phase markers, and comments
// carrying an EXC: token.
func (v *AICommentValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	content := getWriteOrEditContent(hookCtx)
	if content == "" {
		return validator.Pass()
	}

	path := hookCtx.GetFilePath()
	strict := v.strictForPath(path)
	allowTestPhaseMarkers := allowsTestPhaseMarkers(path)

	cov := fileCoverage(hookCtx, false)

	violations := findAICommentViolations(
		content,
		v.patterns,
		strict,
		allowTestPhaseMarkers,
		newCommentScan(hookCtx),
	)
	if len(violations) == 0 {
		return cov.mark(validator.Pass())
	}

	header := aiCommentHeader
	if strict {
		header = aiCommentStrictHeader
	}

	return cov.mark(validator.FailWithRef(
		validator.RefAIComments,
		formatPatternViolations(header, violations),
	))
}

// strictForPath reports whether the strict block-all policy applies to the given
// file. Filler mode disables it, and config/markup/data/shell files always use
// pattern-based behaviour so their ordinary documentation comments are allowed.
func (v *AICommentValidator) strictForPath(path string) bool {
	if v.config == nil || v.config.Mode != config.AICommentModeStrict {
		return false
	}

	base := strings.ToLower(filepath.Base(path))
	if nonStrictBasenames[base] {
		return false
	}

	// filepath.Ext(".env") is "" — treat a leading-dot name with no other dot
	// as its own extension so dotfiles like .env classify correctly.
	ext := strings.ToLower(filepath.Ext(base))
	if ext == "" && strings.HasPrefix(base, ".") {
		ext = base
	}

	return !nonStrictExtensions[ext]
}

// allowsTestPhaseMarkers reports whether BDD-style test phase comments should
// be exempted for the given file.
func allowsTestPhaseMarkers(path string) bool {
	base := strings.ToLower(filepath.Base(path))

	return strings.HasSuffix(base, "_test.go")
}

// findAICommentViolations reports blocked comments. Task markers, machine
// directives, exception tokens, doc comments, and Go test phase markers
// are always exempt. In strict mode every other comment is a violation; in
// filler mode only comments matching a pattern are. An Edit is checked from
// each of its leads, and a line blocked from any lead is reported once.
func findAICommentViolations(
	content string,
	patterns []*regexp.Regexp,
	strict bool,
	allowTestPhaseMarkers bool,
	scan commentScan,
) []violation {
	leads := scan.leads
	if len(leads) == 0 {
		leads = []editLead{{}}
	}

	var violations []violation

	seen := make(map[int]bool)

	for _, lead := range leads {
		for _, v := range findLeadViolations(
			content, patterns, strict, allowTestPhaseMarkers, scan, lead,
		) {
			if !seen[v.line] {
				seen[v.line] = true
				violations = append(violations, v)
			}
		}
	}

	slices.SortStableFunc(violations, func(a, b violation) int {
		return cmp.Compare(a.line, b.line)
	})

	return violations
}

// findLeadViolations is findAICommentViolations for one lead: content is
// scanned from the lead's state, with its prefix on the first line and its
// suffix as context for doc comments.
func findLeadViolations(
	content string,
	patterns []*regexp.Regexp,
	strict bool,
	allowTestPhaseMarkers bool,
	scan commentScan,
	lead editLead,
) []violation {
	var violations []violation

	lines := strings.Split(content, "\n")
	lines[0] = lead.prefix + lines[0]
	docLines := withFollowingSource(lines, lead.suffix)
	metadata := pep723Metadata(lines, docLines, scan, lead)

	state := lead.state

	for i, line := range lines {
		var idx int

		idx, state = findCommentStart(line, state, scan.syntax)
		state = scan.lineStart(state)

		if idx < 0 {
			continue
		}

		body := commentBody(line, idx)

		if aiTodoMarker.MatchString(body) ||
			isShebangOrDocMarker(body) ||
			aiDirectiveMarker.MatchString(strings.TrimLeft(body, " \t")) ||
			aiExceptionToken.MatchString(body) ||
			metadata[i] {
			continue
		}

		if allowTestPhaseMarkers && isFullLineComment(line) &&
			aiTestPhaseMarker.MatchString(body) {
			continue
		}

		if isFullLineComment(line) && precedesDocDecl(docLines, i) &&
			!aiGenericDocComment.MatchString(body) {
			continue
		}

		if strict {
			violations = append(violations, violation{
				line:      i + 1,
				directive: strings.TrimSpace(line[idx:]),
			})

			continue
		}

		for _, pattern := range patterns {
			if match := pattern.FindString(line); match != "" {
				violations = append(violations, violation{
					line:      i + 1,
					directive: strings.TrimSpace(match),
				})

				break
			}
		}
	}

	return violations
}

// pep723Metadata reports which payload lines belong to a PEP 723 metadata
// block in a Python file. An Edit's block is matched against the file text
// around it, so a line edited inside an existing block is still recognised.
func pep723Metadata(lines, docLines []string, scan commentScan, lead editLead) []bool {
	inBlock := make([]bool, len(lines))
	if !scan.syntax.python || scan.metadataTaken || lead.metadataElsewhere {
		return inBlock
	}

	around := make([]string, 0, len(lead.before)+len(docLines))
	around = append(append(around, lead.before...), docLines...)

	copy(inBlock, pep723Lines(around, lead.beforeState, scan)[len(lead.before):])

	return inBlock
}

// maxDocContextLines bounds the source lines after an Edit that are read to
// find the declaration a comment documents.
const maxDocContextLines = 256

// withFollowingSource returns lines with the file text after an Edit's
// old_string appended, so a comment the Edit touches still sees the
// declaration it documents. Only the returned copy holds that text.
func withFollowingSource(lines []string, suffix string) []string {
	if suffix == "" {
		return lines
	}

	rest := strings.Split(suffix, "\n")
	out := make([]string, 0, len(lines)+len(rest)-1)
	out = append(out, lines...)
	out[len(out)-1] += rest[0]

	return append(out, rest[1:]...)
}

// isFullLineComment reports whether the line is a standalone comment rather
// than a trailing (inline) comment; only standalone comments can document a
// declaration.
func isFullLineComment(line string) bool {
	trimmed := strings.TrimSpace(line)

	return strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#")
}

// precedesDocDecl reports whether the comment at index i is part of a leading
// comment block whose first non-comment line declares a symbol or package. A
// blank line breaks the association (it is no longer a doc comment).
func precedesDocDecl(lines []string, i int) bool {
	for j := i + 1; j < len(lines); j++ {
		trimmed := strings.TrimSpace(lines[j])
		if trimmed == "" {
			return false
		}

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}

		return aiDocDecl.MatchString(lines[j])
	}

	return false
}

// getPatterns returns the configured patterns or defaults.
func (v *AICommentValidator) getPatterns() []string {
	if v.config != nil && len(v.config.Patterns) > 0 {
		return v.config.Patterns
	}

	return defaultAICommentPatterns
}

// Category returns the validator category for parallel execution.
// AICommentValidator uses CategoryCPU because it only does pattern matching.
func (*AICommentValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}
