package validator

// ResultChecker is implemented by validators that, once a tool ran, check the
// whole file as the tool left it on disk. Only their runs prove a file clean;
// validators that look at the tool input alone cannot tell a repair apart from
// an edit elsewhere in the file.
type ResultChecker interface {
	ChecksToolResult() bool
}

// ChecksToolResult reports whether v checks the whole file after a tool ran.
func ChecksToolResult(v Validator) bool {
	checker, ok := v.(ResultChecker)

	return ok && checker.ChecksToolResult()
}
