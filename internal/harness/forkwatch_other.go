//go:build !darwin

package harness

type forkWatcher struct{}

func newForkWatcher(*Sandbox) (*forkWatcher, error) { return &forkWatcher{}, nil }

func (*forkWatcher) start(int) {}

func (*forkWatcher) close() {}
