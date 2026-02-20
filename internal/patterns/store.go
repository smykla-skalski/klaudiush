// Package patterns tracks failure pattern sequences across validation runs.
// When one validation error commonly follows another (e.g., fixing GIT013
// often causes GIT004), the package records these sequences and generates
// warnings to help Claude fix all issues in one pass.
package patterns

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	// stateFilePermissions is the permission mode for pattern data files.
	stateFilePermissions = 0o600

	// stateDirPermissions is the permission mode for pattern data directories.
	stateDirPermissions = 0o700

	// patternDataVersion is the current version of the data format.
	patternDataVersion = 1
)

// FailurePattern represents a known sequence where one error follows another.
type FailurePattern struct {
	SourceCode string    `json:"source_code"`
	TargetCode string    `json:"target_code"`
	Count      int       `json:"count"`
	LastSeen   time.Time `json:"last_seen"`
	FirstSeen  time.Time `json:"first_seen"`
	Seed       bool      `json:"seed,omitempty"`
}

// PatternData is the on-disk format for pattern storage.
type PatternData struct {
	Patterns    map[string]*FailurePattern `json:"patterns"`
	Sessions    map[string][]string        `json:"sessions,omitempty"`
	LastUpdated time.Time                  `json:"last_updated"`
	Version     int                        `json:"version"`
}

// PatternStore reads and writes failure patterns.
type PatternStore interface {
	Load() error
	Save() error
	RecordSequence(sourceCode, targetCode string)
	GetFollowUps(sourceCode string, minCount int) []*FailurePattern
	GetAllPatterns() []*FailurePattern
	Cleanup(maxAge time.Duration) int
}

// FilePatternStore implements PatternStore with file-based persistence.
// It uses dual storage: project-local for seeds/shared and global for learned.
type FilePatternStore struct {
	projectPath string
	globalPath  string
	projectData *PatternData
	globalData  *PatternData
}

// NewFilePatternStore creates a store with dual storage paths.
// projectPath holds seed/shared patterns, globalPath holds learned patterns.
func NewFilePatternStore(cfg *config.PatternsConfig, projectDir string) *FilePatternStore {
	projectFile := filepath.Join(projectDir, cfg.GetProjectDataFile())

	// Global path uses a hash of the project directory for isolation
	globalDir := resolvePath(cfg.GetGlobalDataDir())
	hash := hashProjectPath(projectDir)
	globalFile := filepath.Join(globalDir, hash+".json")

	return &FilePatternStore{
		projectPath: projectFile,
		globalPath:  globalFile,
		projectData: newPatternData(),
		globalData:  newPatternData(),
	}
}

// Load reads both project and global pattern files.
func (s *FilePatternStore) Load() error {
	if data, err := loadPatternFile(s.projectPath); err == nil {
		s.projectData = data
	}

	if data, err := loadPatternFile(s.globalPath); err == nil {
		s.globalData = data
	}

	return nil
}

// Save writes the global pattern file (learned patterns only).
func (s *FilePatternStore) Save() error {
	s.globalData.LastUpdated = time.Now()

	return savePatternFile(s.globalPath, s.globalData)
}

// SaveProject writes the project-local pattern file (seeds/shared).
func (s *FilePatternStore) SaveProject() error {
	s.projectData.LastUpdated = time.Now()

	return savePatternFile(s.projectPath, s.projectData)
}

// RecordSequence records a source->target error sequence in global storage.
func (s *FilePatternStore) RecordSequence(sourceCode, targetCode string) {
	key := sourceCode + "->" + targetCode
	now := time.Now()

	if existing, ok := s.globalData.Patterns[key]; ok {
		existing.Count++
		existing.LastSeen = now
	} else {
		s.globalData.Patterns[key] = &FailurePattern{
			SourceCode: sourceCode,
			TargetCode: targetCode,
			Count:      1,
			FirstSeen:  now,
			LastSeen:   now,
		}
	}
}

// GetFollowUps returns patterns where sourceCode matches and count >= minCount.
// Merges project and global stores, summing counts for overlapping keys.
func (s *FilePatternStore) GetFollowUps(sourceCode string, minCount int) []*FailurePattern {
	merged := s.mergePatterns()

	var results []*FailurePattern

	for _, p := range merged {
		if p.SourceCode == sourceCode && p.Count >= minCount {
			results = append(results, p)
		}
	}

	return results
}

// GetAllPatterns returns all patterns from both stores, merged.
func (s *FilePatternStore) GetAllPatterns() []*FailurePattern {
	merged := s.mergePatterns()

	results := make([]*FailurePattern, 0, len(merged))
	for _, p := range merged {
		results = append(results, p)
	}

	return results
}

// Cleanup removes patterns older than maxAge from global storage.
// Returns the number of patterns removed.
func (s *FilePatternStore) Cleanup(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for key, p := range s.globalData.Patterns {
		if p.LastSeen.Before(cutoff) {
			delete(s.globalData.Patterns, key)

			removed++
		}
	}

	return removed
}

// GetSessionCodes returns the previous blocking codes for a session.
func (s *FilePatternStore) GetSessionCodes(sessionID string) []string {
	if s.globalData.Sessions == nil {
		return nil
	}

	return s.globalData.Sessions[sessionID]
}

// SetSessionCodes stores the blocking codes for a session.
func (s *FilePatternStore) SetSessionCodes(sessionID string, codes []string) {
	if s.globalData.Sessions == nil {
		s.globalData.Sessions = make(map[string][]string)
	}

	s.globalData.Sessions[sessionID] = codes
}

// ClearSessionCodes removes stored codes for a session.
func (s *FilePatternStore) ClearSessionCodes(sessionID string) {
	if s.globalData.Sessions == nil {
		return
	}

	delete(s.globalData.Sessions, sessionID)
}

// SetProjectData sets the project-local pattern data directly.
// Used for writing seed data.
func (s *FilePatternStore) SetProjectData(data *PatternData) {
	s.projectData = data
}

// HasProjectData returns true if the project-local file exists on disk.
func (s *FilePatternStore) HasProjectData() bool {
	_, err := os.Stat(s.projectPath)

	return err == nil
}

// mergePatterns combines project and global data, summing counts for same keys.
func (s *FilePatternStore) mergePatterns() map[string]*FailurePattern {
	merged := make(map[string]*FailurePattern)

	for key, p := range s.projectData.Patterns {
		merged[key] = &FailurePattern{
			SourceCode: p.SourceCode,
			TargetCode: p.TargetCode,
			Count:      p.Count,
			FirstSeen:  p.FirstSeen,
			LastSeen:   p.LastSeen,
			Seed:       p.Seed,
		}
	}

	for key, p := range s.globalData.Patterns {
		if existing, ok := merged[key]; ok {
			existing.Count += p.Count

			if p.LastSeen.After(existing.LastSeen) {
				existing.LastSeen = p.LastSeen
			}

			if p.FirstSeen.Before(existing.FirstSeen) {
				existing.FirstSeen = p.FirstSeen
			}
		} else {
			merged[key] = &FailurePattern{
				SourceCode: p.SourceCode,
				TargetCode: p.TargetCode,
				Count:      p.Count,
				FirstSeen:  p.FirstSeen,
				LastSeen:   p.LastSeen,
				Seed:       p.Seed,
			}
		}
	}

	return merged
}

func newPatternData() *PatternData {
	return &PatternData{
		Patterns: make(map[string]*FailurePattern),
		Version:  patternDataVersion,
	}
}

func loadPatternFile(path string) (*PatternData, error) {
	//nolint:gosec // G304: path is from trusted config
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "reading pattern file")
	}

	var pd PatternData
	if err := json.Unmarshal(data, &pd); err != nil {
		return nil, errors.Wrap(err, "parsing pattern file")
	}

	if pd.Patterns == nil {
		pd.Patterns = make(map[string]*FailurePattern)
	}

	return &pd, nil
}

func savePatternFile(path string, data *PatternData) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, stateDirPermissions); err != nil {
		return errors.Wrap(err, "creating pattern directory")
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errors.Wrap(err, "marshaling pattern data")
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, stateFilePermissions); err != nil {
		return errors.Wrap(err, "writing temp pattern file")
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, "renaming pattern file")
	}

	return nil
}

func resolvePath(path string) string {
	if len(path) > 1 && path[0] == '~' && path[1] == '/' {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}

	return path
}

func hashProjectPath(projectDir string) string {
	h := sha256.Sum256([]byte(projectDir))

	return hex.EncodeToString(h[:8])
}
