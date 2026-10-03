package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("GitCommand", func() {
	var p *parser.BashParser

	BeforeEach(func() {
		p = parser.NewBashParser()
	})

	Describe("ParseGitCommand", func() {
		Context("with non-git command", func() {
			It("returns error", func() {
				cmd := parser.Command{Name: "ls", Args: []string{"-la"}}
				_, err := parser.ParseGitCommand(cmd)
				Expect(err).To(MatchError(parser.ErrNotGitCommand))
			})
		})

		Context("with git command without subcommand", func() {
			It("returns error", func() {
				cmd := parser.Command{Name: "git", Args: []string{}}
				_, err := parser.ParseGitCommand(cmd)
				Expect(err).To(MatchError(parser.ErrNoSubcommand))
			})
		})

		DescribeTable("with blank subcommand",
			func(args []string) {
				_, err := parser.ParseGitCommand(parser.Command{Name: "git", Args: args})
				Expect(err).To(MatchError(parser.ErrNoSubcommand))
			},
			Entry("empty", []string{""}),
			Entry("space", []string{" "}),
			Entry("tab", []string{"\t"}),
			Entry("newline", []string{"\n"}),
			Entry("after global option", []string{"--no-pager", "", "commit"}),
			Entry("after -C value", []string{"-C", "/repo", " "}),
		)

		Context("with git commit command", func() {
			It("parses basic commit", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"commit", "-sS", "-m", "test message"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.Subcommand).To(Equal("commit"))
				// Combined flags -sS are split into individual flags
				Expect(gitCmd.Flags).To(ContainElements("-s", "-S", "-m"))
				Expect(gitCmd.GetFlagValue("-m")).To(Equal("test message"))
			})

			It("extracts commit message", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"commit", "-m", "feat: add feature"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.ExtractCommitMessage()).To(Equal("feat: add feature"))
			})

			It("extracts commit message with --message flag", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"commit", "--message", "fix: bug fix"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.ExtractCommitMessage()).To(Equal("fix: bug fix"))
			})

			It("captures the commit ref for -c/-C reuse flags", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"commit", "-c", "HEAD~1"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.GetFlagValue("-c")).To(Equal("HEAD~1"))
				Expect(gitCmd.Args).To(BeEmpty())
			})

			It("captures the commit ref for the --reedit-message long form", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"commit", "--reedit-message", "HEAD~1"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.GetFlagValue("--reedit-message")).To(Equal("HEAD~1"))
				Expect(gitCmd.Args).To(BeEmpty())
			})

			It("uses first -m flag as commit message when multiple -m flags provided", func() {
				// git commit -m "title" -m "body" should use "title" as the message
				cmd := parser.Command{
					Name: "git",
					Args: []string{
						"commit",
						"-sS",
						"-m",
						"feat: short title",
						"-m",
						"This is a longer body paragraph",
					},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.ExtractCommitMessage()).To(Equal("feat: short title"))
				// Both -m flags should be in Flags list
				Expect(gitCmd.Flags).To(ContainElements("-s", "-S", "-m", "-m"))
			})

			It("extracts heredoc from command substitution in commit message", func() {
				cmdStr := `git commit -sS -m "$(cat <<'EOF'
feat(validators): add new validator

This is a multi-line commit message
from a heredoc.
EOF
)"`
				result, err := p.Parse(cmdStr)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.GitOperations).To(HaveLen(1))

				gitCmd, err := parser.ParseGitCommand(result.GitOperations[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(
					gitCmd.ExtractCommitMessage(),
				).To(ContainSubstring("feat(validators): add new validator"))
				Expect(
					gitCmd.ExtractCommitMessage(),
				).To(ContainSubstring("This is a multi-line commit message"))
				Expect(gitCmd.ExtractCommitMessage()).To(ContainSubstring("from a heredoc."))
			})

			It("extracts heredoc without quotes in delimiter", func() {
				cmdStr := `git commit -sS -m "$(cat <<EOF
fix: quick fix

Body text here
EOF
)"`
				result, err := p.Parse(cmdStr)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.GitOperations).To(HaveLen(1))

				gitCmd, err := parser.ParseGitCommand(result.GitOperations[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.ExtractCommitMessage()).To(ContainSubstring("fix: quick fix"))
				Expect(gitCmd.ExtractCommitMessage()).To(ContainSubstring("Body text here"))
			})
		})

		Context("with git push command", func() {
			It("parses push with remote and branch", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"push", "upstream", "main"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.Subcommand).To(Equal("push"))
				Expect(gitCmd.ExtractRemote()).To(Equal("upstream"))
				Expect(gitCmd.ExtractBranchName()).To(Equal("main"))
			})

			It("parses push with force flag", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"push", "--force-with-lease", "origin", "feature"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.HasFlag("--force-with-lease")).To(BeTrue())
				Expect(gitCmd.ExtractRemote()).To(Equal("origin"))
			})
		})

		Context("with git checkout command", func() {
			It("parses checkout existing branch", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"checkout", "main"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.Subcommand).To(Equal("checkout"))
				Expect(gitCmd.ExtractBranchName()).To(Equal("main"))
			})

			It("parses checkout with -b flag", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"checkout", "-b", "feat/new-feature"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.HasFlag("-b")).To(BeTrue())
				Expect(gitCmd.ExtractBranchName()).To(Equal("feat/new-feature"))
			})

			It("captures the -b branch name despite a trailing flag", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"checkout", "-b", "feat/new-feature", "main", "--quiet"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.HasFlag("--quiet")).To(BeTrue())
				Expect(gitCmd.ExtractBranchName()).To(Equal("feat/new-feature"))
			})
		})

		Context("with git branch command", func() {
			It("parses branch creation", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"branch", "new-branch"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.ExtractBranchName()).To(Equal("new-branch"))
			})

			It("returns the new branch, not the start-point, on creation", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"branch", "new-branch", "upstream/main"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.ExtractBranchName()).To(Equal("new-branch"))
			})

			It("parses branch rename with -m", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"branch", "-m", "old-name", "new-name"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.HasFlag("-m")).To(BeTrue())
				Expect(gitCmd.ExtractBranchName()).To(Equal("new-name"))
			})

			It("parses single-argument branch rename with -m", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"branch", "-m", "renamed-branch"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.HasFlag("-m")).To(BeTrue())
				Expect(gitCmd.ExtractBranchName()).To(Equal("renamed-branch"))
			})
		})

		Context("with git switch command", func() {
			It("parses switch existing branch", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"switch", "main"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.ExtractBranchName()).To(Equal("main"))
			})

			It("parses switch with -c flag", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"switch", "-c", "feat/new-feature"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.HasFlag("-c")).To(BeTrue())
				Expect(gitCmd.ExtractBranchName()).To(Equal("feat/new-feature"))
			})
		})

		Context("with git add command", func() {
			It("extracts file paths", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"add", "file1.txt", "file2.txt"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.ExtractFilePaths()).To(Equal([]string{"file1.txt", "file2.txt"}))
			})

			It("extracts all files", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"add", "."},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.ExtractFilePaths()).To(Equal([]string{"."}))
			})
		})

		Context("with flags and values", func() {
			It("parses multiple flags", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"commit", "-sS", "--no-verify", "-m", "message"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				// Combined flag -sS is split into individual flags
				Expect(gitCmd.HasFlag("-s")).To(BeTrue())
				Expect(gitCmd.HasFlag("-S")).To(BeTrue())
				Expect(gitCmd.HasFlag("--no-verify")).To(BeTrue())
				Expect(gitCmd.HasFlag("-m")).To(BeTrue())
			})

			It("separates flags from positional args", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"push", "--force-with-lease", "upstream", "main"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.Flags).To(ContainElement("--force-with-lease"))
				Expect(gitCmd.Args).To(Equal([]string{"upstream", "main"}))
			})
		})
	})

	Describe("Global Options", func() {
		Context("with -C option", func() {
			It("parses git command with -C path before subcommand", func() {
				result, err := p.Parse(
					"git -C /path/to/repo checkout -b feat/new-feature upstream/main",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.GitOperations).To(HaveLen(1))

				gitCmd, err := parser.ParseGitCommand(result.GitOperations[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.Subcommand).To(Equal("checkout"))
				Expect(gitCmd.HasFlag("-b")).To(BeTrue())
				Expect(gitCmd.GetWorkingDirectory()).To(Equal("/path/to/repo"))
				Expect(gitCmd.ExtractBranchName()).To(Equal("feat/new-feature"))
			})

			It("parses git command with long path containing spaces", func() {
				result, err := p.Parse(`git -C "/path/with spaces/repo" commit -sS -m "test"`)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.GitOperations).To(HaveLen(1))

				gitCmd, err := parser.ParseGitCommand(result.GitOperations[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.Subcommand).To(Equal("commit"))
				Expect(gitCmd.GetWorkingDirectory()).To(Equal("/path/with spaces/repo"))
			})
		})

		Context("with --git-dir option", func() {
			It("parses git command with --git-dir", func() {
				result, err := p.Parse("git --git-dir=/custom/.git status")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.GitOperations).To(HaveLen(1))

				gitCmd, err := parser.ParseGitCommand(result.GitOperations[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.Subcommand).To(Equal("status"))
				Expect(gitCmd.GetGitDir()).To(Equal("/custom/.git"))
			})

			It("parses git command with --git-dir=value format", func() {
				result, err := p.Parse("git --git-dir=/custom/.git commit -m 'test'")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.GitOperations).To(HaveLen(1))

				gitCmd, err := parser.ParseGitCommand(result.GitOperations[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.Subcommand).To(Equal("commit"))
				Expect(gitCmd.GetGitDir()).To(Equal("/custom/.git"))
			})
		})

		Context("with multiple global options", func() {
			It("parses git command with -C and --no-pager", func() {
				result, err := p.Parse("git -C /repo --no-pager log --oneline")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.GitOperations).To(HaveLen(1))

				gitCmd, err := parser.ParseGitCommand(result.GitOperations[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.Subcommand).To(Equal("log"))
				Expect(gitCmd.GetWorkingDirectory()).To(Equal("/repo"))
				Expect(gitCmd.HasGlobalOption("--no-pager")).To(BeTrue())
			})
		})

		Context("with no global options", func() {
			It("parses normally without global options", func() {
				result, err := p.Parse("git checkout -b feat/feature")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.GitOperations).To(HaveLen(1))

				gitCmd, err := parser.ParseGitCommand(result.GitOperations[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.Subcommand).To(Equal("checkout"))
				Expect(gitCmd.HasFlag("-b")).To(BeTrue())
				Expect(gitCmd.GetWorkingDirectory()).To(BeEmpty())
			})
		})

		Context("with -c config option", func() {
			It("parses git command with -c config", func() {
				result, err := p.Parse("git -c user.name=Test commit -m 'test'")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.GitOperations).To(HaveLen(1))

				gitCmd, err := parser.ParseGitCommand(result.GitOperations[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.Subcommand).To(Equal("commit"))
				Expect(gitCmd.HasGlobalOption("-c")).To(BeTrue())
			})
		})
	})

	Describe("Integration with BashParser", func() {
		It("parses and extracts git command details", func() {
			result, err := p.Parse("git commit -sS -m 'feat: add feature'")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.GitOperations).To(HaveLen(1))

			gitCmd, err := parser.ParseGitCommand(result.GitOperations[0])
			Expect(err).NotTo(HaveOccurred())
			Expect(gitCmd.Subcommand).To(Equal("commit"))
			Expect(gitCmd.ExtractCommitMessage()).To(Equal("feat: add feature"))
		})

		It("parses chained git commands", func() {
			result, err := p.Parse("git add . && git commit -m 'msg' && git push upstream main")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.GitOperations).To(HaveLen(3))

			// First command: git add
			gitAdd, err := parser.ParseGitCommand(result.GitOperations[0])
			Expect(err).NotTo(HaveOccurred())
			Expect(gitAdd.Subcommand).To(Equal("add"))

			// Second command: git commit
			gitCommit, err := parser.ParseGitCommand(result.GitOperations[1])
			Expect(err).NotTo(HaveOccurred())
			Expect(gitCommit.Subcommand).To(Equal("commit"))

			// Third command: git push
			gitPush, err := parser.ParseGitCommand(result.GitOperations[2])
			Expect(err).NotTo(HaveOccurred())
			Expect(gitPush.Subcommand).To(Equal("push"))
			Expect(gitPush.ExtractRemote()).To(Equal("upstream"))
		})

		It("keeps a variable -F value aligned with the -- separator", func() {
			// "$MSG" renders to a stable braced token, so -F captures it instead
			// of grabbing "--", and the pathspec stays a positional argument.
			result, err := p.Parse(`git commit -sS -F "$MSG" -- file.go`)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.GitOperations).To(HaveLen(1))

			gitCmd, err := parser.ParseGitCommand(result.GitOperations[0])
			Expect(err).NotTo(HaveOccurred())
			Expect(gitCmd.GetFlagValue("-F")).To(Equal("${MSG}"))
			Expect(gitCmd.Args).To(ContainElement("file.go"))
		})
	})

	Describe("GetWorkingDirectory", func() {
		Context("with -C flag", func() {
			It("returns -C flag directory", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"-C", "/path/to/repo", "status"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.GetWorkingDirectory()).To(Equal("/path/to/repo"))
			})
		})

		Context("with WorkingDirectory from cd command", func() {
			It("returns WorkingDirectory when no -C flag", func() {
				cmd := parser.Command{
					Name:             "git",
					Args:             []string{"status"},
					WorkingDirectory: "/tmp/project",
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.GetWorkingDirectory()).To(Equal("/tmp/project"))
			})
		})

		Context("with both -C flag and WorkingDirectory", func() {
			It("lets an absolute -C override the cd directory", func() {
				cmd := parser.Command{
					Name:             "git",
					Args:             []string{"-C", "/explicit/path", "status"},
					WorkingDirectory: "/cd/path",
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.GetWorkingDirectory()).To(Equal("/explicit/path"))
			})

			It("joins a relative -C onto the cd directory", func() {
				// "cd a && git -C b" runs in a/b.
				cmd := parser.Command{
					Name:             "git",
					Args:             []string{"-C", "b", "status"},
					WorkingDirectory: "a",
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.GetWorkingDirectory()).To(Equal("a/b"))
			})
		})

		Context("with a relative -C flag and no cd", func() {
			It("returns the -C value", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"-C", "sub", "status"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.GetWorkingDirectory()).To(Equal("sub"))
			})
		})

		Context("with neither -C flag nor WorkingDirectory", func() {
			It("returns empty string", func() {
				cmd := parser.Command{
					Name: "git",
					Args: []string{"status"},
				}

				gitCmd, err := parser.ParseGitCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(gitCmd.GetWorkingDirectory()).To(BeEmpty())
			})
		})

		Context("in user's failing scenario", func() {
			It("extracts working directory from cd command for git operations", func() {
				result, err := p.Parse(
					"cd ~/Projects/github.com/smykla-skalski/smyklot && git fetch upstream main",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.GitOperations).To(HaveLen(1))

				gitFetch, err := parser.ParseGitCommand(result.GitOperations[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(gitFetch.Subcommand).To(Equal("fetch"))
				Expect(gitFetch.GetWorkingDirectory()).To(
					Equal("~/Projects/github.com/smykla-skalski/smyklot"),
				)
			})
		})
	})
})
