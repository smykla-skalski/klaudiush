package xdg

import "path/filepath"

// PathResolver resolves XDG-based paths for klaudiush.
// The default implementation uses os.UserHomeDir() and XDG env vars.
// Use ResolverFor() when paths should be relative to a specific home directory.
type PathResolver interface {
	GlobalConfigFile() string
	ConfigDir() string
}

// DefaultResolver returns a PathResolver using real XDG paths.
func DefaultResolver() PathResolver {
	return defaultResolver{}
}

type defaultResolver struct{}

func (defaultResolver) GlobalConfigFile() string { return GlobalConfigFile() }
func (defaultResolver) ConfigDir() string        { return ConfigDir() }

// ResolverFor returns a PathResolver that uses homeDir as fallback
// when XDG env vars are not set.
func ResolverFor(homeDir string) PathResolver {
	return homeResolver{homeDir: homeDir}
}

type homeResolver struct {
	homeDir string
}

func (r homeResolver) configHome() string {
	return filepath.Join(r.homeDir, ".config")
}

func (r homeResolver) ConfigDir() string {
	return filepath.Join(r.configHome(), appName)
}

func (r homeResolver) GlobalConfigFile() string {
	return filepath.Join(r.ConfigDir(), "config.toml")
}
