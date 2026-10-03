package harness_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
)

var _ = Describe("parseProcArgs", func() {
	DescribeTable("reads the environment after the arguments",
		func(raw string, want []string) {
			Expect(harness.ParseProcArgs([]byte(raw))).To(Equal(want))
		},
		Entry("arguments then environment",
			"\x02\x00\x00\x00/bin/sleep\x00\x00\x00sleep\x00300\x00HOME=/h\x00A=b\x00\x00junk",
			[]string{"HOME=/h", "A=b"}),
		Entry("no environment", "\x01\x00\x00\x00/bin/x\x00x\x00", nil),
		Entry("truncated header", "\x01\x00", nil),
	)
})
