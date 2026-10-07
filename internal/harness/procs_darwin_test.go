package harness_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"

	"github.com/smykla-skalski/klaudiush/internal/harness"
)

const (
	fastDetachHelper = "harness-test-fast-detach"
	fastDetachPID    = "fast-detached.pid"
)

func init() {
	testHelpers[fastDetachHelper] = runFastDetach
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

	// Keep lineage visible to NOTE_FORK, but not the 50 ms poller.
	time.Sleep(10 * time.Millisecond)

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
})
