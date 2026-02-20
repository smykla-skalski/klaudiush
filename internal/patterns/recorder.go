package patterns

import "sync"

// Recorder tracks error code sequences per session.
// After each dispatch, it records (previous, current) error pairs
// so the system can learn which errors commonly follow each other.
type Recorder struct {
	store    PatternStore
	mu       sync.Mutex
	previous map[string][]string // sessionID -> previous blocking codes
}

// NewRecorder creates a recorder backed by the given store.
func NewRecorder(store PatternStore) *Recorder {
	return &Recorder{
		store:    store,
		previous: make(map[string][]string),
	}
}

// Observe records the current blocking error codes for a session.
// If previous codes exist, it records all (prev, current) pairs.
// If codes is empty (validation passed), clears the session state.
func (r *Recorder) Observe(sessionID string, codes []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(codes) == 0 {
		delete(r.previous, sessionID)

		return
	}

	prev := r.previous[sessionID]
	if len(prev) > 0 {
		for _, prevCode := range prev {
			for _, curCode := range codes {
				if prevCode != curCode {
					r.store.RecordSequence(prevCode, curCode)
				}
			}
		}
	}

	r.previous[sessionID] = codes
}

// ClearSession removes stored codes for a session.
func (r *Recorder) ClearSession(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.previous, sessionID)
}
