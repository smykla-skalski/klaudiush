package git

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	locationTitle   = "title"
	locationMessage = "message"
	locationCommand = "command arguments outside the message"
)

// RuleResult contains the result of a rule validation including reference.
type RuleResult struct {
	// Message is the primary error (no emoji, no indentation).
	Message string

	// Context contains supplementary lines (no emoji, no indentation).
	Context []string

	// Reference is the URL that uniquely identifies this type of validation failure.
	Reference validator.Reference

	// Findings lists each violation the rule found, with its repair.
	Findings []validator.Finding
}

// CommitRule represents a validation rule for commit messages.
type CommitRule interface {
	// Name returns the rule name.
	Name() string

	// Validate checks the commit against the rule and returns a RuleResult.
	Validate(commit *ParsedCommit, message string) *RuleResult
}

// TitleLengthRule validates the commit title length.
type TitleLengthRule struct {
	MaxLength                 int
	AllowUnlimitedRevertTitle bool
}

func (*TitleLengthRule) Name() string {
	return "title-length"
}

func (r *TitleLengthRule) Validate(commit *ParsedCommit, _ string) *RuleResult {
	// Skip length validation for revert commits if configured
	if r.AllowUnlimitedRevertTitle && isRevertCommit(commit.Title) {
		return nil
	}

	// Use rune count to properly handle Unicode characters
	titleLength := len([]rune(commit.Title))
	if titleLength <= r.MaxLength {
		return nil
	}

	return &RuleResult{
		Reference: validator.RefGitBadTitle,
		Message: fmt.Sprintf(
			"Title exceeds %d characters (%d chars): '%s'",
			r.MaxLength,
			titleLength,
			commit.Title,
		),
		Context: []string{
			"type(scope): prefix counts toward the limit",
			"Revert commits are exempt",
		},
		Findings: []validator.Finding{
			{
				Reference: validator.RefGitBadTitle,
				Location:  locationTitle,
				Message:   fmt.Sprintf("Title is %d characters long", titleLength),
				Actual:    commit.Title,
				Required: fmt.Sprintf(
					"at most %d characters including any type(scope): prefix",
					r.MaxLength,
				),
				Repair: fmt.Sprintf(
					"Shorten the title to %d characters or fewer (%d over)",
					r.MaxLength,
					titleLength-r.MaxLength,
				),
			},
		},
	}
}

// ConventionalFormatRule validates conventional commit format.
type ConventionalFormatRule struct {
	ValidTypes   []string
	RequireScope bool

	// TitleMaxLength is the effective title limit quoted in guidance; zero
	// means the default.
	TitleMaxLength int
}

func (*ConventionalFormatRule) Name() string {
	return "conventional-format"
}

func (r *ConventionalFormatRule) Validate(commit *ParsedCommit, _ string) *RuleResult {
	// Skip validation for revert commits
	if isRevertCommit(commit.Title) {
		return nil
	}

	invalid := !commit.Valid || commit.ParseError != ""
	if !invalid && (!r.RequireScope || commit.Scope != "") {
		return nil
	}

	maxLength := r.titleMaxLength()

	ctx := []string{}

	if r.RequireScope {
		ctx = append(ctx, "Scope is mandatory")
	}

	ctx = append(ctx,
		"Valid types: "+strings.Join(r.ValidTypes, ", "),
		"Alternative: Revert \"original commit title\"",
		fmt.Sprintf("Current title: '%s'", commit.Title),
		fmt.Sprintf("type(scope): prefix counts toward %d-char limit", maxLength),
	)

	return &RuleResult{
		Reference: validator.RefGitConventionalCommit,
		Message:   "Title doesn't follow conventional commits format: type(scope): description",
		Context:   ctx,
		Findings:  []validator.Finding{r.finding(commit, invalid, maxLength)},
	}
}

func (r *ConventionalFormatRule) titleMaxLength() int {
	if r.TitleMaxLength > 0 {
		return r.TitleMaxLength
	}

	return config.DefaultTitleMaxLength
}

func (r *ConventionalFormatRule) finding(
	commit *ParsedCommit,
	invalid bool,
	maxLength int,
) validator.Finding {
	form := "type(scope): description"
	if !r.RequireScope {
		form = "type(scope): description or type: description"
	}

	message := "Title is not in conventional commits format"
	if !invalid {
		message = "Title has no scope"
	}

	return validator.Finding{
		Reference: validator.RefGitConventionalCommit,
		Location:  locationTitle,
		Message:   message,
		Actual:    commit.Title,
		Required: fmt.Sprintf(
			"%s with type one of: %s",
			form,
			strings.Join(r.ValidTypes, ", "),
		),
		Repair: fmt.Sprintf(
			"Rewrite the title as %s, keeping it within %d characters",
			form,
			maxLength,
		),
	}
}

// scopeOnlyTitleRegex matches "scope: description" format used by projects like home-manager.
// The scope can be any lowercase identifier with optional path separators (/, -, .).
var scopeOnlyTitleRegex = regexp.MustCompile(`^[a-z][a-z0-9./_-]*: .+`)

// ScopeOnlyFormatRule validates "scope: description" commit titles (no type prefix).
// This matches the convention used by home-manager, linux kernel patches, and similar
// projects where the scope is a module or file path, not a semantic type like feat/fix.
type ScopeOnlyFormatRule struct{}

func (*ScopeOnlyFormatRule) Name() string {
	return "scope-only-format"
}

func (*ScopeOnlyFormatRule) Validate(commit *ParsedCommit, _ string) *RuleResult {
	if isRevertCommit(commit.Title) {
		return nil
	}

	if scopeOnlyTitleRegex.MatchString(commit.Title) {
		return nil
	}

	return &RuleResult{
		Reference: validator.RefGitConventionalCommit,
		Message:   "Title doesn't follow scope-only format: scope: description",
		Context: []string{
			"Scope must start with a lowercase letter (a-z)",
			"Valid characters in scope: letters, digits, '.', '/', '_', '-'",
			"Examples: 'home-environment: use nix profile', 'modules/systemd: add unit'",
			fmt.Sprintf("Current title: '%s'", commit.Title),
		},
		Findings: []validator.Finding{{
			Reference: validator.RefGitConventionalCommit,
			Location:  locationTitle,
			Message:   "Title is not in scope-only format",
			Actual:    commit.Title,
			Required:  "scope: description, scope in lowercase letters, digits, '.', '/', '_', '-'",
			Repair:    "Rewrite the title as scope: description, e.g. 'modules/systemd: add unit'",
		}},
	}
}

// CustomPatternRule validates commit titles against a user-supplied regex.
type CustomPatternRule struct {
	Pattern *regexp.Regexp
}

// NewCustomPatternRule creates a CustomPatternRule from a regex string.
// Panics if the pattern is invalid (callers should validate first).
func NewCustomPatternRule(pattern string) *CustomPatternRule {
	return &CustomPatternRule{Pattern: regexp.MustCompile(pattern)}
}

func (*CustomPatternRule) Name() string {
	return "custom-pattern"
}

func (r *CustomPatternRule) Validate(commit *ParsedCommit, _ string) *RuleResult {
	if isRevertCommit(commit.Title) {
		return nil
	}

	if r.Pattern.MatchString(commit.Title) {
		return nil
	}

	return &RuleResult{
		Reference: validator.RefGitConventionalCommit,
		Message:   "Title doesn't match the required pattern",
		Context: []string{
			"Pattern: " + r.Pattern.String(),
			fmt.Sprintf("Current title: '%s'", commit.Title),
		},
		Findings: []validator.Finding{{
			Reference: validator.RefGitConventionalCommit,
			Location:  locationTitle,
			Message:   "Title does not match the configured pattern",
			Actual:    commit.Title,
			Required:  "matches " + r.Pattern.String(),
			Repair:    "Rewrite the title so it matches " + r.Pattern.String(),
		}},
	}
}

// InfraScopeMisuseRule blocks feat/fix with infrastructure scopes.
type InfraScopeMisuseRule struct {
	infraScopeMisuseRegex *regexp.Regexp
}

func NewInfraScopeMisuseRule() *InfraScopeMisuseRule {
	return &InfraScopeMisuseRule{
		infraScopeMisuseRegex: regexp.MustCompile(`^(feat|fix)\((ci|test|docs|build)\):`),
	}
}

func (*InfraScopeMisuseRule) Name() string {
	return "infra-scope-misuse"
}

func (r *InfraScopeMisuseRule) Validate(commit *ParsedCommit, _ string) *RuleResult {
	if !r.infraScopeMisuseRegex.MatchString(commit.Title) {
		return nil
	}

	matches := r.infraScopeMisuseRegex.FindStringSubmatch(commit.Title)

	const minMatchGroups = 3 // Full match + type + scope groups

	if len(matches) < minMatchGroups {
		return nil
	}

	typeMatch := matches[1]  // feat or fix
	scopeMatch := matches[2] // ci, test, docs, or build

	return &RuleResult{
		Reference: validator.RefGitFeatCI,
		Message: fmt.Sprintf(
			"Use '%s(...)' not '%s(%s)' for infrastructure changes",
			scopeMatch,
			typeMatch,
			scopeMatch,
		),
		Context: []string{
			"feat/fix should only be used for user-facing changes",
		},
		Findings: []validator.Finding{{
			Reference: validator.RefGitFeatCI,
			Location:  locationTitle,
			Message:   "feat/fix used for an infrastructure scope",
			Actual:    typeMatch + "(" + scopeMatch + ")",
			Required:  scopeMatch + "(...) for " + scopeMatch + " changes",
			Repair: fmt.Sprintf(
				"Replace '%s(%s):' with '%s(<scope>):'",
				typeMatch,
				scopeMatch,
				scopeMatch,
			),
		}},
	}
}

// BodyLineLengthRule validates body line lengths.
type BodyLineLengthRule struct {
	MaxLength int
	Tolerance int
	urlRegex  *regexp.Regexp
}

func NewBodyLineLengthRule(maxLength, tolerance int) *BodyLineLengthRule {
	return &BodyLineLengthRule{
		MaxLength: maxLength,
		Tolerance: tolerance,
		urlRegex:  regexp.MustCompile(`https?://`),
	}
}

func (*BodyLineLengthRule) Name() string {
	return "body-line-length"
}

func (r *BodyLineLengthRule) Validate(_ *ParsedCommit, commitMsg string) *RuleResult {
	lines := strings.Split(commitMsg, "\n")
	maxLenWithTolerance := r.MaxLength + r.Tolerance

	var primary string

	ctx := make([]string, 0)
	findings := make([]validator.Finding, 0)

	for lineNum, line := range lines {
		// Skip title (first line)
		if lineNum == 0 {
			continue
		}

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Allow URLs to break the rule
		if r.urlRegex.MatchString(line) {
			continue
		}

		lineLen := utf8.RuneCountInString(line)
		if lineLen > maxLenWithTolerance {
			truncated := truncateLine(line)
			msg := fmt.Sprintf(
				"Line %d exceeds %d characters (%d chars, >%d over limit)",
				lineNum+1,
				r.MaxLength,
				lineLen,
				r.Tolerance,
			)

			if primary == "" {
				primary = msg

				ctx = append(ctx, fmt.Sprintf("Line: '%s'", truncated))
			} else {
				ctx = append(ctx, msg, fmt.Sprintf("Line: '%s'", truncated))
			}

			findings = append(findings, r.finding(lineNum+1, lineLen, line))
		}
	}

	if primary == "" {
		return nil
	}

	return &RuleResult{
		Reference: validator.RefGitBadBody,
		Message:   primary,
		Context:   ctx,
		Findings:  findings,
	}
}

func (r *BodyLineLengthRule) finding(lineNum, lineLen int, line string) validator.Finding {
	required := fmt.Sprintf("at most %d characters per body line", r.MaxLength)
	if r.Tolerance > 0 {
		required = fmt.Sprintf(
			"at most %d characters per body line (up to %d tolerated)",
			r.MaxLength,
			r.MaxLength+r.Tolerance,
		)
	}

	return validator.Finding{
		Reference: validator.RefGitBadBody,
		Location:  fmt.Sprintf("message line %d", lineNum),
		Message:   fmt.Sprintf("Body line is %d characters long", lineLen),
		Actual:    line,
		Required:  required,
		Repair:    fmt.Sprintf("Wrap line %d at %d characters", lineNum, r.MaxLength),
	}
}

// ListFormattingRule validates list item formatting.
type ListFormattingRule struct {
	listItemRegex *regexp.Regexp
	trailerRegex  *regexp.Regexp
}

func NewListFormattingRule() *ListFormattingRule {
	return &ListFormattingRule{
		listItemRegex: regexp.MustCompile(`^\s*[-*]\s+|^\s*[0-9]+\.\s+`),
		trailerRegex:  regexp.MustCompile(`^[A-Za-z][-A-Za-z0-9 ]*:\s`),
	}
}

func (*ListFormattingRule) Name() string {
	return "list-formatting"
}

func (r *ListFormattingRule) Validate(_ *ParsedCommit, message string) *RuleResult {
	lines := strings.Split(message, "\n")
	prevLineEmpty := false
	foundFirstList := false

	for lineNum, line := range lines {
		// Skip title (first line)
		if lineNum == 0 {
			continue
		}

		// Check if blank line
		if strings.TrimSpace(line) == "" {
			prevLineEmpty = true

			continue
		}

		// Skip git trailer lines (Signed-off-by:, Co-authored-by:, etc.)
		if r.trailerRegex.MatchString(line) {
			continue
		}

		// Check for list items
		if r.listItemRegex.MatchString(line) {
			// Check if this is the first list item and there was no empty line before it
			if !foundFirstList && !prevLineEmpty {
				truncated := truncateLine(line)

				return &RuleResult{
					Reference: validator.RefGitListFormat,
					Message: fmt.Sprintf(
						"Missing empty line before first list item at line %d",
						lineNum+1,
					),
					Context: []string{
						"List items must be preceded by an empty line",
						fmt.Sprintf("Line: '%s'", truncated),
					},
					Findings: []validator.Finding{{
						Reference: validator.RefGitListFormat,
						Location:  fmt.Sprintf("message line %d", lineNum+1),
						Message:   "List starts without an empty line before it",
						Actual:    line,
						Required:  "an empty line before the first list item",
						Repair:    fmt.Sprintf("Insert an empty line before line %d", lineNum+1),
					}},
				}
			}

			foundFirstList = true
		}

		prevLineEmpty = false
	}

	return nil
}

// PRReferenceRule blocks PR references in commit messages.
type PRReferenceRule struct {
	prReferenceRegex *regexp.Regexp
	hashRefRegex     *regexp.Regexp
	urlRefRegex      *regexp.Regexp
}

func NewPRReferenceRule() *PRReferenceRule {
	return &PRReferenceRule{
		prReferenceRegex: regexp.MustCompile(
			`#[0-9]{1,10}\b|(?:^|://|[^/a-zA-Z0-9])github\.com/[^/]+/[^/]+/pull/[0-9]{1,10}\b`,
		),
		hashRefRegex: regexp.MustCompile(`#[0-9]{1,10}\b`),
		urlRefRegex: regexp.MustCompile(
			`(?:^|://|[^/a-zA-Z0-9])github\.com/[^/]+/[^/]+/pull/[0-9]{1,10}\b`,
		),
	}
}

func (*PRReferenceRule) Name() string {
	return "pr-reference"
}

func (r *PRReferenceRule) Validate(_ *ParsedCommit, message string) *RuleResult {
	if !r.prReferenceRegex.MatchString(message) {
		return nil
	}

	ctx := make([]string, 0)
	findings := make([]validator.Finding, 0)

	// Show examples for hash references
	if hashMatch := r.hashRefRegex.FindString(message); hashMatch != "" {
		fix := strings.TrimPrefix(hashMatch, "#")
		ctx = append(ctx, fmt.Sprintf("Found: '%s' -> Should be: '%s'", hashMatch, fix))
	}

	for _, hashMatch := range r.hashRefRegex.FindAllString(message, -1) {
		findings = append(findings, prRefFinding(hashMatch, strings.TrimPrefix(hashMatch, "#")))
	}

	// Show examples for URL references
	if urlMatch := r.urlRefRegex.FindString(message); urlMatch != "" {
		prNumRegex := regexp.MustCompile(`[0-9]{1,10}$`)
		prNum := prNumRegex.FindString(urlMatch)

		// Strip any prefix captured by the anchor pattern (e.g., "://", space, etc.)
		cleanURL := urlMatch
		if idx := strings.Index(urlMatch, "github.com"); idx > 0 {
			cleanURL = urlMatch[idx:]
		}

		ctx = append(
			ctx,
			fmt.Sprintf("Found: 'https://%s' -> Should be: '%s'", cleanURL, prNum),
		)
	}

	for _, loc := range r.urlRefRegex.FindAllStringIndex(message, -1) {
		found := prURLAt(message, loc[0], loc[1])
		findings = append(findings, prRefFinding(found, prNumberRegex.FindString(found)))
	}

	return &RuleResult{
		Reference: validator.RefGitPRRef,
		Message:   "PR references found - remove '#' prefix or convert URLs to plain numbers",
		Context:   ctx,
		Findings:  findings,
	}
}

var prNumberRegex = regexp.MustCompile(`[0-9]{1,10}$`)

// prURLAt returns the pull request URL as written in the message, scheme
// included, for a match of urlRefRegex that may carry one leading anchor
// character.
func prURLAt(message string, start, end int) string {
	host := start + strings.Index(message[start:end], "github.com")

	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasSuffix(message[:host], scheme) {
			return message[host-len(scheme) : end]
		}
	}

	return message[host:end]
}

func prRefFinding(found, replacement string) validator.Finding {
	return validator.Finding{
		Reference: validator.RefGitPRRef,
		Location:  locationMessage,
		Message:   "PR reference in commit message",
		Actual:    found,
		Required:  "no '#' references or pull request URLs",
		Repair:    fmt.Sprintf("Replace '%s' with '%s'", found, replacement),
	}
}

// AIAttributionRule blocks AI attribution patterns.
type AIAttributionRule struct{}

func NewAIAttributionRule() *AIAttributionRule {
	return &AIAttributionRule{}
}

func (*AIAttributionRule) Name() string {
	return "ai-attribution"
}

// aiAttributionResult builds the shared GIT012 failure. subject names what
// carried the attribution, since the same rule guards commit messages, merge
// bodies and pull request descriptions.
func aiAttributionResult(text, subject string) *validator.Result {
	if !containsAIAttribution(text) {
		return nil
	}

	return validator.FailWithRef(
		validator.RefGitClaudeAttr,
		subject+" contains AI attribution - remove any AI generation attribution",
	).AddFinding(aiAttributionFinding())
}

func aiAttributionFinding() validator.Finding {
	return validator.Finding{
		Reference: validator.RefGitClaudeAttr,
		Location:  locationMessage,
		Message:   "AI attribution found",
		Required:  "no AI generation credit, co-author trailer or session link",
		Repair:    "Delete the line that credits an AI assistant",
	}
}

func commandAttributionFinding() validator.Finding {
	return validator.Finding{
		Reference: validator.RefGitClaudeAttr,
		Location:  locationCommand,
		Message:   "AI attribution in a --trailer, extra -m or other argument",
		Required:  "no AI generation credit in any command argument",
		Repair:    "Remove the AI credit from every --trailer and extra message argument",
	}
}

func (*AIAttributionRule) Validate(_ *ParsedCommit, message string) *RuleResult {
	if !containsAIAttribution(message) {
		return nil
	}

	return &RuleResult{
		Reference: validator.RefGitClaudeAttr,
		Message:   "Commit message contains AI attribution - remove any AI generation attribution",
		Findings:  []validator.Finding{aiAttributionFinding()},
	}
}

// ForbiddenPatternRule blocks forbidden patterns in commit messages.
type ForbiddenPatternRule struct {
	Patterns []string
}

func (*ForbiddenPatternRule) Name() string {
	return "forbidden-pattern"
}

func (r *ForbiddenPatternRule) Validate(_ *ParsedCommit, message string) *RuleResult {
	if len(r.Patterns) == 0 {
		return nil
	}

	var primary string

	ctx := make([]string, 0)
	findings := make([]validator.Finding, 0)

	for _, pattern := range r.Patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}

		if re.MatchString(message) {
			match := re.FindString(message)
			msg := fmt.Sprintf("Forbidden pattern found: '%s'", match)

			if primary == "" {
				primary = msg

				ctx = append(ctx, "Pattern: "+pattern)
			} else {
				ctx = append(ctx, msg, "Pattern: "+pattern)
			}

			findings = append(findings, validator.Finding{
				Reference: validator.RefGitForbiddenPattern,
				Location:  locationMessage,
				Message:   "Forbidden pattern found",
				Actual:    match,
				Required:  "no match for " + pattern,
				Repair:    fmt.Sprintf("Remove or reword '%s'", match),
			})
		}
	}

	if primary == "" {
		return nil
	}

	return &RuleResult{
		Reference: validator.RefGitForbiddenPattern,
		Message:   primary,
		Context:   ctx,
		Findings:  findings,
	}
}

// SignoffRule validates the Signed-off-by trailer.
type SignoffRule struct {
	ExpectedSignoff string
}

func (*SignoffRule) Name() string {
	return "signoff"
}

func (r *SignoffRule) Validate(_ *ParsedCommit, message string) *RuleResult {
	if r.ExpectedSignoff == "" {
		return nil
	}

	if !strings.Contains(message, "Signed-off-by:") {
		return nil
	}

	lines := strings.Split(message, "\n")
	signoffLine := ""

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Signed-off-by:") {
			signoffLine = strings.TrimSpace(line)

			break
		}
	}

	expectedSignoffLine := "Signed-off-by: " + r.ExpectedSignoff
	if signoffLine != expectedSignoffLine {
		return &RuleResult{
			Reference: validator.RefGitSignoffMismatch,
			Message:   "Wrong signoff identity",
			Context: []string{
				"Found: " + signoffLine,
				"Expected: " + expectedSignoffLine,
			},
			Findings: []validator.Finding{{
				Reference: validator.RefGitSignoffMismatch,
				Location:  "Signed-off-by trailer",
				Message:   "Signoff identity does not match",
				Actual:    signoffLine,
				Required:  expectedSignoffLine,
				Repair:    "Replace the trailer with '" + expectedSignoffLine + "'",
			}},
		}
	}

	return nil
}

var aiAssistantNames = []string{
	"claude",
	"copilot",
	"codex",
}

// aiAttributionMarkers are the words that turn a mention of an AI assistant
// into attribution: they assign credit rather than name a tool. The robot emoji
// is included because every generated footer carries one.
var aiAttributionMarkers = []string{
	"generated",
	"created",
	"written",
	"authored",
	"assisted",
	"made with",
	"made by",
	"built with",
	"built by",
	"built using",
	"powered by",
	"with help from",
	"\U0001F916",
}

// aiOnlyLinkPattern matches a line that is nothing but a link, markdown or
// bare, optionally led by an emoji or bullet. A footer often puts its link on
// its own line with the credit wording above it, and a line that is only a link
// to an assistant is never prose citing one.
var aiOnlyLinkPattern = regexp.MustCompile(`^\W*(?:\[[^\]]*\]\()?https?://[^\s)]+\)?\W*$`)

// aiVendorEmailPattern matches an assistant's own address, as a generated
// Co-authored-by trailer carries. Only the address form counts: a link to the
// same vendor's documentation is not attribution.
var aiVendorEmailPattern = regexp.MustCompile(`@(?:[\w.-]+\.)?(?:anthropic|openai)\.com`)

// confusableLetters maps the Cyrillic letters that look like the Latin ones in
// "claude", "copilot" and "codex" onto their Latin twins, so a homoglyph
// spelling is read as the name it imitates.
var confusableLetters = strings.NewReplacer(
	"\u0430", "a", // а
	"\u0435", "e", // е
	"\u043e", "o", // о
	"\u0441", "c", // с
	"\u0440", "p", // р
	"\u0445", "x", // х
	"\u0443", "y", // у
	"\u0456", "i", // і
	"\u0455", "s", // ѕ
	"\u0501", "d", // ԁ
	"\u04cf", "l", // ӏ
)

// aiSessionLinkPattern matches an assistant session URL, such as
// "https://claude.ai/code/session_01ABC". A session link only ever appears in a
// generated footer, so - unlike a product or documentation link, which a commit
// may legitimately cite - it is attribution on its own.
var aiSessionLinkPattern = regexp.MustCompile(`claude\.ai/code/session[_/]`)

// legitimateAIReferences name things that carry an assistant's name without
// crediting it - the guidance file, the hook tools - so their "claude" is not
// read as an assistant mention.
var legitimateAIReferences = []string{
	"claude.md",
	"claude-hooks",
	"klaudiush",
}

// Path words are built from these pieces. A path takes only the ASCII
// characters paths are spelled with, so it ends at punctuation, emoji,
// invisible characters and Unicode spaces and never swallows a footer glued to
// it. An escaped space ("My\ Dir") may only sit in a directory, never in the
// last segment, so a footer behind "\ " is not taken as part of a path.
const (
	pathBoundary = `(^|[\s\p{Z}"'\x60=;&|()<>@])`
	pathChar     = `[\w.+%@~-]`
	pathDirs     = `(?:` + pathChar + `+(?:\\ ` + pathChar + `+)*/)*`
	pathTail     = `(?:` + pathChar + `+)?`
)

// anchoredPathPattern matches a path word rooted at "/": absolute,
// home-relative, variable-rooted (including "${VAR:-/tmp}"), dot-relative,
// under a dot directory such as .claude/, a file:// URL, or glued to a short
// flag as in "-C/path". It may stand alone or follow "=", "@", a redirect or a
// code span. A word opening with "//" stops at the first slash and a link keeps
// its scheme or host in front of the slash, so links never match.
var anchoredPathPattern = regexp.MustCompile(
	pathBoundary +
		`(?:-[a-zA-Z])?` +
		`(?:file://|~|\$\{[^}\s]*\}|\$\w+|\.\.?|\.[\w-]+)?` +
		`/` + pathDirs + pathTail,
)

// withoutPaths removes the anchored filesystem path words from lowercased
// text, keeping the character before each so the surrounding words stay apart.
// Checks run line by line, so an assistant name in a temp dir or agent
// worktree path would otherwise pair with an ordinary word such as "written"
// on the same line. A word whose first directory is an assistant's own name
// ("/claude", "./claude/opus") reads as the name, not a path, so it is kept.
// Relative paths are ambiguous in prose ("w/Claude", "Cursor/Copilot"), so
// only withoutPathArguments drops them, and only where the parsed command uses
// them as arguments.
func withoutPaths(text string) string {
	return anchoredPathPattern.ReplaceAllStringFunc(text, func(match string) string {
		boundary, word := splitBoundary(match)
		if namesAssistantFirst(word) {
			return match
		}

		return boundary + markersIn(word)
	})
}

// namesAssistantFirst reports whether the first segment after a path word's
// leading slash is an assistant name, version or product suffix removed
// ("claude", "claude-code", "codex-cli").
func namesAssistantFirst(word string) bool {
	_, rest, _ := strings.Cut(word, "/")
	first, _, _ := strings.Cut(rest, "/")
	first = strings.TrimSuffix(
		strings.TrimSuffix(strings.TrimRight(first, "-.0123456789"), "-code"),
		"-cli",
	)

	return slices.Contains(aiAssistantNames, first)
}

// markersIn returns the attribution markers in the last segment of a removed
// path word, space-separated, so a footer glued to a path ("/x" + "Generated
// by ...") keeps its marker while the path, and any name in it, goes. Earlier
// segments are directories, whose names are not credit.
func markersIn(word string) string {
	tail := word[strings.LastIndex(word, "/")+1:]

	var found []string

	for _, marker := range aiAttributionMarkers {
		if strings.Contains(tail, marker) {
			found = append(found, marker)
		}
	}

	if len(found) == 0 {
		return ""
	}

	return " " + strings.Join(found, " ") + " "
}

// splitBoundary separates the leading boundary character a path match keeps
// from the path word itself.
func splitBoundary(match string) (boundary, word string) {
	r, size := utf8.DecodeRuneInString(match)
	if r == utf8.RuneError || size == 0 {
		return "", match
	}

	if strings.ContainsRune(`"'=;&|()<>@`+"`", r) || unicode.IsSpace(r) ||
		unicode.Is(unicode.Z, r) {
		return match[:size], match[size:]
	}

	return "", match
}

// containsAIAttribution reports whether a message credits an AI assistant. It
// works line by line: a bare mention is not attribution - "we should try Claude
// Code" is a sentence about a tool - so an assistant name counts only next to a
// credit marker ("generated", "co-authored by", the robot emoji) or a link to
// an assistant product, which is what every generated footer carries.
func containsAIAttribution(message string) bool {
	lower := confusableLetters.Replace(
		strings.ReplaceAll(strings.ToLower(message), `\n`, "\n"),
	)

	// A session link is attribution wherever it sits, even spelled like a
	// path, so it is matched before path words are dropped.
	if aiSessionLinkPattern.MatchString(lower) {
		return true
	}

	lower = withoutPaths(lower)

	// Every form of attribution names an assistant, so text that names none -
	// the overwhelmingly common case, and now up to a megabyte of body file -
	// is rejected in one pass instead of per line.
	if !containsAIAssistantName(lower) {
		return false
	}

	for line := range strings.SplitSeq(lower, "\n") {
		// Stripping allocates, so it only runs on a line that names an
		// assistant at all; stripping can only remove a name, never add one.
		if !containsAIAssistantName(line) {
			continue
		}

		if lineCreditsAIAssistant(stripLegitimateAIReferences(line)) {
			return true
		}
	}

	// A command line carries its arguments inline, so a footer that stands
	// alone in a commit body sits mid-line here, behind "--body ". Quotes
	// delimit those arguments, so they bound the standalone-footer test too.
	for chunk := range strings.SplitSeq(argumentQuotes.Replace(lower), "\n") {
		chunk = stripLegitimateAIReferences(strings.TrimSpace(chunk))
		if containsAIAssistantName(chunk) && aiOnlyLinkPattern.MatchString(chunk) {
			return true
		}
	}

	return false
}

// argumentQuotes turns the quotes that delimit a command argument into line
// breaks, so an argument's text is bounded the way a body line is.
var argumentQuotes = strings.NewReplacer(`"`, "\n", `'`, "\n")

// stripLegitimateAIReferences removes the allow-listed identifiers from a
// lowercased line. Removing them rather than skipping the whole line keeps a
// real footer from slipping through just because the line also says klaudiush.
func stripLegitimateAIReferences(lower string) string {
	for _, reference := range legitimateAIReferences {
		lower = strings.ReplaceAll(lower, reference, "")
	}

	return lower
}

// lineCreditsAIAssistant reports whether one lowercased, allow-list-stripped
// line credits an AI assistant.
func lineCreditsAIAssistant(line string) bool {
	if !containsAIAssistantName(line) {
		return false
	}

	// Checked after the name so the regexes only run on a line that could
	// match: every session link carries the assistant's name in its host.
	if aiSessionLinkPattern.MatchString(line) ||
		aiOnlyLinkPattern.MatchString(strings.TrimSpace(line)) ||
		aiVendorEmailPattern.MatchString(line) {
		return true
	}

	for _, marker := range aiAttributionMarkers {
		if strings.Contains(line, marker) {
			return true
		}
	}

	for _, assistant := range aiAssistantNames {
		if strings.Contains(line, assistant+" ai") {
			return true
		}
	}

	// "Co-authored-by:" is spelled with a hyphen, so the "authored" marker above
	// does not catch it.
	return strings.Contains(line, "co-authored")
}

func containsAIAssistantName(lower string) bool {
	for _, assistant := range aiAssistantNames {
		if strings.Contains(lower, assistant) {
			return true
		}
	}

	return false
}
