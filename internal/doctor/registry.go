package doctor

import (
	"context"
	"maps"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Registry manages health checkers and fixers
type Registry struct {
	mu       sync.RWMutex
	checkers map[Category][]HealthChecker
	fixers   map[string]Fixer
}

// NewRegistry creates a new Registry
func NewRegistry() *Registry {
	return &Registry{
		checkers: make(map[Category][]HealthChecker),
		fixers:   make(map[string]Fixer),
	}
}

// RegisterChecker registers a health checker
func (r *Registry) RegisterChecker(checker HealthChecker) {
	r.mu.Lock()
	defer r.mu.Unlock()

	category := checker.Category()
	r.checkers[category] = append(r.checkers[category], checker)
}

// RegisterFixer registers a fixer
func (r *Registry) RegisterFixer(fixer Fixer) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fixers[fixer.ID()] = fixer
}

// RunAll executes all registered health checkers concurrently
func (r *Registry) RunAll(ctx context.Context) []CheckResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect all checkers across categories
	totalCheckers := 0
	for _, checkers := range r.checkers {
		totalCheckers += len(checkers)
	}

	allCheckers := make([]HealthChecker, 0, totalCheckers)
	for _, checkers := range r.checkers {
		allCheckers = append(allCheckers, checkers...)
	}

	return r.runCheckers(ctx, allCheckers)
}

// RunCategory executes all health checkers in a specific category concurrently
func (r *Registry) RunCategory(ctx context.Context, category Category) []CheckResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	checkers, ok := r.checkers[category]
	if !ok {
		return []CheckResult{}
	}

	return r.runCheckers(ctx, checkers)
}

// runCheckers executes the given checkers concurrently
func (*Registry) runCheckers(ctx context.Context, checkers []HealthChecker) []CheckResult {
	results := make([]CheckResult, len(checkers))
	g, gctx := errgroup.WithContext(ctx)

	for i := range checkers {
		checker := checkers[i]

		g.Go(func() error {
			result := checker.Check(gctx)
			result.Category = checker.Category()
			results[i] = result

			return nil
		})
	}

	// Wait for all checks to complete
	_ = g.Wait()

	return results
}

// GetFixer retrieves a fixer by ID.
//
//nolint:ireturn // Fixer interface for polymorphism
func (r *Registry) GetFixer(fixID string) (Fixer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fixer, ok := r.fixers[fixID]

	return fixer, ok
}

// GetFixers returns all registered fixers
func (r *Registry) GetFixers() map[string]Fixer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modifications
	fixers := make(map[string]Fixer, len(r.fixers))
	maps.Copy(fixers, r.fixers)

	return fixers
}

// Categories returns all registered categories
func (r *Registry) Categories() []Category {
	r.mu.RLock()
	defer r.mu.RUnlock()

	categories := make([]Category, 0, len(r.checkers))
	for category := range r.checkers {
		categories = append(categories, category)
	}

	return categories
}

// CheckerCount returns the total number of registered checkers
func (r *Registry) CheckerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, checkers := range r.checkers {
		count += len(checkers)
	}

	return count
}

// FixerCount returns the total number of registered fixers
func (r *Registry) FixerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.fixers)
}
