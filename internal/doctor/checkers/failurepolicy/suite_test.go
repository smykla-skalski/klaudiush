package failurepolicy_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFailurePolicyCheckers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Failure Policy Checkers Suite")
}
