// Package prompt provides utilities for interactive user prompts.
package prompt

//go:generate mockgen -source=prompt.go -destination=prompt_mock.go -package=prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
)

var (
	// ErrEmptyInput is returned when the user provides empty input and no default is set.
	ErrEmptyInput = errors.New("empty input")

	// ErrInvalidInput is returned when the user provides invalid input.
	ErrInvalidInput = errors.New("invalid input")
)

// Prompter defines the interface for interactive prompts.
type Prompter interface {
	// Input prompts for a single line of text input.
	Input(prompt string, defaultValue string) (string, error)

	// Confirm prompts for a yes/no confirmation.
	Confirm(prompt string, defaultValue bool) (bool, error)
}

// StdPrompter is the standard implementation of Prompter using stdin/stdout.
type StdPrompter struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewStdPrompter creates a new StdPrompter.
func NewStdPrompter() *StdPrompter {
	return &StdPrompter{
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stdout,
	}
}

// NewPrompter creates a new Prompter with custom reader and writer (for testing).
func NewPrompter(reader io.Reader, writer io.Writer) *StdPrompter {
	return &StdPrompter{
		reader: bufio.NewReader(reader),
		writer: writer,
	}
}

// Input prompts for a single line of text input.
func (p *StdPrompter) Input(prompt string, defaultValue string) (string, error) {
	// Format prompt with default value if provided
	if defaultValue != "" {
		if _, err := fmt.Fprintf(p.writer, "%s [%s]: ", prompt, defaultValue); err != nil {
			return "", errors.Wrap(err, "failed to write prompt")
		}
	} else {
		if _, err := fmt.Fprintf(p.writer, "%s: ", prompt); err != nil {
			return "", errors.Wrap(err, "failed to write prompt")
		}
	}

	// Read input
	input, err := p.reader.ReadString('\n')
	if err != nil {
		return "", errors.Wrap(err, "failed to read input")
	}

	// Trim whitespace
	input = strings.TrimSpace(input)

	// Use default if input is empty
	if input == "" {
		if defaultValue == "" {
			return "", ErrEmptyInput
		}

		return defaultValue, nil
	}

	return input, nil
}

// Confirm prompts for a yes/no confirmation.
func (p *StdPrompter) Confirm(prompt string, defaultValue bool) (bool, error) {
	// Format prompt with default value
	defaultStr := "y/N"
	if defaultValue {
		defaultStr = "Y/n"
	}

	if _, err := fmt.Fprintf(p.writer, "%s [%s]: ", prompt, defaultStr); err != nil {
		return false, errors.Wrap(err, "failed to write prompt")
	}

	// Read input
	input, err := p.reader.ReadString('\n')
	if err != nil {
		return false, errors.Wrap(err, "failed to read input")
	}

	// Trim whitespace and convert to lowercase
	input = strings.TrimSpace(strings.ToLower(input))

	// Use default if input is empty
	if input == "" {
		return defaultValue, nil
	}

	// Parse yes/no
	switch input {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, errors.Wrapf(ErrInvalidInput, "expected y/n, got %q", input)
	}
}
