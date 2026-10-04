package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// cwdResolver is a fakeResolver that knows the directory commands start in.
type cwdResolver struct {
	fakeResolver

	cwd string
}

func (r cwdResolver) WorkingDir() (string, bool) {
	return r.cwd, r.cwd != ""
}

var _ = Describe("Single-word command output in launcher options", func() {
	parseIn := func(cwd, command string) *parser.ParseResult {
		resolver := cwdResolver{cwd: cwd}

		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	DescribeTable("reads known one-word output as one word",
		func(command string) {
			Expect(parseIn("/home/u/proj", command).Truncated).To(BeFalse(), command)
		},
		Entry("pwd in a volume", "docker run -v $(pwd):/w img make"),
		Entry("PWD in a volume", "docker run --rm -v $PWD:/w -w /w img make"),
		Entry("pwd after a literal cd", "cd /srv/app && docker run -v $(pwd):/w img ls"),
		Entry("pwd after a relative cd", "cd sub && podman run -v $(pwd):/w img ls"),
		Entry("user and group ids", "docker run -u $(id -u):$(id -g) img ls"),
		Entry("a user name", "docker run --user $(id -un) img ls"),
		Entry("a user name in an image tag", "docker run --rm img:$(id -un) ls"),
		Entry("compose run", "docker compose run -u $(id -u) -v $(pwd):/w svc ls"),
		Entry("parallel jobs from nproc", "parallel -j $(nproc) gzip ::: a"),
		Entry("parallel jobs from getconf",
			"parallel -j $(getconf _NPROCESSORS_ONLN) gzip ::: a"),
	)

	DescribeTable("fails closed on output that may not be one word",
		func(cwd, command string) {
			Expect(parseIn(cwd, command).Truncated).To(BeTrue(), command)
		},
		Entry("pwd with an argument", "/home/u", "docker run -v $(pwd x):/w img ls"),
		Entry("pwd with an option", "/home/u", "docker run -v $(pwd -P):/w img ls"),
		Entry("pwd with a redirect", "/home/u", "docker run -v $(pwd 2>&1):/w img ls"),
		Entry("pwd with an assignment", "/home/u", "docker run -v $(X=1 pwd):/w img ls"),
		Entry("two commands", "/home/u", "docker run -v $(cd /; pwd):/w img ls"),
		Entry("a pwd function", "/home/u",
			"pwd() { echo 'a --entrypoint=git b'; }; docker run -v $(pwd):/w img push"),
		Entry("a pwd alias", "/home/u",
			"alias pwd='echo a b'; docker run -v $(pwd):/w img push"),
		Entry("a start directory with spaces", "/home/u/my proj",
			"docker run -v $(pwd):/w img push"),
		Entry("a cd to a directory with spaces", "/home/u",
			`cd "/tmp/a --entrypoint=git b" && docker run -v $(pwd):/w img push`),
		Entry("a cd to a glob directory", "/home/u", `cd "/tmp/*" && docker run -v $PWD:/w img`),
		Entry("a cd to an unknown directory", "/home/u",
			`cd "$D" && docker run -v $(pwd):/w img push`),
		Entry("an unknown start directory", "", "docker run -v $(pwd):/w img push"),
		Entry("PWD read from input", "/home/u", "read PWD; docker run -v $PWD:/w img push"),
		Entry("a changed IFS", "/home/u", "IFS=/; docker run -v $(pwd):/w img push"),
		Entry("another id option", "/home/u", "docker run -u $(id -G) img ls"),
		Entry("an nproc option", "/home/u", "parallel -j $(nproc --all) gzip ::: a"),
		Entry("other command output", "/home/u", "docker run -e $(cat f) img ls"),
		Entry("output as the image", "/home/u", "docker run --rm $(id -un) ls"),
		Entry("output in a parallel command word", "/home/u", "parallel echo $(nproc) ::: a"),
	)
})
