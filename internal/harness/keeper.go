package harness

import (
	"encoding/json"
	"io"
	"os"
	"sync"

	"github.com/cockroachdb/errors"
)

// keeperArg makes a binary that links this package run as a keeper instead
// of its own main. No test binary is started with it as its only argument.
const keeperArg = "klaudiush-harness-keeper"

// keeperMessage is one line the sandbox sends its keeper: the sandbox paths
// first, then each session and process it learns about.
type keeperMessage struct {
	Root    string   `json:"root,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
	Session int      `json:"session,omitempty"`
	PID     int      `json:"pid,omitempty"`
	Start   int64    `json:"start,omitempty"`
}

// keeper is a copy of the running binary in its own session that holds the
// read end of a pipe from the sandbox. When the pipe closes, because the
// sandbox is closed or because the process holding it died (even by
// SIGKILL), the keeper stops every sandbox process it knows of and exits.
type keeper struct {
	mu   sync.Mutex
	proc *os.Process
	in   *os.File
	enc  *json.Encoder
	pid  int
}

func init() {
	if len(os.Args) == 2 && os.Args[1] == keeperArg {
		os.Exit(runKeeper(os.Stdin))
	}
}

// startKeeper starts a keeper for the sandbox with the pipe as its stdin
// and its output discarded, so it holds none of the caller's pipes open.
func startKeeper(root string, aliases []string) (*keeper, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, errors.Wrap(err, "finding the keeper binary")
	}

	r, w, err := os.Pipe()
	if err != nil {
		return nil, errors.Wrap(err, "keeper pipe")
	}

	defer func() { _ = r.Close() }()

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		_ = w.Close()

		return nil, errors.Wrap(err, "opening null device")
	}

	defer func() { _ = devNull.Close() }()

	proc, err := os.StartProcess(self, []string{self, keeperArg}, &os.ProcAttr{
		Files: []*os.File{r, devNull, devNull},
		Sys:   keeperAttr(),
	})
	if err != nil {
		_ = w.Close()

		return nil, errors.Wrap(err, "starting keeper")
	}

	k := &keeper{proc: proc, in: w, enc: json.NewEncoder(w), pid: proc.Pid}

	if err := k.send(keeperMessage{Root: root, Aliases: aliases}); err != nil {
		return nil, errors.CombineErrors(err, k.close())
	}

	return k, nil
}

// send passes msg to the keeper. Callers past startup drop the error: a
// keeper that is gone can no longer act on anything.
func (k *keeper) send(msg keeperMessage) error {
	if k == nil {
		return nil
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	if err := k.enc.Encode(msg); err != nil {
		return errors.Wrap(err, "writing to keeper")
	}

	return nil
}

// close ends the pipe and waits for the keeper to finish its sweep.
func (k *keeper) close() error {
	if k == nil {
		return nil
	}

	k.mu.Lock()
	closeErr := k.in.Close()
	k.mu.Unlock()

	state, err := k.proc.Wait()
	if err == nil && !state.Success() {
		err = errors.Newf("keeper %s", state)
	}

	return errors.CombineErrors(
		errors.Wrap(closeErr, "closing keeper pipe"),
		errors.Wrap(err, "waiting for keeper"),
	)
}

// runKeeper reads sandbox messages until the pipe closes, then stops the
// sandbox processes. It returns 1 when it could not stop them.
func runKeeper(r io.Reader) int {
	dec := json.NewDecoder(r)

	var first keeperMessage
	if dec.Decode(&first) != nil || first.Root == "" {
		return 1
	}

	sb := &Sandbox{
		Root:     first.Root,
		aliases:  first.Aliases,
		sessions: map[int]struct{}{},
		known:    map[int]int64{},
	}

	for {
		var msg keeperMessage
		if dec.Decode(&msg) != nil {
			break
		}

		if msg.Session > 0 {
			sb.sessions[msg.Session] = struct{}{}
		}

		if msg.PID > 0 {
			sb.known[msg.PID] = msg.Start
		}
	}

	if sb.StopProcesses() != nil {
		return 1
	}

	return 0
}
