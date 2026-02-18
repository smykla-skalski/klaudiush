package git_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalgit "github.com/smykla-skalski/klaudiush/internal/git"
)

var _ = Describe("ExcludeManager", func() {
	var (
		tempDir string
		manager *internalgit.ExcludeManager
		err     error
	)

	BeforeEach(func() {
		tempDir, err = os.MkdirTemp("", "exclude-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Resolve symlinks (macOS /var -> /private/var)
		tempDir, err = filepath.EvalSymlinks(tempDir)
		Expect(err).NotTo(HaveOccurred())

		_, err = git.PlainInit(tempDir, false)
		Expect(err).NotTo(HaveOccurred())

		manager = internalgit.NewExcludeManagerFromRoot(tempDir)
	})

	AfterEach(func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Describe("GetExcludePath", func() {
		It("returns the correct path to .git/info/exclude", func() {
			expected := filepath.Join(tempDir, ".git", "info", "exclude")
			Expect(manager.GetExcludePath()).To(Equal(expected))
		})
	})

	Describe("HasEntry", func() {
		Context("when the exclude file does not exist", func() {
			It("returns false without error", func() {
				has, err := manager.HasEntry("*.log") //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(has).To(BeFalse())
			})
		})

		Context("when the exclude file exists with entries", func() {
			BeforeEach(func() {
				infoDir := filepath.Join(tempDir, ".git", "info")
				err = os.MkdirAll(infoDir, internalgit.ExcludeDirMode)
				Expect(err).NotTo(HaveOccurred())

				content := "# a comment\n*.log\ntmp/\n"
				err = os.WriteFile(
					manager.GetExcludePath(),
					[]byte(content),
					internalgit.ExcludeFileMode,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns true for an existing entry", func() {
				has, err := manager.HasEntry("*.log") //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(has).To(BeTrue())
			})

			It("returns true for another existing entry", func() {
				has, err := manager.HasEntry("tmp/") //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(has).To(BeTrue())
			})

			It("returns false for a non-existing pattern", func() {
				has, err := manager.HasEntry("*.tmp") //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(has).To(BeFalse())
			})

			It("does not match comment lines as entries", func() {
				has, err := manager.HasEntry("# a comment") //nolint:govet // shadow
				Expect(err).NotTo(HaveOccurred())
				Expect(has).To(BeFalse())
			})
		})
	})

	Describe("AddEntry", func() {
		Context("when the .git/info directory does not exist", func() {
			BeforeEach(func() {
				_ = os.RemoveAll(filepath.Join(tempDir, ".git", "info"))
			})

			It("creates the directory and adds the entry", func() {
				err = manager.AddEntry("*.log")
				Expect(err).NotTo(HaveOccurred())

				has, hasErr := manager.HasEntry("*.log")
				Expect(hasErr).NotTo(HaveOccurred())
				Expect(has).To(BeTrue())
			})
		})

		Context("when adding a new entry to an empty file", func() {
			It("writes the comment header and pattern", func() {
				err = manager.AddEntry("tmp/")
				Expect(err).NotTo(HaveOccurred())

				content, readErr := os.ReadFile(manager.GetExcludePath())
				Expect(readErr).NotTo(HaveOccurred())
				Expect(string(content)).To(ContainSubstring("# Added by klaudiush"))
				Expect(string(content)).To(ContainSubstring("tmp/"))
			})

			It("makes the entry discoverable via HasEntry", func() {
				err = manager.AddEntry("build/")
				Expect(err).NotTo(HaveOccurred())

				has, hasErr := manager.HasEntry("build/")
				Expect(hasErr).NotTo(HaveOccurred())
				Expect(has).To(BeTrue())
			})
		})

		Context("when the existing file has no trailing newline", func() {
			BeforeEach(func() {
				infoDir := filepath.Join(tempDir, ".git", "info")
				err = os.MkdirAll(infoDir, internalgit.ExcludeDirMode)
				Expect(err).NotTo(HaveOccurred())

				err = os.WriteFile(
					manager.GetExcludePath(),
					[]byte("*.log"),
					internalgit.ExcludeFileMode,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("inserts a newline so entries are on separate lines", func() {
				err = manager.AddEntry("tmp/")
				Expect(err).NotTo(HaveOccurred())

				content, readErr := os.ReadFile(manager.GetExcludePath())
				Expect(readErr).NotTo(HaveOccurred())
				Expect(strings.Contains(string(content), "*.log\n")).To(BeTrue())
				Expect(string(content)).To(ContainSubstring("tmp/"))
			})
		})

		Context("when adding a duplicate entry", func() {
			BeforeEach(func() {
				err = manager.AddEntry("*.log")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns ErrEntryAlreadyExists", func() {
				err = manager.AddEntry("*.log")
				Expect(err).To(MatchError(internalgit.ErrEntryAlreadyExists))
			})
		})
	})
})

var _ = Describe("IsInGitRepo", func() {
	var (
		tempDir string
		origDir string
		err     error
	)

	BeforeEach(func() {
		origDir, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		tempDir, err = os.MkdirTemp("", "isingitrepo-test-*")
		Expect(err).NotTo(HaveOccurred())

		tempDir, err = filepath.EvalSymlinks(tempDir)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.Chdir(origDir)
		_ = os.RemoveAll(tempDir)
	})

	Context("when inside a git repository", func() {
		BeforeEach(func() {
			_, err = git.PlainInit(tempDir, false)
			Expect(err).NotTo(HaveOccurred())

			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			internalgit.ResetRepositoryCache()
		})

		It("returns true", func() {
			Expect(internalgit.IsInGitRepo()).To(BeTrue())
		})
	})

	Context("when outside any git repository", func() {
		BeforeEach(func() {
			err = os.Chdir(tempDir)
			Expect(err).NotTo(HaveOccurred())

			internalgit.ResetRepositoryCache()
		})

		It("returns false", func() {
			Expect(internalgit.IsInGitRepo()).To(BeFalse())
		})
	})
})
