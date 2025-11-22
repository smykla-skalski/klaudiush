package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/claude-hooks/pkg/parser"
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
				Expect(gitCmd.ExtractCommitMessage()).To(ContainSubstring("feat(validators): add new validator"))
				Expect(gitCmd.ExtractCommitMessage()).To(ContainSubstring("This is a multi-line commit message"))
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
	})
})
