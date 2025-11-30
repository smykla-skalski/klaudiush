// Package exec provides abstractions for executing external commands.
package exec

//go:generate mockgen -source=command.go -destination=command_mock.go -package=exec

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"time"

	"github.com/cockroachdb/errors"
)

// CommandResult contains the result of a command execution.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// Success returns true if the command executed without error.
func (r CommandResult) Success() bool {
	return r.Err == nil
}

// Failed returns true if the command execution failed.
func (r CommandResult) Failed() bool {
	return r.Err != nil
}

// CommandRunner executes external commands with timeout and output capture.
type CommandRunner interface {
	// Run executes a command and returns the result.
	// The result is always valid; check result.Err for execution errors.
	Run(ctx context.Context, name string, args ...string) CommandResult

	// RunWithStdin executes a command with stdin input.
	// The result is always valid; check result.Err for execution errors.
	RunWithStdin(
		ctx context.Context,
		stdin io.Reader,
		name string,
		args ...string,
	) CommandResult

	// RunWithTimeout executes a command with a specific timeout.
	// The result is always valid; check result.Err for execution errors.
	RunWithTimeout(timeout time.Duration, name string, args ...string) CommandResult
}

// commandRunner implements CommandRunner.
type commandRunner struct {
	defaultTimeout time.Duration
}

// NewCommandRunner creates a new CommandRunner with the given default timeout.
func NewCommandRunner(defaultTimeout time.Duration) *commandRunner {
	return &commandRunner{
		defaultTimeout: defaultTimeout,
	}
}

// Run executes a command and returns the result.
func (*commandRunner) Run(
	ctx context.Context,
	name string,
	args ...string,
) CommandResult {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		result.Err = err
	} else if err != nil {
		result.Err = errors.Wrapf(err, "executing %s", name)
	}

	return result
}

// RunWithStdin executes a command with stdin input.
func (*commandRunner) RunWithStdin(
	ctx context.Context,
	stdin io.Reader,
	name string,
	args ...string,
) CommandResult {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		result.Err = err
	} else if err != nil {
		result.Err = errors.Wrapf(err, "executing %s", name)
	}

	return result
}

// RunWithTimeout executes a command with a specific timeout.
func (r *commandRunner) RunWithTimeout(
	timeout time.Duration,
	name string,
	args ...string,
) CommandResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return r.Run(ctx, name, args...)
}
