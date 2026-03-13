package config

import (
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v6"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func initGitRepo(dir string) {
	_, err := gogit.PlainInit(dir, false)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("ResolveProjectRoot", func() {
	It("returns the walked-up project config root from a nested subdirectory", func() {
		_, homeDir, projectRoot, workDir := newWalkupLoader(2)

		DeferCleanup(func() { os.RemoveAll(homeDir) })

		writeConfigAt(projectRoot, "version = 1\n")

		root, err := ResolveProjectRootWithDirs(homeDir, workDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(root).To(Equal(projectRoot))
	})

	It("falls back to the git repository root when no project config exists", func() {
		homeDir, err := os.MkdirTemp("", "project-root-home-")
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() { os.RemoveAll(homeDir) })

		projectRoot := filepath.Join(homeDir, "projects", "git-only")
		workDir := filepath.Join(projectRoot, "sub", "dir")
		Expect(os.MkdirAll(workDir, 0o755)).To(Succeed())
		initGitRepo(projectRoot)

		root, err := ResolveProjectRootWithDirs(homeDir, workDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(root).To(Equal(projectRoot))
	})

	It("prefers the project config root over the outer git repository root", func() {
		homeDir, err := os.MkdirTemp("", "project-root-home-")
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() { os.RemoveAll(homeDir) })

		repoRoot := filepath.Join(homeDir, "projects", "monorepo")
		projectRoot := filepath.Join(repoRoot, "apps", "service")
		workDir := filepath.Join(projectRoot, "sub")
		Expect(os.MkdirAll(workDir, 0o755)).To(Succeed())
		initGitRepo(repoRoot)
		writeConfigAt(projectRoot, "version = 1\n")

		root, err := ResolveProjectRootWithDirs(homeDir, workDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(root).To(Equal(projectRoot))
	})

	It("falls back to the current work directory when no config or git repo exists", func() {
		homeDir, err := os.MkdirTemp("", "project-root-home-")
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() { os.RemoveAll(homeDir) })

		workDir := filepath.Join(homeDir, "scratch", "nested")
		Expect(os.MkdirAll(workDir, 0o755)).To(Succeed())

		root, err := ResolveProjectRootWithDirs(homeDir, workDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(root).To(Equal(workDir))
	})
})
