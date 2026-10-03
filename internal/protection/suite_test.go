package protection_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProtection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Protection Suite")
}
