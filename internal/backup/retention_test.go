package backup_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/backup"
)

var _ = Describe("Retention Policies", func() {
	var (
		now       time.Time
		chain1    []backup.Snapshot
		chain2    []backup.Snapshot
		allSnaps  []backup.Snapshot
		totalSize int64
	)

	BeforeEach(func() {
		now = time.Now()

		// Chain 1: 3 snapshots (oldest chain)
		chain1 = []backup.Snapshot{
			{
				ID:          "snap1",
				ChainID:     "chain-1",
				SequenceNum: 1,
				Timestamp:   now.Add(-10 * 24 * time.Hour), // 10 days ago
				Size:        1000,
			},
			{
				ID:          "snap2",
				ChainID:     "chain-1",
				SequenceNum: 2,
				Timestamp:   now.Add(-9 * 24 * time.Hour), // 9 days ago
				Size:        500,
			},
			{
				ID:          "snap3",
				ChainID:     "chain-1",
				SequenceNum: 3,
				Timestamp:   now.Add(-8 * 24 * time.Hour), // 8 days ago
				Size:        500,
			},
		}

		// Chain 2: 2 snapshots (newer chain)
		chain2 = []backup.Snapshot{
			{
				ID:          "snap4",
				ChainID:     "chain-2",
				SequenceNum: 1,
				Timestamp:   now.Add(-2 * 24 * time.Hour), // 2 days ago
				Size:        2000,
			},
			{
				ID:          "snap5",
				ChainID:     "chain-2",
				SequenceNum: 2,
				Timestamp:   now.Add(-1 * 24 * time.Hour), // 1 day ago
				Size:        1000,
			},
		}

		allSnaps = append(chain1, chain2...)
		totalSize = int64(5000) // 1000 + 500 + 500 + 2000 + 1000
	})

	Describe("CountRetentionPolicy", func() {
		It("should create a valid policy", func() {
			policy, err := backup.NewCountRetentionPolicy(5)
			Expect(err).ToNot(HaveOccurred())
			Expect(policy).ToNot(BeNil())
		})

		It("should reject zero max backups", func() {
			_, err := backup.NewCountRetentionPolicy(0)
			Expect(err).To(MatchError(backup.ErrInvalidMaxBackups))
		})

		It("should reject negative max backups", func() {
			_, err := backup.NewCountRetentionPolicy(-1)
			Expect(err).To(MatchError(backup.ErrInvalidMaxBackups))
		})

		It("should retain all chains when under limit", func() {
			policy, err := backup.NewCountRetentionPolicy(2)
			Expect(err).ToNot(HaveOccurred())

			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        []backup.Snapshot{snap},
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)
				Expect(shouldRetain).To(BeTrue())
			}
		})

		It("should remove oldest chain when over limit", func() {
			policy, err := backup.NewCountRetentionPolicy(1)
			Expect(err).ToNot(HaveOccurred())

			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        []backup.Snapshot{snap},
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)

				// Chain 1 is older, should be removed
				// Chain 2 is newer, should be retained
				if snap.ChainID == "chain-1" {
					Expect(shouldRetain).To(BeFalse())
				} else {
					Expect(shouldRetain).To(BeTrue())
				}
			}
		})

		It("should count chains not individual snapshots", func() {
			policy, err := backup.NewCountRetentionPolicy(2)
			Expect(err).ToNot(HaveOccurred())

			// Even though we have 5 snapshots, we have 2 chains
			// Both chains should be retained
			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        []backup.Snapshot{snap},
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)
				Expect(shouldRetain).To(BeTrue())
			}
		})
	})

	Describe("AgeRetentionPolicy", func() {
		It("should create a valid policy", func() {
			policy, err := backup.NewAgeRetentionPolicy(7 * 24 * time.Hour)
			Expect(err).ToNot(HaveOccurred())
			Expect(policy).ToNot(BeNil())
		})

		It("should reject zero max age", func() {
			_, err := backup.NewAgeRetentionPolicy(0)
			Expect(err).To(MatchError(backup.ErrInvalidMaxAge))
		})

		It("should reject negative max age", func() {
			_, err := backup.NewAgeRetentionPolicy(-1 * time.Hour)
			Expect(err).To(MatchError(backup.ErrInvalidMaxAge))
		})

		It("should retain chains younger than max age", func() {
			policy, err := backup.NewAgeRetentionPolicy(30 * 24 * time.Hour) // 30 days
			Expect(err).ToNot(HaveOccurred())

			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        getChainSnapshots(allSnaps, snap.ChainID),
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)
				Expect(shouldRetain).To(BeTrue())
			}
		})

		It("should remove chains older than max age", func() {
			policy, err := backup.NewAgeRetentionPolicy(5 * 24 * time.Hour) // 5 days
			Expect(err).ToNot(HaveOccurred())

			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        getChainSnapshots(allSnaps, snap.ChainID),
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)

				// Chain 1 oldest snapshot is 10 days old (> 5 days)
				// Chain 2 oldest snapshot is 2 days old (< 5 days)
				if snap.ChainID == "chain-1" {
					Expect(shouldRetain).To(BeFalse())
				} else {
					Expect(shouldRetain).To(BeTrue())
				}
			}
		})

		It("should remove entire chain if oldest snapshot exceeds age", func() {
			policy, err := backup.NewAgeRetentionPolicy(9 * 24 * time.Hour) // 9 days
			Expect(err).ToNot(HaveOccurred())

			// Chain 1 has snapshots at 10, 9, and 8 days old
			// Oldest is 10 days, so entire chain should be removed
			for _, snap := range chain1 {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        chain1,
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)
				Expect(shouldRetain).To(BeFalse())
			}
		})
	})

	Describe("SizeRetentionPolicy", func() {
		It("should create a valid policy", func() {
			policy, err := backup.NewSizeRetentionPolicy(10000)
			Expect(err).ToNot(HaveOccurred())
			Expect(policy).ToNot(BeNil())
		})

		It("should reject zero max size", func() {
			_, err := backup.NewSizeRetentionPolicy(0)
			Expect(err).To(MatchError(backup.ErrInvalidMaxSize))
		})

		It("should reject negative max size", func() {
			_, err := backup.NewSizeRetentionPolicy(-1)
			Expect(err).To(MatchError(backup.ErrInvalidMaxSize))
		})

		It("should retain all chains when under size limit", func() {
			policy, err := backup.NewSizeRetentionPolicy(10000) // 10KB > 5KB total
			Expect(err).ToNot(HaveOccurred())

			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        getChainSnapshots(allSnaps, snap.ChainID),
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)
				Expect(shouldRetain).To(BeTrue())
			}
		})

		It("should remove oldest chains when over size limit", func() {
			policy, err := backup.NewSizeRetentionPolicy(3000) // 3KB < 5KB total
			Expect(err).ToNot(HaveOccurred())

			// Total size is 5KB, limit is 3KB
			// Chain 1: 2KB (oldest)
			// Chain 2: 3KB (newer)
			// Should remove chain 1 to get under limit

			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        getChainSnapshots(allSnaps, snap.ChainID),
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)

				if snap.ChainID == "chain-1" {
					Expect(shouldRetain).To(BeFalse())
				} else {
					Expect(shouldRetain).To(BeTrue())
				}
			}
		})

		It("should remove multiple chains if needed to get under limit", func() {
			policy, err := backup.NewSizeRetentionPolicy(500) // Very small limit
			Expect(err).ToNot(HaveOccurred())

			// Both chains should be marked for removal to get under 500 bytes
			removedChains := make(map[string]bool)

			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        getChainSnapshots(allSnaps, snap.ChainID),
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)
				if !shouldRetain {
					removedChains[snap.ChainID] = true
				}
			}

			// Should remove both chains
			Expect(removedChains).To(HaveLen(2))
		})
	})

	Describe("CompositeRetentionPolicy", func() {
		It("should create a valid composite policy", func() {
			countPolicy, _ := backup.NewCountRetentionPolicy(5)
			agePolicy, _ := backup.NewAgeRetentionPolicy(7 * 24 * time.Hour)

			policy := backup.NewCompositeRetentionPolicy(countPolicy, agePolicy)
			Expect(policy).ToNot(BeNil())
		})

		It("should retain when all policies agree to retain", func() {
			countPolicy, _ := backup.NewCountRetentionPolicy(5)               // Keep 5 chains
			agePolicy, _ := backup.NewAgeRetentionPolicy(30 * 24 * time.Hour) // Keep 30 days

			policy := backup.NewCompositeRetentionPolicy(countPolicy, agePolicy)

			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        getChainSnapshots(allSnaps, snap.ChainID),
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)
				Expect(shouldRetain).To(BeTrue())
			}
		})

		It("should remove when any policy wants to remove", func() {
			countPolicy, _ := backup.NewCountRetentionPolicy(1)               // Keep only 1 chain
			agePolicy, _ := backup.NewAgeRetentionPolicy(30 * 24 * time.Hour) // Keep 30 days

			policy := backup.NewCompositeRetentionPolicy(countPolicy, agePolicy)

			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        getChainSnapshots(allSnaps, snap.ChainID),
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)

				// Count policy wants to remove chain-1 (oldest)
				// Age policy wants to keep both
				// Result: chain-1 removed (AND logic)
				if snap.ChainID == "chain-1" {
					Expect(shouldRetain).To(BeFalse())
				} else {
					Expect(shouldRetain).To(BeTrue())
				}
			}
		})

		It("should work with empty policies list", func() {
			policy := backup.NewCompositeRetentionPolicy()

			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        getChainSnapshots(allSnaps, snap.ChainID),
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)
				Expect(shouldRetain).To(BeTrue())
			}
		})

		It("should combine count, age, and size policies", func() {
			countPolicy, _ := backup.NewCountRetentionPolicy(2)
			agePolicy, _ := backup.NewAgeRetentionPolicy(5 * 24 * time.Hour)
			sizePolicy, _ := backup.NewSizeRetentionPolicy(3000)

			policy := backup.NewCompositeRetentionPolicy(countPolicy, agePolicy, sizePolicy)

			for _, snap := range allSnaps {
				context := backup.RetentionContext{
					AllSnapshots: allSnaps,
					Chain:        getChainSnapshots(allSnaps, snap.ChainID),
					TotalSize:    totalSize,
					Now:          now,
				}

				shouldRetain := policy.ShouldRetain(snap, context)

				// All policies want to remove chain-1:
				// - Age: oldest snapshot is 10 days (> 5 days)
				// - Size: need to free space
				// Result: chain-1 removed
				if snap.ChainID == "chain-1" {
					Expect(shouldRetain).To(BeFalse())
				} else {
					Expect(shouldRetain).To(BeTrue())
				}
			}
		})
	})
})

// getChainSnapshots returns all snapshots for a given chain ID.
func getChainSnapshots(snapshots []backup.Snapshot, chainID string) []backup.Snapshot {
	chain := make([]backup.Snapshot, 0)

	for _, snap := range snapshots {
		if snap.ChainID == chainID {
			chain = append(chain, snap)
		}
	}

	return chain
}
