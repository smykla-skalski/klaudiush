package harness

import (
	"bytes"
	"encoding/binary"
	"os"

	"github.com/cockroachdb/errors"
	"golang.org/x/sys/unix"
)

const (
	argcSize   = 4
	zombieStat = 5
	noSession  = -1
)

// listProcesses returns the live processes the caller's user owns. macOS
// hides the environment of Apple binaries, so Env is nil for them.
func listProcesses() ([]process, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.uid", os.Getuid())
	if err != nil {
		return nil, errors.Wrap(err, "kern.proc.uid")
	}

	out := make([]process, 0, len(procs))

	for i := range procs {
		if procs[i].Proc.P_stat == zombieStat {
			continue
		}

		pid := int(procs[i].Proc.P_pid)

		sid, err := unix.Getsid(pid)
		if err != nil {
			sid = noSession
		}

		p := process{PID: pid, PPID: int(procs[i].Eproc.Ppid), SID: sid}

		if raw, err := unix.SysctlRaw("kern.procargs2", pid); err == nil {
			p.Env = parseProcArgs(raw)
		}

		out = append(out, p)
	}

	return out, nil
}

// parseProcArgs extracts the environment from a kern.procargs2 buffer:
// argc, the executable path, NUL padding, argc arguments, then the
// environment up to the first empty string.
func parseProcArgs(raw []byte) []string {
	if len(raw) < argcSize {
		return nil
	}

	argc := int(binary.LittleEndian.Uint32(raw[:argcSize]))

	_, rest, _ := bytes.Cut(raw[argcSize:], []byte{0})
	rest = bytes.TrimLeft(rest, "\x00")

	for range argc {
		_, rest, _ = bytes.Cut(rest, []byte{0})
	}

	var env []string

	for len(rest) > 0 {
		entry, next, _ := bytes.Cut(rest, []byte{0})
		if len(entry) == 0 {
			break
		}

		env = append(env, string(entry))
		rest = next
	}

	return env
}

func freezeProcess(pid int) { _ = unix.Kill(pid, unix.SIGSTOP) }

func killProcess(pid int) { _ = unix.Kill(pid, unix.SIGKILL) }
