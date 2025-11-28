package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/plugin"
)

const (
	// defaultExecPluginTimeout is the default timeout for exec plugin operations.
	defaultExecPluginTimeout = 5 * time.Second
)

var (
	// ErrPluginInfoFailed is returned when plugin --info execution fails.
	ErrPluginInfoFailed = errors.New("plugin --info exited with non-zero code")

	// ErrPluginExecFailed is returned when plugin execution fails.
	ErrPluginExecFailed = errors.New("plugin execution failed with non-zero code")
)

// ExecLoader loads plugins as external executables that communicate via JSON.
//
// Protocol:
// - Request: JSON-encoded plugin.ValidateRequest on stdin
// - Response: JSON-encoded plugin.ValidateResponse on stdout
// - Info: Execute with --info flag, returns JSON-encoded plugin.Info
type ExecLoader struct {
	runner exec.CommandRunner
}

// NewExecLoader creates a new exec plugin loader.
func NewExecLoader(runner exec.CommandRunner) *ExecLoader {
	return &ExecLoader{
		runner: runner,
	}
}

// Load loads an exec plugin from the specified path.
//
//nolint:ireturn // interface return is required by Loader interface
func (l *ExecLoader) Load(cfg *config.PluginInstanceConfig) (Plugin, error) {
	if cfg.Path == "" {
		return nil, errors.New("path is required for exec plugins")
	}

	// Defense-in-depth: reject paths with shell metacharacters
	// Even though exec.Command doesn't use shell, this prevents suspicious paths
	if metaErr := ValidateMetachars(cfg.Path); metaErr != nil {
		return nil, errors.Wrap(metaErr, "invalid characters in plugin path")
	}

	// Validate path is in allowed directory (defense-in-depth)
	allowedDirs, allowedErr := GetAllowedDirs(cfg.ProjectRoot)
	if allowedErr != nil {
		return nil, errors.Wrap(allowedErr, "failed to determine allowed directories")
	}

	if pathErr := ValidatePath(cfg.Path, allowedDirs); pathErr != nil {
		return nil, errors.Wrapf(pathErr, "plugin path validation failed: %s", cfg.Path)
	}

	// Verify the plugin executable exists and is executable
	if execErr := l.verifyExecutable(cfg.Path); execErr != nil {
		return nil, errors.Wrap(execErr, "plugin executable verification failed")
	}

	// Fetch plugin info
	info, err := l.fetchInfo(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch plugin info")
	}

	return &execPluginAdapter{
		path:    cfg.Path,
		args:    cfg.Args,
		timeout: cfg.GetTimeout(defaultExecPluginTimeout),
		config:  cfg.Config,
		info:    info,
		runner:  l.runner,
	}, nil
}

// Close releases any resources held by the loader.
func (*ExecLoader) Close() error {
	// No global resources to clean up
	return nil
}

// verifyExecutable checks if the plugin path exists and is executable.
func (l *ExecLoader) verifyExecutable(path string) error {
	// Try to execute with --version to verify it's executable
	ctx, cancel := context.WithTimeout(context.Background(), defaultExecPluginTimeout)
	defer cancel()

	result := l.runner.Run(ctx, path, "--version")
	if result.Err != nil {
		return errors.Wrapf(result.Err, "failed to execute plugin at path %q with --version", path)
	}

	if result.ExitCode != 0 {
		return errors.Errorf(
			"plugin at path %q --version exited with code %d: %s",
			path,
			result.ExitCode,
			result.Stderr,
		)
	}

	return nil
}

// fetchInfo fetches plugin metadata by executing with --info flag.
func (l *ExecLoader) fetchInfo(cfg *config.PluginInstanceConfig) (plugin.Info, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		cfg.GetTimeout(defaultExecPluginTimeout),
	)
	defer cancel()

	args := append([]string{"--info"}, cfg.Args...)

	result := l.runner.Run(ctx, cfg.Path, args...)
	if result.Err != nil {
		return plugin.Info{}, errors.Wrap(result.Err, "failed to execute plugin --info")
	}

	if result.ExitCode != 0 {
		return plugin.Info{}, errors.Wrapf(
			ErrPluginInfoFailed,
			"exit code %d: %s",
			result.ExitCode,
			result.Stderr,
		)
	}

	var info plugin.Info
	if err := json.Unmarshal([]byte(result.Stdout), &info); err != nil {
		return plugin.Info{}, errors.Wrap(err, "failed to parse plugin info JSON")
	}

	return info, nil
}

// execPluginAdapter adapts an external executable to the internal Plugin interface.
type execPluginAdapter struct {
	path    string
	args    []string
	timeout time.Duration
	config  map[string]any
	info    plugin.Info
	runner  exec.CommandRunner
}

// Info returns metadata about the plugin.
func (a *execPluginAdapter) Info() plugin.Info {
	return a.info
}

// Validate performs validation by executing the plugin and passing JSON via stdin.
func (a *execPluginAdapter) Validate(
	ctx context.Context,
	req *plugin.ValidateRequest,
) (*plugin.ValidateResponse, error) {
	// Add plugin-specific config to the request
	if req.Config == nil && len(a.config) > 0 {
		req.Config = a.config
	}

	// Marshal request to JSON
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal request to JSON")
	}

	// Apply timeout if context doesn't have one
	execCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc

		execCtx, cancel = context.WithTimeout(ctx, a.timeout)

		defer cancel()
	}

	// Execute the plugin with JSON input via stdin
	stdin := bytes.NewReader(reqJSON)
	result := a.runner.RunWithStdin(execCtx, stdin, a.path, a.args...)

	// Check for execution errors
	if result.Err != nil {
		return nil, errors.Wrap(result.Err, "plugin execution failed")
	}

	if result.ExitCode != 0 {
		return nil, errors.Wrapf(
			ErrPluginExecFailed,
			"exit code %d: %s",
			result.ExitCode,
			result.Stderr,
		)
	}

	// Parse response JSON from stdout
	var resp plugin.ValidateResponse
	if err := json.Unmarshal([]byte(result.Stdout), &resp); err != nil {
		return nil, errors.Wrap(err, "failed to parse response JSON")
	}

	return &resp, nil
}

// Close releases any resources held by the plugin.
func (*execPluginAdapter) Close() error {
	// No resources to clean up for exec plugins
	return nil
}
