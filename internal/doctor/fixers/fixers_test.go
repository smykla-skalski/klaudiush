package fixers

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFixers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fixers Suite")
}
