//go:build darwin || linux

package harness

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// keeperSupported is true where sandbox processes can be listed.
const keeperSupported = true

// keeperAttr puts the keeper in its own session, out of reach of the
// terminal's signals and of any sweep of the sessions it watches.
func keeperAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

func freezeProcess(p process) { signalProcess(p.PID, p.Start, unix.SIGSTOP) }

func killProcess(p process) { signalProcess(p.PID, p.Start, unix.SIGKILL) }

// signalProcess signals pid only while it is still the process that started
// at start. Where the platform offers a handle (a Linux pidfd) the check and
// the signal reach the same process; otherwise the pid is checked before
// and after, and a SIGSTOP that reached a newcomer is undone.
func signalProcess(pid int, start int64, sig unix.Signal) bool {
	if pid <= 0 {
		return false
	}

	if sent, handled := signalByHandle(pid, start, sig); handled {
		return sent
	}

	if got, ok := processStart(pid); !ok || got != start {
		return false
	}

	if unix.Kill(pid, sig) != nil {
		return false
	}

	if got, ok := processStart(pid); sig == unix.SIGSTOP && ok && got != start {
		_ = unix.Kill(pid, unix.SIGCONT)

		return false
	}

	return true
}
