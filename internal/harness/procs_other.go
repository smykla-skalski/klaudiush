//go:build !darwin && !linux

package harness

import "syscall"

// listProcesses cannot see other processes here, so no sandbox process is
// found and Close only removes files.
func listProcesses(bool) ([]process, error) { return nil, nil }

const keeperSupported = false

func keeperAttr() *syscall.SysProcAttr { return nil }

func freezeProcess(process) {}

func killProcess(process) {}

func processStart(int) (int64, bool) { return 0, false }

func signalProcess(int, int64, syscall.Signal) bool { return false }
