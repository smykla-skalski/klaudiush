package failpolicy_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFailPolicy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Failure Policy Suite")
}
