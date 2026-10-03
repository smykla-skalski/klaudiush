//go:build unix

package exec

import (
	"os/exec"
	"syscall"
)

// startNewSession makes the command a session leader and has cancellation
// kill its whole process group, not only the leader.
func startNewSession(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.Setsid = true
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
