package hooksession

import (
	"maps"
	"slices"
	"time"

	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// BaselineUnknown is the baseline of a check first seen after the session
// already changed files: what it changed cannot be told, so the check is
// required.
const BaselineUnknown = "unknown"

// repoEvidence holds the latest receipt of each check in one repository.
// Receipts are tied to content, not to a session: a check that passed on
// exactly this content passed, whoever ran it.
// Passed keeps each check's latest pass, so a later run that never finished
// (denied, interrupted, still running) does not hide it. A failure on the
// same content drops it.
type repoEvidence struct {
	UpdatedAt time.Time                    `json:"updated_at"`
	Receipts  map[string]*evidence.Receipt `json:"receipts,omitempty"`
	Passed    map[string]*evidence.Receipt `json:"passed,omitempty"`
}

// EvidenceBaselines returns, by check ID, the content digests the session
// started from in a repository.
func (s *Store) EvidenceBaselines(
	provider hook.Provider,
	sessionID string,
	repo string,
) (map[string]string, error) {
	var baselines map[string]string

	err := s.updateEntry(provider, sessionID, false, func(entry *sessionEntry) bool {
		baselines = maps.Clone(entry.Baselines[repo])

		return false
	})

	return baselines, err
}

// EvidenceRepos returns the repositories the session recorded baselines for
// or used a tool that can change files in.
func (s *Store) EvidenceRepos(provider hook.Provider, sessionID string) ([]string, error) {
	var repos []string

	err := s.updateEntry(provider, sessionID, false, func(entry *sessionEntry) bool {
		repos = slices.Collect(maps.Keys(entry.Baselines))

		for repo := range entry.Touched {
			if _, ok := entry.Baselines[repo]; !ok {
				repos = append(repos, repo)
			}
		}

		slices.Sort(repos)

		return false
	})

	return repos, err
}

// AddEvidenceBaselines records baselines for checks that have none yet in
// the session; existing baselines are kept.
func (s *Store) AddEvidenceBaselines(
	provider hook.Provider,
	sessionID string,
	repo string,
	baselines map[string]string,
) error {
	if len(baselines) == 0 {
		return nil
	}

	return s.updateEntry(provider, sessionID, true, func(entry *sessionEntry) bool {
		if entry.Baselines == nil {
			entry.Baselines = make(map[string]map[string]string)
		}

		current := entry.Baselines[repo]
		if current == nil {
			current = make(map[string]string, len(baselines))
			entry.Baselines[repo] = current
		}

		changed := false

		for id, digest := range baselines {
			if _, ok := current[id]; !ok {
				current[id] = digest
				changed = true
			}
		}

		return changed
	})
}

// Passes returns the latest pass of each check in a repository.
func (s *Store) Passes(repo string) (map[string]*evidence.Receipt, error) {
	var passes map[string]*evidence.Receipt

	err := s.update(func(st *state) bool {
		if repoState := st.Evidence[repo]; repoState != nil {
			passes = maps.Clone(repoState.Passed)
		}

		return false
	})

	return passes, err
}

// MarkTouched records that the session used a tool that can change files in
// a repository.
func (s *Store) MarkTouched(provider hook.Provider, sessionID, repo string) error {
	return s.updateEntry(provider, sessionID, true, func(entry *sessionEntry) bool {
		if entry.Touched[repo] {
			return false
		}

		if entry.Touched == nil {
			entry.Touched = make(map[string]bool)
		}

		entry.Touched[repo] = true

		return true
	})
}

// Touched reports whether the session used a tool that can change files in
// a repository.
func (s *Store) Touched(provider hook.Provider, sessionID, repo string) (bool, error) {
	touched := false

	err := s.updateEntry(provider, sessionID, false, func(entry *sessionEntry) bool {
		touched = entry.Touched[repo]

		return false
	})

	return touched, err
}

// keepPass records a finished pass, and drops the kept pass when the same
// content has since failed.
func (r *repoEvidence) keepPass(receipt *evidence.Receipt) {
	if receipt.Status == evidence.StatusPassed {
		if r.Passed == nil {
			r.Passed = make(map[string]*evidence.Receipt)
		}

		kept := *receipt
		r.Passed[receipt.Check] = &kept

		return
	}

	kept := r.Passed[receipt.Check]
	if receipt.Status == evidence.StatusFailed && kept != nil && kept.Digest == receipt.Digest {
		delete(r.Passed, receipt.Check)
	}
}

// Receipts returns the latest receipt of each check in a repository.
func (s *Store) Receipts(repo string) (map[string]*evidence.Receipt, error) {
	var receipts map[string]*evidence.Receipt

	err := s.update(func(st *state) bool {
		if repoState := st.Evidence[repo]; repoState != nil {
			receipts = maps.Clone(repoState.Receipts)
		}

		return false
	})

	return receipts, err
}

// PutReceipt makes receipt the latest one of its check in a repository.
func (s *Store) PutReceipt(repo string, receipt *evidence.Receipt) error {
	return s.update(func(st *state) bool {
		repoState := st.repoEvidence(repo)
		repoState.Receipts[receipt.Check] = receipt
		repoState.UpdatedAt = s.now()

		return true
	})
}

// FinishReceipt applies finish to the latest receipt of a check when match
// accepts it, and reports whether it did. A run that a newer one replaced
// is left alone.
func (s *Store) FinishReceipt(
	repo string,
	check string,
	match func(*evidence.Receipt) bool,
	finish func(*evidence.Receipt),
) (bool, error) {
	found := false

	err := s.update(func(st *state) bool {
		repoState := st.Evidence[repo]
		if repoState == nil {
			return false
		}

		receipt := repoState.Receipts[check]
		if receipt == nil || !match(receipt) {
			return false
		}

		finish(receipt)
		repoState.keepPass(receipt)

		repoState.UpdatedAt = s.now()
		found = true

		return true
	})

	return found, err
}

// FindReceipt returns the latest receipt in a repository that match accepts.
func (s *Store) FindReceipt(
	repo string,
	match func(*evidence.Receipt) bool,
) (*evidence.Receipt, error) {
	var found *evidence.Receipt

	err := s.update(func(st *state) bool {
		repoState := st.Evidence[repo]
		if repoState == nil {
			return false
		}

		for _, receipt := range repoState.Receipts {
			if match(receipt) {
				clone := *receipt
				found = &clone

				return false
			}
		}

		return false
	})

	return found, err
}

func (st *state) repoEvidence(repo string) *repoEvidence {
	if st.Evidence == nil {
		st.Evidence = make(map[string]*repoEvidence)
	}

	repoState := st.Evidence[repo]
	if repoState == nil {
		repoState = &repoEvidence{}
		st.Evidence[repo] = repoState
	}

	if repoState.Receipts == nil {
		repoState.Receipts = make(map[string]*evidence.Receipt)
	}

	return repoState
}

// cleanupExpiredEvidence drops repositories whose receipts nobody touched
// within the retention.
func (s *Store) cleanupExpiredEvidence(st *state) bool {
	changed := false
	now := s.now()

	for repo, repoState := range st.Evidence {
		if repoState == nil || now.Sub(repoState.UpdatedAt) > s.retention {
			delete(st.Evidence, repo)

			changed = true
		}
	}

	return changed
}
