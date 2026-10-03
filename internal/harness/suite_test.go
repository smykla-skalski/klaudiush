package harness_test

import (
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// blockEnv makes the test binary only sleep, so specs can start a process
// whose environment macOS does not hide, unlike Apple binaries such as sleep.
const blockEnv = "KLAUDIUSH_HARNESS_TEST_BLOCK"

// testHelpers are modes the test binary runs in when started with the
// helper name as its first argument, so specs can drive it as a harness.
var testHelpers = map[string]func(args []string) int{}

func TestMain(m *testing.M) {
	if os.Getenv(blockEnv) == "1" {
		time.Sleep(time.Hour)

		return
	}

	if len(os.Args) > 1 {
		if helper, ok := testHelpers[os.Args[1]]; ok {
			os.Exit(helper(os.Args[2:]))
		}
	}

	os.Exit(m.Run())
}

func TestHarness(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Harness Suite")
}
