package evidence_test

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

func git(dir string, args ...string) {
	cmd := osexec.Command("git", args...)
	cmd.Dir = dir

	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
	)
	out, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), string(out))
}

func write(dir, name, content string) {
	path := filepath.Join(dir, name)
	Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
	Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())
}

var _ = Describe("Snapshot", func() {
	var (
		ctx    context.Context
		repo   string
		tests  *evidence.Check
		review *evidence.Check
	)

	digest := func(check *evidence.Check) (string, int) {
		snap, err := evidence.TakeSnapshot(ctx, repo)
		Expect(err).NotTo(HaveOccurred())

		sum, files, err := snap.ContentDigest(ctx, check)
		Expect(err).NotTo(HaveOccurred())

		return sum, files
	}

	diff := func() evidence.Diff {
		snap, err := evidence.TakeSnapshot(ctx, repo)
		Expect(err).NotTo(HaveOccurred())

		result, err := snap.ReviewDiff(ctx, review)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	BeforeEach(func() {
		ctx = context.Background()
		repo = GinkgoT().TempDir()

		git(repo, "init", "-q")
		write(repo, "a.go", "package a\n")
		write(repo, "pkg/b.go", "package pkg\n")
		write(repo, "README.md", "readme\n")
		write(repo, ".gitignore", "ignored.go\n")
		git(repo, "add", "-A")
		git(repo, "commit", "-qm", "init")
		git(repo, "branch", "feat/base")

		checks, err := evidence.Compile(&config.EvidenceConfig{
			Checks: []*config.EvidenceCheckConfig{
				{Name: "tests", Commands: []string{"t"}, Paths: []string{"**/*.go"}},
				{
					Name:     "review",
					Kind:     config.EvidenceKindReview,
					Commands: []string{"r"},
					Paths:    []string{"**/*.go"},
					Base:     "feat/base",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		tests, review = checks[0], checks[1]
	})

	It("finds the repository root from a subdirectory", func() {
		root, err := evidence.RepoRoot(ctx, filepath.Join(repo, "pkg"))
		Expect(err).NotTo(HaveOccurred())

		expected, err := filepath.EvalSymlinks(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(root).To(Equal(expected))
	})

	It("refuses directories outside a repository", func() {
		_, err := evidence.RepoRoot(ctx, GinkgoT().TempDir())
		Expect(err).To(MatchError(evidence.ErrNotRepository))

		_, err = evidence.RepoRoot(ctx, "")
		Expect(err).To(MatchError(evidence.ErrNotRepository))
	})

	It("ignores the caller's git environment", func() {
		GinkgoT().Setenv("GIT_DIR", filepath.Join(GinkgoT().TempDir(), "nowhere"))

		snap, err := evidence.TakeSnapshot(ctx, repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(snap.Root()).To(Equal(repo))

		_, files, err := snap.ContentDigest(ctx, tests)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(Equal(2))
	})

	It("is stable while nothing changes", func() {
		first, files := digest(tests)
		second, _ := digest(tests)

		Expect(first).To(HavePrefix("sha256:"))
		Expect(first).To(Equal(second))
		Expect(files).To(Equal(2))
	})

	It("changes with covered files only", func() {
		before, _ := digest(tests)

		write(repo, "README.md", "changed\n")
		write(repo, "ignored.go", "package ignored\n")

		unrelated, _ := digest(tests)
		Expect(unrelated).To(Equal(before))

		write(repo, "pkg/b.go", "package pkg\n\nvar X = 1\n")

		changed, _ := digest(tests)
		Expect(changed).NotTo(Equal(before))

		write(repo, "pkg/b.go", "package pkg\n")

		reverted, _ := digest(tests)
		Expect(reverted).To(Equal(before))
	})

	It("counts untracked and deleted covered files", func() {
		before, _ := digest(tests)

		write(repo, "new.go", "package a\n")

		added, files := digest(tests)
		Expect(added).NotTo(Equal(before))
		Expect(files).To(Equal(3))

		Expect(os.Remove(filepath.Join(repo, "new.go"))).To(Succeed())
		Expect(os.Remove(filepath.Join(repo, "a.go"))).To(Succeed())

		removed, files := digest(tests)
		Expect(removed).NotTo(Equal(before))
		Expect(files).To(Equal(1))
	})

	It("changes when a covered file becomes executable or a symlink", func() {
		before, _ := digest(tests)

		Expect(os.Chmod(filepath.Join(repo, "a.go"), 0o755)).To(Succeed())

		chmodded, _ := digest(tests)
		Expect(chmodded).NotTo(Equal(before))

		Expect(os.Remove(filepath.Join(repo, "a.go"))).To(Succeed())
		Expect(os.Symlink("pkg/b.go", filepath.Join(repo, "a.go"))).To(Succeed())

		linked, _ := digest(tests)
		Expect(linked).NotTo(Equal(before))
		Expect(linked).NotTo(Equal(chmodded))
	})

	It("identifies the exact diff against the review base", func() {
		clean := diff()
		Expect(clean.Changed).To(BeEmpty())
		Expect(clean.Base).To(HaveLen(40))

		write(repo, "a.go", "package a\n\nvar Y = 2\n")
		write(repo, "c.go", "package a\n")
		write(repo, "README.md", "changed\n")

		changed := diff()
		Expect(changed.Changed).To(Equal([]string{"a.go", "c.go"}))
		Expect(changed.Digest).NotTo(Equal(clean.Digest))
		Expect(changed.Base).To(Equal(clean.Base))

		write(repo, "a.go", "package a\n\nvar Y = 3\n")

		edited := diff()
		Expect(edited.Changed).To(Equal(changed.Changed))
		Expect(edited.Digest).NotTo(Equal(changed.Digest))

		git(repo, "add", "-A")
		git(repo, "commit", "-qm", "next")

		committed := diff()
		Expect(committed).To(Equal(edited))
	})

	It("reports a review base that does not exist", func() {
		checks, err := evidence.Compile(&config.EvidenceConfig{
			Checks: []*config.EvidenceCheckConfig{{
				Name:     "review",
				Kind:     config.EvidenceKindReview,
				Commands: []string{"r"},
				Base:     "no-such-branch",
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		snap, err := evidence.TakeSnapshot(ctx, repo)
		Expect(err).NotTo(HaveOccurred())

		_, err = snap.ReviewDiff(ctx, checks[0])
		Expect(err).To(MatchError(ContainSubstring("no-such-branch")))
	})

	It("stops hashing when the context ends", func() {
		snap, err := evidence.TakeSnapshot(ctx, repo)
		Expect(err).NotTo(HaveOccurred())

		canceled, cancel := context.WithCancel(ctx)
		cancel()

		_, _, err = snap.ContentDigest(canceled, tests)
		Expect(err).To(MatchError(context.Canceled))
	})

	It("changes when the repository exclude file changes", func() {
		before, _ := digest(tests)

		write(repo, ".git/info/exclude", "hack.go\n")

		after, _ := digest(tests)
		Expect(after).NotTo(Equal(before))
	})

	It("tracks a submodule by its commit and uncommitted changes", func() {
		sub := filepath.Join(repo, "sub")
		write(repo, "sub/s.go", "package s\n")
		git(sub, "init", "-q")
		git(sub, "add", "-A")
		git(sub, "commit", "-qm", "sub")
		git(repo, "add", "sub")

		checks, err := evidence.Compile(&config.EvidenceConfig{
			Checks: []*config.EvidenceCheckConfig{{Name: "all", Commands: []string{"t"}}},
		})
		Expect(err).NotTo(HaveOccurred())

		all := checks[0]
		clean, _ := digest(all)

		write(repo, "sub/s.go", "package s\n\nvar Z = 1\n")

		dirty, _ := digest(all)
		Expect(dirty).NotTo(Equal(clean))

		Expect(os.RemoveAll(sub)).To(Succeed())
		Expect(os.Mkdir(sub, 0o755)).To(Succeed())

		uninitialized, _ := digest(all)
		Expect(uninitialized).NotTo(Equal(clean))
	})

	It("fails when git cannot read part of the work tree", func() {
		if os.Geteuid() == 0 {
			Skip("root reads every directory")
		}

		hidden := filepath.Join(repo, "hidden")
		write(repo, "hidden/new.go", "package hidden\n")
		Expect(os.Chmod(hidden, 0o000)).To(Succeed())
		DeferCleanup(os.Chmod, hidden, os.FileMode(0o755))

		_, err := evidence.TakeSnapshot(ctx, repo)
		Expect(err).To(MatchError(evidence.ErrUnreadable))

		Expect(os.Chmod(hidden, 0o755)).To(Succeed())

		snap, err := evidence.TakeSnapshot(ctx, repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chmod(hidden, 0o000)).To(Succeed())

		_, err = snap.ReviewDiff(ctx, review)
		Expect(err).To(MatchError(evidence.ErrUnreadable))
	})

	It("fails to list a directory that is not a repository", func() {
		_, err := evidence.TakeSnapshot(ctx, GinkgoT().TempDir())
		Expect(err).To(HaveOccurred())
	})
})
