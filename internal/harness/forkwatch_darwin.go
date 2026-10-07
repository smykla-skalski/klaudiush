package harness

import (
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"golang.org/x/sys/unix"
)

const forkWatchPoll = 10 * time.Millisecond

type forkWatcher struct {
	sandbox *Sandbox
	kq      int
	stop    chan struct{}
	done    chan struct{}
}

func newForkWatcher(s *Sandbox) (*forkWatcher, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, errors.Wrap(err, "creating process fork watcher")
	}

	w := &forkWatcher{
		sandbox: s,
		kq:      kq,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}

	go w.run()

	return w, nil
}

func (w *forkWatcher) start(pid int) {
	w.add(pid)
	w.scan()
}

func (w *forkWatcher) add(pid int) {
	ident, err := strconv.ParseUint(strconv.Itoa(pid), 10, 64)
	if err != nil {
		return
	}

	change := unix.Kevent_t{
		Ident:  ident,
		Filter: unix.EVFILT_PROC,
		Flags:  unix.EV_ADD | unix.EV_CLEAR,
		Fflags: unix.NOTE_FORK,
	}

	_, _ = unix.Kevent(w.kq, []unix.Kevent_t{change}, nil, nil)
}

func (w *forkWatcher) run() {
	defer close(w.done)

	events := make([]unix.Kevent_t, 1)
	timeout := unix.NsecToTimespec(forkWatchPoll.Nanoseconds())

	for {
		n, err := unix.Kevent(w.kq, nil, events, &timeout)
		if err != nil && !errors.Is(err, unix.EINTR) {
			return
		}

		if n > 0 && events[0].Fflags&unix.NOTE_FORK != 0 {
			w.scan()
		}

		select {
		case <-w.stop:
			return
		default:
		}
	}
}

func (w *forkWatcher) scan() {
	procs, err := w.sandbox.owned(false)
	if err != nil {
		return
	}

	for _, p := range procs {
		w.add(p.PID)
	}
}

func (w *forkWatcher) close() {
	close(w.stop)
	<-w.done
	_ = unix.Close(w.kq)
}
