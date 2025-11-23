// Package exec provides abstractions for executing external commands.
package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// CommandResult contains the result of a command execution.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// CommandRunner executes external commands with timeout and output capture.
type CommandRunner interface {
	// Run executes a command and returns the result.
	Run(ctx context.Context, name string, args ...string) (*CommandResult, error)

	// RunWithStdin executes a command with stdin input.
	RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) (*CommandResult, error)

	// RunWithTimeout executes a command with a specific timeout.
	RunWithTimeout(timeout time.Duration, name string, args ...string) (*CommandResult, error)
}

// commandRunner implements CommandRunner.
type commandRunner struct {
	defaultTimeout time.Duration
}

// NewCommandRunner creates a new CommandRunner with the given default timeout.
func NewCommandRunner(defaultTimeout time.Duration) CommandRunner {
	return &commandRunner{
		defaultTimeout: defaultTimeout,
	}
}

// Run executes a command and returns the result.
func (r *commandRunner) Run(ctx context.Context, name string, args ...string) (*CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		return result, fmt.Errorf("executing %s: %w", name, err)
	}

	return result, err
}

// RunWithStdin executes a command with stdin input.
func (r *commandRunner) RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) (*CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		return result, fmt.Errorf("executing %s: %w", name, err)
	}

	return result, err
}

// RunWithTimeout executes a command with a specific timeout.
func (r *commandRunner) RunWithTimeout(timeout time.Duration, name string, args ...string) (*CommandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return r.Run(ctx, name, args...)
}
