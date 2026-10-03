package hook

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

const openCodeAPICheckName = "opencode loads the bridge plugin"

// OpenCodeAPIChecker checks that the installed bridge plugin follows the plugin
// API of the installed opencode.
//
// opencode only logs a warning when it rejects a plugin and then runs every
// tool unchecked, so a 1.x bridge under opencode 2.x (or the reverse) looks
// healthy to every other check: the binary path and the subscribed events are
// all there.
type OpenCodeAPIChecker struct {
	cfg      *pkgConfig.OpenCodeProviderConfig
	openCode settings.OpenCodeVersionDetector
}

// NewOpenCodeAPIChecker creates a checker for the bridge plugin API.
func NewOpenCodeAPIChecker(cfg *pkgConfig.OpenCodeProviderConfig) *OpenCodeAPIChecker {
	return &OpenCodeAPIChecker{cfg: cfg, openCode: settings.NewOpenCodeVersionDetector()}
}

// WithOpenCodeVersionDetector replaces how the installed opencode version is
// found.
func (c *OpenCodeAPIChecker) WithOpenCodeVersionDetector(
	detector settings.OpenCodeVersionDetector,
) *OpenCodeAPIChecker {
	c.openCode = detector

	return c
}

// Name returns the name of the check.
func (*OpenCodeAPIChecker) Name() string {
	return openCodeAPICheckName
}

// Category returns the category of the check.
func (*OpenCodeAPIChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check compares the plugin API of the installed bridge and opencode.
func (c *OpenCodeAPIChecker) Check(ctx context.Context) doctor.CheckResult {
	registrationChecker := &OpenCodeRegistrationChecker{cfg: c.cfg}
	if result, ready := registrationChecker.preflight(openCodeAPICheckName); !ready {
		return result
	}

	pluginPath := registrationChecker.pluginPath()

	source, err := settings.NewOpenCodePluginParser(pluginPath).Read()
	if err != nil {
		if errors.Is(err, settings.ErrPluginNotFound) {
			return doctor.Skip(openCodeAPICheckName, "Bridge plugin not installed")
		}

		return registrationChecker.failForParseError(openCodeAPICheckName, err)
	}

	have := settings.DetectOpenCodePluginAPI(source)

	version, err := c.openCode.Detect(ctx)
	if err != nil {
		return doctor.FailWarning(
			openCodeAPICheckName,
			"Could not detect the opencode version, so it is unverified that opencode loads "+
				"the bridge plugin",
		).WithDetails(
			fmt.Sprintf("Error: %v", err),
			"File: "+pluginPath,
			"Plugin API: "+describeAPI(have),
			"opencode 1.x and 2.x reject each other's plugin and then run every tool unchecked",
		)
	}

	want := settings.OpenCodeAPIForVersion(version)

	if want == settings.OpenCodeAPIUnknown {
		return doctor.FailWarning(
			openCodeAPICheckName,
			fmt.Sprintf("Unrecognized opencode version %q", version),
		)
	}

	if have != want {
		return doctor.FailError(
			openCodeAPICheckName,
			fmt.Sprintf(
				"opencode %s rejects the bridge plugin and runs every tool unchecked",
				version,
			),
		).
			WithDetails(
				"File: "+pluginPath,
				fmt.Sprintf("Plugin API: %s, opencode %s loads: %s", describeAPI(have), version, want),
				"Regenerate with: klaudiush doctor --fix",
			).
			WithFixID("install_hook")
	}

	if !settings.OpenCodeAPIVerified(version) {
		return doctor.FailWarning(
			openCodeAPICheckName,
			fmt.Sprintf(
				"opencode %s is newer than the plugin APIs klaudiush knows; "+
					"check that it loads the %s bridge",
				version,
				want,
			),
		).WithDetails("File: " + pluginPath)
	}

	return doctor.Pass(
		openCodeAPICheckName,
		fmt.Sprintf("opencode %s, %s plugin API", version, want),
	)
}

func describeAPI(api settings.OpenCodeAPI) string {
	if api == settings.OpenCodeAPIUnknown {
		return "unrecognized"
	}

	return string(api)
}
