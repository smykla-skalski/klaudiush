package backup_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/backup"
)

var _ = Describe("Snapshot", func() {
	Describe("GenerateSnapshotID", func() {
		It("generates consistent IDs for same inputs", func() {
			timestamp := time.Date(2025, 1, 2, 15, 4, 5, 0, time.UTC)
			contentHash := "abc123"

			id1 := backup.GenerateSnapshotID(timestamp, contentHash)
			id2 := backup.GenerateSnapshotID(timestamp, contentHash)

			Expect(id1).To(Equal(id2))
			Expect(id1).To(HaveLen(16))
		})

		It("generates different IDs for different inputs", func() {
			timestamp1 := time.Date(2025, 1, 2, 15, 4, 5, 0, time.UTC)
			timestamp2 := time.Date(2025, 1, 2, 16, 4, 5, 0, time.UTC)
			contentHash := "abc123"

			id1 := backup.GenerateSnapshotID(timestamp1, contentHash)
			id2 := backup.GenerateSnapshotID(timestamp2, contentHash)

			Expect(id1).NotTo(Equal(id2))
		})
	})

	Describe("ComputeContentHash", func() {
		It("computes consistent hashes for same content", func() {
			content := []byte("test content")

			hash1 := backup.ComputeContentHash(content)
			hash2 := backup.ComputeContentHash(content)

			Expect(hash1).To(Equal(hash2))
			Expect(hash1).To(HaveLen(64))
		})

		It("computes different hashes for different content", func() {
			content1 := []byte("test content 1")
			content2 := []byte("test content 2")

			hash1 := backup.ComputeContentHash(content1)
			hash2 := backup.ComputeContentHash(content2)

			Expect(hash1).NotTo(Equal(hash2))
		})
	})

	Describe("Snapshot", func() {
		var snapshot backup.Snapshot

		BeforeEach(func() {
			snapshot = backup.Snapshot{
				ID:          "test-id",
				StorageType: backup.StorageTypeFull,
				ConfigType:  backup.ConfigTypeGlobal,
			}
		})

		Describe("IsFull", func() {
			It("returns true for full snapshots", func() {
				snapshot.StorageType = backup.StorageTypeFull
				Expect(snapshot.IsFull()).To(BeTrue())
			})

			It("returns false for patch snapshots", func() {
				snapshot.StorageType = backup.StorageTypePatch
				Expect(snapshot.IsFull()).To(BeFalse())
			})
		})

		Describe("IsPatch", func() {
			It("returns true for patch snapshots", func() {
				snapshot.StorageType = backup.StorageTypePatch
				Expect(snapshot.IsPatch()).To(BeTrue())
			})

			It("returns false for full snapshots", func() {
				snapshot.StorageType = backup.StorageTypeFull
				Expect(snapshot.IsPatch()).To(BeFalse())
			})
		})

		Describe("IsGlobal", func() {
			It("returns true for global configs", func() {
				snapshot.ConfigType = backup.ConfigTypeGlobal
				Expect(snapshot.IsGlobal()).To(BeTrue())
			})

			It("returns false for project configs", func() {
				snapshot.ConfigType = backup.ConfigTypeProject
				Expect(snapshot.IsGlobal()).To(BeFalse())
			})
		})

		Describe("IsProject", func() {
			It("returns true for project configs", func() {
				snapshot.ConfigType = backup.ConfigTypeProject
				Expect(snapshot.IsProject()).To(BeTrue())
			})

			It("returns false for global configs", func() {
				snapshot.ConfigType = backup.ConfigTypeGlobal
				Expect(snapshot.IsProject()).To(BeFalse())
			})
		})
	})

	Describe("SnapshotIndex", func() {
		var index *backup.SnapshotIndex

		BeforeEach(func() {
			index = backup.NewSnapshotIndex()
		})

		Describe("NewSnapshotIndex", func() {
			It("creates empty index", func() {
				Expect(index.Version).To(Equal(1))
				Expect(index.Snapshots).To(BeEmpty())
				Expect(index.Updated).To(BeTemporally("~", time.Now(), time.Second))
			})
		})

		Describe("Add", func() {
			It("adds snapshot to index", func() {
				snapshot := backup.Snapshot{
					ID:        "test-id",
					Timestamp: time.Now(),
				}

				index.Add(snapshot)

				Expect(index.Snapshots).To(HaveLen(1))
				Expect(index.Snapshots["test-id"]).To(Equal(snapshot))
			})

			It("updates timestamp", func() {
				initialTime := index.Updated
				time.Sleep(10 * time.Millisecond)

				snapshot := backup.Snapshot{ID: "test-id"}
				index.Add(snapshot)

				Expect(index.Updated).To(BeTemporally(">", initialTime))
			})
		})

		Describe("Get", func() {
			It("retrieves existing snapshot", func() {
				snapshot := backup.Snapshot{ID: "test-id"}
				index.Add(snapshot)

				result, err := index.Get("test-id")

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(snapshot))
			})

			It("returns error for non-existent snapshot", func() {
				_, err := index.Get("non-existent")

				Expect(err).To(MatchError(ContainSubstring("snapshot not found")))
			})
		})

		Describe("Delete", func() {
			It("removes existing snapshot", func() {
				snapshot := backup.Snapshot{ID: "test-id"}
				index.Add(snapshot)

				err := index.Delete("test-id")

				Expect(err).NotTo(HaveOccurred())
				Expect(index.Snapshots).To(BeEmpty())
			})

			It("returns error for non-existent snapshot", func() {
				err := index.Delete("non-existent")

				Expect(err).To(MatchError(ContainSubstring("snapshot not found")))
			})

			It("updates timestamp", func() {
				snapshot := backup.Snapshot{ID: "test-id"}
				index.Add(snapshot)
				initialTime := index.Updated
				time.Sleep(10 * time.Millisecond)

				err := index.Delete("test-id")

				Expect(err).NotTo(HaveOccurred())
				Expect(index.Updated).To(BeTemporally(">", initialTime))
			})
		})

		Describe("List", func() {
			It("returns all snapshots", func() {
				snapshot1 := backup.Snapshot{ID: "id-1"}
				snapshot2 := backup.Snapshot{ID: "id-2"}
				index.Add(snapshot1)
				index.Add(snapshot2)

				snapshots := index.List()

				Expect(snapshots).To(HaveLen(2))
				Expect(snapshots).To(ContainElements(snapshot1, snapshot2))
			})

			It("returns empty slice for empty index", func() {
				snapshots := index.List()

				Expect(snapshots).To(BeEmpty())
			})
		})

		Describe("FindByHash", func() {
			It("finds snapshot with matching hash", func() {
				snapshot := backup.Snapshot{
					ID: "test-id",
					Metadata: backup.SnapshotMetadata{
						ConfigHash: "abc123",
					},
				}
				index.Add(snapshot)

				result, found := index.FindByHash("abc123")

				Expect(found).To(BeTrue())
				Expect(result).To(Equal(snapshot))
			})

			It("returns false for non-matching hash", func() {
				snapshot := backup.Snapshot{
					ID: "test-id",
					Metadata: backup.SnapshotMetadata{
						ConfigHash: "abc123",
					},
				}
				index.Add(snapshot)

				_, found := index.FindByHash("xyz789")

				Expect(found).To(BeFalse())
			})
		})

		Describe("GetChain", func() {
			It("returns snapshots in chain", func() {
				snapshot1 := backup.Snapshot{ID: "id-1", ChainID: "chain-A", SequenceNum: 1}
				snapshot2 := backup.Snapshot{ID: "id-2", ChainID: "chain-A", SequenceNum: 2}
				snapshot3 := backup.Snapshot{ID: "id-3", ChainID: "chain-B", SequenceNum: 1}
				index.Add(snapshot1)
				index.Add(snapshot2)
				index.Add(snapshot3)

				chain := index.GetChain("chain-A")

				Expect(chain).To(HaveLen(2))
				Expect(chain).To(ContainElements(snapshot1, snapshot2))
			})

			It("returns empty slice for non-existent chain", func() {
				chain := index.GetChain("non-existent")

				Expect(chain).To(BeEmpty())
			})
		})
	})
})
