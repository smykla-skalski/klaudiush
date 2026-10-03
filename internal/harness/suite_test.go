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

func TestMain(m *testing.M) {
	if os.Getenv(blockEnv) == "1" {
		time.Sleep(time.Hour)

		return
	}

	os.Exit(m.Run())
}

func TestHarness(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Harness Suite")
}
