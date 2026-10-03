//go:build darwin || linux

package harness_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"

	"github.com/smykla-skalski/klaudiush/internal/harness"
)

const (
	detachHelper = "harness-test-detach"
	orphanHelper = "harness-test-orphan"
)

// The detach helper writes its child's pid to detachPIDFile and exits once
// detachRelease exists, so the spec decides when the parent goes away.
const (
	detachPIDFile = "detached.pid"
	detachRelease = "release"
)

// owner is a sandbox process as the orphan helper reports it.
type owner struct {
	PID   int   `json:"pid"`
	Start int64 `json:"start"`
}

type orphanReport struct {
	Keeper owner   `json:"keeper"`
	Procs  []owner `json:"procs"`
}

func init() {
	testHelpers[detachHelper] = runDetach
	testHelpers[orphanHelper] = runOrphan
}

// runDetach starts a sleeper in a new session with none of the sandbox
// environment, reports its pid and exits when released, the way a daemon
// detaches.
func runDetach([]string) int {
	self, err := os.Executable()
	if err != nil {
		return 1
	}

	child := exec.Command(self)
	child.Env = []string{blockEnv + "=1"}
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if child.Start() != nil {
		return 1
	}

	if os.WriteFile(detachPIDFile, []byte(strconv.Itoa(child.Process.Pid)), 0o600) != nil {
		return 1
	}

	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); {
		if _, err := os.Stat(detachRelease); err == nil {
			return 0
		}

		time.Sleep(10 * time.Millisecond)
	}

	return 1
}

// runOrphan holds a sandbox with a running harness and lingering children,
// reports them and the keeper, and waits to be killed.
func runOrphan(args []string) int {
	sb, err := harness.NewSandbox(args[0])
	if err != nil {
		return 1
	}

	ctx := context.Background()

	go func() { _, _ = harness.RunIn(ctx, sb, sb.Work, "sleep", "300") }()

	if _, err := harness.RunIn(ctx, sb, sb.Work, "sh", "-c", lingeringHarness); err != nil {
		return 1
	}

	var pids []int

	for deadline := time.Now().Add(5 * time.Second); len(pids) <= lingeringCount; {
		if time.Now().After(deadline) {
			return 1
		}

		time.Sleep(50 * time.Millisecond)

		pids, _ = sb.Processes()
	}

	report := orphanReport{Keeper: identify(sb.KeeperPID())}
	for _, pid := range pids {
		report.Procs = append(report.Procs, identify(pid))
	}

	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return 1
	}

	time.Sleep(time.Hour)

	return 0
}

// startDetachedBlocker starts the test binary as a sleeper leading its own
// session, with none of the sandbox environment.
func startDetachedBlocker() *exec.Cmd {
	self, err := os.Executable()
	Expect(err).NotTo(HaveOccurred())

	cmd := exec.Command(self)
	cmd.Env = []string{blockEnv + "=1"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	Expect(cmd.Start()).To(Succeed())
	DeferCleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	return cmd
}

func identify(pid int) owner {
	start, _ := harness.StartOf(pid)

	return owner{PID: pid, Start: start}
}

func running(o owner) bool {
	if o.PID <= 0 {
		return false
	}

	start, ok := harness.StartOf(o.PID)

	return ok && start == o.Start
}

var _ = Describe("Sandbox processes outside the harness tree", func() {
	It("finds a child that left its session after its parent exited", func() {
		sb, err := harness.NewSandbox(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(sb.Close)

		self, err := os.Executable()
		Expect(err).NotTo(HaveOccurred())

		done := make(chan error, 1)

		go func() {
			out, runErr := harness.RunIn(context.Background(), sb, sb.Work, self, detachHelper)
			if runErr != nil {
				runErr = errors.Wrapf(runErr, "%s", out)
			}

			done <- runErr
		}()

		var pid int

		Eventually(func() error {
			raw, readErr := os.ReadFile(filepath.Join(sb.Work, detachPIDFile))
			if readErr == nil {
				pid, readErr = strconv.Atoi(string(raw))
			}

			return readErr
		}).WithTimeout(30 * time.Second).Should(Succeed())

		child := identify(pid)
		Expect(child.Start).NotTo(BeZero())
		DeferCleanup(func() { harness.KillIfSame(child.PID, child.Start) })

		Eventually(sb.Processes).Should(ContainElement(pid), "seen while its parent runs")
		Expect(os.WriteFile(filepath.Join(sb.Work, detachRelease), nil, 0o600)).To(Succeed())
		Eventually(done).WithTimeout(30 * time.Second).Should(Receive(BeNil()))

		sid, err := unix.Getsid(pid)
		Expect(err).NotTo(HaveOccurred())
		Expect(sid).To(Equal(pid), "the child runs in a session the sandbox did not start")

		Expect(sb.Processes()).To(ContainElement(pid))
		Expect(sb.StopProcesses()).To(Succeed())
		Expect(running(child)).To(BeFalse())
	})

	It("stops the sandbox processes when the process holding the sandbox is killed", func() {
		self, err := os.Executable()
		Expect(err).NotTo(HaveOccurred())

		holder := exec.Command(self, orphanHelper, GinkgoT().TempDir())
		stdout, err := holder.StdoutPipe()
		Expect(err).NotTo(HaveOccurred())
		Expect(holder.Start()).To(Succeed())

		DeferCleanup(func() {
			_ = holder.Process.Kill()
			_ = holder.Wait()
		})

		lines := make(chan string, 1)

		go func() {
			line, _ := bufio.NewReader(stdout).ReadString('\n')
			lines <- line
		}()

		var line string

		Eventually(lines).WithTimeout(30 * time.Second).Should(Receive(&line))

		var report orphanReport
		Expect(json.Unmarshal([]byte(line), &report)).To(Succeed(), line)

		everything := append([]owner{report.Keeper}, report.Procs...)

		DeferCleanup(func() {
			for _, o := range everything {
				harness.KillIfSame(o.PID, o.Start)
			}
		})

		Expect(report.Keeper.PID).NotTo(BeZero())
		Expect(len(report.Procs)).
			To(BeNumerically(">", lingeringCount), "lingering jobs and the harness")

		Expect(holder.Process.Kill()).To(Succeed())
		Expect(holder.Wait()).To(HaveOccurred())

		for _, o := range everything {
			Eventually(func() bool { return running(o) }).
				WithTimeout(15*time.Second).
				Should(BeFalse(), "pid %d still running", o.PID)
		}
	})
})

var _ = Describe("Keeper", func() {
	It("stops the processes it was told about once its input ends", func() {
		blocker := startBlocker()

		var target owner

		Eventually(func() int64 {
			target = identify(blocker.Process.Pid)

			return target.Start
		}).ShouldNot(BeZero())

		input := fmt.Sprintf("{\"root\":%q}\n{\"pid\":%d,\"start\":%d}\n",
			GinkgoT().TempDir(), target.PID, target.Start)

		Expect(harness.KeeperMain(strings.NewReader(input))).To(Equal(0))

		state, err := blocker.Process.Wait()
		Expect(err).NotTo(HaveOccurred())
		Expect(state.Sys().(syscall.WaitStatus).Signal()).To(Equal(syscall.SIGKILL))
	})

	It("leaves a process with another start time alone", func() {
		blocker := startBlocker()
		pid := blocker.Process.Pid

		Eventually(func() int64 { return identify(pid).Start }).ShouldNot(BeZero())

		input := fmt.Sprintf("{\"root\":%q}\n{\"session\":%d}\n{\"pid\":%d,\"start\":1}\n",
			GinkgoT().TempDir(), 1<<30, pid)

		Expect(harness.KeeperMain(strings.NewReader(input))).To(Equal(0))
		Expect(blocker.Process.Signal(syscall.Signal(0))).To(Succeed())
	})

	It("leaves a session alone once the sandbox forgot it", func() {
		blocker := startDetachedBlocker()
		pid := blocker.Process.Pid

		Eventually(func() int64 { return identify(pid).Start }).ShouldNot(BeZero())

		input := fmt.Sprintf("{\"root\":%q}\n{\"session\":%d}\n{\"forget\":%d}\n",
			GinkgoT().TempDir(), pid, pid)

		Expect(harness.KeeperMain(strings.NewReader(input))).To(Equal(0))
		Expect(blocker.Process.Signal(syscall.Signal(0))).To(Succeed())
	})

	It("stops a session it was told about", func() {
		blocker := startDetachedBlocker()

		Eventually(func() int64 { return identify(blocker.Process.Pid).Start }).ShouldNot(BeZero())

		input := fmt.Sprintf("{\"root\":%q}\n{\"session\":%d}\n",
			GinkgoT().TempDir(), blocker.Process.Pid)

		Expect(harness.KeeperMain(strings.NewReader(input))).To(Equal(0))

		state, err := blocker.Process.Wait()
		Expect(err).NotTo(HaveOccurred())
		Expect(state.Sys().(syscall.WaitStatus).Signal()).To(Equal(syscall.SIGKILL))
	})

	It("never signals a process group or every process", func() {
		Expect(harness.KillIfSame(0, 0)).To(BeFalse())
		Expect(harness.KillIfSame(-1, 0)).To(BeFalse())
	})

	It("fails without the sandbox root", func() {
		Expect(harness.KeeperMain(strings.NewReader("{}\n"))).To(Equal(1))
		Expect(harness.KeeperMain(strings.NewReader("not json"))).To(Equal(1))
	})

	It("ends with the sandbox", func() {
		sb, err := harness.NewSandbox(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())

		out, err := harness.RunIn(context.Background(), sb, sb.Work, "true")
		Expect(err).NotTo(HaveOccurred(), string(out))

		keeper := identify(sb.KeeperPID())
		Expect(keeper.Start).NotTo(BeZero())

		Expect(sb.Close()).To(Succeed())
		Expect(sb.KeeperPID()).To(BeZero())
		Expect(running(keeper)).To(BeFalse())

		_, _ = harness.RunIn(context.Background(), sb, sb.Work, "true")
		Expect(sb.KeeperPID()).To(BeZero(), "a closed sandbox starts no keeper")
	})
})
