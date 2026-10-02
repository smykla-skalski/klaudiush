// Package hooksession persists provider hook findings across hook invocations.
package hooksession

import (
	"encoding/json"
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
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/xdg"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	defaultRetention = 7 * 24 * time.Hour
	stateFileMode    = 0o600

	// maxResolvedHistory bounds the resolved findings kept per session.
	maxResolvedHistory = 50
)

type state struct {
	Sessions map[string]*sessionEntry `json:"sessions"`
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
	ResolvedAt    time.Time           `json:"resolved_at,omitzero"`
}

// checkKey is one validator checking one resource.
type checkKey struct {
	validator string
	resource  string
}

// Store persists per-session hook findings across hook invocations. Every
// change is one load, modify, save transaction (see update).
type Store struct {
	stateFile string
	now       func() time.Time
	retention time.Duration
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

// NewStore creates a persisted session findings store.
func NewStore(opts ...Option) *Store {
	store := &Store{
		stateFile: xdg.HookSessionStateFile(),
		now:       time.Now,
		retention: defaultRetention,
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
	return s.updateEntry(provider, sessionID, true, func(*sessionEntry) {})
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
	if hookCtx == nil {
		return nil
	}

	return s.updateEntry(hookCtx.Provider, hookCtx.SessionID, true, func(entry *sessionEntry) {
		entry.record(hookCtx, errs, checks, s.now())
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

// UnresolvedFiles returns the canonical paths of files with unresolved
// findings in a provider/session pair.
func (s *Store) UnresolvedFiles(provider hook.Provider, sessionID string) ([]string, error) {
	if provider == hook.ProviderUnknown || sessionID == "" {
		return nil, nil
	}

	st, err := s.loadState()
	if err != nil {
		return nil, err
	}

	entry := st.Sessions[sessionKey(provider, sessionID)]
	if entry == nil {
		return nil, nil
	}

	var files []string

	for _, item := range entry.Findings {
		if path, ok := strings.CutPrefix(item.resource(), hook.ResourceFilePrefix); ok &&
			!slices.Contains(files, path) {
			files = append(files, path)
		}
	}

	sort.Strings(files)

	return files, nil
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

		for _, item := range entry.Findings {
			if include(item) {
				combined = append(combined, item.validationError())
			}
		}

		return false
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

	err := s.updateEntry(provider, sessionID, true, func(entry *sessionEntry) {
		if entry.CompletionBlocks == nil {
			entry.CompletionBlocks = make(map[string]int)
		}

		if !continued {
			entry.CompletionBlocks[gate] = 0
		}

		entry.CompletionBlocks[gate]++
		count = entry.CompletionBlocks[gate]
	})

	return count, err
}

// ResetCompletionBlocks clears a gate's consecutive-block counter.
func (s *Store) ResetCompletionBlocks(provider hook.Provider, sessionID, gate string) error {
	return s.updateEntry(provider, sessionID, false, func(entry *sessionEntry) {
		delete(entry.CompletionBlocks, gate)
	})
}

// updateEntry applies fn to a provider/session entry and saves the state. A
// missing entry is created only when create is true; otherwise fn is skipped.
func (s *Store) updateEntry(
	provider hook.Provider,
	sessionID string,
	create bool,
	fn func(*sessionEntry),
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

		fn(entry)

		entry.UpdatedAt = now

		return true
	})
}

// update runs one load, modify, save transaction. fn reports whether it
// changed the state; expired sessions are dropped either way. Every write
// goes through here, so serializing writers only needs to wrap this.
func (s *Store) update(fn func(*state) bool) error {
	st, err := s.loadState()
	if err != nil {
		return err
	}

	changed := s.cleanupExpired(st)

	if fn(st) {
		changed = true
	}

	if !changed {
		return nil
	}

	return s.saveState(st)
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
		return nil, errors.Wrap(err, "failed to parse hook session state")
	}

	if st.Sessions == nil {
		st.Sessions = make(map[string]*sessionEntry)
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

	if err := xdg.EnsureDir(filepath.Dir(s.stateFile)); err != nil {
		return err
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal hook session state")
	}

	data = append(data, '\n')

	tmpFile := s.stateFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, stateFileMode); err != nil {
		return errors.Wrap(err, "failed to write hook session temp file")
	}

	if err := os.Rename(tmpFile, s.stateFile); err != nil {
		_ = os.Remove(tmpFile)
		return errors.Wrap(err, "failed to replace hook session state")
	}

	return nil
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

// record resolves the findings the checks cleared and upserts errs.
func (e *sessionEntry) record(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	checks []dispatcher.Check,
	now time.Time,
) {
	current := make([]*finding, 0, len(errs))
	reported := make(map[string]bool, len(errs))

	for _, verr := range errs {
		if verr == nil {
			continue
		}

		item := findingFromValidationError(hookCtx, verr, now)
		current = append(current, item)
		reported[item.identityKey()] = true
	}

	e.resolve(checks, reported, now)

	for _, item := range current {
		e.upsert(item, now)
	}
}

// resolve moves findings a check covered but did not report to the history.
func (e *sessionEntry) resolve(
	checks []dispatcher.Check,
	reported map[string]bool,
	now time.Time,
) {
	if len(checks) == 0 || len(e.Findings) == 0 {
		return
	}

	checked := make(map[checkKey]bool, len(checks))
	for _, check := range checks {
		checked[checkKey{validator: check.Validator, resource: check.Resource}] = true
	}

	unresolved := e.Findings[:0]

	for _, item := range e.Findings {
		key := checkKey{validator: item.Validator, resource: item.resource()}
		if !checked[key] || reported[item.identityKey()] {
			unresolved = append(unresolved, item)

			continue
		}

		item.ResolvedAt = now
		e.Resolved = append(e.Resolved, item)
	}

	clear(e.Findings[len(unresolved):])
	e.Findings = unresolved

	if extra := len(e.Resolved) - maxResolvedHistory; extra > 0 {
		e.Resolved = slices.Delete(e.Resolved, 0, extra)
	}
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
		Resource:     f.resource(),
	}
}

// resource returns what the finding is about. Findings stored before
// resources were recorded fall back to the evidence they kept.
func (f *finding) resource() string {
	if f.Resource != "" {
		return f.Resource
	}

	switch {
	case f.FilePath != "" && f.Command == "":
		return hook.ResourceFilePrefix + hook.CanonicalFilePath("", f.FilePath)
	case f.Command != "":
		return hook.ResourceCommandPrefix + f.Command
	default:
		return hook.ResourceToolPrefix + f.ToolName
	}
}

// identityKey tells findings apart within one session: the same problem on
// the same resource is one finding however often it is reported.
func (f *finding) identityKey() string {
	return strings.Join([]string{
		f.Validator,
		f.resource(),
		f.Message,
		strconv.FormatBool(f.ShouldBlock),
		f.Reference,
		strconv.FormatBool(f.Bypassed),
	}, "\x1f")
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
