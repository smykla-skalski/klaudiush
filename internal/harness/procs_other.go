//go:build !darwin && !linux

package harness

// listProcesses cannot see other processes here, so no sandbox process is
// found and Close only removes files.
func listProcesses() ([]process, error) { return nil, nil }

func freezeProcess(int) {}

func killProcess(int) {}
