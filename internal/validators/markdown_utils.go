// Package validators provides shared markdown validation utilities
package validators

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"

	"github.com/smykla-labs/klaudiush/pkg/mdtable"
)

const (
	maxTruncateLength = 60

	// consecutiveEmptyLinesToEndList is the number of consecutive empty lines
	// that ends a list context in markdown
	consecutiveEmptyLinesToEndList = 2

	// preambleHeaderLines is the number of lines in the basic preamble header
	preambleHeaderLines = 2

	// minRegexMatches is the minimum number of matches expected from list marker regex
	minRegexMatches = 2

	// maxHeadingLevel is the maximum heading level supported by markdown (h1-h6)
	maxHeadingLevel = 6
)

// ListItemInfo represents information about a single list item in the stack
type ListItemInfo struct {
	MarkerIndent  int    // indentation of the list marker (leading spaces)
	ContentIndent int    // where content should start (after marker + space)
	IsOrdered     bool   // true if this is an ordered list (1. 2. etc.)
	OrderNumber   int    // current number for ordered lists (1, 2, 3...)
	Marker        string // the marker used (-, *, +, or "1.", "2.", etc.)
}

// MarkdownState represents the parsing state at a given position
type MarkdownState struct {
	InCodeBlock bool
	StartLine   int  // 0-indexed line number where this state begins (0 = start of file)
	EndsAtEOF   bool // true if this fragment includes the last line of the file

	// List context tracking for proper indentation validation
	InList        bool // true if currently inside a list structure
	ListIndent    int  // expected indentation level for nested content (0 if not in list)
	ListItemDepth int  // nesting depth of list items (0 = not in list, 1 = top-level, etc.)

	// Detailed list stack for preamble generation
	ListStack []ListItemInfo // stack of list items from outermost to innermost

	// Tracks if there was a blank line before the fragment start
	// (needed for MD032 blanks-around-lists validation)
	HadBlankLineBeforeFragment bool

	// Heading context tracking for MD001 (heading-increment) validation
	// Tracks the last heading level seen before the fragment
	LastHeadingLevel int // 0 = no heading seen, 1-6 = h1-h6
}

// MarkdownAnalysisResult contains markdown validation warnings
type MarkdownAnalysisResult struct {
	Warnings       []string
	TableSuggested map[int]string // Line number -> suggested formatted table
}

// AnalysisOptions contains options for markdown analysis.
type AnalysisOptions struct {
	// CheckTableFormatting enables table formatting validation.
	// Default: true
	CheckTableFormatting bool

	// TableWidthMode controls how table column widths are calculated.
	// Default: mdtable.WidthModeDisplay
	TableWidthMode mdtable.WidthMode
}

// DefaultAnalysisOptions returns the default analysis options.
func DefaultAnalysisOptions() AnalysisOptions {
	return AnalysisOptions{
		CheckTableFormatting: true,
		TableWidthMode:       mdtable.WidthModeDisplay,
	}
}

// listContext tracks the context of a list item for indentation validation
type listContext struct {
	lineNum           int
	indent            int
	sawEmptyLineAfter bool
}

var (
	codeBlockRegex = regexp.MustCompile(`^[[:space:]]*` + "```")
	listItemRegex  = regexp.MustCompile(
		`^[[:space:]]*[-*+][[:space:]]|^[[:space:]]*[0-9]+\.[[:space:]]`,
	)
	orderedListRegex   = regexp.MustCompile(`^([[:space:]]*)([0-9]+)\.[[:space:]]`)
	unorderedListRegex = regexp.MustCompile(`^([[:space:]]*)([*+-])[[:space:]]`)
	headerRegex        = regexp.MustCompile(`^#{1,6}[[:space:]]`)
	commentRegex       = regexp.MustCompile(`^<!--`)
	emptyLineRegex     = regexp.MustCompile(`^[[:space:]]*$`)
)

// getHeadingLevel extracts the heading level from a line (1-6) or 0 if not a heading
func getHeadingLevel(line string) int {
	if !isHeader(line) {
		return 0
	}

	level := 0

	for _, ch := range line {
		if ch != '#' {
			break
		}

		level++
	}

	// Markdown only supports h1-h6
	if level > maxHeadingLevel {
		level = maxHeadingLevel
	}

	return level
}

// parseListMarker extracts list marker information from a line
// Returns: isOrdered, orderNumber (or 0), marker string
func parseListMarker(line string) (bool, int, string) {
	// Try ordered list first
	if matches := orderedListRegex.FindStringSubmatch(line); len(matches) >= minRegexMatches+1 {
		num := 0
		_, _ = fmt.Sscanf(matches[2], "%d", &num)

		return true, num, matches[2] + "."
	}

	// Try unordered list
	if matches := unorderedListRegex.FindStringSubmatch(line); len(matches) >= minRegexMatches+1 {
		return false, 0, matches[2]
	}

	return false, 0, ""
}

// GeneratePreamble creates synthetic markdown content that establishes the correct
// context for a fragment. This allows markdownlint to validate the fragment
// with proper understanding of heading hierarchy, list nesting, and ordering.
//
// Returns the preamble string and the number of lines in the preamble.
func GeneratePreamble(state *MarkdownState) (string, int) {
	if state == nil {
		return "", 0
	}

	// Fragment doesn't start at beginning - need some context
	if state.StartLine > 0 {
		return generatePreambleWithContext(state)
	}

	return "", 0
}

// generatePreambleWithContext creates a preamble for fragments that start mid-file.
// It establishes heading hierarchy, list context, and code block context.
func generatePreambleWithContext(state *MarkdownState) (string, int) {
	var builder strings.Builder

	lineCount := 0

	// Generate heading hierarchy to establish context for MD001
	// If the last heading before fragment was h2, we need h1 → h2
	// If last heading was h3, we need h1 → h2 → h3
	headingLines := generateHeadingHierarchy(&builder, state.LastHeadingLevel)
	lineCount += headingLines

	// If no heading context was established, add a basic h1 for MD041
	if headingLines == 0 {
		builder.WriteString("# Preamble\n\n")

		lineCount += preambleHeaderLines
	}

	// Generate list context if we're in a list
	if state.InList && len(state.ListStack) > 0 {
		listLines := generateListPreamble(&builder, state)

		lineCount += listLines
	}

	// Add a blank line before the fragment if needed for MD032 (blanks-around-lists)
	// However, don't add if we already generated heading hierarchy (which ends with blank)
	// or list context, as that would create consecutive blank lines (MD012)
	if state.HadBlankLineBeforeFragment && headingLines == 0 && !state.InList {
		builder.WriteString("\n")

		lineCount++
	}

	// If fragment starts inside a code block, add opening fence
	// This pairs with the closing fence that may be in the fragment
	if state.InCodeBlock {
		builder.WriteString("```text\n")

		lineCount++
	}

	return builder.String(), lineCount
}

// generateHeadingHierarchy writes the heading hierarchy to establish context.
// Returns the number of lines written.
func generateHeadingHierarchy(builder *strings.Builder, lastHeadingLevel int) int {
	if lastHeadingLevel <= 0 {
		return 0
	}

	lineCount := 0

	// Generate h1 through lastHeadingLevel
	for level := 1; level <= lastHeadingLevel; level++ {
		hashes := strings.Repeat("#", level)
		fmt.Fprintf(builder, "%s Preamble H%d\n\n", hashes, level)

		lineCount += 2 // heading + blank line
	}

	return lineCount
}

// generateListPreamble writes the list context preamble.
// Returns the number of lines written.
func generateListPreamble(builder *strings.Builder, state *MarkdownState) int {
	lineCount := 0

	// Generate list items for each level in the stack
	// We need to establish context for each nesting level
	for i, item := range state.ListStack {
		indent := strings.Repeat(" ", item.MarkerIndent)

		if item.IsOrdered {
			// For ordered lists, generate all preceding items (1 to N-1)
			// plus the current item N to establish context
			// The fragment will continue from or after item N
			for j := 1; j <= item.OrderNumber; j++ {
				fmt.Fprintf(builder, "%s%d. Item %d\n", indent, j, j)

				lineCount++
			}

			// Add blank line if there are more nested levels
			if i < len(state.ListStack)-1 {
				builder.WriteString("\n")

				lineCount++
			}
		} else {
			// For unordered lists, one item establishes context at this level
			fmt.Fprintf(builder, "%s%s Item\n", indent, item.Marker)

			lineCount++

			// Add blank line if there are more nested levels
			if i < len(state.ListStack)-1 {
				builder.WriteString("\n")

				lineCount++
			}
		}
	}

	return lineCount
}

// listTracker manages the list context stack during markdown parsing
type listTracker struct {
	stack                 []ListItemInfo
	consecutiveEmptyLines int
	lastLineWasEmpty      bool
}

// processCodeBlockMarker handles code block markers for list tracking
func (lt *listTracker) processCodeBlockMarker() {
	lt.consecutiveEmptyLines = 0
	lt.lastLineWasEmpty = false
}

// processCodeBlockContent handles content inside code blocks
func (lt *listTracker) processCodeBlockContent() {
	lt.consecutiveEmptyLines = 0
	lt.lastLineWasEmpty = false
}

// processEmptyLine handles empty lines for list tracking
func (lt *listTracker) processEmptyLine() {
	lt.consecutiveEmptyLines++
	lt.lastLineWasEmpty = true

	if lt.consecutiveEmptyLines >= consecutiveEmptyLinesToEndList {
		lt.stack = nil
	}
}

// processListItem handles list item lines
func (lt *listTracker) processListItem(line string) {
	lt.consecutiveEmptyLines = 0
	lt.lastLineWasEmpty = false

	markerIndent := getIndentation(line)
	contentIndent := getListIndent(line)
	isOrdered, orderNum, marker := parseListMarker(line)

	// Pop items from stack where this marker is at or before their content indent
	for len(lt.stack) > 0 && markerIndent < lt.stack[len(lt.stack)-1].ContentIndent {
		lt.stack = lt.stack[:len(lt.stack)-1]
	}

	// Check if this is a continuation of an existing list at the same level
	if len(lt.stack) > 0 {
		lastItem := &lt.stack[len(lt.stack)-1]

		if lastItem.MarkerIndent == markerIndent && lastItem.IsOrdered == isOrdered {
			// Same list, update the order number
			lastItem.OrderNumber = orderNum
			lastItem.Marker = marker

			return
		}
	}

	lt.stack = append(lt.stack, ListItemInfo{
		MarkerIndent:  markerIndent,
		ContentIndent: contentIndent,
		IsOrdered:     isOrdered,
		OrderNumber:   orderNum,
		Marker:        marker,
	})
}

// processNonListContent handles non-list content lines
func (lt *listTracker) processNonListContent(line string) {
	lt.consecutiveEmptyLines = 0
	lt.lastLineWasEmpty = false

	if len(lt.stack) == 0 {
		return
	}

	lineIndent := getIndentation(line)
	lastEntry := lt.stack[len(lt.stack)-1]

	if lineIndent < lastEntry.ContentIndent {
		// Pop stack entries until we find one that contains this indent level
		for len(lt.stack) > 0 {
			entry := lt.stack[len(lt.stack)-1]

			if lineIndent >= entry.ContentIndent {
				break
			}

			lt.stack = lt.stack[:len(lt.stack)-1]
		}
	}
}

// getState returns the current list state
func (lt *listTracker) getState() (
	inList bool,
	indent int,
	depth int,
	stack []ListItemInfo,
	hadBlank bool,
) {
	if len(lt.stack) == 0 {
		return false, 0, 0, nil, lt.lastLineWasEmpty
	}

	// Return a copy of the stack to avoid mutation
	stackCopy := make([]ListItemInfo, len(lt.stack))
	copy(stackCopy, lt.stack)

	lastIndent := lt.stack[len(lt.stack)-1].ContentIndent

	return true, lastIndent, len(lt.stack), stackCopy, lt.lastLineWasEmpty
}

// DetectMarkdownState scans content up to a given line to determine the state.
// This allows fragment validation to start with the correct context.
func DetectMarkdownState(content string, upToLine int) MarkdownState {
	state := MarkdownState{InCodeBlock: false}

	if upToLine <= 0 {
		return state
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	tracker := &listTracker{}

	for scanner.Scan() && lineNum < upToLine {
		line := scanner.Text()
		lineNum++

		// Track code block state
		if isCodeBlockMarker(line) {
			state.InCodeBlock = !state.InCodeBlock

			tracker.processCodeBlockMarker()

			continue
		}

		// Skip tracking inside code blocks
		if state.InCodeBlock {
			tracker.processCodeBlockContent()

			continue
		}

		// Track heading levels (outside code blocks)
		if headingLevel := getHeadingLevel(line); headingLevel > 0 {
			state.LastHeadingLevel = headingLevel
		}

		// Track empty lines
		if isEmptyLine(line) {
			tracker.processEmptyLine()

			continue
		}

		// Process list items or regular content
		if isListItem(line) {
			tracker.processListItem(line)
		} else {
			tracker.processNonListContent(line)
		}
	}

	// Update state with list context
	state.InList, state.ListIndent, state.ListItemDepth, state.ListStack, state.HadBlankLineBeforeFragment = tracker.getState()

	return state
}

// AnalyzeMarkdown performs line-by-line markdown analysis and returns warnings.
// If initialState is provided, it uses that as the starting state (for fragment validation).
// Options can be provided to control table formatting validation.
func AnalyzeMarkdown(
	content string,
	initialState *MarkdownState,
	opts ...AnalysisOptions,
) MarkdownAnalysisResult {
	result := MarkdownAnalysisResult{
		Warnings:       []string{},
		TableSuggested: make(map[int]string),
	}

	if content == "" {
		return result
	}

	// Use provided options or defaults
	options := DefaultAnalysisOptions()
	if len(opts) > 0 {
		options = opts[0]
	}

	// Check for table issues and collect suggestions if enabled
	if options.CheckTableFormatting {
		checkTables(content, &result, options.TableWidthMode)
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	prevLine := ""
	prevPrevLine := ""

	// Use initial state if provided, otherwise start fresh
	inCodeBlock := false
	if initialState != nil {
		inCodeBlock = initialState.InCodeBlock
	}

	var lastList *listContext

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Track list context for indentation validation
		switch {
		case isListItem(line):
			lastList = &listContext{
				lineNum:           lineNum,
				indent:            getListIndent(line),
				sawEmptyLineAfter: false,
			}
		case lastList != nil && isEmptyLine(line):
			lastList.sawEmptyLineAfter = true
		case lastList != nil && !isEmptyLine(line) && !isListItem(line) && !isCodeBlockMarker(line):
			// Reset list context if we encounter non-list, non-empty, non-code content
			lastList = nil
		}

		// Check for code block markers and indentation
		if isCodeBlockMarker(line) {
			checkCodeBlockIndentation(line, lastList, lineNum, &result.Warnings)
			checkMultipleEmptyLinesBeforeCodeBlock(
				prevLine,
				prevPrevLine,
				lineNum,
				&result.Warnings,
			)
			inCodeBlock = checkCodeBlock(line, prevLine, lineNum, inCodeBlock, &result.Warnings)
			// Reset list context after code block
			if !inCodeBlock {
				lastList = nil
			}
		} else {
			inCodeBlock = checkCodeBlock(line, prevLine, lineNum, inCodeBlock, &result.Warnings)
		}

		// Skip header/list validation inside code blocks or on code block markers.
		// For closing markers, this prevents treating the previous line (from inside
		// the code block) as markdown. Opening markers are already handled above.
		skipValidation := inCodeBlock || isCodeBlockMarker(line)

		// Validate header/list spacing for non-code-block content after first line
		if !skipValidation && lineNum > 1 {
			checkListItem(line, prevLine, lineNum, &result.Warnings)
			checkHeader(line, prevLine, lineNum, &result.Warnings)
		}

		prevPrevLine = prevLine
		prevLine = line
	}

	return result
}

// checkCodeBlock checks for code block markers and validates spacing
func checkCodeBlock(line, prevLine string, lineNum int, inCodeBlock bool, warnings *[]string) bool {
	if !isCodeBlockMarker(line) {
		return inCodeBlock
	}

	if !inCodeBlock {
		// Opening code block
		if !isEmptyLine(prevLine) && prevLine != "" {
			*warnings = append(*warnings,
				fmt.Sprintf("⚠️  Line %d: Code block should have empty line before it", lineNum),
				fmt.Sprintf("   Previous line: '%s'", truncate(prevLine)),
			)
		}

		return true
	}

	// Closing code block
	return false
}

// checkCodeBlockIndentation validates code block indentation within list items
func checkCodeBlockIndentation(
	line string,
	lastList *listContext,
	lineNum int,
	warnings *[]string,
) {
	if lastList == nil || !lastList.sawEmptyLineAfter {
		return
	}

	indent := getIndentation(line)

	// If code block has no indentation at all, it's a separate block, not part of the list
	// Only warn if it has some indentation but not enough (partial indentation suggests
	// it was intended to be part of the list)
	if indent > 0 && indent < lastList.indent {
		*warnings = append(
			*warnings,
			fmt.Sprintf(
				"⚠️  Line %d: Code block in list item should be indented by at least %d spaces",
				lineNum,
				lastList.indent,
			),
			fmt.Sprintf(
				"   Found: %d spaces, expected: at least %d spaces",
				indent,
				lastList.indent,
			),
		)
	}
}

// checkMultipleEmptyLinesBeforeCodeBlock validates that there's only one empty line before code blocks
func checkMultipleEmptyLinesBeforeCodeBlock(
	prevLine, prevPrevLine string,
	lineNum int,
	warnings *[]string,
) {
	// Check if we have two consecutive empty lines before the code block
	// lineNum > 3 ensures we have at least 3 lines processed, so prevPrevLine is from actual content
	if lineNum > 3 && isEmptyLine(prevLine) && isEmptyLine(prevPrevLine) {
		*warnings = append(
			*warnings,
			fmt.Sprintf(
				"⚠️  Line %d: Code block should have only one empty line before it, not multiple",
				lineNum,
			),
			"   Found multiple consecutive empty lines before code block",
		)
	}
}

// checkListItem validates list item spacing
func checkListItem(line, prevLine string, lineNum int, warnings *[]string) {
	if !isListItem(line) {
		return
	}

	if shouldWarnAboutListSpacing(prevLine) {
		*warnings = append(*warnings,
			fmt.Sprintf("⚠️  Line %d: First list item should have empty line before it", lineNum),
			fmt.Sprintf("   Previous line: '%s'", truncate(prevLine)),
		)
	}
}

// shouldWarnAboutListSpacing determines if a list item needs spacing before it
func shouldWarnAboutListSpacing(prevLine string) bool {
	return !isEmptyLine(prevLine) &&
		prevLine != "" &&
		!isListItem(prevLine) &&
		!isHeader(prevLine)
}

// checkHeader validates header spacing
func checkHeader(line, prevLine string, lineNum int, warnings *[]string) {
	if !isHeader(prevLine) {
		return
	}

	// Lists are allowed directly after headers
	if shouldWarnAboutHeaderSpacing(line) {
		*warnings = append(*warnings,
			fmt.Sprintf("⚠️  Line %d: Header should have empty line after it", lineNum-1),
			fmt.Sprintf("   Header: '%s'", truncate(prevLine)),
			fmt.Sprintf("   Next line: '%s'", truncate(line)),
		)
	}
}

// shouldWarnAboutHeaderSpacing determines if content after a header needs spacing.
// Per MD022, blank lines are required around ALL headings, including before lists.
func shouldWarnAboutHeaderSpacing(line string) bool {
	return line != "" &&
		!isEmptyLine(line) &&
		!isHeader(line) &&
		!isComment(line)
}

// getListIndent calculates the required indentation for list item content
// For "4. text" → returns 3 (length of "4. ")
// For "- text" → returns 2 (length of "- ")
// For "  - text" → returns 4 (length of "  - ")
func getListIndent(line string) int {
	re := regexp.MustCompile(`^([[:space:]]*)([-*+]|[0-9]+\.)[[:space:]]`)
	matches := re.FindStringSubmatch(line)

	const minRequiredMatches = 3 // Full match + 2 capture groups

	if len(matches) < minRequiredMatches {
		return 0
	}

	leadingSpace := matches[1]
	marker := matches[2]

	return len(leadingSpace) + len(marker) + 1 // +1 for space after marker
}

// getIndentation returns the number of leading spaces in a line
func getIndentation(line string) int {
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			return i
		}
	}

	return len(line)
}

// isCodeBlockMarker checks if line starts a code block
func isCodeBlockMarker(line string) bool {
	return codeBlockRegex.MatchString(line)
}

// isListItem checks if line is a list item
func isListItem(line string) bool {
	return listItemRegex.MatchString(line)
}

// isHeader checks if line is a header
func isHeader(line string) bool {
	return headerRegex.MatchString(line)
}

// isComment checks if line is an HTML comment
func isComment(line string) bool {
	return commentRegex.MatchString(line)
}

// isEmptyLine checks if line is empty or whitespace-only
func isEmptyLine(line string) bool {
	return emptyLineRegex.MatchString(line)
}

// truncate truncates string to maxTruncateLength
func truncate(s string) string {
	if len(s) <= maxTruncateLength {
		return s
	}

	return s[:maxTruncateLength]
}

// checkTables parses markdown tables and checks for formatting issues.
// When issues are found, it adds warnings and suggests properly formatted tables.
func checkTables(content string, result *MarkdownAnalysisResult, widthMode mdtable.WidthMode) {
	parseResult := mdtable.Parse(content)

	for _, table := range parseResult.Tables {
		// Check if the table needs reformatting by comparing original vs formatted
		formatted := mdtable.FormatTableWithMode(&table, widthMode)
		original := strings.Join(table.RawLines, "\n") + "\n"

		if formatted != original {
			msg := fmt.Sprintf(
				"Line %d: Markdown table has formatting issues (use consistent column widths)",
				table.StartLine,
			)
			result.Warnings = append(result.Warnings, msg)
			result.TableSuggested[table.StartLine] = formatted
		}
	}

	// Add any specific issues from parsing
	for _, issue := range parseResult.Issues {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Line %d: %s", issue.Line, issue.Message),
		)
	}
}
