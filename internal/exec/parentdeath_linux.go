package exec

import (
	"os/exec"
	"syscall"
)

// killWithParent has the kernel SIGKILL the command when the thread that
// started it exits. RunWithOptions keeps that thread locked to its
// goroutine until the command ends, so in practice the signal comes when
// this process dies.
func killWithParent(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
