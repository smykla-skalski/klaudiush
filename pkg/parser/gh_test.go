package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/parser"
)

var _ = Describe("GHMergeCommand", func() {
	Describe("ParseGHMergeCommand", func() {
		Context("with non-gh command", func() {
			It("returns error", func() {
				cmd := parser.Command{Name: "git", Args: []string{"commit", "-m", "test"}}
				_, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).To(MatchError(parser.ErrNotGHCommand))
			})
		})

		Context("with gh command but not pr merge", func() {
			It("returns error for gh pr create", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "create"}}
				_, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).To(MatchError(parser.ErrNotPRMergeCommand))
			})

			It("returns error for gh issue list", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"issue", "list"}}
				_, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).To(MatchError(parser.ErrNotPRMergeCommand))
			})

			It("returns error for gh with insufficient args", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr"}}
				_, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).To(MatchError(parser.ErrNotPRMergeCommand))
			})
		})

		Context("with basic gh pr merge command", func() {
			It("parses simple merge", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.PRNumber).To(Equal(0))
				Expect(ghCmd.Squash).To(BeFalse())
				Expect(ghCmd.Auto).To(BeFalse())
			})

			It("parses merge with PR number", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "123"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.PRNumber).To(Equal(123))
			})

			It("parses merge with PR URL", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "https://github.com/org/repo/pull/456"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.PRNumber).To(Equal(456))
			})
		})

		Context("with merge method flags", func() {
			It("parses --squash flag", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "--squash"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Squash).To(BeTrue())
				Expect(ghCmd.IsSquashMerge()).To(BeTrue())
			})

			It("parses -s flag (squash)", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "-s"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Squash).To(BeTrue())
			})

			It("parses --merge flag", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "--merge"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Merge).To(BeTrue())
				Expect(ghCmd.IsSquashMerge()).To(BeFalse())
			})

			It("parses -m flag (merge)", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "-m"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Merge).To(BeTrue())
			})

			It("parses --rebase flag", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "--rebase"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Rebase).To(BeTrue())
				Expect(ghCmd.IsSquashMerge()).To(BeFalse())
			})

			It("parses -r flag (rebase)", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "-r"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Rebase).To(BeTrue())
			})
		})

		Context("with auto-merge flags", func() {
			It("parses --auto flag", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "--auto"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Auto).To(BeTrue())
				Expect(ghCmd.IsAutoMerge()).To(BeTrue())
			})

			It("parses --disable-auto flag", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "--disable-auto"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.DisableAuto).To(BeTrue())
			})

			It("parses auto merge with squash", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "--auto", "--squash"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Auto).To(BeTrue())
				Expect(ghCmd.Squash).To(BeTrue())
			})
		})

		Context("with other flags", func() {
			It("parses --delete-branch flag", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "--delete-branch"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Delete).To(BeTrue())
			})

			It("parses -d flag (delete)", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "-d"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Delete).To(BeTrue())
			})

			It("parses --admin flag", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "--admin"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Admin).To(BeTrue())
			})
		})

		Context("with value flags", func() {
			It("parses --subject flag", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "--subject", "Merge PR"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Subject).To(Equal("Merge PR"))
			})

			It("parses -t flag (subject)", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "-t", "feat: new feature"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Subject).To(Equal("feat: new feature"))
			})

			It("parses --subject=value format", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "--subject=Merge PR"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Subject).To(Equal("Merge PR"))
			})

			It("parses --body flag", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "--body", "PR body content"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Body).To(Equal("PR body content"))
			})

			It("parses -b flag (body)", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "-b", "Body text"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Body).To(Equal("Body text"))
			})

			It("parses --body-file flag", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "--body-file", "/path/to/body.md"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.BodyFile).To(Equal("/path/to/body.md"))
			})

			It("parses -F flag (body-file)", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "-F", "body.txt"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.BodyFile).To(Equal("body.txt"))
			})

			It("parses --repo flag", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "--repo", "owner/repo"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Repo).To(Equal("owner/repo"))
			})

			It("parses -R flag (repo)", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "-R", "org/project"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Repo).To(Equal("org/project"))
			})

			It("parses --repo=value format", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "--repo=owner/repo"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Repo).To(Equal("owner/repo"))
			})

			It("parses --match-head-commit flag", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "--match-head-commit", "abc123"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Match).To(Equal("abc123"))
			})
		})

		Context("with complex command combinations", func() {
			It("parses full squash merge command", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{
						"pr", "merge", "42",
						"--squash",
						"--delete-branch",
						"--subject", "feat(api): add new endpoint",
						"--body", "Adds new API endpoint",
						"--repo", "owner/repo",
					},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.PRNumber).To(Equal(42))
				Expect(ghCmd.Squash).To(BeTrue())
				Expect(ghCmd.Delete).To(BeTrue())
				Expect(ghCmd.Subject).To(Equal("feat(api): add new endpoint"))
				Expect(ghCmd.Body).To(Equal("Adds new API endpoint"))
				Expect(ghCmd.Repo).To(Equal("owner/repo"))
			})

			It("parses auto-merge with squash command", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{
						"pr", "merge",
						"--auto",
						"--squash",
						"-d",
					},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.Auto).To(BeTrue())
				Expect(ghCmd.Squash).To(BeTrue())
				Expect(ghCmd.Delete).To(BeTrue())
				Expect(ghCmd.IsAutoMerge()).To(BeTrue())
				Expect(ghCmd.IsSquashMerge()).To(BeTrue())
			})

			It("stores raw args for debugging", func() {
				cmd := parser.Command{
					Name: "gh",
					Args: []string{"pr", "merge", "123", "--squash"},
				}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.RawArgs).To(Equal([]string{"pr", "merge", "123", "--squash"}))
			})
		})

		Context("IsSquashMerge logic", func() {
			It("returns true when no method specified (default)", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.IsSquashMerge()).To(BeTrue())
			})

			It("returns true when --squash specified", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "--squash"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.IsSquashMerge()).To(BeTrue())
			})

			It("returns false when --merge specified", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "--merge"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.IsSquashMerge()).To(BeFalse())
			})

			It("returns false when --rebase specified", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge", "--rebase"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.IsSquashMerge()).To(BeFalse())
			})
		})

		Context("NeedsPRFetch", func() {
			It("always returns true", func() {
				cmd := parser.Command{Name: "gh", Args: []string{"pr", "merge"}}
				ghCmd, err := parser.ParseGHMergeCommand(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(ghCmd.NeedsPRFetch()).To(BeTrue())
			})
		})
	})

	Describe("IsGHPRMerge", func() {
		It("returns true for gh pr merge command", func() {
			cmd := &parser.Command{Name: "gh", Args: []string{"pr", "merge"}}
			Expect(parser.IsGHPRMerge(cmd)).To(BeTrue())
		})

		It("returns false for non-gh command", func() {
			cmd := &parser.Command{Name: "git", Args: []string{"commit"}}
			Expect(parser.IsGHPRMerge(cmd)).To(BeFalse())
		})

		It("returns false for gh pr create", func() {
			cmd := &parser.Command{Name: "gh", Args: []string{"pr", "create"}}
			Expect(parser.IsGHPRMerge(cmd)).To(BeFalse())
		})

		It("returns false for insufficient args", func() {
			cmd := &parser.Command{Name: "gh", Args: []string{"pr"}}
			Expect(parser.IsGHPRMerge(cmd)).To(BeFalse())
		})
	})
})
