package updater_test

import (
	"context"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/github"
	"github.com/smykla-skalski/klaudiush/internal/updater"
)

// mockClient implements github.Client for testing.
type mockClient struct {
	latestRelease *github.Release
	latestErr     error
	tagReleases   map[string]*github.Release
	tagErr        error
}

func (m *mockClient) GetLatestRelease(_ context.Context, _, _ string) (*github.Release, error) {
	return m.latestRelease, m.latestErr
}

func (m *mockClient) GetReleaseByTag(_ context.Context, _, _, tag string) (*github.Release, error) {
	if m.tagErr != nil {
		return nil, m.tagErr
	}

	if rel, ok := m.tagReleases[tag]; ok {
		return rel, nil
	}

	return nil, github.ErrRepositoryNotFound
}

func (*mockClient) GetTags(_ context.Context, _, _ string) ([]*github.Tag, error) {
	return nil, nil
}

func (*mockClient) IsAuthenticated() bool {
	return false
}

var _ = Describe("Updater", func() {
	Describe("CheckLatest", func() {
		It("returns tag when newer version available", func() {
			client := &mockClient{
				latestRelease: &github.Release{TagName: "v2.0.0"},
			}
			up := updater.NewUpdater("1.0.0", client)

			tag, err := up.CheckLatest(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(tag).To(Equal("v2.0.0"))
		})

		It("returns ErrAlreadyLatest when already up to date", func() {
			client := &mockClient{
				latestRelease: &github.Release{TagName: "v1.0.0"},
			}
			up := updater.NewUpdater("1.0.0", client)

			_, err := up.CheckLatest(context.Background())
			Expect(err).To(MatchError(updater.ErrAlreadyLatest))
		})

		It("returns ErrAlreadyLatest when current is newer", func() {
			client := &mockClient{
				latestRelease: &github.Release{TagName: "v1.0.0"},
			}
			up := updater.NewUpdater("2.0.0", client)

			_, err := up.CheckLatest(context.Background())
			Expect(err).To(MatchError(updater.ErrAlreadyLatest))
		})

		It("always returns latest for dev builds", func() {
			client := &mockClient{
				latestRelease: &github.Release{TagName: "v1.0.0"},
			}
			up := updater.NewUpdater("dev", client)

			tag, err := up.CheckLatest(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(tag).To(Equal("v1.0.0"))
		})

		It("returns error on API failure", func() {
			client := &mockClient{
				latestErr: errors.New("network error"),
			}
			up := updater.NewUpdater("1.0.0", client)

			_, err := up.CheckLatest(context.Background())
			Expect(err).To(HaveOccurred())
		})

		It("handles version without v prefix", func() {
			client := &mockClient{
				latestRelease: &github.Release{TagName: "v2.0.0"},
			}
			up := updater.NewUpdater("1.0.0", client)

			tag, err := up.CheckLatest(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(tag).To(Equal("v2.0.0"))
		})

		It("detects patch version differences", func() {
			client := &mockClient{
				latestRelease: &github.Release{TagName: "v1.13.1"},
			}
			up := updater.NewUpdater("1.13.0", client)

			tag, err := up.CheckLatest(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(tag).To(Equal("v1.13.1"))
		})
	})

	Describe("ValidateTargetVersion", func() {
		var releases map[string]*github.Release

		BeforeEach(func() {
			releases = map[string]*github.Release{
				"v1.13.0": {TagName: "v1.13.0"},
				"v1.12.0": {TagName: "v1.12.0"},
			}
		})

		It("accepts version with v prefix", func() {
			client := &mockClient{tagReleases: releases}
			up := updater.NewUpdater("1.0.0", client)

			tag, err := up.ValidateTargetVersion(context.Background(), "v1.13.0")
			Expect(err).NotTo(HaveOccurred())
			Expect(tag).To(Equal("v1.13.0"))
		})

		It("accepts version without v prefix", func() {
			client := &mockClient{tagReleases: releases}
			up := updater.NewUpdater("1.0.0", client)

			tag, err := up.ValidateTargetVersion(context.Background(), "1.13.0")
			Expect(err).NotTo(HaveOccurred())
			Expect(tag).To(Equal("v1.13.0"))
		})

		It("rejects invalid semver", func() {
			client := &mockClient{tagReleases: releases}
			up := updater.NewUpdater("1.0.0", client)

			_, err := up.ValidateTargetVersion(context.Background(), "not-a-version")
			Expect(err).To(HaveOccurred())
		})

		It("returns error for nonexistent release", func() {
			client := &mockClient{tagReleases: releases}
			up := updater.NewUpdater("1.0.0", client)

			_, err := up.ValidateTargetVersion(context.Background(), "v99.99.99")
			Expect(err).To(HaveOccurred())
		})

		It("returns error on API failure", func() {
			client := &mockClient{tagErr: errors.New("API failure")}
			up := updater.NewUpdater("1.0.0", client)

			_, err := up.ValidateTargetVersion(context.Background(), "v1.0.0")
			Expect(err).To(HaveOccurred())
		})
	})
})
