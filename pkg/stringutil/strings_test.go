package stringutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/stringutil"
)

func TestStringutil(t *testing.T) {
	t.Parallel()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Stringutil Suite")
}

var _ = Describe("ContainsCaseInsensitive", func() {
	It("should find exact match", func() {
		Expect(stringutil.ContainsCaseInsensitive(config.ValidToolTypes, "Bash")).To(BeTrue())
	})

	It("should find lowercase match", func() {
		Expect(stringutil.ContainsCaseInsensitive(config.ValidToolTypes, "bash")).To(BeTrue())
	})

	It("should find uppercase match", func() {
		Expect(stringutil.ContainsCaseInsensitive(config.ValidToolTypes, "BASH")).To(BeTrue())
	})

	It("should find mixed case match", func() {
		Expect(stringutil.ContainsCaseInsensitive(config.ValidToolTypes, "bAsH")).To(BeTrue())
	})

	It("should return false for non-match", func() {
		Expect(stringutil.ContainsCaseInsensitive(config.ValidToolTypes, "NotATool")).To(BeFalse())
	})

	It("should return false for empty target", func() {
		Expect(stringutil.ContainsCaseInsensitive(config.ValidToolTypes, "")).To(BeFalse())
	})

	It("should return false for empty slice", func() {
		Expect(stringutil.ContainsCaseInsensitive([]string{}, "Bash")).To(BeFalse())
	})
})
