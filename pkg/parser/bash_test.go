package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/parser"
)

var _ = Describe("BashParser", func() {
	var p *parser.BashParser

	BeforeEach(func() {
		p = parser.NewBashParser()
	})

	Describe("Parse", func() {
		Context("with empty command", func() {
			It("returns error", func() {
				_, err := p.Parse("")
				Expect(err).To(MatchError(parser.ErrEmptyCommand))
			})

			It("returns error for whitespace-only", func() {
				_, err := p.Parse("   \t\n")
				Expect(err).To(MatchError(parser.ErrEmptyCommand))
			})
		})

		Context("with simple commands", func() {
			It("parses single command", func() {
				result, err := p.Parse("git status")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(1))

				cmd := result.Commands[0]
				Expect(cmd.Name).To(Equal("git"))
				Expect(cmd.Args).To(Equal([]string{"status"}))
				Expect(cmd.Type).To(Equal(parser.CmdTypeSimple))
			})

			It("parses command with multiple arguments", func() {
				result, err := p.Parse("git add file1.txt file2.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(1))

				cmd := result.Commands[0]
				Expect(cmd.Name).To(Equal("git"))
				Expect(cmd.Args).To(Equal([]string{"add", "file1.txt", "file2.txt"}))
			})

			It("parses command with flags", func() {
				result, err := p.Parse("git commit -sS -m 'test message'")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(1))

				cmd := result.Commands[0]
				Expect(cmd.Name).To(Equal("git"))
				Expect(cmd.Args).To(ContainElements("-sS", "-m", "test message"))
			})
		})

		Context("with chained commands", func() {
			It("parses AND chain (&&)", func() {
				result, err := p.Parse("git add file.txt && git commit -m 'msg'")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(2))

				Expect(result.Commands[0].Name).To(Equal("git"))
				Expect(result.Commands[0].Args).To(Equal([]string{"add", "file.txt"}))

				Expect(result.Commands[1].Name).To(Equal("git"))
				Expect(result.Commands[1].Args).To(ContainElements("commit", "-m", "msg"))
			})

			It("parses OR chain (||)", func() {
				result, err := p.Parse("git commit -m 'msg' || echo 'failed'")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(2))

				Expect(result.Commands[0].Name).To(Equal("git"))
				Expect(result.Commands[1].Name).To(Equal("echo"))
			})

			It("parses semicolon chain", func() {
				result, err := p.Parse("git status; git diff")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(2))

				Expect(result.Commands[0].Name).To(Equal("git"))
				Expect(result.Commands[0].Args).To(Equal([]string{"status"}))
				Expect(result.Commands[1].Name).To(Equal("git"))
				Expect(result.Commands[1].Args).To(Equal([]string{"diff"}))
			})

			It("parses complex chain", func() {
				result, err := p.Parse("git add . && git commit -m 'msg' && git push upstream main")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(3))

				Expect(result.Commands[0].Name).To(Equal("git"))
				Expect(result.Commands[1].Name).To(Equal("git"))
				Expect(result.Commands[2].Name).To(Equal("git"))
			})
		})

		Context("with pipes", func() {
			It("parses simple pipe", func() {
				result, err := p.Parse("ls | grep foo")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(2))

				Expect(result.Commands[0].Name).To(Equal("ls"))
				Expect(result.Commands[1].Name).To(Equal("grep"))
				Expect(result.Commands[1].Args).To(Equal([]string{"foo"}))
			})

			It("parses multi-stage pipe", func() {
				result, err := p.Parse("cat file.txt | grep pattern | wc -l")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(3))

				Expect(result.Commands[0].Name).To(Equal("cat"))
				Expect(result.Commands[1].Name).To(Equal("grep"))
				Expect(result.Commands[2].Name).To(Equal("wc"))
			})
		})

		Context("with subshells", func() {
			It("parses subshell", func() {
				result, err := p.Parse("(cd dir && git commit -m 'msg')")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(2))

				Expect(result.Commands[0].Name).To(Equal("cd"))
				Expect(result.Commands[1].Name).To(Equal("git"))
			})

			It("parses command substitution", func() {
				result, err := p.Parse("echo $(git log -1 --format=%h)")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(2))

				Expect(result.Commands[0].Name).To(Equal("echo"))
				Expect(result.Commands[1].Name).To(Equal("git"))
			})
		})

		Context("with quoted strings", func() {
			It("handles single quotes", func() {
				result, err := p.Parse("git commit -m 'test message'")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(1))

				cmd := result.Commands[0]
				Expect(cmd.Args).To(ContainElement("test message"))
			})

			It("handles double quotes", func() {
				result, err := p.Parse(`git commit -m "test message"`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(1))

				cmd := result.Commands[0]
				Expect(cmd.Args).To(ContainElement("test message"))
			})

			It("does not split on chain operators in quotes", func() {
				result, err := p.Parse(`git commit -m "msg && trick"`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(1))

				cmd := result.Commands[0]
				Expect(cmd.Args).To(ContainElement("msg && trick"))
			})
		})

		Context("with redirections", func() {
			It("detects output redirection", func() {
				result, err := p.Parse("echo 'test' > file.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("file.txt"))
				Expect(fw.Operation).To(Equal(parser.WriteOpRedirect))
			})

			It("detects append redirection", func() {
				result, err := p.Parse("echo 'test' >> file.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("file.txt"))
				Expect(fw.Operation).To(Equal(parser.WriteOpAppend))
			})
		})

		Context("with heredoc", func() {
			It("detects heredoc with output redirection", func() {
				cmd := `cat > file.txt << 'EOF'
line 1
line 2
EOF`
				result, err := p.Parse(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("file.txt"))
				Expect(fw.Operation).To(Equal(parser.WriteOpHeredoc))
				Expect(fw.Content).To(Equal("line 1\nline 2\n"))
			})

			It("detects heredoc with unquoted delimiter", func() {
				cmd := `cat > output.md << EOF
# Header
Content here
EOF`
				result, err := p.Parse(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("output.md"))
				Expect(fw.Operation).To(Equal(parser.WriteOpHeredoc))
				Expect(fw.Content).To(Equal("# Header\nContent here\n"))
			})

			It("detects heredoc with dash variant", func() {
				cmd := `cat > script.sh <<- 'EOF'
	#!/bin/bash
	echo "test"
EOF`
				result, err := p.Parse(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("script.sh"))
				Expect(fw.Operation).To(Equal(parser.WriteOpHeredoc))
				Expect(fw.Content).To(ContainSubstring("#!/bin/bash"))
			})

			It("handles empty heredoc content", func() {
				cmd := `cat > empty.txt << 'EOF'
EOF`
				result, err := p.Parse(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("empty.txt"))
				Expect(fw.Operation).To(Equal(parser.WriteOpHeredoc))
				Expect(fw.Content).To(Equal(""))
			})

			It("handles heredoc with special characters", func() {
				cmd := "cat > data.txt << 'EOF'\nSpecial chars: $VAR ${FOO} $(cmd)\nEOF"
				result, err := p.Parse(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Content).To(ContainSubstring("$VAR"))
				Expect(fw.Content).To(ContainSubstring("$(cmd)"))
			})
		})

		Context("with file write commands", func() {
			It("detects tee command", func() {
				result, err := p.Parse("echo 'test' | tee output.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("output.txt"))
				Expect(fw.Operation).To(Equal(parser.WriteOpTee))
			})

			It("detects cp command", func() {
				result, err := p.Parse("cp source.txt dest.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("dest.txt"))
				Expect(fw.Operation).To(Equal(parser.WriteOpCopy))
			})

			It("detects mv command", func() {
				result, err := p.Parse("mv old.txt new.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(1))

				fw := result.FileWrites[0]
				Expect(fw.Path).To(Equal("new.txt"))
				Expect(fw.Operation).To(Equal(parser.WriteOpMove))
			})

			It("detects multiple tee targets", func() {
				result, err := p.Parse("echo 'test' | tee file1.txt file2.txt")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.FileWrites).To(HaveLen(2))

				Expect(result.FileWrites[0].Path).To(Equal("file1.txt"))
				Expect(result.FileWrites[1].Path).To(Equal("file2.txt"))
			})
		})

		Context("with git operations", func() {
			It("extracts git commands", func() {
				result, err := p.Parse("git status && git diff")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.GitOperations).To(HaveLen(2))

				Expect(result.GitOperations[0].Args).To(Equal([]string{"status"}))
				Expect(result.GitOperations[1].Args).To(Equal([]string{"diff"}))
			})

			It("filters non-git commands", func() {
				result, err := p.Parse("ls -la && git status && echo done")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(3))
				Expect(result.GitOperations).To(HaveLen(1))
				Expect(result.GitOperations[0].Args).To(Equal([]string{"status"}))
			})
		})
	})

	Describe("ParseResult methods", func() {
		It("HasCommand checks command existence", func() {
			result, err := p.Parse("git status && echo done")
			Expect(err).NotTo(HaveOccurred())

			Expect(result.HasCommand("git")).To(BeTrue())
			Expect(result.HasCommand("echo")).To(BeTrue())
			Expect(result.HasCommand("ls")).To(BeFalse())
		})

		It("HasGitCommand checks git command existence", func() {
			result, err := p.Parse("git status")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.HasGitCommand()).To(BeTrue())

			result, err = p.Parse("echo done")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.HasGitCommand()).To(BeFalse())
		})

		It("GetCommands filters by name", func() {
			result, err := p.Parse("git status && git diff && echo done")
			Expect(err).NotTo(HaveOccurred())

			gitCmds := result.GetCommands("git")
			Expect(gitCmds).To(HaveLen(2))
			Expect(gitCmds[0].Args).To(Equal([]string{"status"}))
			Expect(gitCmds[1].Args).To(Equal([]string{"diff"}))
		})
	})
})
