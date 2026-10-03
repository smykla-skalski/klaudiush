// Package hooksession persists provider hook findings across hook invocations.
package hooksession

import (
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/filelock"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/xdg"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	defaultRetention = 7 * 24 * time.Hour

	// maxResolvedHistory bounds the resolved findings kept per session.
	maxResolvedHistory = 50

	// maxUnresolved bounds the unresolved findings kept per session; the
	// oldest are dropped first.
	maxUnresolved = 100

	// defaultLockTimeout bounds how long a hook waits for other hooks of
	// the same user to finish their state transactions.
	defaultLockTimeout = 5 * time.Second

	// maxTouchInterval bounds how stale UpdatedAt of a session that is
	// still in use may get before a hook without changes refreshes it.
	maxTouchInterval = time.Hour

	// touchDivisor keeps the refresh interval well inside a short retention.
	touchDivisor = 4
)

// errCorruptState marks a state file that exists but cannot be parsed.
var errCorruptState = errors.New("corrupt hook session state")

type state struct {
	Sessions map[string]*sessionEntry `json:"sessions"`

	// Evidence holds check receipts by repository root.
	Evidence map[string]*repoEvidence `json:"evidence,omitempty"`
}

// sessionEntry holds one provider session. Findings lists only what is still
// unresolved; Resolved is a bounded audit trail that never blocks anything.
type sessionEntry struct {
	Provider  string     `json:"provider"`
	SessionID string     `json:"session_id"`
	StartedAt time.Time  `json:"started_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Findings  []*finding `json:"findings,omitempty"`
	Resolved  []*finding `json:"resolved,omitempty"`

	// CompletionBlocks counts consecutive completion-gate blocks per gate.
	CompletionBlocks map[string]int `json:"completion_blocks,omitempty"`

	// Baselines holds, by repository root and check ID, the content digest
	// each required check covered when the session first touched the
	// repository.
	Baselines map[string]map[string]string `json:"evidence_baselines,omitempty"`

	// Touched lists the repositories in which the session used a tool that
	// can change files. Changes in a repository the session only read are
	// someone else's and require nothing of it.
	Touched map[string]bool `json:"evidence_touched,omitempty"`
}

type finding struct {
	Validator     string              `json:"validator"`
	Resource      string              `json:"resource,omitempty"`
	AgentID       string              `json:"agent_id,omitempty"`
	Message       string              `json:"message"`
	Details       map[string]string   `json:"details,omitempty"`
	ShouldBlock   bool                `json:"should_block"`
	Reference     string              `json:"reference,omitempty"`
	FixHint       string              `json:"fix_hint,omitempty"`
	Bypassed      bool                `json:"bypassed,omitempty"`
	BypassReason  string              `json:"bypass_reason,omitempty"`
	Findings      []validator.Finding `json:"findings,omitempty"`
	Unavailable   bool                `json:"unavailable,omitempty"`
	Event         string              `json:"event,omitempty"`
	RawEventName  string              `json:"raw_event_name,omitempty"`
	ToolName      string              `json:"tool_name,omitempty"`
	ToolFamily    string              `json:"tool_family,omitempty"`
	Command       string              `json:"command,omitempty"`
	FilePath      string              `json:"file_path,omitempty"`
	AffectedPaths []string            `json:"affected_paths,omitempty"`
	Count         int                 `json:"count"`
	FirstSeen     time.Time           `json:"first_seen"`
	LastSeen      time.Time           `json:"last_seen"`
	CheckedAt     time.Time           `json:"checked_at,omitzero"`
	Recheckable   bool                `json:"recheckable,omitempty"`
	ResolvedAt    time.Time           `json:"resolved_at,omitzero"`
}

// checkKey is one validator checking one resource.
type checkKey struct {
	validator string
	resource  string
}

// Store persists per-session hook findings across hook invocations. Every
// change is one load, modify, save transaction (see update) that holds an
// exclusive lock, so concurrent hooks of one user cannot lose each other's
// updates.
type Store struct {
	stateFile   string
	now         func() time.Time
	retention   time.Duration
	lockTimeout time.Duration
}

// Option configures a Store.
type Option func(*Store)

// WithStateFile overrides the persisted state path.
func WithStateFile(path string) Option {
	return func(s *Store) {
		s.stateFile = path
	}
}

// WithTimeFunc overrides the clock used by the store.
func WithTimeFunc(fn func() time.Time) Option {
	return func(s *Store) {
		if fn != nil {
			s.now = fn
		}
	}
}

// WithRetention overrides stale-session retention.
func WithRetention(retention time.Duration) Option {
	return func(s *Store) {
		if retention > 0 {
			s.retention = retention
		}
	}
}

// WithLockTimeout overrides how long a transaction waits for the state lock.
func WithLockTimeout(timeout time.Duration) Option {
	return func(s *Store) {
		if timeout > 0 {
			s.lockTimeout = timeout
		}
	}
}

// NewStore creates a persisted session findings store.
func NewStore(opts ...Option) *Store {
	store := &Store{
		stateFile:   xdg.HookSessionStateFile(),
		now:         time.Now,
		retention:   defaultRetention,
		lockTimeout: defaultLockTimeout,
	}

	for _, opt := range opts {
		opt(store)
	}

	return store
}

// Start registers a provider/session entry. An existing entry keeps its
// unresolved findings and completion-gate counters: resumed, compacted and
// forked sessions, and subagents in some providers, also report a start.
func (s *Store) Start(provider hook.Provider, sessionID string) error {
	return s.updateEntry(provider, sessionID, true, func(*sessionEntry) bool { return true })
}

// Append records findings without any checks, so nothing is resolved.
func (s *Store) Append(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
) error {
	return s.Record(hookCtx, errs, nil)
}

// Record updates the unresolved findings of the hook context session with
// one dispatch. A stored finding whose validator checked its resource again
// without reporting it is resolved and moved to the history; current errors
// are added or refreshed. Findings about other resources, or from validators
// that did not run, stay as they are.
func (s *Store) Record(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	checks []dispatcher.Check,
) error {
	if hookCtx == nil || hookCtx.Provider == hook.ProviderUnknown || hookCtx.SessionID == "" {
		return nil
	}

	return s.update(func(st *state) bool {
		key := sessionKey(hookCtx.Provider, hookCtx.SessionID)
		now := s.now()

		entry := st.Sessions[key]
		if entry == nil {
			if len(errs) == 0 {
				return false
			}

			entry = &sessionEntry{
				Provider:  hookCtx.ProviderName(),
				SessionID: hookCtx.SessionID,
				StartedAt: now,
			}
			st.Sessions[key] = entry
		}

		if !entry.record(hookCtx, errs, checks, now) {
			return entry.touch(now, s.touchInterval())
		}

		entry.UpdatedAt = now

		return true
	})
}

// CombinedErrors returns every unresolved finding of a provider/session pair,
// including those recorded inside subagents.
func (s *Store) CombinedErrors(
	provider hook.Provider,
	sessionID string,
) ([]*dispatcher.ValidationError, error) {
	return s.unresolved(provider, sessionID, func(*finding) bool { return true })
}

// AgentErrors returns the unresolved findings recorded inside one subagent.
func (s *Store) AgentErrors(
	provider hook.Provider,
	sessionID string,
	agentID string,
) ([]*dispatcher.ValidationError, error) {
	return s.unresolved(provider, sessionID, func(item *finding) bool {
		return item.AgentID == agentID
	})
}

// FilesToRecheck returns the canonical paths of files with unresolved
// findings that changed on disk since they were last checked. Only findings
// a whole-file check reported count: nothing else could resolve them.
func (s *Store) FilesToRecheck(provider hook.Provider, sessionID string) ([]string, error) {
	if provider == hook.ProviderUnknown || sessionID == "" {
		return nil, nil
	}

	lastSeen := make(map[string]time.Time)

	err := s.update(func(st *state) bool {
		entry := st.Sessions[sessionKey(provider, sessionID)]
		if entry != nil {
			collectRecheckTimes(entry, lastSeen)
		}

		return false
	})
	if err != nil {
		return nil, err
	}

	var files []string

	for path, seen := range lastSeen {
		if info, err := os.Stat(path); err == nil && info.ModTime().After(seen) {
			files = append(files, path)
		}
	}

	sort.Strings(files)

	return files, nil
}

// collectRecheckTimes records, per file with recheckable findings, when the
// file was last reported or checked.
func collectRecheckTimes(entry *sessionEntry, lastSeen map[string]time.Time) {
	for _, item := range entry.Findings {
		path, ok := item.filePath()
		if !ok || !item.Recheckable {
			continue
		}

		for _, at := range []time.Time{item.LastSeen, item.CheckedAt} {
			if at.After(lastSeen[path]) {
				lastSeen[path] = at
			}
		}
	}
}

func (s *Store) unresolved(
	provider hook.Provider,
	sessionID string,
	include func(*finding) bool,
) ([]*dispatcher.ValidationError, error) {
	if provider == hook.ProviderUnknown || sessionID == "" {
		return nil, nil
	}

	var combined []*dispatcher.ValidationError

	err := s.update(func(st *state) bool {
		entry := st.Sessions[sessionKey(provider, sessionID)]
		if entry == nil {
			return false
		}

		now := s.now()

		changed := entry.retire(now, (*finding).fileGone)
		if changed {
			entry.UpdatedAt = now
		} else {
			changed = entry.touch(now, s.touchInterval())
		}

		shown := make(map[string]bool, len(entry.Findings))

		for _, item := range entry.Findings {
			if include(item) && !shown[item.problemKey()] {
				shown[item.problemKey()] = true

				combined = append(combined, item.validationError())
			}
		}

		return changed
	})
	if err != nil {
		return nil, err
	}

	return combined, nil
}

// RecordCompletionBlock counts a completion-gate block and returns how many
// consecutive blocks the gate has issued. A gate the provider did not reach
// through a previous block (continued is false) starts a new streak.
func (s *Store) RecordCompletionBlock(
	provider hook.Provider,
	sessionID string,
	gate string,
	continued bool,
) (int, error) {
	count := 0

	err := s.updateEntry(provider, sessionID, true, func(entry *sessionEntry) bool {
		if entry.CompletionBlocks == nil {
			entry.CompletionBlocks = make(map[string]int)
		}

		if !continued {
			entry.CompletionBlocks[gate] = 0
		}

		entry.CompletionBlocks[gate]++
		count = entry.CompletionBlocks[gate]

		return true
	})

	return count, err
}

// ResetCompletionBlocks clears a gate's consecutive-block counter.
func (s *Store) ResetCompletionBlocks(provider hook.Provider, sessionID, gate string) error {
	return s.updateEntry(provider, sessionID, false, func(entry *sessionEntry) bool {
		if _, ok := entry.CompletionBlocks[gate]; !ok {
			return false
		}

		delete(entry.CompletionBlocks, gate)

		return true
	})
}

// updateEntry applies fn to a provider/session entry and saves the state when
// fn reports a change. A missing entry is created only when create is true;
// otherwise fn is skipped.
func (s *Store) updateEntry(
	provider hook.Provider,
	sessionID string,
	create bool,
	fn func(*sessionEntry) bool,
) error {
	if provider == hook.ProviderUnknown || sessionID == "" {
		return nil
	}

	return s.update(func(st *state) bool {
		key := sessionKey(provider, sessionID)
		now := s.now()

		entry := st.Sessions[key]
		if entry == nil {
			if !create {
				return false
			}

			entry = &sessionEntry{
				Provider:  string(provider),
				SessionID: sessionID,
				StartedAt: now,
			}
			st.Sessions[key] = entry
		}

		if !fn(entry) {
			return false
		}

		entry.UpdatedAt = now

		return true
	})
}

// update runs one load, modify, save transaction under the state lock. fn
// reports whether it changed the state; expired sessions are dropped either
// way. Every write goes through here. When the lock cannot be taken in time
// the transaction is skipped and the error wraps filelock.ErrTimeout.
func (s *Store) update(fn func(*state) bool) (err error) {
	if err = xdg.EnsureDir(filepath.Dir(s.stateFile)); err != nil {
		return err
	}

	lock, err := filelock.Acquire(s.lockFile(), s.lockTimeout)
	if err != nil {
		return errors.Wrap(err, "failed to lock hook session state")
	}

	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil && err == nil {
			err = errors.Wrap(releaseErr, "failed to unlock hook session state")
		}
	}()

	st, err := s.loadState()
	if errors.Is(err, errCorruptState) {
		st, err = s.quarantineCorruptState()
	}

	if err != nil {
		return err
	}

	changed := s.cleanupExpired(st)
	if s.cleanupExpiredEvidence(st) {
		changed = true
	}

	if fn(st) {
		changed = true
	}

	if !changed {
		return nil
	}

	s.removeOrphanedTempFiles()

	return s.saveState(st)
}

// quarantineCorruptState moves an unparsable state file aside, keeping it
// for inspection, so one bad write by an older release cannot block every
// later update. Callers hold the state lock.
func (s *Store) quarantineCorruptState() (*state, error) {
	corrupt := s.stateFile + ".corrupt-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.Rename(s.stateFile, corrupt); err != nil {
		return nil, errors.Wrap(err, "failed to move corrupt hook session state aside")
	}

	return &state{Sessions: make(map[string]*sessionEntry)}, nil
}

func (s *Store) lockFile() string {
	return s.stateFile + ".lock"
}

// touchInterval is how stale a session in use may get before a hook with
// nothing else to save refreshes it, so cleanup never drops it.
func (s *Store) touchInterval() time.Duration {
	return min(maxTouchInterval, s.retention/touchDivisor)
}

// Clear removes a provider/session entry.
func (s *Store) Clear(provider hook.Provider, sessionID string) error {
	if provider == hook.ProviderUnknown || sessionID == "" {
		return nil
	}

	return s.update(func(st *state) bool {
		key := sessionKey(provider, sessionID)
		if _, ok := st.Sessions[key]; !ok {
			return false
		}

		delete(st.Sessions, key)

		return true
	})
}

func (s *Store) loadState() (*state, error) {
	data, err := os.ReadFile(s.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &state{Sessions: make(map[string]*sessionEntry)}, nil
		}

		return nil, errors.Wrap(err, "failed to read hook session state")
	}

	if len(data) == 0 {
		return &state{Sessions: make(map[string]*sessionEntry)}, nil
	}

	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, errors.Mark(
			errors.Wrap(err, "failed to parse hook session state"),
			errCorruptState,
		)
	}

	if st.Sessions == nil {
		st.Sessions = make(map[string]*sessionEntry)
	}

	for _, entry := range st.Sessions {
		if entry != nil {
			entry.Findings = slices.DeleteFunc(entry.Findings, isLegacyFinding)
		}
	}

	return &st, nil
}

func (s *Store) saveState(st *state) error {
	if st == nil {
		st = &state{Sessions: make(map[string]*sessionEntry)}
	}

	if st.Sessions == nil {
		st.Sessions = make(map[string]*sessionEntry)
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal hook session state")
	}

	data = append(data, '\n')

	return writeFileAtomic(s.stateFile, data)
}

func (s *Store) cleanupExpired(st *state) bool {
	if st == nil || len(st.Sessions) == 0 {
		return false
	}

	now := s.now()
	changed := false

	for key, entry := range st.Sessions {
		if entry == nil {
			delete(st.Sessions, key)

			changed = true

			continue
		}

		updatedAt := entry.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = entry.StartedAt
		}

		if !updatedAt.IsZero() && now.Sub(updatedAt) > s.retention {
			delete(st.Sessions, key)

			changed = true
		}
	}

	return changed
}

// touch refreshes UpdatedAt of a session still in use once it is older than
// interval, and reports whether it did.
func (e *sessionEntry) touch(now time.Time, interval time.Duration) bool {
	if now.Sub(e.UpdatedAt) < interval {
		return false
	}

	e.UpdatedAt = now

	return true
}

// record resolves the findings the checks cleared or whose file is gone,
// upserts errs, and reports whether anything changed.
func (e *sessionEntry) record(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	checks []dispatcher.Check,
	now time.Time,
) bool {
	current := make([]*finding, 0, len(errs))
	reported := make(map[string]bool, len(errs))

	for _, verr := range errs {
		if verr == nil {
			continue
		}

		item := findingFromValidationError(hookCtx, verr, now)
		current = append(current, item)
		reported[item.problemKey()] = true
	}

	checked := make(map[checkKey]bool, len(checks))
	for _, check := range checks {
		checked[checkKey{validator: check.Validator, resource: check.Resource}] = true
	}

	for _, item := range current {
		item.Recheckable = checked[checkKey{validator: item.Validator, resource: item.Resource}]
	}

	changed := e.retire(now, func(item *finding) bool {
		if checked[checkKey{validator: item.Validator, resource: item.Resource}] {
			return !reported[item.problemKey()]
		}

		return item.fileGone()
	})

	for _, item := range current {
		e.upsert(item, now)
	}

	if e.markChecked(checks, now) {
		changed = true
	}

	e.evictOverflow()

	return changed || len(current) > 0
}

// markChecked records when a whole-file check last looked at each file with
// unresolved findings, so an unchanged file is not offered for a recheck.
func (e *sessionEntry) markChecked(checks []dispatcher.Check, now time.Time) bool {
	files := make(map[string]bool)

	for _, check := range checks {
		if strings.HasPrefix(check.Resource, hook.ResourceFilePrefix) {
			files[check.Resource] = true
		}
	}

	changed := false

	for _, item := range e.Findings {
		if files[item.Resource] {
			item.CheckedAt = now
			changed = true
		}
	}

	return changed
}

// evictOverflow drops the oldest findings past maxUnresolved, advisory ones
// before blocking ones, so a flood of warnings cannot hide a blocker.
func (e *sessionEntry) evictOverflow() {
	extra := len(e.Findings) - maxUnresolved
	if extra <= 0 {
		return
	}

	for _, blocking := range []bool{false, true} {
		e.Findings = slices.DeleteFunc(e.Findings, func(item *finding) bool {
			if extra > 0 && item.ShouldBlock == blocking {
				extra--

				return true
			}

			return false
		})
	}
}

// retire moves the unresolved findings resolved reports on to the history
// and reports whether there were any.
func (e *sessionEntry) retire(now time.Time, resolved func(*finding) bool) bool {
	unresolved := make([]*finding, 0, len(e.Findings))
	changed := false

	for _, item := range e.Findings {
		if !resolved(item) {
			unresolved = append(unresolved, item)

			continue
		}

		changed = true

		e.Resolved = append(e.Resolved, item.historyEntry(now))
	}

	if !changed {
		return false
	}

	e.Findings = unresolved

	if extra := len(e.Resolved) - maxResolvedHistory; extra > 0 {
		e.Resolved = slices.Delete(e.Resolved, 0, extra)
	}

	return true
}

// upsert adds item or refreshes the matching unresolved finding with the
// latest evidence.
func (e *sessionEntry) upsert(item *finding, now time.Time) {
	itemKey := item.identityKey()

	for _, existing := range e.Findings {
		if existing.identityKey() != itemKey {
			continue
		}

		existing.Count++
		existing.LastSeen = now
		existing.Recheckable = existing.Recheckable || item.Recheckable

		if item.Details != nil {
			existing.Details = item.Details
		}

		existing.FixHint = item.FixHint
		existing.BypassReason = item.BypassReason
		existing.Findings = item.Findings
		existing.Unavailable = item.Unavailable
		existing.Event = item.Event
		existing.RawEventName = item.RawEventName
		existing.ToolName = item.ToolName
		existing.ToolFamily = item.ToolFamily
		existing.Command = item.Command
		existing.FilePath = item.FilePath
		existing.AffectedPaths = item.AffectedPaths

		return
	}

	e.Findings = append(e.Findings, item)
}

func findingFromValidationError(
	hookCtx *hook.Context,
	verr *dispatcher.ValidationError,
	now time.Time,
) *finding {
	item := &finding{
		Validator:    verr.Validator,
		Resource:     verr.Resource,
		Message:      verr.Message,
		Details:      cloneDetails(verr.Details),
		ShouldBlock:  verr.ShouldBlock,
		Reference:    string(verr.Reference),
		FixHint:      verr.FixHint,
		Bypassed:     verr.Bypassed,
		BypassReason: verr.BypassReason,
		Findings:     slices.Clone(verr.Findings),
		Unavailable:  verr.Unavailable,
		Count:        1,
		FirstSeen:    now,
		LastSeen:     now,
	}

	if hookCtx != nil {
		item.AgentID = hookCtx.AgentID
		item.Event = string(hookCtx.Event)
		item.RawEventName = hookCtx.EventName()
		item.ToolName = hookCtx.ToolNameString()
		item.ToolFamily = string(hookCtx.ToolFamily)
		item.Command = hookCtx.GetCommand()
		item.FilePath = hookCtx.GetFilePath()
		item.AffectedPaths = append([]string(nil), hookCtx.AffectedPaths...)

		if item.Resource == "" {
			item.Resource = hookCtx.Resource()
		}
	}

	return item
}

func (f *finding) validationError() *dispatcher.ValidationError {
	details := cloneDetails(f.Details)
	if f.Count > 1 {
		if details == nil {
			details = make(map[string]string)
		}

		details["occurrences"] = strconv.Itoa(f.Count)
	}

	return &dispatcher.ValidationError{
		Validator:    f.Validator,
		Message:      f.Message,
		Details:      details,
		ShouldBlock:  f.ShouldBlock,
		Reference:    validator.Reference(f.Reference),
		FixHint:      f.FixHint,
		Bypassed:     f.Bypassed,
		BypassReason: f.BypassReason,
		Findings:     slices.Clone(f.Findings),
		Unavailable:  f.Unavailable,
		Resource:     f.Resource,
	}
}

// historyEntry is the slim record of a resolved finding kept for audit.
func (f *finding) historyEntry(now time.Time) *finding {
	return &finding{
		Validator:   f.Validator,
		Resource:    f.Resource,
		AgentID:     f.AgentID,
		Message:     f.Message,
		ShouldBlock: f.ShouldBlock,
		Reference:   f.Reference,
		Count:       f.Count,
		FirstSeen:   f.FirstSeen,
		LastSeen:    f.LastSeen,
		ResolvedAt:  now,
	}
}

// filePath returns the file a finding is about, if it is about one.
func (f *finding) filePath() (string, bool) {
	return strings.CutPrefix(f.Resource, hook.ResourceFilePrefix)
}

// fileGone reports a finding about a file that no longer exists.
func (f *finding) fileGone() bool {
	path, ok := f.filePath()
	if !ok || !filepath.IsAbs(path) {
		return false
	}

	_, err := os.Lstat(path)

	return errors.Is(err, fs.ErrNotExist)
}

// identityKey tells findings apart within one session: the same problem
// reported by the same agent is one finding however often it is reported.
func (f *finding) identityKey() string {
	return f.AgentID + "\x1f" + f.problemKey()
}

// problemKey identifies the problem on its resource whoever reported it, so
// a check that still sees it keeps every agent's copy unresolved.
func (f *finding) problemKey() string {
	return strings.Join([]string{
		f.Validator,
		f.Resource,
		f.Message,
		strconv.FormatBool(f.ShouldBlock),
		f.Reference,
		strconv.FormatBool(f.Bypassed),
	}, "\x1f")
}

// isLegacyFinding reports a finding stored before findings named the
// resource they are about. Nothing could resolve it, and the releases that
// wrote it dropped findings at every completion gate anyway.
func isLegacyFinding(item *finding) bool {
	return item == nil || item.Resource == ""
}

func sessionKey(provider hook.Provider, sessionID string) string {
	return string(provider) + ":" + sessionID
}

func cloneDetails(details map[string]string) map[string]string {
	if len(details) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(details))
	maps.Copy(cloned, details)

	return cloned
}
