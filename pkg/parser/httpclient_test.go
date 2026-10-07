package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("ParseHTTPClientCommands", func() {
	parseFirst := func(command, name string) parser.Command {
		GinkgoHelper()

		result, err := parser.NewBashParser().Parse(command)
		Expect(err).NotTo(HaveOccurred())

		commands := result.GetCommands(name)
		Expect(commands).NotTo(BeEmpty())

		return commands[0]
	}

	parseOne := func(command, name string) *parser.HTTPRequest {
		GinkgoHelper()

		requests := parser.ParseHTTPClientCommands(parseFirst(command, name))
		Expect(requests).To(HaveLen(1))

		return requests[0]
	}

	It("records httpie query items and keeps them in the body", func() {
		req := parseOne(`http PUT https://api.github.com/x a=1 b==2 b==3 c:=4 'd\==5'`, "http")
		Expect(req.Query).To(Equal(map[string][]string{"b": {"2", "3"}}))
		Expect(req.Body).To(Equal("a=1\nb==2\nb==3\nc:=4\nd\\==5"))
		Expect(req.DataBody).To(Equal("a=1\nc:=4\nd\\==5"))
	})

	It("keeps a raw body intact in DataBody", func() {
		req := parseOne(
			"http PUT https://api.github.com/x page==1 --raw '{\"a\":\n\"b\", \"x==\":\n\"c\"}'",
			"http",
		)
		Expect(req.DataBody).To(Equal("{\"a\":\n\"b\", \"x==\":\n\"c\"}"))
	})

	DescribeTable(
		"resolves the method and URL",
		func(command, tool, wantMethod, wantURL string) {
			req := parseOne(command, tool)
			Expect(req.Method).To(Equal(wantMethod))
			Expect(req.URL).To(Equal(wantURL))
		},
		Entry(
			"curl defaults to GET",
			"curl https://api.github.com/repos/o/r/contents/x",
			"curl", "GET", "https://api.github.com/repos/o/r/contents/x",
		),
		Entry(
			"curl -X before the URL",
			"curl -X PUT https://api.github.com/repos/o/r/contents/x",
			"curl", "PUT", "https://api.github.com/repos/o/r/contents/x",
		),
		Entry(
			"curl -X after the URL",
			"curl https://api.github.com/repos/o/r/contents/x -X DELETE",
			"curl", "DELETE", "https://api.github.com/repos/o/r/contents/x",
		),
		Entry(
			"curl with the method attached",
			"curl -XPATCH https://api.github.com/repos/o/r/contents/x",
			"curl", "PATCH", "https://api.github.com/repos/o/r/contents/x",
		),
		Entry(
			"curl --data implies POST",
			`curl --data '{"a":1}' https://api.github.com/repos/o/r/merges`,
			"curl", "POST", "https://api.github.com/repos/o/r/merges",
		),
		Entry(
			"curl --upload-file implies PUT",
			"curl -T body.json https://api.github.com/repos/o/r/contents/x",
			"curl", "PUT", "https://api.github.com/repos/o/r/contents/x",
		),
		Entry(
			"curl --url carries the URL",
			"curl -X PUT --url https://api.github.com/repos/o/r/contents/x",
			"curl", "PUT", "https://api.github.com/repos/o/r/contents/x",
		),
		Entry(
			"curl header value is not mistaken for the URL",
			`curl -H "Accept: application/json" https://api.github.com/repos/o/r`,
			"curl", "GET", "https://api.github.com/repos/o/r",
		),
		Entry(
			"wget --method",
			"wget --method=PUT --body-data=x https://api.github.com/repos/o/r/contents/x",
			"wget", "PUT", "https://api.github.com/repos/o/r/contents/x",
		),
		Entry(
			"wget --post-data implies POST",
			"wget --post-data=x https://api.github.com/repos/o/r/merges",
			"wget", "POST", "https://api.github.com/repos/o/r/merges",
		),
		Entry(
			"httpie takes the method as a positional",
			"http PUT https://api.github.com/repos/o/r/contents/x message=y",
			"http", "PUT", "https://api.github.com/repos/o/r/contents/x",
		),
		Entry(
			"httpie request items imply POST",
			"http https://api.github.com/repos/o/r/merges base=main head=topic",
			"http", "POST", "https://api.github.com/repos/o/r/merges",
		),
		Entry(
			"xh takes the method as a positional",
			"xh DELETE https://api.github.com/repos/o/r/contents/x",
			"xh", "DELETE", "https://api.github.com/repos/o/r/contents/x",
		),
	)

	It("records an inline body", func() {
		req := parseOne(
			`curl -X POST https://api.github.com/graphql -d '{"query":"mutation {}"}'`, "curl",
		)
		Expect(req.Body).To(ContainSubstring("mutation"))
	})

	It("follows curl's @path form to a body file", func() {
		req := parseOne("curl -X POST https://api.github.com/graphql -d @query.json", "curl")
		Expect(req.BodyFile).To(Equal("query.json"))
	})

	It("treats -d @- as stdin rather than a file named -", func() {
		req := parseOne("curl -X POST https://api.github.com/graphql -d @-", "curl")
		Expect(req.BodyFile).To(BeEmpty())
	})

	It("reports no request when the command carries no URL", func() {
		Expect(parser.ParseHTTPClientCommands(parseFirst("curl --version", "curl"))).To(BeEmpty())
	})

	It("reports no request for a command that is not a client", func() {
		Expect(parser.ParseHTTPClientCommands(
			parser.Command{Name: "git", Args: []string{"status"}},
		)).To(BeEmpty())
	})

	Describe("several requests in one command", func() {
		It("returns one request per URL, sharing the segment's options", func() {
			requests := parser.ParseHTTPClientCommands(parseFirst(
				"curl -X PUT https://example.com/a https://api.github.com/repos/o/r/contents/x",
				"curl",
			))

			Expect(requests).To(HaveLen(2))
			Expect(requests[0].URL).To(Equal("https://example.com/a"))
			Expect(requests[1].URL).To(Equal("https://api.github.com/repos/o/r/contents/x"))
			Expect(requests[1].Method).To(Equal("PUT"))
		})

		It("gives each --next segment its own options", func() {
			requests := parser.ParseHTTPClientCommands(parseFirst(
				"curl https://example.com/a --next -X DELETE https://api.github.com/repos/o/r/x",
				"curl",
			))

			Expect(requests).To(HaveLen(2))
			Expect(requests[0].Method).To(Equal("GET"))
			Expect(requests[1].Method).To(Equal("DELETE"))
		})
	})

	Describe("SplitURL", func() {
		DescribeTable(
			"separates host from path",
			func(raw, wantHost, wantPath string) {
				host, path := parser.SplitURL(raw)
				Expect(host).To(Equal(wantHost))
				Expect(path).To(Equal(wantPath))
			},
			Entry(
				"https URL",
				"https://api.github.com/repos/o/r", "api.github.com", "/repos/o/r",
			),
			Entry(
				"http URL",
				"http://ghe.example.com/api/v3/repos/o/r", "ghe.example.com", "/api/v3/repos/o/r",
			),
			Entry("host with no path", "https://api.github.com", "api.github.com", ""),
			Entry("bare path", "repos/o/r/contents/x", "", "repos/o/r/contents/x"),
			Entry(
				"upper-case scheme and host",
				"HTTPS://API.GITHUB.COM/repos/o/r", "api.github.com", "/repos/o/r",
			),
			Entry(
				"userinfo is dropped",
				"https://x-token:secret@api.github.com/repos/o/r", "api.github.com", "/repos/o/r",
			),
			Entry(
				"port is dropped",
				"https://api.github.com:443/repos/o/r", "api.github.com", "/repos/o/r",
			),
		)
	})

	Describe("SplitRequestURL", func() {
		It("supplies the scheme a client would add", func() {
			host, path := parser.SplitRequestURL("api.github.com/repos/o/r/contents/x")
			Expect(host).To(Equal("api.github.com"))
			Expect(path).To(Equal("/repos/o/r/contents/x"))
		})

		It("leaves a path that names no host alone", func() {
			host, path := parser.SplitRequestURL("/repos/o/r/contents/x")
			Expect(host).To(BeEmpty())
			Expect(path).To(Equal("/repos/o/r/contents/x"))
		})

		It("leaves a relative path alone", func() {
			host, path := parser.SplitRequestURL("repos/o/r/contents/x")
			Expect(host).To(BeEmpty())
			Expect(path).To(Equal("repos/o/r/contents/x"))
		})
	})

	Describe("FindAPICallsInText", func() {
		It("finds an octokit request call and marks it explicit", func() {
			calls := parser.FindAPICallsInText(
				`await octokit.request("PUT /repos/{owner}/{repo}/contents/{path}", opts)`,
			)
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Method).To(Equal("PUT"))
			Expect(calls[0].URL).To(Equal("/repos/{owner}/{repo}/contents/{path}"))
			Expect(calls[0].ExplicitAPICall).To(BeTrue())
		})

		It("finds a verb-named client method and leaves it implicit", func() {
			calls := parser.FindAPICallsInText(
				`axios.put("https://api.github.com/repos/o/r/contents/x", body)`,
			)
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Method).To(Equal("PUT"))
			Expect(calls[0].ExplicitAPICall).To(BeFalse())
		})

		It("finds nothing in text with no call", func() {
			Expect(parser.FindAPICallsInText("PUT /repos/o/r/contents/x is blocked")).To(BeEmpty())
		})
	})

	Describe("FindCallsInText", func() {
		calls := []string{"repos.createOrUpdateFileContents", "pulls.merge"}

		It("matches a name in call form", func() {
			Expect(
				parser.FindCallsInText(`octokit.rest.repos.createOrUpdateFileContents({})`, calls),
			).
				To(Equal("repos.createOrUpdateFileContents"))
		})

		It("ignores a name that is only mentioned", func() {
			Expect(
				parser.FindCallsInText("see repos.createOrUpdateFileContents", calls),
			).To(BeEmpty())
		})
	})
})
