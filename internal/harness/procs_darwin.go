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
	usecPerSec = 1_000_000
)

// listProcesses returns the live processes the caller's user owns, with
// their environment when withEnv is set. macOS hides the environment of
// Apple binaries, so Env is nil for them.
func listProcesses(withEnv bool) ([]process, error) {
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

		p := process{
			PID: pid, PPID: int(procs[i].Eproc.Ppid), SID: sid,
			Start: startTime(&procs[i]),
		}

		if withEnv {
			if raw, err := unix.SysctlRaw("kern.procargs2", pid); err == nil {
				p.Env = parseProcArgs(raw)
			}
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

// startTime is the process start time in microseconds since the epoch.
func startTime(kp *unix.KinfoProc) int64 {
	tv := kp.Proc.P_starttime

	return tv.Sec*usecPerSec + int64(tv.Usec)
}

// processStart returns the start time of a live, non-zombie process.
func processStart(pid int) (int64, bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(kp.Proc.P_pid) != pid || kp.Proc.P_stat == zombieStat {
		return 0, false
	}

	return startTime(kp), true
}

// signalByHandle reports that macOS has no process handle to signal by.
func signalByHandle(int, int64, unix.Signal) (sent, handled bool) { return false, false }
