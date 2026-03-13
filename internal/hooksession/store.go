// Package hooksession persists provider hook findings across hook invocations.
package hooksession

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
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
)

type state struct {
	Sessions map[string]*sessionEntry `json:"sessions"`
}

type sessionEntry struct {
	Provider  string     `json:"provider"`
	SessionID string     `json:"session_id"`
	StartedAt time.Time  `json:"started_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Findings  []*finding `json:"findings,omitempty"`
}

type finding struct {
	Validator     string            `json:"validator"`
	Message       string            `json:"message"`
	Details       map[string]string `json:"details,omitempty"`
	ShouldBlock   bool              `json:"should_block"`
	Reference     string            `json:"reference,omitempty"`
	FixHint       string            `json:"fix_hint,omitempty"`
	Bypassed      bool              `json:"bypassed,omitempty"`
	BypassReason  string            `json:"bypass_reason,omitempty"`
	Event         string            `json:"event,omitempty"`
	RawEventName  string            `json:"raw_event_name,omitempty"`
	ToolName      string            `json:"tool_name,omitempty"`
	ToolFamily    string            `json:"tool_family,omitempty"`
	Command       string            `json:"command,omitempty"`
	FilePath      string            `json:"file_path,omitempty"`
	AffectedPaths []string          `json:"affected_paths,omitempty"`
	Count         int               `json:"count"`
	FirstSeen     time.Time         `json:"first_seen"`
	LastSeen      time.Time         `json:"last_seen"`
}

// Store persists per-session hook findings across hook invocations.
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

// Start initializes or resets a provider/session entry.
func (s *Store) Start(provider hook.Provider, sessionID string) error {
	if provider == hook.ProviderUnknown || sessionID == "" {
		return nil
	}

	st, err := s.loadState()
	if err != nil {
		return err
	}

	s.cleanupExpired(st)

	now := s.now()
	st.Sessions[sessionKey(provider, sessionID)] = &sessionEntry{
		Provider:  string(provider),
		SessionID: sessionID,
		StartedAt: now,
		UpdatedAt: now,
	}

	return s.saveState(st)
}

// Append records current findings under the hook context session.
func (s *Store) Append(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
) error {
	if hookCtx == nil ||
		hookCtx.Provider == hook.ProviderUnknown ||
		hookCtx.SessionID == "" {
		return nil
	}

	st, err := s.loadState()
	if err != nil {
		return err
	}

	s.cleanupExpired(st)

	key := sessionKey(hookCtx.Provider, hookCtx.SessionID)

	entry := st.Sessions[key]
	if entry == nil {
		now := s.now()
		entry = &sessionEntry{
			Provider:  hookCtx.ProviderName(),
			SessionID: hookCtx.SessionID,
			StartedAt: now,
			UpdatedAt: now,
		}
		st.Sessions[key] = entry
	}

	now := s.now()
	for _, verr := range errs {
		entry.upsertFinding(hookCtx, verr, now)
	}

	entry.UpdatedAt = now

	return s.saveState(st)
}

// CombinedErrors returns aggregated findings for a provider/session pair.
func (s *Store) CombinedErrors(
	provider hook.Provider,
	sessionID string,
) ([]*dispatcher.ValidationError, error) {
	if provider == hook.ProviderUnknown || sessionID == "" {
		return nil, nil
	}

	st, err := s.loadState()
	if err != nil {
		return nil, err
	}

	changed := s.cleanupExpired(st)

	entry := st.Sessions[sessionKey(provider, sessionID)]
	if entry == nil {
		if changed {
			if err := s.saveState(st); err != nil {
				return nil, err
			}
		}

		return nil, nil
	}

	combined := make([]*dispatcher.ValidationError, 0, len(entry.Findings))
	for _, item := range entry.Findings {
		details := cloneDetails(item.Details)
		if item.Count > 1 {
			if details == nil {
				details = make(map[string]string)
			}

			details["occurrences"] = strconv.Itoa(item.Count)
		}

		combined = append(combined, &dispatcher.ValidationError{
			Validator:    item.Validator,
			Message:      item.Message,
			Details:      details,
			ShouldBlock:  item.ShouldBlock,
			Reference:    validator.Reference(item.Reference),
			FixHint:      item.FixHint,
			Bypassed:     item.Bypassed,
			BypassReason: item.BypassReason,
		})
	}

	if changed {
		if err := s.saveState(st); err != nil {
			return nil, err
		}
	}

	return combined, nil
}

// Clear removes a provider/session entry.
func (s *Store) Clear(provider hook.Provider, sessionID string) error {
	if provider == hook.ProviderUnknown || sessionID == "" {
		return nil
	}

	st, err := s.loadState()
	if err != nil {
		return err
	}

	s.cleanupExpired(st)
	delete(st.Sessions, sessionKey(provider, sessionID))

	return s.saveState(st)
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

func (e *sessionEntry) upsertFinding(
	hookCtx *hook.Context,
	verr *dispatcher.ValidationError,
	now time.Time,
) {
	if verr == nil {
		return
	}

	item := findingFromValidationError(hookCtx, verr, now)
	itemKey := item.identityKey()

	for _, existing := range e.Findings {
		if existing.identityKey() != itemKey {
			continue
		}

		existing.Count++

		existing.LastSeen = now

		if existing.Details == nil && item.Details != nil {
			existing.Details = cloneDetails(item.Details)
		}

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
		Message:      verr.Message,
		Details:      cloneDetails(verr.Details),
		ShouldBlock:  verr.ShouldBlock,
		Reference:    string(verr.Reference),
		FixHint:      verr.FixHint,
		Bypassed:     verr.Bypassed,
		BypassReason: verr.BypassReason,
		Count:        1,
		FirstSeen:    now,
		LastSeen:     now,
	}

	if hookCtx != nil {
		item.Event = string(hookCtx.Event)
		item.RawEventName = hookCtx.EventName()
		item.ToolName = hookCtx.ToolNameString()
		item.ToolFamily = string(hookCtx.ToolFamily)
		item.Command = hookCtx.GetCommand()
		item.FilePath = hookCtx.GetFilePath()
		item.AffectedPaths = append([]string(nil), hookCtx.AffectedPaths...)
	}

	return item
}

func (f *finding) identityKey() string {
	if f == nil {
		return ""
	}

	paths := append([]string(nil), f.AffectedPaths...)
	sort.Strings(paths)

	return strings.Join([]string{
		f.Validator,
		f.Message,
		strconv.FormatBool(f.ShouldBlock),
		f.Reference,
		strconv.FormatBool(f.Bypassed),
		f.Event,
		f.RawEventName,
		f.ToolName,
		f.ToolFamily,
		f.Command,
		f.FilePath,
		strings.Join(paths, "\x00"),
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
