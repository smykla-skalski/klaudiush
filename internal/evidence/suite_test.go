package evidence_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEvidence(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Evidence Suite")
}
