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

	Describe("CmdType String", func() {
		It("returns Simple for CmdTypeSimple", func() {
			Expect(parser.CmdTypeSimple.String()).To(Equal("Simple"))
		})

		It("returns Pipe for CmdTypePipe", func() {
			Expect(parser.CmdTypePipe.String()).To(Equal("Pipe"))
		})

		It("returns Subshell for CmdTypeSubshell", func() {
			Expect(parser.CmdTypeSubshell.String()).To(Equal("Subshell"))
		})

		It("returns CmdSubst for CmdTypeCmdSubst", func() {
			Expect(parser.CmdTypeCmdSubst.String()).To(Equal("CmdSubst"))
		})

		It("returns Chain for CmdTypeChain", func() {
			Expect(parser.CmdTypeChain.String()).To(Equal("Chain"))
		})

		It("returns Unknown for undefined CmdType", func() {
			unknownType := parser.CmdType(99)
			Expect(unknownType.String()).To(Equal("Unknown"))
		})
	})

	Describe("Command methods", func() {
		Context("String", func() {
			It("returns just name when no args", func() {
				cmd := &parser.Command{Name: "ls"}
				Expect(cmd.String()).To(Equal("ls"))
			})

			It("returns name with args joined", func() {
				cmd := &parser.Command{
					Name: "git",
					Args: []string{"commit", "-m", "message"},
				}
				Expect(cmd.String()).To(Equal("git commit -m message"))
			})
		})

		Context("FullCommand", func() {
			It("returns slice with name only when no args", func() {
				cmd := &parser.Command{Name: "ls"}
				Expect(cmd.FullCommand()).To(Equal([]string{"ls"}))
			})

			It("returns slice with name and all args", func() {
				cmd := &parser.Command{
					Name: "git",
					Args: []string{"commit", "-m", "message"},
				}
				Expect(cmd.FullCommand()).To(Equal([]string{"git", "commit", "-m", "message"}))
			})
		})
	})

	Describe("FindDoubleQuotedBackticks", func() {
		Context("with backticks in double quotes", func() {
			It("detects backticks in git commit message", func() {
				issues, err := p.FindDoubleQuotedBackticks(
					"git commit -m \"Fix bug in `parser` module\"",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(issues).To(HaveLen(1))
				Expect(issues[0].ArgIndex).To(Equal(3))
				// ArgValue contains the argument with backticks detected
				Expect(issues[0].ArgValue).NotTo(BeEmpty())
			})

			It("detects backticks in gh pr create", func() {
				issues, err := p.FindDoubleQuotedBackticks(
					"gh pr create --body \"Updated `config.toml` handling\"",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(issues).To(HaveLen(1))
				// gh=0, pr=1, create=2, --body=3, "..."=4
				Expect(issues[0].ArgIndex).To(Equal(4))
			})

			It("detects backticks in gh issue create", func() {
				issues, err := p.FindDoubleQuotedBackticks(
					"gh issue create --title \"Bug in `validate` function\"",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(issues).To(HaveLen(1))
				// gh=0, issue=1, create=2, --title=3, "..."=4
				Expect(issues[0].ArgIndex).To(Equal(4))
			})

			It("detects multiple arguments with backticks", func() {
				issues, err := p.FindDoubleQuotedBackticks(
					"git commit -m \"Fix `parser`\" -m \"Update `validator`\"",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(issues).To(HaveLen(2))
				Expect(issues[0].ArgIndex).To(Equal(3))
				Expect(issues[1].ArgIndex).To(Equal(5))
			})
		})

		Context("with single quotes (no issue)", func() {
			It("ignores backticks in single quotes", func() {
				issues, err := p.FindDoubleQuotedBackticks(
					"git commit -m 'Fix bug in `parser` module'",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(issues).To(BeEmpty())
			})
		})

		Context("with escaped backticks", func() {
			It("does not detect escaped backticks", func() {
				// In the shell, \` becomes a literal backtick, not command substitution
				// The shell parser will see this as a literal backslash + backtick
				// which gets interpreted as command substitution, so we should still detect it
				issues, err := p.FindDoubleQuotedBackticks(
					"git commit -m \"Fix bug in \\`parser\\` module\"",
				)
				Expect(err).NotTo(HaveOccurred())
				// Escaped backticks are NOT command substitution in the AST
				Expect(issues).To(BeEmpty())
			})
		})

		Context("with no backticks", func() {
			It("returns empty for normal double-quoted strings", func() {
				issues, err := p.FindDoubleQuotedBackticks(
					"git commit -m \"Fix bug in parser module\"",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(issues).To(BeEmpty())
			})

			It("returns empty for single-quoted strings", func() {
				issues, err := p.FindDoubleQuotedBackticks(
					"git commit -m 'Fix bug in parser module'",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(issues).To(BeEmpty())
			})
		})

		Context("with $() command substitution (recommended pattern)", func() {
			It("detects $() as command substitution in double quotes", func() {
				// $(cat <<'EOF' ... EOF) is the recommended HEREDOC pattern
				// This should also be detected as command substitution
				cmd := "git commit -m \"$(cat <<'EOF'\nFix bug in parser module\nEOF\n)\""
				issues, err := p.FindDoubleQuotedBackticks(cmd)
				Expect(err).NotTo(HaveOccurred())
				// This is command substitution, which is fine for HEREDOC pattern
				// but we're detecting ANY command substitution in double quotes
				Expect(issues).To(HaveLen(1))
			})
		})

		Context("with empty command", func() {
			It("returns error for empty string", func() {
				_, err := p.FindDoubleQuotedBackticks("")
				Expect(err).To(MatchError(parser.ErrEmptyCommand))
			})
		})

		Context("with invalid syntax", func() {
			It("returns error for malformed command", func() {
				_, err := p.FindDoubleQuotedBackticks("git commit -m \"unclosed")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("FindAllBacktickIssues", func() {
		Context("with unquoted backticks", func() {
			It("detects unquoted backticks", func() {
				locations, err := p.FindAllBacktickIssues("echo `date`")
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(HaveLen(1))
				Expect(locations[0].Context).To(Equal(parser.QuotingContextUnquoted))
			})

			It("detects multiple unquoted backticks", func() {
				locations, err := p.FindAllBacktickIssues("echo `date` `hostname`")
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(HaveLen(2))
			})
		})

		Context("with double-quoted backticks", func() {
			It("detects backticks in double quotes", func() {
				locations, err := p.FindAllBacktickIssues("echo \"Fix `parser`\"")
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(HaveLen(1))
				Expect(locations[0].Context).To(Equal(parser.QuotingContextDoubleQuoted))
			})

			It("sets SuggestSingle when no variables present", func() {
				locations, err := p.FindAllBacktickIssues("echo \"Fix `parser`\"")
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(HaveLen(1))
				Expect(locations[0].SuggestSingle).To(BeTrue())
				Expect(locations[0].HasVariables).To(BeFalse())
			})

			It("sets HasVariables when variables present", func() {
				locations, err := p.FindAllBacktickIssues("echo \"Fix `parser` for $VERSION\"")
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(HaveLen(1))
				Expect(locations[0].HasVariables).To(BeTrue())
				Expect(locations[0].SuggestSingle).To(BeFalse())
			})
		})

		Context("with single-quoted backticks", func() {
			It("does not detect backticks in single quotes", func() {
				// Single quotes prevent command substitution at shell level
				// The parser should not find any issues
				locations, err := p.FindAllBacktickIssues("echo 'Fix `parser`'")
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(BeEmpty())
			})
		})

		Context("with mixed quoting", func() {
			It("detects multiple contexts in one command", func() {
				// Unquoted and double-quoted backticks
				locations, err := p.FindAllBacktickIssues("command `arg1` \"arg2 `test`\"")
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(HaveLen(2))
			})

			It("handles complex command with variables", func() {
				locations, err := p.FindAllBacktickIssues(
					"command \"arg1 `test`\" `arg2` \"arg3 `foo` with $VAR\"",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(HaveLen(3))

				// Check that the one with $VAR has HasVariables=true
				var varLocation *parser.BacktickLocation
				for i := range locations {
					if locations[i].HasVariables {
						varLocation = &locations[i]

						break
					}
				}
				Expect(varLocation).NotTo(BeNil())
				Expect(varLocation.SuggestSingle).To(BeFalse())
			})
		})

		Context("with empty command", func() {
			It("returns error for empty string", func() {
				_, err := p.FindAllBacktickIssues("")
				Expect(err).To(MatchError(parser.ErrEmptyCommand))
			})

			It("returns error for whitespace-only", func() {
				_, err := p.FindAllBacktickIssues("   ")
				Expect(err).To(MatchError(parser.ErrEmptyCommand))
			})
		})

		Context("with invalid syntax", func() {
			It("returns error for malformed command", func() {
				_, err := p.FindAllBacktickIssues("echo \"unclosed")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with no backticks", func() {
			It("returns empty for normal commands", func() {
				locations, err := p.FindAllBacktickIssues("echo hello world")
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(BeEmpty())
			})

			It("returns empty for double-quoted strings without backticks", func() {
				locations, err := p.FindAllBacktickIssues("echo \"hello world\"")
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(BeEmpty())
			})
		})

		Context("with $() command substitution", func() {
			It("detects $() in double quotes as backtick issue", func() {
				locations, err := p.FindAllBacktickIssues("echo \"$(date)\"")
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(HaveLen(1))
			})

			It("detects unquoted $()", func() {
				locations, err := p.FindAllBacktickIssues("echo $(date)")
				Expect(err).NotTo(HaveOccurred())
				Expect(locations).To(HaveLen(1))
				Expect(locations[0].Context).To(Equal(parser.QuotingContextUnquoted))
			})
		})

		Context("directory context tracking with cd commands", func() {
			It("tracks directory change from cd command", func() {
				result, err := p.Parse("cd /tmp && git status")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(2))

				cdCmd := result.Commands[0]
				Expect(cdCmd.Name).To(Equal("cd"))
				Expect(cdCmd.Args).To(Equal([]string{"/tmp"}))
				Expect(cdCmd.WorkingDirectory).To(BeEmpty())

				gitCmd := result.Commands[1]
				Expect(gitCmd.Name).To(Equal("git"))
				Expect(gitCmd.WorkingDirectory).To(Equal("/tmp"))
			})

			It("tracks multiple directory changes", func() {
				result, err := p.Parse("cd /home && ls && cd /tmp && git status")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(4))

				Expect(result.Commands[0].Name).To(Equal("cd"))
				Expect(result.Commands[0].WorkingDirectory).To(BeEmpty())

				Expect(result.Commands[1].Name).To(Equal("ls"))
				Expect(result.Commands[1].WorkingDirectory).To(Equal("/home"))

				Expect(result.Commands[2].Name).To(Equal("cd"))
				Expect(result.Commands[2].WorkingDirectory).To(Equal("/home"))

				Expect(result.Commands[3].Name).To(Equal("git"))
				Expect(result.Commands[3].WorkingDirectory).To(Equal("/tmp"))
			})

			It("tracks directory context in user's failing scenario", func() {
				result, err := p.Parse(
					"cd ~/Projects/github.com/smykla-labs/smyklot && " +
						"git fetch upstream main && " +
						"git checkout -b feat/sync-smyklot-version upstream/main",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(3))

				cdCmd := result.Commands[0]
				Expect(cdCmd.Name).To(Equal("cd"))
				Expect(cdCmd.Args).To(Equal([]string{"~/Projects/github.com/smykla-labs/smyklot"}))
				Expect(cdCmd.WorkingDirectory).To(BeEmpty())

				fetchCmd := result.Commands[1]
				Expect(fetchCmd.Name).To(Equal("git"))
				Expect(fetchCmd.Args[0]).To(Equal("fetch"))
				Expect(fetchCmd.WorkingDirectory).To(
					Equal("~/Projects/github.com/smykla-labs/smyklot"),
				)

				checkoutCmd := result.Commands[2]
				Expect(checkoutCmd.Name).To(Equal("git"))
				Expect(checkoutCmd.Args[0]).To(Equal("checkout"))
				Expect(checkoutCmd.WorkingDirectory).To(
					Equal("~/Projects/github.com/smykla-labs/smyklot"),
				)
			})

			It("handles cd without affecting commands before it", func() {
				result, err := p.Parse("git status && cd /tmp && git log")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(3))

				Expect(result.Commands[0].Name).To(Equal("git"))
				Expect(result.Commands[0].WorkingDirectory).To(BeEmpty())

				Expect(result.Commands[1].Name).To(Equal("cd"))
				Expect(result.Commands[1].WorkingDirectory).To(BeEmpty())

				Expect(result.Commands[2].Name).To(Equal("git"))
				Expect(result.Commands[2].WorkingDirectory).To(Equal("/tmp"))
			})

			It("ignores cd without arguments", func() {
				result, err := p.Parse("cd && git status")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Commands).To(HaveLen(2))

				Expect(result.Commands[0].Name).To(Equal("cd"))
				Expect(result.Commands[0].WorkingDirectory).To(BeEmpty())

				Expect(result.Commands[1].Name).To(Equal("git"))
				Expect(result.Commands[1].WorkingDirectory).To(BeEmpty())
			})
		})
	})
})
