package parser_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Dynamic launch words", func() {
	resolver := fakeResolver{env: map[string]string{"G": "git", "HOME": "/home/u"}}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	pushes := func(result *parser.ParseResult) int {
		count := 0

		for _, cmd := range result.GitOperations {
			gitCmd, err := parser.ParseGitCommand(cmd)
			if err == nil && gitCmd.Subcommand == "push" {
				count++
			}
		}

		return count
	}

	failsClosed := func(command, operation, detail string, origin string) {
		result := parse(command)

		Expect(result.Truncated).To(BeTrue(), command)
		Expect(result.Opacities).To(ContainElement(SatisfyAll(
			HaveField("Cause", parser.OpacityUnresolvedWord),
			HaveField("Operation", operation),
			HaveField("Detail", detail),
			HaveField("Origin", ContainElement(origin)),
		)), command)
	}

	DescribeTable("fails closed on a container run it cannot read up to the program",
		func(command, operation, detail string) {
			failsClosed(command, operation, detail, "docker")
		},
		Entry("an option from a variable",
			"docker run $O img push", parser.ContainerRunOperation, parser.DetailWordVariable),
		Entry("the subcommand from a variable",
			"docker $SUB img push", parser.ContainerRunOperation, parser.DetailWordVariable),
		Entry("an unknown global option before a variable",
			`docker --foo "$C" img ls`,
			parser.ContainerRunOperation, parser.DetailWordVariable),
		Entry("a global option value that may split",
			"docker -H $(h) run img ls", parser.ContainerRunOperation, parser.DetailWordUnquoted),
		Entry("an unquoted option value",
			"docker run -e $X img ls", parser.ContainerRunOperation, parser.DetailWordUnquoted),
		Entry("an unquoted command output in a volume",
			"docker run --rm -v $(pwd):/w img ls",
			parser.ContainerRunOperation, parser.DetailWordUnquoted),
		Entry("an unquoted attached option value",
			"docker run --name=$N img ls", parser.ContainerRunOperation, parser.DetailWordUnquoted),
		Entry("an option glob",
			"docker run -v *:/w img ls", parser.ContainerRunOperation, parser.DetailWordUnquoted),
		Entry("an unquoted glob image",
			"docker run --rm img* ls", parser.ContainerRunOperation, parser.DetailWordUnquoted),
		Entry(
			"an option brace expansion",
			"docker run -e {A,B}=1 img ls",
			parser.ContainerRunOperation,
			parser.DetailWordUnquoted,
		),
		Entry("an unknown short global option before a variable",
			`docker -Z "$S" img ls`, parser.ContainerRunOperation, parser.DetailWordVariable),
		Entry("an unquoted short global option value",
			"docker -H $H run img ls", parser.ContainerRunOperation, parser.DetailWordUnquoted),
		Entry(
			"an unquoted global option",
			"docker --context=$C run img ls",
			parser.ContainerRunOperation,
			parser.DetailWordUnquoted,
		),
		Entry(
			"an array",
			`docker run "${opts[@]}" img ls`,
			parser.ContainerRunOperation,
			parser.DetailWordVariable,
		),
		Entry("an image from a variable",
			`docker run --rm "$IMG" ls`, parser.ContainerRunOperation, parser.DetailWordVariable),
		Entry("an unquoted image tag",
			"docker run --rm img:$TAG ls", parser.ContainerRunOperation, parser.DetailWordUnquoted),
		Entry("a brace expansion in the image's place",
			"docker run --rm {a,b} ls", parser.ContainerRunOperation, parser.DetailWordOutput),
		Entry("the program from a variable",
			"docker run img $X", parser.ProgramWordOperation, parser.DetailWordVariable),
		Entry("the program from command output",
			"docker run --rm img $(echo git) push",
			parser.ProgramWordOperation, parser.DetailWordOutput),
		Entry("the program from a glob",
			"docker run img g* push", parser.ProgramWordOperation, parser.DetailWordOutput),
		Entry("compose run",
			"docker compose -f c.yml run svc $X",
			parser.ProgramWordOperation, parser.DetailWordVariable),
		Entry("a context named run",
			"docker --context run -D run img $X",
			parser.ProgramWordOperation, parser.DetailWordVariable),
		Entry("too many run words",
			"docker "+strings.Repeat("run ", 10)+"img $X",
			parser.ContainerRunOperation, parser.DetailEntrypointOptions),
	)

	It("names the launcher that ran docker", func() {
		result := parse("sudo docker run img $(echo git) push")

		Expect(result.Opacities).To(ContainElement(SatisfyAll(
			HaveField("Operation", parser.ProgramWordOperation),
			HaveField("Origin", Equal([]string{"sudo", "docker"})),
		)))
	})

	It("follows docker found among another program's arguments", func() {
		failsClosed("ssh host docker run img $X push",
			parser.ProgramWordOperation, parser.DetailWordVariable, "docker")
	})

	It("follows a literal container entrypoint behind ssh", func() {
		result := parse("ssh host docker run --entrypoint git img push --force")

		Expect(result.Truncated).To(BeFalse())
		Expect(pushes(result)).To(BeNumerically(">", 0))
	})

	DescribeTable(
		"fails closed on dynamic container subcommands behind remote launchers",
		func(command string) {
			failsClosed(
				command,
				parser.ContainerRunOperation,
				parser.DetailWordVariable,
				"docker",
			)
		},
		Entry("ssh", "ssh host docker $SUB img push"),
		Entry("quoted ssh", `ssh host 'docker $SUB img push'`),
		Entry("mosh", "mosh host docker $SUB img push"),
		Entry("kubectl exec", "kubectl exec pod -- docker $SUB img push"),
		Entry("kubectl value option",
			"kubectl --certificate-authority ca exec pod -- docker $SUB img push"),
		Entry("oc exec", "oc exec pod -- docker $SUB img push"),
		Entry("gcloud compute ssh", "gcloud compute ssh host -- docker $SUB img push"),
		Entry("gcloud value option",
			"gcloud --impersonate-service-account svc compute ssh host -- docker $SUB img push"),
		Entry("gcloud group value option",
			"gcloud compute --project p ssh host -- docker $SUB img push"),
	)

	DescribeTable(
		"leaves container words outside remote commands alone",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
		},
		Entry("systemctl data", `systemctl restart docker "$SUB"`),
		Entry("ssh option value", `ssh -i docker "$SUB"`),
		Entry("ssh no command", `ssh -N host docker $SUB`),
		Entry("kubectl argument", `kubectl get exec pod -- docker $SUB`),
		Entry("gcloud argument", `gcloud storage compute ssh host -- docker $SUB`),
	)

	It("passes unseen xargs input to git add as {}", func() {
		result := parse("ls -m | xargs git add")

		Expect(result.Truncated).To(BeFalse())
		Expect(result.GitOperations).To(ConsistOf(SatisfyAll(
			HaveField("Name", "git"),
			HaveField("Args", Equal([]string{"add", "{}"})),
		)))
	})

	It("reports an opaque --entrypoint once", func() {
		result := parse("docker run $O --entrypoint git img push")

		Expect(result.Opacities).To(ConsistOf(
			HaveField("Operation", parser.EntrypointOperation),
		))
	})

	DescribeTable("follows the program a variable puts after the image",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(pushes(result)).To(BeNumerically(">", 0), command)
		},
		Entry("the image and program in one variable", `X="img git push"; docker run $X`),
		Entry("the program in a variable from the line", "X=git; docker run img $X push"),
		Entry("the program in a variable from the environment", `docker run img "$G" push`),
		Entry("a literal program", "docker run --rm img git push"),
	)

	DescribeTable("leaves container commands it can read alone",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(result.GitOperations).To(BeEmpty(), command)
		},
		Entry("quoted command output in a volume",
			`docker run --rm -v "$(pwd)":/w -w /w img make test`),
		Entry("quoted option values and image", `docker run -e "T=$T" "img:$TAG" ls`),
		Entry("a variable in the program's arguments", "docker run --rm img echo $X"),
		Entry("a glob in the program's arguments", "docker run --rm img ls *.go"),
		Entry("a quoted glob image", "docker run --rm 'img*' ls"),
		Entry("docker build", "docker build -t img:$TAG ."),
		Entry("a template", "docker ps --format '{{.Names}}'"),
		Entry("docker exec arguments", "docker exec ctr ls $X"),
		Entry("docker among other arguments", "foo docker ps $X"),
		Entry("a global option value", "docker --context prod run --rm img ls"),
		Entry("a quoted global option value", `docker --context "$C" run --rm img ls`),
	)

	DescribeTable(
		"fails closed on a parallel command line it cannot read",
		func(command, detail string) {
			failsClosed(command, parser.ParallelOperation, detail, "parallel")
		},
		Entry("a command from a variable", "parallel $X ::: a", parser.DetailWordVariable),
		Entry("a quoted command word from a variable",
			`parallel echo "$X" ::: a`, parser.DetailWordVariable),
		Entry("a command word from command output",
			"parallel echo $(cmd) ::: a", parser.DetailWordOutput),
		Entry("a command glob", "parallel echo * ::: a", parser.DetailWordUnquoted),
		Entry(
			"an unquoted option value",
			"parallel -j $(cmd) gzip ::: a",
			parser.DetailWordUnquoted,
		),
		Entry("command lines from a variable", "parallel ::: $X", parser.DetailParallelInput),
		Entry("command lines from stdin", "ls | parallel", parser.DetailParallelInput),
		Entry("command lines from a file", "parallel :::: cmds.txt", parser.DetailParallelInput),
		Entry("command lines from an arg file", "parallel -a cmds.txt", parser.DetailParallelInput),
	)

	DescribeTable("fails closed on input filled in where a program or git word goes",
		func(command string) {
			Expect(parse(command).Truncated).To(BeTrue(), command)
		},
		Entry("parallel appending stdin to git", "ls | parallel git"),
		Entry("parallel filling a file into git", "parallel git {} :::: subs.txt"),
		Entry("parallel filling a glob into the program", "parallel {} push ::: g*"),
		Entry("parallel filling perl output into git", `parallel 'git {= $_ =}' ::: a`),
		Entry("parallel positional input past the inputs", "parallel 'git {2}' ::: push"),
		Entry("parallel input from an unknown variable", "parallel git ::: $UNSET"),
		Entry("parallel without separators and stdin", "parallel git"),
		Entry("a quoted container option name", `docker run "--$X" img push`),
		Entry("a quoted container option name after another", `docker run -e A=1 --"$X" img push`),
		Entry("a quoted short container option name", `docker run "-$X" img push`),
		Entry("a quoted global option name", `docker "--$X" run img ls`),

		Entry("parallel --colsep positional", `parallel --colsep , {1} {2} ::: "git,push"`),
		Entry("parallel -C positional", `parallel -C , {1} {2} ::: "git,push"`),
		Entry("parallel --colsep without command", `parallel --colsep , ::: "git,push"`),
		Entry("parallel --plus suffix removal", "parallel --plus {%.x} push ::: git.x"),
		Entry("parallel --plus prefix removal", "parallel --plus {#x} push ::: xgit"),
		Entry("parallel path part of several inputs", "parallel {/} push ::: a/git ::: b"),
		Entry("a quoted parallel option name", `parallel --"$O" echo ::: a`),
		Entry("a quoted short parallel option name", `parallel "-$O" echo ::: a`),
		Entry("sem with a variable", "sem $X"),
		Entry("env_parallel with a variable", "env_parallel $X ::: a"),
		Entry("parset with a variable", "parset out $X ::: a"),
		Entry("xargs appending unseen stdin to git", "cat f | xargs git"),
		Entry("xargs appending an arg file to git", "xargs -a f git"),
		Entry("BSD xargs -J", "ls | xargs -J % git %"),
		Entry("xargs --replace", "xargs --replace sh -c {}"),
		Entry("xargs -I with sh -c", "xargs -I % sh -c %"),
		Entry("xargs -i with an attached string", "xargs -i% sh -c %"),
		Entry("xargs -I in a cluster", "xargs -0I % sh -c %"),
		Entry("xargs --replace with a string", "xargs --replace=@ sh -c @"),
		Entry("xargs -I as the program", "xargs -I % % push"),
		Entry("xargs -I as a git word", "ls | xargs -I % git %"),
		Entry("xargs reading an arg file", "echo push | xargs -a f -I % git %"),
	)

	DescribeTable("follows the command lines parallel and xargs build from literal input",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(pushes(result)).To(BeNumerically(">", 0), command)
		},
		Entry("appended input", "parallel git ::: push"),
		Entry("a replacement string", "parallel git {} ::: push"),
		Entry("a quoted command line", "parallel 'git {}' ::: push"),
		Entry("a custom replacement string", "parallel -I @ 'git @' ::: push"),
		Entry("an attached replacement string", "parallel --replace=@ 'git @' ::: push"),
		Entry("positional replacement strings", "parallel git {1} {2} ::: push ::: origin"),
		Entry("a path replacement string", "parallel 'git {.}' ::: push.x"),
		Entry("options before the command", "parallel -j4 -k --tag git ::: push"),
		Entry("input lines as commands", "parallel ::: 'git push'"),
		Entry("stdin lines as commands", "echo 'git push' | parallel"),
		Entry("a command from a variable on the line", "X=git; parallel $X ::: push"),
		Entry("perl in a replacement string", `parallel echo '{= system("git push") =}' ::: a`),
		Entry("perl in --rpl", `parallel --rpl '{x} system("git push")' echo ::: a`),
		Entry("a basename", "parallel 'git {/}' ::: a/push"),
		Entry("a basename without extension", "parallel 'git {/.}' ::: a/push.x"),
		Entry("a directory", "parallel 'git {//}' ::: push/a"),
		Entry("a job number beside the input", "parallel 'git {} {#} {%}' ::: push"),
		Entry("a positional path part", "parallel 'git {1.} {2/}' ::: push.x ::: a/o"),
		Entry("an extension replacement string", "parallel --er @ 'git @' ::: push.x"),
		Entry("--plus strings", "parallel --plus 'git {}' ::: push"),
		Entry("{} with several inputs", "parallel {} ::: git ::: push"),
		Entry("-X {} with several inputs", "parallel -X {} ::: git ::: push"),
		Entry("a custom separator", "parallel --arg-sep ,, git ,, push"),
		Entry("a custom file separator", "parallel --arg-file-sep ,,, git ::: push"),
		Entry("the end of options", "parallel -j2 -- git ::: push"),
		Entry("quoted command words", "parallel -q git push ::: origin"),
		Entry("an unknown option", "parallel --frob git push ::: a"),
		Entry("perl in a tag string", `parallel --tagstring '{= system("git push") =}' echo ::: a`),
		Entry("an --ssh command", "parallel --ssh='git push' -S h echo ::: a"),
		Entry("an input from a variable on the line", "X=push; parallel git ::: $X"),
		Entry("parset", "parset out git ::: push"),
		Entry("BSD xargs -J with literal stdin", "echo push | xargs -J % git %"),
		Entry("xargs -I with literal stdin", "echo push | xargs -I % git %"),
		Entry("xargs -I in a cluster with literal stdin", "echo push | xargs -tI % git %"),
	)

	DescribeTable("leaves parallel and xargs running nothing tracked alone",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(result.GitOperations).To(BeEmpty(), command)
		},
		Entry("a glob of inputs", "parallel gzip ::: *.log"),
		Entry("path replacement strings", "parallel -j4 'convert {} {.}.png' ::: a.jpg b.jpg"),
		Entry("stdin appended", "ls *.go | parallel gofmt -l"),
		Entry("piped input", "cat big | parallel --pipe wc -l"),
		Entry("the version", "parallel --version"),
		Entry("many literal inputs", "parallel echo ::: a b c d e f g h i j k l m n o p q r s"),
		Entry("xargs -I with an untracked program", "ls | xargs -I % mv % %.bak"),
		Entry("docker as a unit name", `journalctl -u docker --since "$SINCE"`),
		Entry("docker as a service", `sudo systemctl restart docker "$X"`),
		Entry("docker as a package", `brew upgrade docker "$X"`),
		Entry("parallel without raw command words",
			"parallel -j 4 echo push origin ::: main dev"),
		Entry("xargs -I {} with an untracked program", "find . | xargs -I {} gofmt -l {}"),
	)
})
