package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
)

const harnessFixtureDir = "../../internal/harness/testdata/fixtures"

// Each captured harness payload is fed to the current klaudiush build, the
// way the harness's hook config runs it. The response must still use only
// fields the provider accepts for the event and ask for the outcome the
// harness saw when the fixture was captured.
var _ = Describe("captured harness payloads", func() {
	fixtures, err := harness.LoadFixtures(harnessFixtureDir)
	if err != nil {
		It("load", func() { Expect(err).NotTo(HaveOccurred()) })

		return
	}

	It("cover every live-checked provider", func() {
		providers := map[string]bool{}
		for _, fixture := range fixtures {
			providers[string(fixture.Provider)] = true
		}

		Expect(providers).To(HaveKey("claude"))
		Expect(providers).To(HaveKey("codex"))
		Expect(providers).To(HaveKey("gemini"))
		Expect(providers).To(HaveKey("opencode"))
	})

	for _, fixture := range fixtures {
		name := filepath.Join(
			filepath.Base(filepath.Dir(fixture.Path())),
			filepath.Base(fixture.Path()),
		)

		It("replays "+name, func() {
			Expect(fixture.Validate()).To(Succeed())

			if fixture.ReplaySkip != "" {
				Skip(fixture.ReplaySkip)
			}

			replayFixture(fixture)
		})
	}
})

func replayFixture(fixture harness.Fixture) {
	root := GinkgoT().TempDir()
	work := filepath.Join(root, "work")
	home := filepath.Join(root, "home")

	Expect(os.MkdirAll(work, 0o750)).To(Succeed())
	Expect(os.MkdirAll(home, 0o750)).To(Succeed())

	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"CODEX_HOME=" + filepath.Join(home, ".codex"),
		"KLAUDIUSH_USE_SDK_GIT=false",
	}

	git := exec.Command("git", "init", "--quiet", "--initial-branch=main")
	git.Dir = work
	git.Env = env
	Expect(git.Run()).To(Succeed())

	files := map[string]string{}
	if fixture.Config != "" {
		files[filepath.Join(work, ".klaudiush", "config.toml")] = fixture.Config
	}

	for name, content := range fixture.Workspace {
		path := harness.Expand(name, work, home)
		if !filepath.IsAbs(path) {
			path = filepath.Join(work, path)
		}

		files[path] = content
	}

	for path, content := range files {
		Expect(os.MkdirAll(filepath.Dir(path), 0o750)).To(Succeed())
		Expect(os.WriteFile(path, []byte(harness.Expand(content, work, home)), 0o600)).To(Succeed())
	}

	// Run this test binary itself, not whatever klaudiush is on PATH:
	// testscript.Main (TestMain) runs the CLI when argv[0] is "klaudiush".
	self, err := os.Executable()
	Expect(err).NotTo(HaveOccurred())

	cmd := exec.Command(self, fixture.Args...)
	cmd.Args[0] = "klaudiush"
	cmd.Dir = work
	cmd.Env = env
	cmd.Stdin = strings.NewReader(harness.Expand(string(fixture.Payload), work, home))

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	Expect(cmd.Run()).To(Succeed(), stderr.String())
	Expect(fixture.CheckResponse(stdout.Bytes())).To(Succeed(), stdout.String())
}
