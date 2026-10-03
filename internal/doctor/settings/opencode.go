package settings

import (
	"bytes"
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/xdg"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

//go:embed templates/opencode_plugin.ts templates/opencode_plugin_v2.ts
var openCodePluginTemplate embed.FS

const (
	openCodePluginTemplatePathV1 = "templates/opencode_plugin.ts"
	openCodePluginTemplatePathV2 = "templates/opencode_plugin_v2.ts"

	// openCodePluginRelPath is the plugin location under the XDG config home.
	// opencode loads every file in this directory at startup.
	openCodePluginRelPath = "opencode/plugin/klaudiush.ts"
)

// ErrPluginNotFound is returned when the opencode bridge plugin is absent.
var ErrPluginNotFound = errors.New("opencode plugin not found")

// DefaultOpenCodePluginPath returns the default bridge plugin location.
//
// Resolved through the XDG config home rather than a hardcoded ~/.config,
// because opencode honours XDG_CONFIG_HOME when locating its plugin directory
// and a custom root would otherwise get a plugin opencode never loads.
func DefaultOpenCodePluginPath() string {
	return filepath.Join(xdg.ConfigHome(), openCodePluginRelPath)
}

// ResolveOpenCodePluginPath returns the path the bridge plugin is installed to.
//
// plugin_path is optional: enabling the provider is enough, so an empty
// configured value resolves to the default location. Diagnostics and installers
// share this helper so they cannot disagree about where the plugin lives.
func ResolveOpenCodePluginPath(configured string) string {
	if configured != "" {
		return configured
	}

	return DefaultOpenCodePluginPath()
}

// OpenCodePluginParser inspects the generated opencode bridge plugin.
type OpenCodePluginParser struct {
	pluginPath string
}

// NewOpenCodePluginParser creates a parser for the given plugin path.
func NewOpenCodePluginParser(path string) *OpenCodePluginParser {
	return &OpenCodePluginParser{pluginPath: path}
}

// Read returns the plugin source.
func (p *OpenCodePluginParser) Read() (string, error) {
	resolvedPath, err := resolveSettingsPath(p.pluginPath)
	if err != nil {
		return "", err
	}

	data, err := readFileAt(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrPluginNotFound
		}

		return "", errors.Wrap(err, "failed to read opencode plugin")
	}

	return string(data), nil
}

// IsDispatcherRegistered reports whether the plugin invokes this dispatcher.
func (p *OpenCodePluginParser) IsDispatcherRegistered(dispatcherPath string) (bool, error) {
	source, err := p.Read()
	if err != nil {
		if errors.Is(err, ErrPluginNotFound) {
			return false, nil
		}

		return false, err
	}

	return pluginReferencesBinary(source, dispatcherPath)
}

// HasEventHook reports whether the plugin forwards the given opencode event.
func (p *OpenCodePluginParser) HasEventHook(eventName, dispatcherPath string) (bool, error) {
	source, err := p.Read()
	if err != nil {
		if errors.Is(err, ErrPluginNotFound) {
			return false, nil
		}

		return false, err
	}

	referenced, err := pluginReferencesBinary(source, dispatcherPath)
	if err != nil {
		return false, err
	}

	if !referenced {
		return false, nil
	}

	return pluginRegistersEvent(source, eventName), nil
}

// pluginReferencesBinary reports whether the plugin invokes this dispatcher.
// The comparison is against the encoded literal the template emits, so a path
// needing escapes still matches.
func pluginReferencesBinary(source, binaryPath string) (bool, error) {
	literal, err := openCodeBinaryLiteral(binaryPath)
	if err != nil {
		return false, err
	}

	return strings.Contains(source, literal), nil
}

// pluginRegistersEvent reports whether the plugin source actually subscribes to
// an event, as opposed to merely mentioning its name.
//
// A plain substring search is not enough: every forwarded event appears as an
// argument to invoke(), so any such search reports an event as configured even
// when nothing is listening for it. The plugin subscribes in one of these ways:
//
//   - a 1.x hook key, `"tool.execute.before":`, optionally behind opencode's
//     `experimental.` prefix for hooks that are still unstable
//   - a 2.x hook registration, `ctx.tool.hook("execute.before",`
//   - a case label on the shared event bus, `case "session.idle":`
func pluginRegistersEvent(source, eventName string) bool {
	forms := []string{
		`"` + eventName + `":`,
		`"experimental.` + eventName + `":`,
		`case "` + eventName + `":`,
	}

	if registration, ok := openCodeV2Registrations[eventName]; ok {
		forms = append(forms, registration)
	}

	for _, form := range forms {
		if strings.Contains(source, form) {
			return true
		}
	}

	return false
}

// InstallOpenCodeDispatcher renders the bridge plugin for the given binary and
// opencode plugin API. It reports whether the plugin was already installed and
// up to date, matching the return convention of the other provider installers.
func InstallOpenCodeDispatcher(pluginPath, binaryPath string, api OpenCodeAPI) (bool, error) {
	rendered, err := RenderOpenCodePlugin(binaryPath, api)
	if err != nil {
		return false, err
	}

	resolvedPath, err := resolveSettingsPath(pluginPath)
	if err != nil {
		return false, err
	}

	if existing, readErr := readFileAt(resolvedPath); readErr == nil {
		if bytes.Equal(existing, rendered) {
			return true, nil
		}
	}

	if err := writeOpenCodePlugin(resolvedPath, rendered); err != nil {
		return false, err
	}

	return false, nil
}

// RenderOpenCodePlugin renders the bridge plugin source for a binary path and
// opencode plugin API. An unknown API renders the 1.x bridge, the only one
// earlier releases installed.
func RenderOpenCodePlugin(binaryPath string, api OpenCodeAPI) ([]byte, error) {
	templatePath := openCodePluginTemplatePathV1
	if api == OpenCodeAPIV2 {
		templatePath = openCodePluginTemplatePathV2
	}

	raw, err := openCodePluginTemplate.ReadFile(templatePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read embedded opencode plugin")
	}

	tmpl, err := template.New("opencode_plugin").Parse(string(raw))
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse opencode plugin template")
	}

	var buf bytes.Buffer

	literal, err := openCodeBinaryLiteral(binaryPath)
	if err != nil {
		return nil, err
	}

	data := struct {
		BinaryLiteral string
		TimeoutMs     int
	}{
		BinaryLiteral: literal,
		TimeoutMs:     DefaultCommandHookTimeout * millisecondsPerSecond,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, errors.Wrap(err, "failed to render opencode plugin")
	}

	return buf.Bytes(), nil
}

// openCodeBinaryLiteral encodes a path as a quoted JavaScript string literal.
//
// Interpolating the raw path would emit broken source for any path containing a
// quote or a backslash: a Windows path such as C:\Users\me reads as escape
// sequences, and the generated bridge could not launch klaudiush at all. JSON
// string syntax is a subset of JavaScript's, so encoding produces a literal
// that is both valid and decodes back to the original path.
func openCodeBinaryLiteral(binaryPath string) (string, error) {
	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)
	// Paths are not HTML; escaping <, > and & would only obscure them.
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(binaryPath); err != nil {
		return "", errors.Wrap(err, "failed to encode opencode binary path")
	}

	return strings.TrimSpace(buf.String()), nil
}

// OpenCodeEventNames returns the opencode events the bridge plugin forwards.
func OpenCodeEventNames() []string {
	return hook.OpenCodeEventNames()
}

// readFileAt reads an operator-configured path. Isolated so the gosec
// suppression stays attached to the single call it applies to.
func readFileAt(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // path comes from operator config
}

// writeOpenCodePlugin replaces the plugin atomically.
//
// A direct write truncates the live file first, so an interrupted doctor --fix
// or a full disk would leave a half-written plugin behind. opencode would then
// fail to load it and every hook would silently stop validating. No backup is
// kept: the file is generated and can be re-rendered at any time.
func writeOpenCodePlugin(path string, data []byte) error {
	if err := AtomicWriteFile(path, data, false); err != nil {
		return errors.Wrap(err, "failed to write opencode plugin")
	}

	return nil
}
