package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("REST pull request merge", func() {
	DescribeTable("ParsePRMergeEndpoint",
		func(endpoint, wantRepo string, wantNumber int, wantOK bool) {
			repo, number, ok := parser.ParsePRMergeEndpoint(endpoint)
			Expect(ok).To(Equal(wantOK))
			Expect(repo).To(Equal(wantRepo))
			Expect(number).To(Equal(wantNumber))
		},
		Entry("merge endpoint", "repos/o/r/pulls/42/merge", "o/r", 42, true),
		Entry("pull request itself", "repos/o/r/pulls/42", "", 0, false),
		Entry("non-numeric number", "repos/o/r/pulls/x/merge", "", 0, false),
		Entry("zero number", "repos/o/r/pulls/0/merge", "", 0, false),
		Entry("branch merges", "repos/o/r/merges", "", 0, false),
		Entry("missing owner", "repos//r/pulls/1/merge", "", 0, false),
		Entry("extra segment", "repos/o/r/pulls/1/merge/x", "", 0, false),
	)

	It("IsPRMergeRequest needs PUT", func() {
		Expect(parser.IsPRMergeRequest("PUT", "repos/o/r/pulls/1/merge")).To(BeTrue())
		Expect(parser.IsPRMergeRequest("GET", "repos/o/r/pulls/1/merge")).To(BeFalse())
	})

	DescribeTable("ParseRequestFields",
		func(body string, want map[string]string, wantOK bool) {
			fields, ok := parser.ParseRequestFields(body)
			Expect(ok).To(Equal(wantOK))

			if wantOK {
				Expect(fields).To(Equal(want))
			}
		},
		Entry("empty body", "  ", map[string]string{}, true),
		Entry("JSON object keeps string values",
			`{"merge_method":"squash","sha":"abc","n":1}`,
			map[string]string{"merge_method": "squash", "sha": "abc"}, true),
		Entry("invalid JSON", `{"merge_method":`, nil, false),
		Entry("httpie items",
			"merge_method=squash\ncommit_message:=\"body\"",
			map[string]string{"merge_method": "squash", "commit_message": "body"}, true),
		Entry("httpie query item is not a body field", "page==2", nil, false),
		Entry("non-string JSON item", "draft:=true", nil, false),
		Entry("free text", "hello world", nil, false),
	)
})
