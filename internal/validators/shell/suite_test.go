package shell_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestShell runs with an empty home, so the startup files of the machine
// running the tests never decide what a shell in a test command reads.
func TestShell(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", home)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Shell Suite")
}
