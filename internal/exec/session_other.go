//go:build !unix

package exec

import "os/exec"

func startNewSession(*exec.Cmd) {}
