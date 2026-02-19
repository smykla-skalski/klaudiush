package hookresponse_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHookResponse(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HookResponse Suite")
}
