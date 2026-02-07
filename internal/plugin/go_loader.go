package plugin

import (
	"context"
	"fmt"
	goplugin "plugin"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/plugin"
)

// ErrPluginNilResponse is returned when a plugin returns a nil response.
var ErrPluginNilResponse = errors.New("plugin returned nil response")

// GoLoader loads native Go plugins (.so files).
type GoLoader struct{}

// NewGoLoader creates a new Go plugin loader.
func NewGoLoader() *GoLoader {
	return &GoLoader{}
}

// Load loads a Go plugin from the specified path.
//
//nolint:ireturn // interface return is required by Loader interface
func (*GoLoader) Load(cfg *config.PluginInstanceConfig) (Plugin, error) {
	if cfg.Path == "" {
		return nil, errors.New("path is required for Go plugins")
	}

	// Validate .so extension (defense-in-depth)
	if extErr := ValidateExtension(cfg.Path, []string{".so"}); extErr != nil {
		return nil, errors.Wrap(extErr, "invalid Go plugin extension")
	}

	// Validate path is in allowed directory (defense-in-depth)
	allowedDirs, allowedErr := GetAllowedDirs(cfg.ProjectRoot)
	if allowedErr != nil {
		return nil, errors.Wrap(allowedErr, "failed to determine allowed directories")
	}

	if pathErr := ValidatePath(cfg.Path, allowedDirs); pathErr != nil {
		return nil, errors.Wrapf(pathErr, "plugin path validation failed: %s", cfg.Path)
	}

	// Load the .so file
	p, err := goplugin.Open(cfg.Path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open Go plugin")
	}

	// Look up the "Plugin" symbol
	sym, err := p.Lookup("Plugin")
	if err != nil {
		return nil, errors.Wrap(err, "plugin does not export 'Plugin' symbol")
	}

	// Assert that the symbol implements the plugin.Plugin interface
	pluginImpl, ok := sym.(plugin.Plugin)
	if !ok {
		return nil, errors.New("Plugin symbol does not implement plugin.Plugin interface")
	}

	return &goPluginAdapter{
		impl:   pluginImpl,
		config: cfg.Config,
	}, nil
}

// Close releases any resources held by the loader.
func (*GoLoader) Close() error {
	// Go plugins cannot be unloaded, so this is a no-op
	return nil
}

// goPluginAdapter adapts a public plugin.Plugin to the internal Plugin interface.
type goPluginAdapter struct {
	impl   plugin.Plugin
	config map[string]any
}

// Info returns metadata about the plugin.
func (a *goPluginAdapter) Info() plugin.Info {
	return a.impl.Info()
}

// Validate performs validation and returns a response.
func (a *goPluginAdapter) Validate(
	ctx context.Context,
	req *plugin.ValidateRequest,
) (*plugin.ValidateResponse, error) {
	// Add plugin-specific config to the request
	if req.Config == nil && len(a.config) > 0 {
		req.Config = a.config
	}

	// Go plugins don't inherently support context, but we respect the context
	// by checking if it's already cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Call the plugin's Validate method with panic recovery
	// Note: Go plugins run synchronously in the same process
	var resp *plugin.ValidateResponse

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Sanitize panic message to remove sensitive data (file paths, stack traces)
				sanitizedPanic := SanitizePanicMessage(fmt.Sprintf("%v", r))

				resp = &plugin.ValidateResponse{
					Passed:      false,
					ShouldBlock: true,
					Message:     "Plugin panicked during validation",
					Details: map[string]string{
						"panic":  sanitizedPanic,
						"plugin": a.impl.Info().Name,
					},
				}
			}
		}()

		resp = a.impl.Validate(req)
	}()

	if resp == nil {
		return nil, errors.Wrapf(ErrPluginNilResponse, "plugin %s", a.impl.Info().Name)
	}

	return resp, nil
}

// Close releases any resources held by the plugin.
func (*goPluginAdapter) Close() error {
	// Go plugins cannot be unloaded, so this is a no-op
	return nil
}

// NewGoPluginAdapterForTesting creates a goPluginAdapter for testing purposes.
// This allows tests to inject mock plugin implementations without needing .so files.
//
//nolint:ireturn // interface return is required for testing
func NewGoPluginAdapterForTesting(
	impl plugin.Plugin,
	config map[string]any,
) Plugin {
	return &goPluginAdapter{
		impl:   impl,
		config: config,
	}
}
