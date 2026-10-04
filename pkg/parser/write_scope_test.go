package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Writes to an unknown target within a work tree", func() {
	resolver := fakeResolver{
		env: map[string]string{"PWD": "/repo/sub", "HOME": "/home/u"},
		files: map[string]string{
			"/repo/gradlew": "exec java -jar wrapper.jar\n",
			"/s/a.sh":       "echo hi\n",
		},
		outputs: map[string]string{"git rev-parse --show-toplevel": "/repo"},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	DescribeTable("blocks a shell script inside the work tree",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue())
			Expect(result.Opacities[0].Detail).To(Equal(parser.DetailScriptUnplacedWrite))
		},
		Entry("pull then gradlew", "git pull && /repo/gradlew build"),
		Entry("pull then a relative script", "git pull && bash ../gradlew"),
		Entry("stash then source", "git stash && source /repo/gradlew"),
		Entry("checkout with -C", "git -C /repo checkout main && bash /repo/gradlew"),
	)

	DescribeTable("allows what the work tree change cannot reach",
		func(command string) {
			Expect(parse(command).Truncated).To(BeFalse())
		},
		Entry("a script outside the work tree", "git pull && bash /s/a.sh"),
		Entry("a program klaudiush does not follow", "git pull && make test"),
		Entry("a non-shell interpreter", "git pull && python3 tools/x.py"),
		Entry("checkout -b", "git checkout -b feat && bash /repo/gradlew"),
	)

	It("records the work tree as the scope", func() {
		result := parse("git pull")

		Expect(result.FileWrites).To(HaveLen(1))
		Expect(result.FileWrites[0].Scope).To(Equal("/repo"))
	})

	It("leaves the scope open when the work tree is unknown", func() {
		bare := fakeResolver{env: map[string]string{"PWD": "/repo"}}

		result, err := parser.NewBashParserWithResolver(bare).
			Parse("git pull && bash /s/a.sh")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Truncated).To(BeTrue())
	})

	It("leaves the scope open for --work-tree", func() {
		result := parse("git --work-tree=/x reset --hard")

		Expect(result.FileWrites[0].Scope).To(BeEmpty())
	})

	It("anchors archive scopes on the command's directory", func() {
		result := parse("cd /out && tar -C sub -xf a.tar && unzip -d ~/u b.zip")

		Expect(result.FileWrites).To(HaveLen(2))
		Expect(result.FileWrites[0].Scope).To(Equal("/out/sub"))
		Expect(result.FileWrites[1].Scope).To(Equal("/home/u/u"))
	})

	It("leaves the scope open for a repeated or computed directory", func() {
		result := parse(`unzip -d a -d b x.zip; tar -C "$(echo d)" -xf a.tar`)

		Expect(result.FileWrites).To(HaveLen(2))
		Expect(result.FileWrites[0].Scope).To(BeEmpty())
		Expect(result.FileWrites[1].Scope).To(BeEmpty())
	})
})
