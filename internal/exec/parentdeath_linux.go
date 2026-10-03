package exec

import (
	"os/exec"
	"runtime"
	"syscall"
)

// killWithParent has the kernel SIGKILL the command when the thread that
// starts it exits. That thread stays locked to the calling goroutine until
// the returned function runs after the command ended, so in practice the
// signal comes when this process dies.
func killWithParent(cmd *exec.Cmd) func() {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL

	runtime.LockOSThread()

	return runtime.UnlockOSThread
}
