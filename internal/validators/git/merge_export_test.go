package git

import "github.com/smykla-labs/klaudiush/pkg/parser"

// Export functions for testing.
// These functions expose internal methods for unit testing.

// ExportValidateTitle exposes validateTitle for testing.
func (v *MergeValidator) ExportValidateTitle(title string) []string {
	return v.validateTitle(title)
}

// ExportValidateBody exposes validateBody for testing.
func (v *MergeValidator) ExportValidateBody(body string) []string {
	return v.validateBody(body)
}

// ExportValidateSignoffInText exposes validateSignoffInText for testing.
func (v *MergeValidator) ExportValidateSignoffInText(text string) []string {
	return v.validateSignoffInText(text)
}

// ExportValidateMergeCommandSignoff exposes validateMergeCommandSignoff for testing.
func (v *MergeValidator) ExportValidateMergeCommandSignoff(
	mergeCmd *parser.GHMergeCommand,
) string {
	result := v.validateMergeCommandSignoff(mergeCmd)
	if result.Passed {
		return ""
	}

	return result.Message
}

// ExportIsMessageValidationEnabled exposes isMessageValidationEnabled for testing.
func (v *MergeValidator) ExportIsMessageValidationEnabled() bool {
	return v.isMessageValidationEnabled()
}

// ExportShouldValidateAutomerge exposes shouldValidateAutomerge for testing.
func (v *MergeValidator) ExportShouldValidateAutomerge() bool {
	return v.shouldValidateAutomerge()
}
