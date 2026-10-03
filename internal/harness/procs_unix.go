//go:build darwin || linux

package harness

import "golang.org/x/sys/unix"

func freezeProcess(p process) { signalProcess(p.PID, p.Start, unix.SIGSTOP) }

func killProcess(p process) { signalProcess(p.PID, p.Start, unix.SIGKILL) }

// signalProcess signals pid only while it is still the process that started
// at start. Where the platform offers a handle (a Linux pidfd) the check and
// the signal reach the same process; otherwise the pid is checked before
// and after, and a SIGSTOP that reached a newcomer is undone.
func signalProcess(pid int, start int64, sig unix.Signal) bool {
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
