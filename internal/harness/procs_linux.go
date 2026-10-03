package harness

import (
	"bytes"
	"os"
	"strconv"
	"syscall"

	"github.com/cockroachdb/errors"
	"golang.org/x/sys/unix"
)

// Indexes of the /proc/<pid>/stat fields after the command name.
const (
	statState   = 0
	statPPID    = 1
	statSession = 3
	statStart   = 19
	statFields  = 20
)

// listProcesses returns the live processes the caller's user owns. Env is
// nil for those whose environment cannot be read (non-dumpable ones).
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

	info, err := proc.Stat(strconv.Itoa(pid))
	if err != nil {
		return process{}, false
	}

	if st, ok := info.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Getuid() {
		return process{}, false
	}

	ppid, _ := strconv.Atoi(fields[statPPID])
	sid, _ := strconv.Atoi(fields[statSession])
	start, _ := strconv.ParseInt(fields[statStart], 10, 64)

	p := process{PID: pid, PPID: ppid, SID: sid, Start: start}

	raw, err := proc.ReadFile(dir + "environ")
	if err != nil {
		return p, true
	}

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

// processStart returns the start time of a live, non-zombie process in
// clock ticks since boot.
func processStart(pid int) (int64, bool) {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}

	fields, ok := parseStat(stat)
	if !ok || fields[statState] == "Z" {
		return 0, false
	}

	start, err := strconv.ParseInt(fields[statStart], 10, 64)

	return start, err == nil
}

// signalByHandle signals through a pidfd, which keeps naming the process it
// was opened for, after checking that process is the one seen at start.
// handled is false when no pidfd can be opened for another reason than the
// process being gone (an old kernel, a seccomp filter).
func signalByHandle(pid int, start int64, sig unix.Signal) (sent, handled bool) {
	fd, err := unix.PidfdOpen(pid, 0)
	if errors.Is(err, unix.ESRCH) {
		return false, true
	}

	if err != nil {
		return false, false
	}

	defer func() { _ = unix.Close(fd) }()

	if got, ok := processStart(pid); !ok || got != start {
		return false, true
	}

	return unix.PidfdSendSignal(fd, sig, nil, 0) == nil, true
}
