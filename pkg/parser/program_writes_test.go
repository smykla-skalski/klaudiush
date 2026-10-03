package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Writes by programs", func() {
	resolver := fakeResolver{
		env: map[string]string{"PWD": "/start", "HOME": "/home/u"},
		files: map[string]string{
			"/s/a.sh":           "echo hi\n",
			"/repo/push.sh":     "git push --force\n",
			"/start/sub/run.sh": "git push --force\n",
		},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	paths := func(result *parser.ParseResult) ([]string, bool) {
		var (
			found   []string
			unknown bool
		)

		for _, fw := range result.FileWrites {
			if fw.TargetUnknown {
				unknown = true

				continue
			}

			found = append(found, fw.Path)
		}

		return found, unknown
	}

	DescribeTable(
		"records the files a program writes",
		func(command string, op parser.WriteOp, want []string, unknown bool) {
			result := parse(command)
			got, gotUnknown := paths(result)

			if len(want) == 0 {
				Expect(got).To(BeEmpty())
			} else {
				Expect(got).To(Equal(want))
			}

			Expect(gotUnknown).To(Equal(unknown))

			for _, fw := range result.FileWrites {
				Expect(fw.Operation).To(Equal(op))
			}
		},
		Entry("ln -s", "ln -s target link", parser.WriteOpLink, []string{"link"}, false),
		Entry(
			"ln into the current directory",
			"ln -s /a/b/",
			parser.WriteOpLink,
			[]string{"b"},
			false,
		),
		Entry("ln -t", "ln -st dir a b", parser.WriteOpLink, []string{"dir"}, false),
		Entry("ln of root only", "ln -s /", parser.WriteOpLink, nil, false),
		Entry("sed -i", "sed -i 's/a/b/' f1 f2", parser.WriteOpEdit, []string{"f1", "f2"}, false),
		Entry("BSD sed -i ''", "sed -i '' 's/a/b/' f", parser.WriteOpEdit, []string{"f"}, false),
		Entry(
			"sed -i.bak -e",
			"sed -i.bak -e 's/a/b/' f",
			parser.WriteOpEdit,
			[]string{"f"},
			false,
		),
		Entry(
			"sed --in-place=",
			"sed --in-place=.b -f x.sed f",
			parser.WriteOpEdit,
			[]string{"f"},
			false,
		),
		Entry("sed -nEi cluster", "sed -nEi 's/a/b/' f", parser.WriteOpEdit, []string{"f"}, false),
		Entry(
			"sed abbreviated --expr",
			"sed -i --expr=x f",
			parser.WriteOpEdit,
			[]string{"f"},
			false,
		),
		Entry("sed without -i", "sed 's/a/b/' f", parser.WriteOpNone, nil, false),
		Entry("dd of=", "dd if=a of=b bs=1", parser.WriteOpOutput, []string{"b"}, false),
		Entry("curl -o in a cluster", "curl -sSLo out.sh https://x/y", parser.WriteOpOutput,
			[]string{"out.sh"}, false),
		Entry("curl --output=", "curl --output=o.txt https://x", parser.WriteOpOutput,
			[]string{"o.txt"}, false),
		Entry("curl -o -", "curl -o - https://x", parser.WriteOpNone, nil, false),
		Entry("curl -O", "curl -H 'A: b' -O 'https://x/a/install.sh?x=1#f'", parser.WriteOpOutput,
			[]string{"install.sh"}, false),
		Entry("curl -O with --output-dir", "curl --output-dir d -O https://x/f.sh",
			parser.WriteOpOutput, []string{"d/f.sh"}, false),
		Entry("curl -O --url", "curl -O --url https://x/g.sh", parser.WriteOpOutput,
			[]string{"g.sh"}, false),
		Entry("curl -O of a bare host", "curl -O https://x", parser.WriteOpOutput, nil, true),
		Entry("curl -J", "curl -J -O https://x/f", parser.WriteOpOutput, []string{"f"}, true),
		Entry("curl -K", "curl -K cfg https://x", parser.WriteOpOutput, nil, true),
		Entry("curl -D and -c", "curl -D h.txt -c jar https://x", parser.WriteOpOutput,
			[]string{"h.txt", "jar"}, false),
		Entry("install", "install -m 755 a /usr/local/bin/a", parser.WriteOpOutput,
			[]string{"/usr/local/bin/a"}, false),
		Entry("install -d", "install -d dir", parser.WriteOpNone, nil, false),
		Entry("install -t", "install --target-directory=bin a", parser.WriteOpOutput,
			[]string{"bin"}, false),
		Entry("cp -t", "cp -t dir a b", parser.WriteOpCopy, []string{"dir"}, false),
		Entry("cp abbreviated --target", "cp --target dir a", parser.WriteOpCopy,
			[]string{"dir"}, false),
		Entry("cp after --", "cp -- -a -b", parser.WriteOpCopy, []string{"-b"}, false),
		Entry("mv", "mv -f a b", parser.WriteOpMove, []string{"b"}, false),
		Entry("cp to a substituted destination", `cp a "$(echo d)"`, parser.WriteOpCopy, nil, true),
		Entry("tee to a partly substituted path", `echo x | tee "$(echo d)/f"`, parser.WriteOpTee,
			[]string{"/f"}, false),
		Entry("unzip", "unzip -o a.zip -d out", parser.WriteOpUnpack, nil, true),
		Entry("unzip -l", "unzip -l a.zip", parser.WriteOpNone, nil, false),
		Entry("unzip without archive", "unzip --help", parser.WriteOpNone, nil, false),
		Entry("patch", "patch -p1 < fix.diff", parser.WriteOpUnpack, nil, true),
		Entry("patch --dry-run", "patch --dry-run -p1 < fix.diff", parser.WriteOpNone, nil, false),
		Entry("git reset --hard", "git reset --hard HEAD~1", parser.WriteOpUnpack, nil, true),
		Entry("git reset --soft", "git reset --soft HEAD~1", parser.WriteOpNone, nil, false),
		Entry("git stash", "git stash", parser.WriteOpUnpack, nil, true),
		Entry("git stash -m", "git stash -m wip", parser.WriteOpUnpack, nil, true),
		Entry("git stash pop", "git -C . stash pop", parser.WriteOpUnpack, nil, true),
		Entry("git stash list", "git stash list", parser.WriteOpNone, nil, false),
		Entry("git status", "git status", parser.WriteOpNone, nil, false),
		Entry("tar x", "tar xzf a.tgz", parser.WriteOpUnpack, nil, true),
		Entry("tar -x", "tar -C d -xf a.tar", parser.WriteOpUnpack, nil, true),
		Entry("tar --extract", "tar --extract -f a.tar", parser.WriteOpUnpack, nil, true),
		Entry("tar -c", "tar -cf a.tar d", parser.WriteOpNone, nil, false),
		Entry("git apply", "git apply x.patch", parser.WriteOpUnpack, nil, true),
		Entry("git apply --check", "git apply --check x.patch", parser.WriteOpNone, nil, false),
		Entry("git apply --cached --index", "git apply --cached --index x", parser.WriteOpUnpack,
			nil, true),
		Entry("git checkout -b", "git checkout -b feat", parser.WriteOpNone, nil, false),
		Entry("git switch -c from a start", "git switch -c feat origin/main", parser.WriteOpUnpack,
			nil, true),
		Entry(
			"git checkout a path",
			"git checkout evil -- run.sh",
			parser.WriteOpUnpack,
			nil,
			true,
		),
		Entry("git restore --staged", "git restore --staged f", parser.WriteOpNone, nil, false),
		Entry("git restore", "git restore --source=evil f", parser.WriteOpUnpack, nil, true),
		Entry("git restore -SW", "git restore -S -W f", parser.WriteOpUnpack, nil, true),
		Entry("git pull", "git pull", parser.WriteOpUnpack, nil, true),
		Entry("perl -pi", "perl -pi -e 's/a/b/' f", parser.WriteOpEdit, []string{"f"}, false),
		Entry("perl without -i", "perl -Mstrict -e 'print 1' f", parser.WriteOpNone, nil, false),
		Entry("gsed -i", "gsed -i 's/a/b/' f", parser.WriteOpEdit, []string{"f"}, false),
		Entry("sed abbreviated --in-place", "sed --in s/a/b/ f", parser.WriteOpEdit,
			[]string{"f"}, false),
		Entry("wget -O", "wget -q -O f https://x/y", parser.WriteOpOutput, []string{"f"}, false),
		Entry("wget -O -", "wget -O - https://x/y", parser.WriteOpNone, nil, false),
		Entry("wget by URL name", "wget -P d https://x/a.sh", parser.WriteOpOutput,
			[]string{"d/a.sh"}, false),
		Entry("wget -r", "wget -r https://x/a/", parser.WriteOpOutput, nil, true),
		Entry("rsync", "rsync -a -e ssh src/ dst", parser.WriteOpCopy, []string{"dst"}, false),
		Entry("cp with a split substitution", "cp $(echo a b)", parser.WriteOpCopy, nil, true),
		Entry("install --strip", "install --strip a b", parser.WriteOpOutput, []string{"b"}, false),
		Entry("curl --output-dir only for -o", "curl --output-dir d -D h -o o https://x",
			parser.WriteOpOutput, []string{"h", "d/o"}, false),
		Entry("unzip -c", "unzip -c a.zip", parser.WriteOpNone, nil, false),
		Entry("invalid UTF-8 in an option", `cp -$'\377' a b`, parser.WriteOpCopy,
			[]string{"b"}, false),
		Entry("invalid UTF-8 in a sed cluster", `sed -$'\303'i x f`, parser.WriteOpEdit,
			[]string{"f"}, false),
		Entry("invalid UTF-8 in a curl cluster", `curl -$'\377' https://x`, parser.WriteOpNone,
			nil, false),
	)

	It("marks a partly substituted target dynamic", func() {
		result := parse(`echo x | tee "$(echo d)/f"`)

		Expect(result.FileWrites).To(HaveLen(1))
		Expect(result.FileWrites[0].Dynamic).To(BeTrue())
	})

	It("names each write program by its operation", func() {
		for op, name := range map[parser.WriteOp]string{
			parser.WriteOpLink:   "Link",
			parser.WriteOpEdit:   "Edit",
			parser.WriteOpOutput: "Output",
			parser.WriteOpUnpack: "Unpack",
		} {
			Expect(op.String()).To(Equal(name))
		}
	})

	DescribeTable(
		"fails closed on a script changed earlier on the line",
		func(command, detail string) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), "opacities: %v", result.Opacities)
			Expect(result.Opacities[0].Detail).To(Equal(detail))
		},
		Entry("sed -i", "sed -i 's/hi/x/' /s/a.sh && bash /s/a.sh", parser.DetailScriptWritten),
		Entry("ln -s", "ln -sf /tmp/evil /s/a.sh && bash /s/a.sh", parser.DetailScriptWritten),
		Entry("dd of=", "dd if=/tmp/e of=/s/a.sh && bash /s/a.sh", parser.DetailScriptWritten),
		Entry(
			"curl -o",
			"curl -so /s/a.sh https://x/e && bash /s/a.sh",
			parser.DetailScriptWritten,
		),
		Entry("install", "install /tmp/e /s/a.sh && bash /s/a.sh", parser.DetailScriptWritten),
		Entry("copy into the script's directory", "cp /tmp/a.sh /s && bash /s/a.sh",
			parser.DetailScriptWritten),
		Entry("unzip", "unzip -o x.zip && bash /s/a.sh", parser.DetailScriptUnplacedWrite),
		Entry("patch", "patch -p1 < x.diff; bash /s/a.sh", parser.DetailScriptUnplacedWrite),
		Entry(
			"git reset --hard",
			"git reset --hard && bash /s/a.sh",
			parser.DetailScriptUnplacedWrite,
		),
		Entry("git stash pop", "git stash pop && bash /s/a.sh", parser.DetailScriptUnplacedWrite),
		Entry("copy to a computed name", `cp /tmp/e "$(echo /s/a.sh)" && bash /s/a.sh`,
			parser.DetailScriptUnplacedWrite),
		Entry("write after a computed cd", `cd "$(mktemp -d)" && echo x > a.sh; bash /s/a.sh`,
			parser.DetailScriptUnplacedWrite),
		Entry("relative cd after a computed cd", `cd "$(echo /e)" && cd sub && bash a.sh`,
			parser.DetailScriptDirectory),
		Entry("pushd and popd after a computed cd",
			`cd "$(echo /e)" && pushd /tmp && popd && bash a.sh`, parser.DetailScriptDirectory),
		Entry("redirect to a computed name", `echo x > "$(echo /s/a.sh)" && bash /s/a.sh`,
			parser.DetailScriptUnplacedWrite),
		Entry("git checkout of a path", "git checkout evil -- /s/a.sh && bash /s/a.sh",
			parser.DetailScriptUnplacedWrite),
	)

	DescribeTable(
		"still follows a script nothing on the line changes",
		func(command string) {
			Expect(parse(command).Truncated).To(BeFalse())
		},
		Entry("git reset --soft", "git reset --soft HEAD~1 && bash /s/a.sh"),
		Entry("sed without -i", "sed 's/hi/x/' /s/a.sh && bash /s/a.sh"),
		Entry("copy elsewhere", "cp /tmp/a.sh /other && bash /s/a.sh"),
		Entry("unzip -l", "unzip -l x.zip && bash /s/a.sh"),
		Entry("its own redirect to a computed name", `bash /s/a.sh > "log-$(date +%s)"`),
		Entry(
			"its own redirect after a computed cd",
			`cd "$(mktemp -d)" && bash /s/a.sh > out.log`,
		),
		Entry("git checkout -b", "git checkout -b feat && bash /s/a.sh"),
	)
})

var _ = Describe("The shell's directory", func() {
	resolver := fakeResolver{
		env: map[string]string{"PWD": "/start", "HOME": "/home/u"},
		files: map[string]string{
			"/repo/push.sh":     "git push --force\n",
			"/start/sub/run.sh": "git push --force\n",
		},
		outputs: map[string]string{"git rev-parse --show-toplevel": "/repo"},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	last := func(result *parser.ParseResult) parser.Command {
		return result.Commands[len(result.Commands)-1]
	}

	pushes := func(result *parser.ParseResult) bool {
		for _, op := range result.GitOperations {
			if len(op.Args) > 0 && op.Args[0] == "push" {
				return true
			}
		}

		return false
	}

	DescribeTable("makes a computed cd unknown",
		func(command string) {
			cmd := last(parse(command))

			Expect(cmd.DirUnknown).To(BeTrue())
			Expect(cmd.DirComputed).To(BeTrue())
			Expect(cmd.Vars.IsDynamic("PWD")).To(BeTrue())
		},
		Entry("command substitution", `cd "$(mktemp -d)" && cat x`),
		Entry("substitution with a literal tail", `cd "$(mktemp -d)/sub" && cat x`),
		Entry("variable holding command output", `d=$(mktemp -d); cd "$d"; cat x`),
		Entry("pushd", `pushd "$(mktemp -d)"; cat x`),
		Entry("lookup with another word", `cd -P "$(git rev-parse --show-toplevel)" extra; cat x`),
	)

	It("resolves a cd to an allowed lookup", func() {
		result := parse(`cd "$(git rev-parse --show-toplevel)/." && bash push.sh`)

		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
		Expect(pushes(result)).To(BeTrue())
		Expect(last(result).DirComputed).To(BeFalse())
	})

	It("does not resolve a lookup built from other substitutions", func() {
		cmd := last(parse(`cd "$(git rev-parse --show-toplevel)$(echo x)"; cat x`))

		Expect(cmd.DirUnknown).To(BeTrue())
	})

	It("follows $PWD after a relative cd", func() {
		result := parse(`cd sub && bash "$PWD/run.sh"`)

		Expect(pushes(result)).To(BeTrue())
		Expect(last(result).Vars.Assignments).To(HaveKeyWithValue("PWD", "/start/sub"))
		Expect(last(result).Vars.Assignments).To(HaveKeyWithValue("OLDPWD", "/start"))
	})

	It("follows $PWD after an absolute cd", func() {
		Expect(pushes(parse(`cd /repo; bash "$PWD/push.sh"`))).To(BeTrue())
	})

	It("expands ~ in $PWD", func() {
		vars := last(parse(`cd ~/x; cat y`)).Vars

		Expect(vars.Assignments).To(HaveKeyWithValue("PWD", "/home/u/x"))
	})

	It("leaves $PWD unknown after ~ when HOME changes", func() {
		vars := last(parse(`HOME=/h; cd ~/x; cat y`)).Vars

		Expect(vars.IsDynamic("PWD")).To(BeTrue())
	})

	It("restores $PWD on popd", func() {
		vars := last(parse(`pushd /a; popd; cat y`)).Vars

		Expect(vars.Assignments).To(HaveKeyWithValue("PWD", "/start"))
		Expect(vars.Assignments).To(HaveKeyWithValue("OLDPWD", "/a"))
	})

	It("keeps $PWD known after leaving a computed directory", func() {
		vars := last(parse(`cd "$(mktemp -d)"; cd /repo; cat y`)).Vars

		Expect(vars.Assignments).To(HaveKeyWithValue("PWD", "/repo"))
		Expect(vars.IsDynamic("PWD")).To(BeFalse())
		Expect(vars.IsDynamic("OLDPWD")).To(BeTrue())
	})

	It("blocks a $PWD script after a computed cd", func() {
		result := parse(`cd "$(mktemp -d)" && bash "$PWD/run.sh"`)

		Expect(result.Truncated).To(BeTrue())
	})

	It("leaves $PWD unknown when the line starts without one", func() {
		bare := fakeResolver{}

		result, err := parser.NewBashParserWithResolver(bare).Parse(`cd sub; cat y`)
		Expect(err).NotTo(HaveOccurred())
		Expect(last(result).Vars.IsDynamic("PWD")).To(BeTrue())
	})

	It("keeps a PWD assigned on the line as the old directory", func() {
		vars := last(parse(`PWD=/x; cd /y; cat z`)).Vars

		Expect(vars.Assignments).To(HaveKeyWithValue("OLDPWD", "/x"))
	})
})
