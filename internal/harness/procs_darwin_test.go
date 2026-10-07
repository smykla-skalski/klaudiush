package harness_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"

	"github.com/smykla-skalski/klaudiush/internal/harness"
)

const (
	fastDetachHelper       = "harness-test-fast-detach"
	fastDetachPID          = "fast-detached.pid"
	recursiveRootHelper    = "harness-test-recursive-root"
	recursiveChildHelper   = "harness-test-recursive-child"
	recursiveChildPID      = "recursive-child.pid"
	recursiveProbePID      = "recursive-probe.pid"
	recursiveProbeRelease  = "recursive-probe-release"
	recursiveDetachPID     = "recursive-detached.pid"
	recursiveDetachRelease = "recursive-release"
)

func init() {
	testHelpers[fastDetachHelper] = runFastDetach
	testHelpers[recursiveRootHelper] = runRecursiveRoot
	testHelpers[recursiveChildHelper] = runRecursiveChild
}

func runFastDetach([]string) int {
	child := exec.Command("/bin/sleep", "300")
	child.Env = []string{}
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if child.Start() != nil {
		return 1
	}

	pid := strconv.Itoa(child.Process.Pid)
	if os.WriteFile(fastDetachPID, []byte(pid), 0o600) != nil {
		_ = child.Process.Kill()

		return 1
	}

	if child.Process.Release() != nil {
		_ = child.Process.Kill()

		return 1
	}

	// Keep lineage visible while the event-triggered listing runs.
	time.Sleep(100 * time.Millisecond)

	return 0
}

func runRecursiveRoot([]string) int {
	self, err := os.Executable()
	if err != nil {
		return 1
	}

	child := exec.Command(self, recursiveChildHelper)
	if child.Start() != nil {
		return 1
	}

	if os.WriteFile(recursiveChildPID, []byte(strconv.Itoa(child.Process.Pid)), 0o600) != nil {
		_ = child.Process.Kill()
		_, _ = child.Process.Wait()

		return 1
	}

	if child.Wait() != nil {
		return 1
	}

	return 0
}

func runRecursiveChild([]string) int {
	released := false

	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); {
		if _, err := os.Stat(recursiveProbeRelease); err == nil {
			released = true

			break
		}

		time.Sleep(time.Millisecond)
	}

	if !released {
		return 1
	}

	for {
		if _, err := os.Stat(recursiveDetachRelease); err == nil {
			return runRecursiveDetach()
		}

		probe := exec.Command("/bin/sleep", "1")
		if probe.Start() != nil {
			return 1
		}

		pid := []byte(strconv.Itoa(probe.Process.Pid))
		if os.WriteFile(recursiveProbePID, pid, 0o600) != nil {
			_ = probe.Process.Kill()
			_, _ = probe.Process.Wait()

			return 1
		}

		if probe.Wait() != nil {
			return 1
		}
	}
}

func runRecursiveDetach() int {
	child := exec.Command("/bin/sleep", "300")
	child.Env = []string{}
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if child.Start() != nil {
		return 1
	}

	pid := strconv.Itoa(child.Process.Pid)
	if os.WriteFile(recursiveDetachPID, []byte(pid), 0o600) != nil {
		_ = child.Process.Kill()

		return 1
	}

	if child.Process.Release() != nil {
		_ = child.Process.Kill()

		return 1
	}

	time.Sleep(100 * time.Millisecond)

	return 0
}

var _ = Describe("parseProcArgs", func() {
	DescribeTable("reads the environment after the arguments",
		func(raw string, want []string) {
			Expect(harness.ParseProcArgs([]byte(raw))).To(Equal(want))
		},
		Entry("arguments then environment",
			"\x02\x00\x00\x00/bin/sleep\x00\x00\x00sleep\x00300\x00HOME=/h\x00A=b\x00\x00junk",
			[]string{"HOME=/h", "A=b"}),
		Entry("no environment", "\x01\x00\x00\x00/bin/x\x00x\x00", nil),
		Entry("truncated header", "\x01\x00", nil),
	)
})

var _ = Describe("process fork watcher", func() {
	It("tracks an Apple child that detaches before the polling interval", func() {
		sb, err := harness.NewSandbox(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(sb.Close)
		sb.SetProcessWatchInterval(time.Hour)

		self, err := os.Executable()
		Expect(err).NotTo(HaveOccurred())

		out, err := harness.RunIn(context.Background(), sb, sb.Work, self, fastDetachHelper)
		Expect(err).NotTo(HaveOccurred(), string(out))

		raw, err := os.ReadFile(filepath.Join(sb.Work, fastDetachPID))
		Expect(err).NotTo(HaveOccurred())
		pid, err := strconv.Atoi(string(raw))
		Expect(err).NotTo(HaveOccurred())

		child := identify(pid)
		Expect(child.Start).NotTo(BeZero())
		DeferCleanup(func() { harness.KillIfSame(child.PID, child.Start) })

		sid, err := unix.Getsid(pid)
		Expect(err).NotTo(HaveOccurred())
		Expect(sid).To(Equal(pid))
		Expect(sb.Processes()).To(ContainElement(pid))

		Expect(sb.StopProcesses()).To(Succeed())
		Expect(running(child)).To(BeFalse())
	})

	It("recursively watches descendants for later forks", func() {
		sb, err := harness.NewSandbox(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(sb.Close)
		sb.SetProcessWatchInterval(time.Hour)

		self, err := os.Executable()
		Expect(err).NotTo(HaveOccurred())

		done := make(chan error, 1)

		go func() {
			out, runErr := harness.RunIn(
				context.Background(), sb, sb.Work, self, recursiveRootHelper,
			)
			if runErr != nil {
				runErr = errors.Wrapf(runErr, "%s", out)
			}

			done <- runErr
		}()

		var descendantPID int

		Eventually(func() error {
			raw, readErr := os.ReadFile(filepath.Join(sb.Work, recursiveChildPID))
			if readErr == nil {
				descendantPID, readErr = strconv.Atoi(string(raw))
			}

			return readErr
		}).WithTimeout(10 * time.Second).Should(Succeed())
		Eventually(func() bool { return sb.KnowsProcess(descendantPID) }).
			WithTimeout(10 * time.Second).Should(BeTrue())
		Expect(os.WriteFile(
			filepath.Join(sb.Work, recursiveProbeRelease), nil, 0o600,
		)).To(Succeed())

		Eventually(func() bool {
			raw, readErr := os.ReadFile(filepath.Join(sb.Work, recursiveProbePID))
			if readErr != nil {
				return false
			}

			probePID, atoiErr := strconv.Atoi(string(raw))

			return atoiErr == nil && sb.KnowsProcess(probePID)
		}).WithTimeout(10 * time.Second).Should(BeTrue())

		Expect(os.WriteFile(
			filepath.Join(sb.Work, recursiveDetachRelease), nil, 0o600,
		)).To(Succeed())
		Eventually(done).WithTimeout(10 * time.Second).Should(Receive(BeNil()))

		raw, err := os.ReadFile(filepath.Join(sb.Work, recursiveDetachPID))
		Expect(err).NotTo(HaveOccurred())
		pid, err := strconv.Atoi(string(raw))
		Expect(err).NotTo(HaveOccurred())

		child := identify(pid)
		Expect(child.Start).NotTo(BeZero())
		DeferCleanup(func() { harness.KillIfSame(child.PID, child.Start) })
		Expect(sb.Processes()).To(ContainElement(pid))

		Expect(sb.StopProcesses()).To(Succeed())
		Expect(running(child)).To(BeFalse())
	})
})
