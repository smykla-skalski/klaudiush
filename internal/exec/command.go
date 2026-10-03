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

// pipeWaitDelay bounds how long a canceled command may keep its output pipes
// open. Without it, a child the command started (a shell's background job)
// holds the pipes and the run blocks until that child exits, long past the
// deadline.
const pipeWaitDelay = 2 * time.Second

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

// OptionsRunner executes external commands with a working directory,
// environment and streams of the caller's choice.
type OptionsRunner interface {
	// RunWithOptions executes a command. The result is always valid; check
	// result.Err for execution errors.
	RunWithOptions(ctx context.Context, opts RunOptions, name string, args ...string) CommandResult
}

// RunOptions configures one command run. Zero values inherit the current
// directory and environment and capture output into the CommandResult.
type RunOptions struct {
	Dir    string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
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
func (r *commandRunner) Run(
	ctx context.Context,
	name string,
	args ...string,
) CommandResult {
	return r.RunWithOptions(ctx, RunOptions{}, name, args...)
}

// RunWithStdin executes a command with stdin input.
func (r *commandRunner) RunWithStdin(
	ctx context.Context,
	stdin io.Reader,
	name string,
	args ...string,
) CommandResult {
	return r.RunWithOptions(ctx, RunOptions{Stdin: stdin}, name, args...)
}

// RunWithOptions executes a command in a chosen directory and environment.
func (*commandRunner) RunWithOptions(
	ctx context.Context,
	opts RunOptions,
	name string,
	args ...string,
) CommandResult {
	cmd := exec.CommandContext( //nolint:gosec // G204: subprocess args are the purpose of this abstraction
		ctx,
		name,
		args...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env
	cmd.Stdin = opts.Stdin

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	}

	cmd.Stderr = &stderr
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	}

	cmd.WaitDelay = pipeWaitDelay

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
