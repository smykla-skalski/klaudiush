package harness

import (
	"bytes"
	"os"
	"strconv"

	"github.com/cockroachdb/errors"
	"golang.org/x/sys/unix"
)

const (
	statState   = 0
	statPPID    = 1
	statSession = 3
	statFields  = 4
)

// listProcesses returns the live processes whose environment the caller
// can read, which on Linux means the processes it may signal.
func listProcesses() ([]process, error) {
	proc, err := os.OpenRoot("/proc")
	if err != nil {
		return nil, errors.Wrap(err, "opening /proc")
	}

	defer func() { _ = proc.Close() }()

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, errors.Wrap(err, "reading /proc")
	}

	var out []process

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		if p, ok := readProcess(proc, pid); ok {
			out = append(out, p)
		}
	}

	return out, nil
}

func readProcess(proc *os.Root, pid int) (process, bool) {
	dir := strconv.Itoa(pid) + "/"

	stat, err := proc.ReadFile(dir + "stat")
	if err != nil {
		return process{}, false
	}

	fields, ok := parseStat(stat)
	if !ok || fields[statState] == "Z" {
		return process{}, false
	}

	raw, err := proc.ReadFile(dir + "environ")
	if err != nil {
		return process{}, false
	}

	ppid, _ := strconv.Atoi(fields[statPPID])
	sid, _ := strconv.Atoi(fields[statSession])

	p := process{PID: pid, PPID: ppid, SID: sid}

	for field := range bytes.SplitSeq(raw, []byte{0}) {
		if len(field) > 0 {
			p.Env = append(p.Env, string(field))
		}
	}

	return p, true
}

// parseStat returns the /proc/<pid>/stat fields after the command name,
// which is in parentheses and may itself contain spaces or parentheses.
func parseStat(stat []byte) ([]string, bool) {
	_, rest, ok := bytes.CutLast(stat, []byte(")"))
	if !ok {
		return nil, false
	}

	fields := bytes.Fields(rest)
	if len(fields) < statFields {
		return nil, false
	}

	out := make([]string, statFields)
	for i := range out {
		out[i] = string(fields[i])
	}

	return out, true
}

func freezeProcess(pid int) { _ = unix.Kill(pid, unix.SIGSTOP) }

func killProcess(pid int) { _ = unix.Kill(pid, unix.SIGKILL) }
