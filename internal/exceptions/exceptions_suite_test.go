package exceptions_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExceptions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Exceptions Suite")
}
