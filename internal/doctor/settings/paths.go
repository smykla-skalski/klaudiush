package settings

import (
	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/xdg"
)

func resolveSettingsPath(path string) (string, error) {
	resolved, err := xdg.ExpandPath(path)
	if err != nil {
		return "", errors.Wrapf(err, "failed to resolve path %q", path)
	}

	return resolved, nil
}
