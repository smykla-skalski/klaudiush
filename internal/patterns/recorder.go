package patterns

// Recorder tracks error code sequences per session.
// After each dispatch, it records (previous, current) error pairs
// so the system can learn which errors commonly follow each other.
// Session state is persisted in the store's global data file.
type Recorder struct {
	store *FilePatternStore
}

// NewRecorder creates a recorder backed by the given store.
func NewRecorder(store *FilePatternStore) *Recorder {
	return &Recorder{store: store}
}

// Observe records the current blocking error codes for a session.
// If previous codes exist, it records all (prev, current) pairs.
// If codes is empty (validation passed), clears the session state.
func (r *Recorder) Observe(sessionID string, codes []string) {
	if len(codes) == 0 {
		r.store.ClearSessionCodes(sessionID)

		return
	}

	prev := r.store.GetSessionCodes(sessionID)
	if len(prev) > 0 {
		for _, prevCode := range prev {
			for _, curCode := range codes {
				if prevCode != curCode {
					r.store.RecordSequence(prevCode, curCode)
				}
			}
		}
	}

	r.store.SetSessionCodes(sessionID, codes)
}

// ClearSession removes stored codes for a session.
func (r *Recorder) ClearSession(sessionID string) {
	r.store.ClearSessionCodes(sessionID)
}
