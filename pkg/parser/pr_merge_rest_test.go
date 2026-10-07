package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("REST pull request merge", func() {
	DescribeTable("ParsePRMergeEndpoint",
		func(endpoint, wantPath string, wantOK bool) {
			prPath, ok := parser.ParsePRMergeEndpoint(endpoint)
			Expect(ok).To(Equal(wantOK))
			Expect(prPath).To(Equal(wantPath))
		},
		Entry("merge endpoint", "repos/o/r/pulls/42/merge", "repos/o/r/pulls/42", true),
		Entry("numeric repository ID alias",
			"repositories/123/pulls/42/merge", "repositories/123/pulls/42", true),
		Entry("non-numeric repository ID", "repositories/x/pulls/42/merge", "", false),
		Entry("pull request itself", "repos/o/r/pulls/42", "", false),
		Entry("non-numeric number", "repos/o/r/pulls/x/merge", "", false),
		Entry("zero number", "repos/o/r/pulls/0/merge", "", false),
		Entry("branch merges", "repos/o/r/merges", "", false),
		Entry("missing owner", "repos//r/pulls/1/merge", "", false),
		Entry("extra segment", "repos/o/r/pulls/1/merge/x", "", false),
		Entry("issues instead of pulls", "repos/o/r/issues/1/merge", "", false),
	)

	It("ParseRequestItemList keeps file items apart", func() {
		fields, files, ok := parser.ParseRequestItemList(
			[]string{"a=1", "b=@f.txt", `c:="@x"`, "d=x\ny"},
		)
		Expect(ok).To(BeTrue())
		Expect(fields).To(Equal(map[string]string{"a": "1", "c": "@x", "d": "x\ny"}))
		Expect(files).To(Equal(map[string]string{"b": "f.txt"}))
	})

	It("QueryFields reads every value of the query string", func() {
		Expect(parser.QueryFields("repos/o/r/pulls/1/merge?merge_method=squash&a=1&a=2")).
			To(Equal(map[string][]string{"merge_method": {"squash"}, "a": {"1", "2"}}))
		Expect(parser.QueryFields("repos/o/r/pulls/1/merge")).To(BeNil())
	})

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
