package plugin

//go:generate mockgen -source=loader.go -destination=loader_mock.go -package=plugin

import (
	"github.com/smykla-labs/klaudiush/pkg/config"
)

// Loader loads plugins from various sources (Go plugins, gRPC, exec).
type Loader interface {
	// Load loads a plugin based on the provided configuration.
	// Returns an error if the plugin cannot be loaded.
	Load(cfg *config.PluginInstanceConfig) (Plugin, error)

	// Close releases any resources held by the loader.
	// For gRPC loaders this closes connection pools.
	Close() error
}
