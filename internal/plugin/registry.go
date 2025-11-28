package plugin

import (
	"path/filepath"
	"regexp"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

const (
	// defaultRegistryTimeout is the default timeout for plugin operations.
	defaultRegistryTimeout = 10 * time.Second
)

// Registry manages plugin loading and lifecycle.
type Registry struct {
	loaders map[config.PluginType]Loader
	plugins []*PluginEntry
	logger  logger.Logger
}

// PluginEntry represents a loaded plugin with its configuration and predicate.
type PluginEntry struct {
	Plugin    Plugin
	Config    *config.PluginInstanceConfig
	Predicate *PredicateMatcher
	Validator validator.Validator
}

// PredicateMatcher evaluates whether a plugin should be invoked for a given context.
type PredicateMatcher struct {
	eventTypes      map[string]bool
	toolTypes       map[string]bool
	filePatterns    []string
	commandPatterns []*regexp.Regexp
}

// NewRegistry creates a new plugin registry.
func NewRegistry(log logger.Logger) *Registry {
	runner := exec.NewCommandRunner(defaultRegistryTimeout)

	return &Registry{
		loaders: map[config.PluginType]Loader{
			config.PluginTypeGo:   NewGoLoader(),
			config.PluginTypeGRPC: NewGRPCLoader(),
			config.PluginTypeExec: NewExecLoader(runner),
		},
		plugins: make([]*PluginEntry, 0),
		logger:  log,
	}
}

// LoadPlugins loads all plugins from the given configuration.
func (r *Registry) LoadPlugins(cfg *config.PluginConfig) error {
	if cfg == nil || !cfg.IsEnabled() {
		return nil
	}

	var loadErrors []error

	for _, pluginCfg := range cfg.Plugins {
		if !pluginCfg.IsInstanceEnabled() {
			r.logger.Debug("skipping disabled plugin", "name", pluginCfg.Name)

			continue
		}

		if err := r.LoadPlugin(pluginCfg); err != nil {
			r.logger.Error("failed to load plugin",
				"name", pluginCfg.Name,
				"type", pluginCfg.Type,
				"error", err,
			)

			// Collect error but continue loading other plugins
			if loadErrors == nil {
				loadErrors = []error{}
			}

			loadErrors = append(loadErrors, errors.Wrapf(err, "plugin %s", pluginCfg.Name))

			continue
		}

		r.logger.Info("loaded plugin",
			"name", pluginCfg.Name,
			"type", pluginCfg.Type,
		)
	}

	// Return aggregated errors if any plugins failed to load
	if len(loadErrors) > 0 {
		return errors.Errorf("failed to load %d plugin(s): %v", len(loadErrors), loadErrors)
	}

	return nil
}

// LoadPlugin loads a single plugin.
func (r *Registry) LoadPlugin(cfg *config.PluginInstanceConfig) error {
	loader, ok := r.loaders[cfg.Type]
	if !ok {
		return errors.Errorf("unsupported plugin type: %s", cfg.Type)
	}

	plugin, err := loader.Load(cfg)
	if err != nil {
		return err
	}

	// Build predicate matcher
	predicate, err := NewPredicateMatcher(cfg.Predicate)
	if err != nil {
		return errors.Wrap(err, "failed to build predicate matcher")
	}

	// Determine plugin category (default to CPU for external plugins)
	category := validator.CategoryCPU
	if cfg.Type == config.PluginTypeExec || cfg.Type == config.PluginTypeGRPC {
		// Exec and gRPC plugins are I/O-bound (process spawning and network I/O)
		category = validator.CategoryIO
	}

	// Create validator adapter
	validatorAdapter := NewValidatorAdapter(plugin, category, r.logger)

	entry := &PluginEntry{
		Plugin:    plugin,
		Config:    cfg,
		Predicate: predicate,
		Validator: validatorAdapter,
	}

	r.plugins = append(r.plugins, entry)

	return nil
}

// GetValidators returns validators for plugins that match the given context.
func (r *Registry) GetValidators(hookCtx *hook.Context) []validator.Validator {
	validators := make([]validator.Validator, 0)

	for _, entry := range r.plugins {
		if entry.Predicate.Matches(hookCtx) {
			validators = append(validators, entry.Validator)
		}
	}

	return validators
}

// Close releases all plugin resources.
func (r *Registry) Close() error {
	var firstErr error

	for _, entry := range r.plugins {
		if err := entry.Plugin.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	for _, loader := range r.loaders {
		if err := loader.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// NewPredicateMatcher creates a predicate matcher from configuration.
func NewPredicateMatcher(cfg *config.PluginPredicate) (*PredicateMatcher, error) {
	matcher := &PredicateMatcher{
		eventTypes: make(map[string]bool),
		toolTypes:  make(map[string]bool),
	}

	if cfg == nil {
		return matcher, nil
	}

	matcher.buildEventTypeMap(cfg.EventTypes)
	matcher.buildToolTypeMap(cfg.ToolTypes)

	if err := matcher.setFilePatterns(cfg.FilePatterns); err != nil {
		return nil, err
	}

	if err := matcher.compileCommandPatterns(cfg.CommandPatterns); err != nil {
		return nil, err
	}

	return matcher, nil
}

// buildEventTypeMap populates the event type lookup map.
func (p *PredicateMatcher) buildEventTypeMap(eventTypes []string) {
	for _, et := range eventTypes {
		p.eventTypes[et] = true
	}
}

// buildToolTypeMap populates the tool type lookup map.
func (p *PredicateMatcher) buildToolTypeMap(toolTypes []string) {
	for _, tt := range toolTypes {
		p.toolTypes[tt] = true
	}
}

// setFilePatterns validates and stores file patterns.
func (p *PredicateMatcher) setFilePatterns(patterns []string) error {
	for _, pattern := range patterns {
		if _, err := filepath.Match(pattern, ""); err != nil {
			return errors.Wrapf(err, "invalid file pattern: %s", pattern)
		}
	}

	p.filePatterns = patterns

	return nil
}

// compileCommandPatterns validates and compiles command regex patterns.
func (p *PredicateMatcher) compileCommandPatterns(patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}

	p.commandPatterns = make([]*regexp.Regexp, 0, len(patterns))

	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return errors.Wrapf(err, "invalid command pattern: %s", pattern)
		}

		p.commandPatterns = append(p.commandPatterns, re)
	}

	return nil
}

// Matches returns whether this predicate matches the given hook context.
func (p *PredicateMatcher) Matches(hookCtx *hook.Context) bool {
	if !p.matchesEventType(hookCtx) {
		return false
	}

	if !p.matchesToolType(hookCtx) {
		return false
	}

	if !p.matchesFilePatterns(hookCtx) {
		return false
	}

	if !p.matchesCommandPatterns(hookCtx) {
		return false
	}

	return true
}

// matchesEventType returns whether the event type matches.
func (p *PredicateMatcher) matchesEventType(hookCtx *hook.Context) bool {
	if len(p.eventTypes) == 0 {
		return true
	}

	eventTypeStr := hookCtx.EventType.String()

	return p.eventTypes[eventTypeStr]
}

// matchesToolType returns whether the tool type matches.
func (p *PredicateMatcher) matchesToolType(hookCtx *hook.Context) bool {
	if len(p.toolTypes) == 0 {
		return true
	}

	toolTypeStr := hookCtx.ToolName.String()

	return p.toolTypes[toolTypeStr]
}

// matchesFilePatterns returns whether the file patterns match.
func (p *PredicateMatcher) matchesFilePatterns(hookCtx *hook.Context) bool {
	if len(p.filePatterns) == 0 {
		return true
	}

	// If patterns are specified, only match file tools
	if !hookCtx.IsFileTool() {
		return false
	}

	filePath := hookCtx.GetFilePath()
	if filePath == "" {
		return false
	}

	for _, pattern := range p.filePatterns {
		// Error is impossible here - patterns are validated in NewPredicateMatcher
		if ok, _ := filepath.Match(pattern, filePath); ok {
			return true
		}
	}

	return false
}

// matchesCommandPatterns returns whether the command patterns match.
func (p *PredicateMatcher) matchesCommandPatterns(hookCtx *hook.Context) bool {
	if len(p.commandPatterns) == 0 {
		return true
	}

	// If patterns are specified, only match Bash tools
	if !hookCtx.IsBashTool() {
		return false
	}

	command := hookCtx.GetCommand()
	if command == "" {
		return false
	}

	for _, re := range p.commandPatterns {
		if re.MatchString(command) {
			return true
		}
	}

	return false
}

// LoadPluginForTesting loads a plugin directly for testing purposes.
// This bypasses the loader system and allows injection of mock plugins.
func (r *Registry) LoadPluginForTesting(
	p Plugin,
	cfg *config.PluginInstanceConfig,
) error {
	// Build predicate matcher
	predicate, err := NewPredicateMatcher(cfg.Predicate)
	if err != nil {
		return errors.Wrap(err, "failed to build predicate matcher")
	}

	// Determine plugin category
	category := validator.CategoryCPU
	if cfg.Type == config.PluginTypeExec || cfg.Type == config.PluginTypeGRPC {
		category = validator.CategoryIO
	}

	// Create validator adapter
	validatorAdapter := NewValidatorAdapter(p, category, r.logger)

	entry := &PluginEntry{
		Plugin:    p,
		Config:    cfg,
		Predicate: predicate,
		Validator: validatorAdapter,
	}

	r.plugins = append(r.plugins, entry)

	return nil
}
