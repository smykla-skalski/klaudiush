package backup

import (
	"sort"
	"time"

	"github.com/cockroachdb/errors"
)

var (
	// ErrInvalidMaxBackups is returned when MaxBackups is invalid.
	ErrInvalidMaxBackups = errors.New("max backups must be positive")

	// ErrInvalidMaxAge is returned when MaxAge is invalid.
	ErrInvalidMaxAge = errors.New("max age must be positive")

	// ErrInvalidMaxSize is returned when MaxSize is invalid.
	ErrInvalidMaxSize = errors.New("max size must be positive")
)

// RetentionPolicy defines how backups should be retained or pruned.
type RetentionPolicy interface {
	// ShouldRetain returns true if the snapshot should be retained.
	// Chain-aware: policies should consider full + patch relationships.
	ShouldRetain(snapshot Snapshot, context RetentionContext) bool
}

// RetentionContext provides context for retention decisions.
type RetentionContext struct {
	// AllSnapshots is all snapshots in the index.
	AllSnapshots []Snapshot

	// Chain is all snapshots in the same chain as the evaluated snapshot.
	Chain []Snapshot

	// TotalSize is the total size of all snapshots.
	TotalSize int64

	// Now is the current time for age calculations.
	Now time.Time
}

// CountRetentionPolicy retains only the N most recent backups per chain.
// Chain-aware: Counts chains, not individual snapshots.
type CountRetentionPolicy struct {
	MaxBackups int
}

// NewCountRetentionPolicy creates a new count retention policy.
func NewCountRetentionPolicy(maxBackups int) (*CountRetentionPolicy, error) {
	if maxBackups <= 0 {
		return nil, ErrInvalidMaxBackups
	}

	return &CountRetentionPolicy{
		MaxBackups: maxBackups,
	}, nil
}

// ShouldRetain implements RetentionPolicy.
func (p *CountRetentionPolicy) ShouldRetain(snapshot Snapshot, context RetentionContext) bool {
	// Group all snapshots by chain
	chains := groupByChain(context.AllSnapshots)

	// Get chain IDs sorted by newest snapshot in each chain
	chainIDs := make([]string, 0, len(chains))
	for chainID := range chains {
		chainIDs = append(chainIDs, chainID)
	}

	sort.Slice(chainIDs, func(i, j int) bool {
		iNewest := getNewestSnapshot(chains[chainIDs[i]])
		jNewest := getNewestSnapshot(chains[chainIDs[j]])

		return iNewest.Timestamp.After(jNewest.Timestamp)
	})

	// Keep only the N most recent chains
	keepChains := make(map[string]bool)
	for i := 0; i < len(chainIDs) && i < p.MaxBackups; i++ {
		keepChains[chainIDs[i]] = true
	}

	return keepChains[snapshot.ChainID]
}

// AgeRetentionPolicy removes backups older than MaxAge.
// Chain-aware: If oldest snapshot in chain exceeds MaxAge, removes entire chain.
type AgeRetentionPolicy struct {
	MaxAge time.Duration
}

// NewAgeRetentionPolicy creates a new age retention policy.
func NewAgeRetentionPolicy(maxAge time.Duration) (*AgeRetentionPolicy, error) {
	if maxAge <= 0 {
		return nil, ErrInvalidMaxAge
	}

	return &AgeRetentionPolicy{
		MaxAge: maxAge,
	}, nil
}

// ShouldRetain implements RetentionPolicy.
func (p *AgeRetentionPolicy) ShouldRetain(_ Snapshot, context RetentionContext) bool {
	// Get the oldest snapshot in this chain
	oldestInChain := getOldestSnapshot(context.Chain)

	// If oldest snapshot in chain is too old, remove entire chain
	age := context.Now.Sub(oldestInChain.Timestamp)

	return age <= p.MaxAge
}

// SizeRetentionPolicy removes oldest chains when total size exceeds MaxSize.
// Chain-aware: Removes entire chains, not individual snapshots.
type SizeRetentionPolicy struct {
	MaxSize int64
}

// NewSizeRetentionPolicy creates a new size retention policy.
func NewSizeRetentionPolicy(maxSize int64) (*SizeRetentionPolicy, error) {
	if maxSize <= 0 {
		return nil, ErrInvalidMaxSize
	}

	return &SizeRetentionPolicy{
		MaxSize: maxSize,
	}, nil
}

// ShouldRetain implements RetentionPolicy.
func (p *SizeRetentionPolicy) ShouldRetain(snapshot Snapshot, context RetentionContext) bool {
	if context.TotalSize <= p.MaxSize {
		return true
	}

	// Group by chain and calculate chain sizes
	chains := groupByChain(context.AllSnapshots)
	chainSizes := make(map[string]int64)

	for chainID, chainSnapshots := range chains {
		var totalSize int64
		for _, s := range chainSnapshots {
			totalSize += s.Size
		}

		chainSizes[chainID] = totalSize
	}

	// Sort chains by oldest snapshot timestamp (oldest first)
	chainIDs := make([]string, 0, len(chains))
	for chainID := range chains {
		chainIDs = append(chainIDs, chainID)
	}

	sort.Slice(chainIDs, func(i, j int) bool {
		iOldest := getOldestSnapshot(chains[chainIDs[i]])
		jOldest := getOldestSnapshot(chains[chainIDs[j]])

		return iOldest.Timestamp.Before(jOldest.Timestamp)
	})

	// Remove oldest chains until size is under limit
	currentSize := context.TotalSize

	removeChains := make(map[string]bool)

	for _, chainID := range chainIDs {
		if currentSize <= p.MaxSize {
			break
		}

		removeChains[chainID] = true
		currentSize -= chainSizes[chainID]
	}

	return !removeChains[snapshot.ChainID]
}

// CompositeRetentionPolicy combines multiple policies with AND logic.
// A snapshot is retained only if ALL policies agree to retain it.
type CompositeRetentionPolicy struct {
	Policies []RetentionPolicy
}

// NewCompositeRetentionPolicy creates a new composite retention policy.
func NewCompositeRetentionPolicy(policies ...RetentionPolicy) *CompositeRetentionPolicy {
	return &CompositeRetentionPolicy{
		Policies: policies,
	}
}

// ShouldRetain implements RetentionPolicy.
func (p *CompositeRetentionPolicy) ShouldRetain(snapshot Snapshot, context RetentionContext) bool {
	for _, policy := range p.Policies {
		if !policy.ShouldRetain(snapshot, context) {
			return false
		}
	}

	return true
}

// Helper functions

// groupByChain groups snapshots by chain ID.
func groupByChain(snapshots []Snapshot) map[string][]Snapshot {
	chains := make(map[string][]Snapshot)

	for _, snapshot := range snapshots {
		chains[snapshot.ChainID] = append(chains[snapshot.ChainID], snapshot)
	}

	return chains
}

// getOldestSnapshot returns the oldest snapshot in a slice.
func getOldestSnapshot(snapshots []Snapshot) Snapshot {
	if len(snapshots) == 0 {
		return Snapshot{}
	}

	oldest := snapshots[0]
	for _, snapshot := range snapshots[1:] {
		if snapshot.Timestamp.Before(oldest.Timestamp) {
			oldest = snapshot
		}
	}

	return oldest
}

// getNewestSnapshot returns the newest snapshot in a slice.
func getNewestSnapshot(snapshots []Snapshot) Snapshot {
	if len(snapshots) == 0 {
		return Snapshot{}
	}

	newest := snapshots[0]
	for _, snapshot := range snapshots[1:] {
		if snapshot.Timestamp.After(newest.Timestamp) {
			newest = snapshot
		}
	}

	return newest
}
