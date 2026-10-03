//go:build !linux

package exec

import "os/exec"

func killWithParent(*exec.Cmd) func() { return func() {} }
