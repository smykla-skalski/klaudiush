package parser_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Container exec", func() {
	resolver := fakeResolver{env: map[string]string{"CTR": "web", "SPLIT": "web git"}}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	gitCalls := func(result *parser.ParseResult) []string {
		calls := make([]string, 0, len(result.GitOperations))
		for _, op := range result.GitOperations {
			calls = append(calls, strings.Join(append([]string{op.Name}, op.Args...), " "))
		}

		return calls
	}

	programs := func(result *parser.ParseResult) []string {
		names := make([]string, 0, len(result.Commands))
		for _, cmd := range result.Commands {
			names = append(names, cmd.Name)
		}

		return names
	}

	DescribeTable("validates the program exec runs",
		func(command, want string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(gitCalls(result)).To(ContainElement(want), command)
		},
		Entry("docker exec", "docker exec c git push --force", "git push --force"),
		Entry("options before the container",
			"docker exec -it -u root -w /w -e A=1 --env-file .env c git push --force",
			"git push --force"),
		Entry("attached values", "docker exec -uroot --user=root -eA=1 c git push",
			"git push"),
		Entry("bool options", "docker exec -d --privileged --interactive --tty c git push",
			"git push"),
		Entry("end of options", "docker exec -- c git push", "git push"),
		Entry("end of options after the container", "docker exec c -- git push", "git push"),
		Entry("docker global options",
			"docker --context x -H unix:///s -D --tls exec c git push", "git push"),
		Entry("a global option valued exec", "docker -c 'exec' exec c git push", "git push"),
		Entry("an attached global option", "docker --context=x exec c git push", "git push"),
		Entry("docker container exec", "docker container exec c git push", "git push"),
		Entry("docker compose exec",
			"docker compose -f c.yml -p proj exec -T --index 2 svc git push", "git push"),
		Entry("docker-compose exec", "docker-compose exec svc git push", "git push"),
		Entry("podman exec", "podman exec --preserve-fds 2 c git push", "git push"),
		Entry("podman --latest", "podman exec --latest git push", "git push"),
		Entry("podman -l in a cluster", "podman exec -itl git push", "git push"),
		Entry("podman --latest=true", "podman exec --latest=true git push", "git push"),
		Entry("podman --latest=false", "podman exec --latest=false c git push", "git push"),
		Entry("podman global options", "podman --remote --connection x exec c git push",
			"git push"),
		Entry("nerdctl exec", "nerdctl -n k8s.io exec c git push", "git push"),
		Entry("apple container exec", "container exec c git push", "git push"),
		Entry("docker.exe", "docker.exe exec c git push", "git push"),
		Entry("docker by path", "/usr/bin/docker exec c git push", "git push"),
		Entry("an unknown option read both ways", "docker exec --future c git push",
			"git push"),
		Entry("an unknown short option read both ways", "docker exec -Z c git push",
			"git push"),
		Entry("a dashed option value read both ways", "docker exec -e -x c git push",
			"git push"),
		Entry("a shell in the container", `docker exec c sh -c 'git push'`, "git push"),
		Entry("nested exec", "docker exec c docker exec d git push", "git push"),
		Entry("under sudo", "sudo docker exec c git push", "git push"),
		Entry("under an unknown runner", "foo docker exec c git push", "git push"),
		Entry("inside a script line", `tmux new 'docker exec c git push'`, "git push"),
		Entry("a container from the line", "C=web; docker exec $C git push", "git push"),
		Entry("a container from the environment", `docker exec "$CTR" git push`, "git push"),
		Entry("options from the line", `X="-u root"; docker exec $X c git push`, "git push"),
		Entry("a variable splitting into container and program", "docker exec $SPLIT push",
			"git push"),
		Entry("an empty option value", `docker exec -e "" c git push --force`,
			"git push --force"),
		Entry("an empty user", `docker exec -u '' c git push --force`, "git push --force"),
		Entry("an empty workdir", `docker exec --workdir '' c git push`, "git push"),
	)

	DescribeTable("follows only the program exec runs",
		func(command, program string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(result.GitOperations).To(BeEmpty(), command)
			Expect(programs(result)).To(ContainElement(program), command)
		},
		Entry("echo", "docker exec c echo git commit -m x", "echo"),
		Entry("echo after options", "docker exec -it -uroot -e A=1 c echo git push", "echo"),
		Entry("echo after a separate value", "docker exec -u root c echo git push", "echo"),
		Entry("a run word in the payload",
			"docker exec c echo run --entrypoint git img push", "echo"),
		Entry("compose", "docker compose exec svc echo git push", "echo"),
		Entry("docker-compose", "docker-compose exec -T svc printf git push", "printf"),
		Entry("podman --latest", "podman exec --latest echo git push", "echo"),
		Entry("nerdctl", "nerdctl exec c echo git push", "echo"),
		Entry("a container named git", "docker exec git echo push", "echo"),
		Entry("a container named git after a valued option",
			"docker exec -u root git echo push", "echo"),
		Entry("a container named exec", "docker exec 'exec' echo git push", "echo"),
		Entry("under an unknown runner", "foo docker exec c echo git push", "echo"),
		Entry("a container from the line", "C=web; docker exec $C echo git push", "echo"),
	)

	It("records no command for a bare exec", func() {
		for _, command := range []string{"docker exec", "docker exec c", "docker exec -it", "docker exec --"} {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(programs(result)).To(HaveExactElements("docker"), command)
		}
	})

	DescribeTable("fails closed on a container or option it cannot read",
		func(command, detail string) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(SatisfyAll(
				HaveField("Cause", parser.OpacityUnresolvedWord),
				HaveField("Operation", parser.ContainerExecOperation),
				HaveField("Detail", detail),
				HaveField("Origin", HaveExactElements("docker")),
			)), command)
		},
		Entry("an unknown variable", "docker exec $NOPE echo hi", parser.DetailWordVariable),
		Entry("a quoted unknown variable", `docker exec "$NOPE" ls`, parser.DetailWordVariable),
		Entry("command output", "docker exec $(docker ps -q) ls", parser.DetailWordOutput),
		Entry("a glob", "docker exec * ls", parser.DetailWordOutput),
		Entry("a glob in the place of an option", "docker exec -* c ls",
			parser.DetailWordOutput),
		Entry("brace expansion", "docker exec {a,b} ls", parser.DetailWordOutput),
		Entry("an unknown variable as an option", "docker exec -u root ${OPTS} c ls",
			parser.DetailWordVariable),
		Entry("an unknown variable as an option value", "docker exec -u $NOPE git push",
			parser.DetailWordVariable),
		Entry("an unknown variable inside an option value",
			"docker exec -e A=$NOPE c echo git push", parser.DetailWordVariable),
		Entry("command output as an option value", "docker exec -u $(id -u) c git push",
			parser.DetailWordOutput),
		Entry("command output in an attached value",
			"docker exec --user=$(printf 'root c') git push", parser.DetailWordOutput),
		Entry("command output in a short attached value",
			"docker exec -uroot$(printf ' c') git push", parser.DetailWordOutput),
		Entry("a variable after a literal container prefix", "docker exec c$NOPE push",
			parser.DetailWordVariable),
		Entry("a glob after a literal container prefix", "docker exec g* push",
			parser.DetailWordOutput),
		Entry("a bracket glob container", "docker exec c[12] push", parser.DetailWordOutput),
		Entry("a variable after an unknown option", "docker exec --future $NOPE ls",
			parser.DetailWordVariable),
		Entry("a variable after end of options", "docker exec -- $NOPE ls",
			parser.DetailWordVariable),
	)

	It("fails closed when variables split too many ways", func() {
		result := parse(`A="1 2"; B="1 2"; C="1 2"; D="1 2"; E="1 2"; ` +
			`docker exec -e $A -e $B -e $C -e $D -e $E c ls`)

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities).To(ContainElement(SatisfyAll(
			HaveField("Operation", parser.ContainerExecOperation),
			HaveField("Detail", parser.DetailEntrypointOptions),
		)))
	})

	It("fails closed on too many options of unknown arity", func() {
		var opts strings.Builder
		for i := range 40 {
			opts.WriteString(" --o")
			opts.WriteByte(byte('a' + i%26))
			opts.WriteString(strings.Repeat("x", i/26))
			opts.WriteString(" -v")
		}

		result := parse("docker exec" + opts.String() + " c ls")

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities).To(ContainElement(SatisfyAll(
			HaveField("Operation", parser.ContainerExecOperation),
			HaveField("Detail", parser.DetailEntrypointOptions),
		)))
	})

	DescribeTable("scans as before when exec cannot be placed",
		func(command string) {
			result := parse(command)

			Expect(gitCalls(result)).To(ContainElement("git push"), command)
		},
		Entry("a global option of unknown arity", "docker --future x exec c echo git push"),
		Entry("an unknown short global option", "docker -Z exec c echo git push"),
		Entry("a global option from a variable", "docker $NOPE exec c echo git push"),
		Entry("another subcommand first", "docker ps exec c echo git push"),
	)

	It("reads options in time linear in the arguments", func() {
		words := strings.Repeat("--future -x ", 3000) + "c ls"

		start := time.Now()
		result := parse("docker exec " + words)

		Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second))
		Expect(result).NotTo(BeNil())
	})
})
