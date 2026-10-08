package git_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGitValidators(t *testing.T) {
	isolateGitEnv(t)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Git Validators Suite")
}

// isolateGitEnv keeps the developer's git config and editor out of the
// suite: global and system config (url.insteadOf rewrites remote URLs) and
// exported config or editor variables (the validator treats them as unknown).
func isolateGitEnv(t *testing.T) {
	t.Helper()

	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	for _, name := range []string{"GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "GIT_EDITOR"} {
		t.Setenv(name, "")

		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
	}
}
