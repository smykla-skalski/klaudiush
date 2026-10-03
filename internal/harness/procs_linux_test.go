package harness_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
)

// tail fills stat fields 6 to 22; field 22 (the start time) is 777.
const tail = " 0 -1 4194304 0 0 0 0 0 0 0 0 20 0 1 0 777"

func statFields(state, ppid, pgrp, sid string) []string {
	fields := strings.Fields(state + " " + ppid + " " + pgrp + " " + sid + tail)

	return fields[:20]
}

var _ = Describe("parseStat", func() {
	DescribeTable("reads state, parent and session after the command name",
		func(stat string, want []string, ok bool) {
			fields, parsed := harness.ParseStat([]byte(stat))
			Expect(parsed).To(Equal(ok))
			Expect(fields).To(Equal(want))

			if ok {
				Expect(fields[len(fields)-1]).To(Equal("777"), "start time")
			}
		},
		Entry("plain", "42 (sleep) S 7 42 42"+tail, statFields("S", "7", "42", "42"), true),
		Entry("name with spaces and parentheses", "42 (a ) b) Z 1 2 3"+tail,
			statFields("Z", "1", "2", "3"), true),
		Entry("no closing parenthesis", "42 (sleep S 7 42 42", nil, false),
		Entry("too few fields", "42 (sleep) S 7 42 42 0 -1", nil, false),
	)
})
