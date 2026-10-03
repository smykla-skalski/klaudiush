//go:build unix

package exec

import (
	"os/exec"
	"syscall"
)

func startNewSession(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
