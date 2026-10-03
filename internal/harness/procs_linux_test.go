package harness_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
)

var _ = Describe("parseStat", func() {
	DescribeTable("reads state, parent and session after the command name",
		func(stat string, want []string, ok bool) {
			fields, parsed := harness.ParseStat([]byte(stat))
			Expect(parsed).To(Equal(ok))
			Expect(fields).To(Equal(want))
		},
		Entry("plain", "42 (sleep) S 7 42 42 0 -1", []string{"S", "7", "42", "42"}, true),
		Entry("name with spaces and parentheses", "42 (a ) b) Z 1 2 3 4",
			[]string{"Z", "1", "2", "3"}, true),
		Entry("no closing parenthesis", "42 (sleep S 7 42 42", nil, false),
		Entry("too few fields", "42 (sleep) S 7", nil, false),
	)
})
