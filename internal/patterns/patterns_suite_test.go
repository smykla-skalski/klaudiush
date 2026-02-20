package patterns_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPatterns(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Patterns Suite")
}
