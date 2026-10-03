package parser_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// Timing bounds are relative to echo given the same words, so a slow or
// race-instrumented run scales both; a pass per runner name over the rest of
// the arguments (quadratic) is hundreds of times slower than echo.
const (
	maxLinearFactor = 50
	minLinearBound  = 500 * time.Millisecond
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
		Entry("an empty entrypoint replaced",
			`docker run --entrypoint '' --entrypoint git img push --force`),
		Entry("an empty entrypoint before options",
			`docker run --entrypoint '' -e A=1 --entrypoint git img push --force`),
		Entry("an empty compose entrypoint replaced",
			`docker compose run --entrypoint "" --entrypoint git svc push --force`),
		Entry(
			"a value starting with a dash",
			"docker run --name -e --entrypoint git img push --force",
		),
		Entry("an unknown option taking a value",
			"podman run --hosts-file /x --entrypoint git img push --force"),
		Entry("unknown nerdctl options",
			"nerdctl run --verify none --cosign-key k --entrypoint git img push --force"),
		Entry("a dashed value after an entrypoint",
			"docker run --entrypoint git --name -w img push --force"),
		Entry("a compose v1 abbreviation", "docker-compose run --e git svc push --force"),
		Entry("an attached compose v1 abbreviation",
			"docker-compose run --en=git svc push --force"),
		Entry("a compose abbreviation", "podman-compose run --entry git svc push --force"),
		Entry("an attached compose abbreviation", "docker-compose run --ent=git svc push --force"),
		Entry("a resolved variable holding options",
			`X="--entrypoint git"; docker run $X img push --force`),
		Entry("a resolved option before the entrypoint",
			"OPTS=--rm; docker run $OPTS --entrypoint git img push --force"),
		Entry("a quoted variable with spaces as an option value",
			`V="A=1 B"; docker run -e "$V" --entrypoint git img push --force`),
		Entry("an unknown runner with a line variable",
			`X="--entrypoint git"; foo docker run $X img push --force`),
		Entry("a quoted single compose word",
			`docker compose run --entrypoint "'python3'" svc -c `+
				`"__import__('os').system('git push --force')"`),
		Entry("mixed quoting of resolved variables",
			`V="A=1 B"; X="--entrypoint git"; docker run -e "$V" $X img push --force`),
		Entry("a global option value named run before compose",
			"docker --context run compose run --entrypoint git app push --force"),
		Entry(
			"a compose project named run with a file option",
			"docker compose --project-name run -f compose.yml run --entrypoint=git svc push --force",
		),
		Entry("a runner from a variable", `"$DOCKER" run --entrypoint git img push --force`),
		Entry("apple container", "container run --entrypoint git img push --force"),
		Entry("docker.exe", "docker.exe run --entrypoint git img push --force"),
		Entry("env as the entrypoint", "docker run --entrypoint env img git push --force"),
		Entry("an unknown option standing alone",
			"docker run --future-bool --entrypoint git img push --force"),
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

	DescribeTable("fails closed quickly on entrypoints that fan out",
		func(command string) {
			start := time.Now()
			result := parse(command)

			Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second))
			Expect(result.Truncated).To(BeTrue())
		},
		Entry("nested runners", strings.Repeat("docker run --entrypoint docker run ", 1000)),
		Entry("nested images", strings.Repeat("docker run --entrypoint docker img ", 1000)),
		Entry("many entrypoints", "docker run"+strings.Repeat(" --entrypoint -a", 6000)+" i"),
		Entry("many empty entrypoints",
			"docker run"+strings.Repeat(` --entrypoint ""`, 6000)+" --entrypoint git i push"),
		Entry("many run words", "docker "+strings.Repeat("run --a ", 3000)+"--entrypoint git i"),
	)

	DescribeTable("scans many runner names in time linear in the arguments",
		func(words string) {
			timed := func(command string) time.Duration {
				start := time.Now()
				result := parse(command)
				elapsed := time.Since(start)

				Expect(result).NotTo(BeNil())

				return elapsed
			}

			baseline := timed("echo " + words)
			limit := max(maxLinearFactor*baseline, minLinearBound)

			Expect(timed("foo " + words)).To(BeNumerically("<", limit))
		},
		Entry("runner names", strings.Repeat("docker ", 10000)),
		Entry("runner names before a run", strings.Repeat("docker ", 10000)+"run --entrypoint"),
		Entry("variables", strings.Repeat("$A ", 10000)+"run"),
	)

	It("leaves a quoted script's variables to the container's shell", func() {
		result := parse(
			`SUB=status; docker run --entrypoint=sh alpine -c 'SUB=push; git ${SUB} --force'`,
		)

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities).To(ContainElement(SatisfyAll(
			HaveField("Operation", "git"),
			HaveField("Origin", HaveExactElements("docker", "sh")),
		)))

		for _, op := range result.GitOperations {
			Expect(op.Args).NotTo(ContainElement("status"))
		}
	})

	It("reads a variable holding the entrypoint both quoted and split", func() {
		command := `EP="git push"; docker run --entrypoint "$EP" img --force`

		Expect(forcePushes(command)).NotTo(BeEmpty())
	})

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
		Entry("a substituted option value",
			`docker run -v "$(pwd)":/w -e "X=$HOME" --entrypoint python img app.py`),
		Entry("an unknown image without an entrypoint", `docker run --rm "$IMG" push --force`),
		Entry("run words in the container's command line",
			"docker run --entrypoint /bin/echo alpine run --entrypoint git img push --force"),
		Entry("run words after a plain image",
			"docker run --rm alpine run --entrypoint git img push --force"),
		Entry("a glob in the container's command line",
			"docker run --rm --entrypoint ls alpine *.go"),
		Entry("an image tag from a variable",
			"docker run --rm --entrypoint /app/server myrepo/app:$TAG --port 80"),
		Entry("env options and an unknown image",
			`docker run --env A=1 --expose 80 "$IMG" push --force`),
		Entry("many unknown options without an entrypoint",
			"docker run --a1 x --a2 x --a3 x --a4 x --a5 x --a6 x --a7 x img push --force"),
		Entry(
			"an entrypoint that is not plain words",
			`docker run --entrypoint 'echo $(date)' img`,
		),
		Entry("another program", "docker run --entrypoint /bin/true img push --force"),
		Entry("an unrelated program", "make run --entrypoint git img push --force"),
	)

	DescribeTable(
		"fails closed on an entrypoint it cannot resolve",
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
		Entry("an unknown variable before the entrypoint",
			"docker run $OPTS --entrypoint git img push --force", parser.DetailWordVariable),
		Entry("command output before the entrypoint",
			"docker run $(echo --rm) --entrypoint git img push --force", parser.DetailWordOutput),
		Entry("a brace expansion naming the entrypoint",
			"docker run {--entrypoint,git} img push --force", parser.DetailWordOutput),
		Entry("a brace expansion in the entrypoint",
			"docker run --entrypoint={x,git} img push --force", parser.DetailWordOutput),
		Entry(
			"an unknown image",
			`docker run --entrypoint git "$IMG" push`,
			parser.DetailWordVariable,
		),
		Entry("a JSON array naming an unknown variable",
			`docker run --entrypoint "[\"$X\"]" img push`, parser.DetailWordVariable),
		Entry("too many entrypoints",
			"docker run"+strings.Repeat(" --entrypoint -a", 10)+" img push",
			parser.DetailEntrypointOptions),
		Entry("a glob in an option's place",
			"docker run *rm --entrypoint git img push --force", parser.DetailWordOutput),
		Entry("a glob naming the entrypoint option",
			"docker run --entrypoin* git img push --force", parser.DetailWordOutput),
		Entry("a glob in the image's place",
			"docker run --entrypoint git * push --force", parser.DetailWordOutput),
		Entry("too many variables holding several words",
			`A="a b"; docker run -e $A -e $A -e $A -e $A -e $A --entrypoint git img push`,
			parser.DetailEntrypointOptions),
		Entry("too many run words",
			"docker run --name run --name run --name run --name run --name run "+
				"--name run --name run --name run --entrypoint git img push",
			parser.DetailEntrypointOptions),
		Entry(
			"too many options of unknown arity",
			"docker run --a1 --a2 --a3 --a4 --a5 --a6 --a7 --a8 --a9 --a10 --entrypoint git img push",
			parser.DetailEntrypointOptions,
		),
	)
})
