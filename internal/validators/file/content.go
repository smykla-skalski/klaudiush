package file

import (
	"os"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// ContentInfo holds extracted content and metadata for file validation.
type ContentInfo struct {
	Content    string
	IsFragment bool
}

// ContentExtractor handles content extraction from hook contexts for file validators.
// It supports Write (full content), Edit (fragment with context), and file-read operations.
type ContentExtractor struct {
	logger       logger.Logger
	contextLines int
}

// NewContentExtractor creates a ContentExtractor with the given logger and context line count.
func NewContentExtractor(log logger.Logger, contextLines int) *ContentExtractor {
	return &ContentExtractor{
		logger:       log,
		contextLines: contextLines,
	}
}

// Extract gets content from a hook context.
// For Edit operations, extracts the changed fragment with surrounding context lines.
// For Write operations, returns the full content from the tool input.
// Falls back to reading the file from disk when no content is in the context.
func (e *ContentExtractor) Extract(ctx *hook.Context, filePath string) (*ContentInfo, error) {
	// For Edit operations, validate only the changed fragment with context
	if ctx.EventType == hook.EventTypePreToolUse && ctx.ToolName == hook.ToolTypeEdit {
		content, err := e.extractEditContent(ctx, filePath)
		if err != nil {
			return nil, err
		}

		return &ContentInfo{Content: content, IsFragment: true}, nil
	}

	// Get content from context (Write operation)
	content := ctx.ToolInput.Content
	if content != "" {
		return &ContentInfo{Content: content, IsFragment: false}, nil
	}

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		e.logger.Debug("file does not exist, skipping", "file", filePath)
		return nil, err
	}

	// Read file content
	data, err := os.ReadFile(filePath) //nolint:gosec // filePath is from Claude Code context
	if err != nil {
		e.logger.Debug("failed to read file", "file", filePath, "error", err)
		return nil, err
	}

	return &ContentInfo{Content: string(data), IsFragment: false}, nil
}

// extractEditContent extracts content for Edit operations with surrounding context.
func (e *ContentExtractor) extractEditContent(ctx *hook.Context, filePath string) (string, error) {
	oldStr := ctx.ToolInput.OldString
	newStr := ctx.ToolInput.NewString

	if oldStr == "" || newStr == "" {
		e.logger.Debug("missing old_string or new_string in edit operation")
		return "", os.ErrNotExist
	}

	// Read original file to extract context around the edit
	//nolint:gosec // filePath is from Claude Code tool context, not user input
	originalContent, err := os.ReadFile(filePath)
	if err != nil {
		e.logger.Debug("failed to read file for edit validation", "file", filePath, "error", err)
		return "", err
	}

	originalStr := string(originalContent)

	// Extract fragment with context lines around the edit
	fragment := ExtractEditFragment(
		originalStr,
		oldStr,
		newStr,
		e.contextLines,
		e.logger,
	)
	if fragment == "" {
		e.logger.Debug("could not extract edit fragment, skipping validation")
		return "", os.ErrNotExist
	}

	fragmentLineCount := len(strings.Split(fragment, "\n"))
	e.logger.Debug("validating edit fragment with context",
		"fragment_lines", fragmentLineCount,
	)

	return fragment, nil
}
