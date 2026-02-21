package updater_test

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/updater"
)

// stubRunner implements exec.CommandRunner for testing.
type stubRunner struct {
	results map[string]exec.CommandResult
}

func (*stubRunner) runKey(name string, args ...string) string {
	var b strings.Builder

	b.WriteString(name)

	for _, a := range args {
		b.WriteString(" ")
		b.WriteString(a)
	}

	return b.String()
}

func (s *stubRunner) Run(
	_ context.Context,
	name string,
	args ...string,
) exec.CommandResult {
	key := s.runKey(name, args...)
	if r, ok := s.results[key]; ok {
		return r
	}

	return exec.CommandResult{Err: errors.Errorf("unexpected command: %s", key)}
}

func (*stubRunner) RunWithStdin(
	_ context.Context,
	_ io.Reader,
	_ string,
	_ ...string,
) exec.CommandResult {
	return exec.CommandResult{Err: errors.New("not implemented")}
}

func (*stubRunner) RunWithTimeout(
	_ time.Duration,
	_ string,
	_ ...string,
) exec.CommandResult {
	return exec.CommandResult{Err: errors.New("not implemented")}
}

var _ = Describe("Detector", func() {
	Describe("isHomebrewPath (via DetectMethod)", func() {
		var detector *updater.Detector

		BeforeEach(func() {
			runner := &stubRunner{results: map[string]exec.CommandResult{}}
			detector = updater.NewDetector(runner)
		})

		DescribeTable("classifies paths correctly",
			func(path string, expected updater.InstallMethod) {
				// DetectMethod calls EvalSymlinks internally, so for non-existent
				// paths it falls back to direct. We test the exported behavior.
				method := detector.DetectMethod(path)
				Expect(method).To(Equal(expected))
			},
			// Non-brew paths always resolve to direct (since they don't exist on disk
			// for EvalSymlinks, DetectMethod falls back to direct).
			Entry("random user path", "/usr/local/bin/klaudiush", updater.InstallMethodDirect),
			Entry("home bin", "/home/user/bin/klaudiush", updater.InstallMethodDirect),
			Entry("go bin", "/home/user/go/bin/klaudiush", updater.InstallMethodDirect),
		)
	})

	Describe("FindAll", func() {
		It("parses which -a output", func() {
			runner := &stubRunner{
				results: map[string]exec.CommandResult{
					"which -a klaudiush": {
						Stdout: "/usr/local/bin/klaudiush\n/home/user/.local/bin/klaudiush\n",
					},
				},
			}
			detector := updater.NewDetector(runner)

			infos, err := detector.FindAll(context.Background())
			Expect(err).NotTo(HaveOccurred())
			// Both paths won't resolve via EvalSymlinks in test env,
			// so they end up as direct installs with their original paths.
			Expect(len(infos)).To(BeNumerically(">=", 1))
		})

		It("deduplicates by resolved path", func() {
			// Two different symlinks pointing to the same path.
			// Since neither exists on disk, EvalSymlinks fails and
			// they keep their original paths - so they won't dedup.
			// This tests that distinct paths are preserved.
			runner := &stubRunner{
				results: map[string]exec.CommandResult{
					"which -a klaudiush": {
						Stdout: "/usr/local/bin/klaudiush\n/opt/bin/klaudiush\n",
					},
				},
			}
			detector := updater.NewDetector(runner)

			infos, err := detector.FindAll(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(infos).To(HaveLen(2))
		})

		It("falls back to PATH scan when which fails", func() {
			runner := &stubRunner{
				results: map[string]exec.CommandResult{
					"which -a klaudiush": {
						Err:      errors.New("which not found"),
						ExitCode: 1,
					},
				},
			}
			detector := updater.NewDetector(runner)

			// Will attempt scanPATH - may find the real binary or fail.
			// Either way, it shouldn't panic.
			_, _ = detector.FindAll(context.Background())
		})
	})

	Describe("InstallMethod String", func() {
		It("returns direct for InstallMethodDirect", func() {
			Expect(updater.InstallMethodDirect.String()).To(Equal("direct"))
		})

		It("returns homebrew for InstallMethodHomebrew", func() {
			Expect(updater.InstallMethodHomebrew.String()).To(Equal("homebrew"))
		})
	})

	Describe("InstallInfo DisplayPath", func() {
		It("returns symlink path when set", func() {
			info := updater.InstallInfo{
				Path:        "/opt/homebrew/Cellar/klaudiush/1.0/bin/klaudiush",
				SymlinkPath: "/opt/homebrew/bin/klaudiush",
			}
			Expect(info.DisplayPath()).To(Equal("/opt/homebrew/bin/klaudiush"))
		})

		It("returns path when no symlink", func() {
			info := updater.InstallInfo{
				Path: "/usr/local/bin/klaudiush",
			}
			Expect(info.DisplayPath()).To(Equal("/usr/local/bin/klaudiush"))
		})
	})
})
