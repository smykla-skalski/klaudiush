package mdtable

import (
	"regexp"
	"strings"
)

// ParsedTable represents a parsed markdown table with its location in content.
type ParsedTable struct {
	StartLine  int        // 1-indexed line number where the table starts
	EndLine    int        // 1-indexed line number where the table ends
	Headers    []string   // Parsed header values
	Rows       [][]string // Parsed data rows
	Alignments []Alignment
	RawLines   []string // Original raw lines of the table
}

// TableIssue represents an issue found in a markdown table.
type TableIssue struct {
	Line    int    // 1-indexed line number
	Message string // Description of the issue
}

// ParseResult contains the result of parsing markdown content for tables.
type ParseResult struct {
	Tables []ParsedTable
	Issues []TableIssue
}

const (
	// dataRowStartOffset is the offset from separator to first data row.
	dataRowStartOffset = 2

	// minPipesForCell is the minimum number of pipes needed to form a valid table cell.
	minPipesForCell = 2
)

var (
	// Match table row: starts with |, contains content, ends with |
	tableRowRegex = regexp.MustCompile(`^\s*\|.*\|\s*$`)

	// Match separator row: contains only |, -, :, and whitespace
	separatorRowRegex = regexp.MustCompile(`^\s*\|[-:\s|]+\|\s*$`)

	// Match alignment patterns in separator
	alignCenterRegex = regexp.MustCompile(`^\s*:-+:\s*$`)
	alignRightRegex  = regexp.MustCompile(`^\s*-+:\s*$`)
)

// Parse parses markdown content and extracts all tables with their issues.
func Parse(content string) *ParseResult {
	result := &ParseResult{
		Tables: []ParsedTable{},
		Issues: []TableIssue{},
	}

	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Check if this looks like a table row
		if !tableRowRegex.MatchString(line) {
			continue
		}

		// Try to parse a table starting at this line
		table, endIdx, issues := parseTableAt(lines, i)

		if table != nil {
			result.Tables = append(result.Tables, *table)
			result.Issues = append(result.Issues, issues...)
			i = endIdx // Skip past this table
		}
	}

	return result
}

// parseTableAt attempts to parse a table starting at the given line index.
// Returns the parsed table, the ending index, and any issues found.
func parseTableAt(lines []string, startIdx int) (*ParsedTable, int, []TableIssue) {
	const typicalIssueCount = 5 // Typical number of table issues

	issues := make([]TableIssue, 0, typicalIssueCount)

	// Need at least 2 lines for a valid table (header + separator)
	if startIdx+1 >= len(lines) {
		return nil, startIdx, nil
	}

	headerLine := lines[startIdx]
	separatorLine := lines[startIdx+1]

	// Verify separator line
	if !separatorRowRegex.MatchString(separatorLine) {
		return nil, startIdx, nil
	}

	// Parse header
	headers := parseCells(headerLine)
	if len(headers) == 0 {
		return nil, startIdx, nil
	}

	// Parse alignments from separator
	alignments := parseAlignments(separatorLine)

	// Collect raw lines and data rows
	rawLines := []string{headerLine, separatorLine}
	rows := [][]string{}
	endIdx := startIdx + 1

	// Parse data rows
	for i := startIdx + dataRowStartOffset; i < len(lines); i++ {
		line := lines[i]

		if !tableRowRegex.MatchString(line) {
			break
		}

		rawLines = append(rawLines, line)
		cells := parseCells(line)
		rows = append(rows, cells)
		endIdx = i
	}

	table := &ParsedTable{
		StartLine:  startIdx + 1, // Convert to 1-indexed
		EndLine:    endIdx + 1,   // Convert to 1-indexed
		Headers:    headers,
		Rows:       rows,
		Alignments: alignments,
		RawLines:   rawLines,
	}

	// Check for issues
	issues = append(issues, validateTable(table)...)

	return table, endIdx, issues
}

// parseCells extracts cell values from a table row.
func parseCells(line string) []string {
	// Remove leading/trailing pipes and whitespace
	line = strings.TrimSpace(line)

	if len(line) < 2 || line[0] != '|' || line[len(line)-1] != '|' {
		return nil
	}

	// Remove outer pipes
	line = line[1 : len(line)-1]

	// Split by pipe, handling escaped pipes
	cells := splitByPipe(line)

	// Trim each cell
	for i, cell := range cells {
		cells[i] = strings.TrimSpace(cell)
	}

	return cells
}

// splitByPipe splits a string by pipe characters, respecting escaped pipes.
func splitByPipe(s string) []string {
	var cells []string

	var current strings.Builder

	runes := []rune(s)

	for i := range runes {
		if runes[i] == '|' {
			// Check if escaped
			if i > 0 && runes[i-1] == '\\' {
				// Remove the backslash and add the pipe
				str := current.String()
				current.Reset()
				current.WriteString(str[:len(str)-1])
				current.WriteRune('|')
			} else {
				cells = append(cells, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(runes[i])
		}
	}

	cells = append(cells, current.String())

	return cells
}

// parseAlignments extracts column alignments from a separator row.
func parseAlignments(line string) []Alignment {
	cells := parseCells(line)
	alignments := make([]Alignment, len(cells))

	for i, cell := range cells {
		cell = strings.TrimSpace(cell)

		switch {
		case alignCenterRegex.MatchString(cell):
			alignments[i] = AlignCenter
		case alignRightRegex.MatchString(cell):
			alignments[i] = AlignRight
		default:
			alignments[i] = AlignLeft
		}
	}

	return alignments
}

// validateTable checks a parsed table for common issues.
func validateTable(table *ParsedTable) []TableIssue {
	issues := []TableIssue{}

	numCols := len(table.Headers)

	// Check if alignments match headers
	if len(table.Alignments) != numCols {
		issues = append(issues, TableIssue{
			Line:    table.StartLine + 1,
			Message: "Separator row column count doesn't match header",
		})
	}

	// Check each data row
	for i, row := range table.Rows {
		if len(row) != numCols {
			issues = append(issues, TableIssue{
				Line:    table.StartLine + 2 + i,
				Message: "Row column count doesn't match header",
			})
		}
	}

	// Check for formatting issues in raw lines
	for i, line := range table.RawLines {
		lineNum := table.StartLine + i

		// Check for inconsistent spacing
		if hasInconsistentSpacing(line) {
			issues = append(issues, TableIssue{
				Line:    lineNum,
				Message: "Inconsistent spacing in table row",
			})
		}
	}

	return issues
}

// hasInconsistentSpacing checks if a table row has inconsistent padding.
func hasInconsistentSpacing(line string) bool {
	// Separator rows don't need space padding - skip them
	if separatorRowRegex.MatchString(line) {
		return false
	}

	// Simple check: are there cells without space padding?
	// A well-formatted table should have "| cell |" not "|cell|"

	// Find pipe positions to determine cell boundaries
	pipePositions := findPipePositions(line)
	if len(pipePositions) < minPipesForCell {
		return false
	}

	// Check each cell segment between pipes
	for i := range len(pipePositions) - 1 {
		start := pipePositions[i] + 1
		end := pipePositions[i+1]

		if start >= end {
			continue
		}

		cellContent := line[start:end]

		// Skip empty cells or cells with only whitespace
		if strings.TrimSpace(cellContent) == "" {
			continue
		}

		// Check if cell has proper padding (space after opening pipe, space before closing pipe)
		// A well-formatted cell looks like " content " not "content"
		if len(cellContent) > 0 && cellContent[0] != ' ' {
			return true
		}

		if len(cellContent) > 0 && cellContent[len(cellContent)-1] != ' ' {
			return true
		}
	}

	return false
}

// findPipePositions returns the positions of unescaped pipe characters in the line.
func findPipePositions(line string) []int {
	var positions []int

	for i := range len(line) {
		if line[i] == '|' {
			// Check if escaped
			if i > 0 && line[i-1] == '\\' {
				continue
			}

			positions = append(positions, i)
		}
	}

	return positions
}

// FormatTable formats a parsed table into proper markdown.
func FormatTable(table *ParsedTable) string {
	return FormatTableWithMode(table, WidthModeDisplay)
}

// FormatTableWithMode formats a parsed table with a specific width calculation mode.
func FormatTableWithMode(table *ParsedTable, mode WidthMode) string {
	return FormatWithMode(table.Headers, table.Rows, mode, table.Alignments...)
}

// FindAndFormatTables finds all tables in content and returns formatted versions.
func FindAndFormatTables(content string) map[int]string {
	return FindAndFormatTablesWithMode(content, WidthModeDisplay)
}

// FindAndFormatTablesWithMode finds all tables and formats them with a specific width mode.
func FindAndFormatTablesWithMode(content string, mode WidthMode) map[int]string {
	result := Parse(content)
	formatted := make(map[int]string)

	for _, table := range result.Tables {
		formatted[table.StartLine] = FormatTableWithMode(&table, mode)
	}

	return formatted
}
