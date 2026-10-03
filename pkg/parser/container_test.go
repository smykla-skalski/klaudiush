package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Container --entrypoint", func() {
	resolver := fakeResolver{
		env: map[string]string{"EP": "git"},
		programs: map[string]parser.Program{
			"mygit": parser.ProgramMissing,
		},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	forcePushes := func(command string) []*parser.GitCommand {
		var pushes []*parser.GitCommand

		for _, cmd := range parse(command).GitOperations {
			gitCmd, err := parser.ParseGitCommand(cmd)
			if err == nil && gitCmd.Subcommand == "push" && gitCmd.HasFlag("--force") {
				pushes = append(pushes, gitCmd)
			}
		}

		return pushes
	}

	DescribeTable(
		"runs git as the entrypoint whatever the image",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(forcePushes(command)).NotTo(BeEmpty(), "no git push --force in %q", command)
		},
		Entry("docker run", "docker run --entrypoint git alpine push --force origin main"),
		Entry("attached path", "docker run --entrypoint=/usr/bin/git alpine push --force"),
		Entry("single-quoted", "docker run --entrypoint '/usr/bin/git' alpine push --force"),
		Entry("double-quoted attached", `docker run "--entrypoint=git" alpine push --force`),
		Entry("docker container run", "docker container run --entrypoint git img push --force"),
		Entry("docker create", "docker create --entrypoint git img push --force"),
		Entry("podman", "podman run --rm -it --entrypoint git -v .:/w -w /w img push --force"),
		Entry("nerdctl", "nerdctl run -d --entrypoint git img push --force"),
		Entry("docker-compose", "docker-compose run --entrypoint git svc push --force"),
		Entry("docker compose with global options",
			"docker compose -f c.yml run -T --rm --entrypoint git svc push --force"),
		Entry(
			"docker global options",
			"docker --context run -D run --entrypoint git img push --force",
		),
		Entry(
			"options after the entrypoint",
			"docker run --entrypoint git -e A=1 -p 80:80 img push --force",
		),
		Entry("short clusters", "docker run -itd -eA=1 -ue --entrypoint git img push --force"),
		Entry("end of options", "docker run --entrypoint git -- img push --force"),
		Entry("an empty value dropped", `docker run --name "" --entrypoint git img push --force`),
		Entry("an image that is not git", "docker run --entrypoint git ubuntu:24.04 push --force"),
		Entry("a JSON array", `podman run --entrypoint '["git","push"]' img --force`),
		Entry("compose shell words", `docker compose run --entrypoint "git push" svc --force`),
		Entry("a shell entrypoint", `docker run --entrypoint sh img -c 'git push --force'`),
		Entry(
			"a joined entrypoint",
			"nerdctl run --entrypoint sh --entrypoint -c img 'git push --force'",
		),
		Entry("a name nothing on disk runs", "docker run --entrypoint mygit img push --force"),
		Entry("a variable from the line", "X=git; docker run --entrypoint $X img push --force"),
		Entry("a variable from the environment", `docker run --entrypoint "$EP" img push --force`),
		Entry("under sudo", "sudo docker run --entrypoint git img push --force"),
		Entry(
			"under an unknown runner",
			"mise exec -- docker run --entrypoint git img push --force",
		),
		Entry("docker by path", "/usr/local/bin/docker run --entrypoint git img push --force"),
		Entry("inside a script line", `tmux new 'docker run --entrypoint git img push --force'`),
	)

	It("hands the arguments after the image to git", func() {
		pushes := forcePushes("docker run --rm --entrypoint git alpine push --force origin main")

		Expect(pushes).To(HaveLen(1))
		Expect(pushes[0].Args).To(Equal([]string{"origin", "main"}))
	})

	DescribeTable(
		"leaves other container commands alone",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(result.GitOperations).To(BeEmpty(), "unexpected git in %q", command)
		},
		Entry("no entrypoint", "docker run alpine push --force"),
		Entry("an entrypoint after the image", "docker run alpine --entrypoint git log"),
		Entry("a shell entrypoint running no git", `docker run --entrypoint sh img -c 'echo hi'`),
		Entry("an empty entrypoint", "docker run --entrypoint= img push --force"),
		Entry("docker exec", "docker exec ctr --entrypoint git log"),
		Entry("no image", "docker run --entrypoint git"),
		Entry("no entrypoint value", "docker run --entrypoint"),
		Entry(
			"an entrypoint that is not plain words",
			`docker run --entrypoint 'echo $(date)' img`,
		),
		Entry("another program", "docker run --entrypoint /bin/true img push --force"),
		Entry("an unrelated program", "make run --entrypoint git img push --force"),
	)

	DescribeTable("fails closed on an entrypoint it cannot resolve",
		func(command, detail string) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(SatisfyAll(
				HaveField("Cause", parser.OpacityUnresolvedWord),
				HaveField("Operation", parser.EntrypointOperation),
				HaveField("Detail", detail),
				HaveField("Origin", ContainElement("docker")),
			)), command)
		},
		Entry("an unknown variable",
			`docker run --entrypoint "$UNSET" img push --force`, parser.DetailWordVariable),
		Entry("an attached unknown variable",
			`docker run --entrypoint=$UNSET img push --force`, parser.DetailWordVariable),
		Entry("command output",
			`docker run --entrypoint "$(which git)" img push --force`, parser.DetailWordOutput),
		Entry("attached command output",
			`docker run --entrypoint=$(echo git) img push --force`, parser.DetailWordOutput),
		Entry(
			"a variable assigned in a loop",
			`for x in git; do docker run --entrypoint $x img push; done`,
			parser.DetailWordVariable,
		),
		Entry("under sudo",
			`sudo docker run --entrypoint "$UNSET" img push`, parser.DetailWordVariable),
	)
})
