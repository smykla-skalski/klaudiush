package harness_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
)

// lingeringHarness starts background work the way Codex starts its plugin
// clone, then exits: a job in its own process group that keeps writing
// under the sandbox home, a job without HOME, and a plain sleeper. Their
// output goes to /dev/null so the harness's own pipes close when it exits.
const lingeringHarness = `set -m
clone="$HOME/.codex/.tmp/plugins-clone-test"
(while :; do mkdir -p "$clone"; : > "$clone/pack.$$"; sleep 0.02; done) >/dev/null 2>&1 &
env -u HOME sleep 300 >/dev/null 2>&1 &
sleep 300 >/dev/null 2>&1 &
echo started
`

const lingeringCount = 3

func requireProcessListing() {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		Skip("sandbox processes are found only on darwin and linux")
	}
}

func startLingering(ctx context.Context, sb *harness.Sandbox) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	out, err := harness.RunIn(ctx, sb, sb.Work, "sh", "-c", lingeringHarness)
	Expect(err).NotTo(HaveOccurred(), string(out))

	Eventually(func() int {
		pids, err := sb.Processes()
		Expect(err).NotTo(HaveOccurred())

		return len(pids)
	}).WithTimeout(5 * time.Second).Should(BeNumerically(">=", lingeringCount))
	Eventually(filepath.Join(sb.CodexHome(), ".tmp", "plugins-clone-test")).Should(BeADirectory())
}

func sandboxGone(sb *harness.Sandbox) func() bool {
	return func() bool {
		_, err := os.Lstat(sb.Root)

		return os.IsNotExist(err)
	}
}

func startOutside(name string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	Expect(cmd.Start()).To(Succeed())
	DeferCleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	return cmd
}

var _ = Describe("Sandbox processes", func() {
	BeforeEach(requireProcessListing)

	It("finds no processes in a fresh sandbox", func() {
		sb, err := harness.NewSandbox(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(sb.Close)

		Expect(sb.Processes()).To(BeEmpty())
		Expect(sb.StopProcesses()).To(Succeed())
	})

	It("stops lingering children and leaves no files behind on Close", func() {
		sb, err := harness.NewSandbox(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())

		startLingering(context.Background(), sb)

		Expect(sb.Close()).To(Succeed())
		Expect(sb.Processes()).To(BeEmpty())
		Consistently(sandboxGone(sb)).WithTimeout(500 * time.Millisecond).Should(BeTrue())
	})

	It("leaves processes that do not run with the sandbox environment alone", func() {
		sb, err := harness.NewSandbox(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(sb.Close)

		outside := startOutside("sleep", []string{
			"HOME=" + filepath.Dir(sb.Root),
			"TMPDIR=" + sb.Root + "-other",
			"UNRELATED=" + sb.Home,
		}, "300")

		Expect(sb.StopProcesses()).To(Succeed())
		Expect(outside.Process.Signal(syscall.Signal(0))).To(Succeed())
	})

	It("finds an untracked process by its sandbox environment", func() {
		if runtime.GOOS == "darwin" {
			Skip("macOS hides the environment of Apple binaries such as sleep")
		}

		sb, err := harness.NewSandbox(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(sb.Close)

		stray := startOutside("sleep", []string{"PATH=/usr/bin:/bin", "TMPDIR=" + sb.Root}, "300")

		Expect(sb.Processes()).To(ConsistOf(stray.Process.Pid))
		Expect(sb.StopProcesses()).To(Succeed())
		Expect(sb.Processes()).To(BeEmpty())
	})

	It("fails Close when files keep reappearing from a writer it cannot stop", func() {
		sb, err := harness.NewSandbox(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())

		stop := make(chan struct{})
		done := make(chan struct{})

		go func() {
			defer close(done)

			for {
				select {
				case <-stop:
					return
				case <-time.After(time.Millisecond):
					_ = os.MkdirAll(filepath.Join(sb.Root, "x"), 0o750)
				}
			}
		}()

		DeferCleanup(func() {
			close(stop)
			<-done

			_ = os.RemoveAll(sb.Root)
		})

		Expect(sb.Close()).To(MatchError(ContainSubstring("files reappeared")))
	})
})
