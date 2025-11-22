package git_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGitValidators(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Git Validators Suite")
}
