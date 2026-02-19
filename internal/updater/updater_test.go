package updater

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/github"
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

func TestUpdaterCheckLatest(t *testing.T) {
	t.Run("newer version available", func(t *testing.T) {
		client := &mockClient{
			latestRelease: &github.Release{TagName: "v2.0.0"},
		}
		up := NewUpdater("1.0.0", client)

		tag, err := up.CheckLatest(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tag != "v2.0.0" {
			t.Errorf("tag = %q, want %q", tag, "v2.0.0")
		}
	})

	t.Run("already latest", func(t *testing.T) {
		client := &mockClient{
			latestRelease: &github.Release{TagName: "v1.0.0"},
		}
		up := NewUpdater("1.0.0", client)

		_, err := up.CheckLatest(context.Background())
		if !errors.Is(err, ErrAlreadyLatest) {
			t.Errorf("err = %v, want ErrAlreadyLatest", err)
		}
	})

	t.Run("current is newer than latest", func(t *testing.T) {
		client := &mockClient{
			latestRelease: &github.Release{TagName: "v1.0.0"},
		}
		up := NewUpdater("2.0.0", client)

		_, err := up.CheckLatest(context.Background())
		if !errors.Is(err, ErrAlreadyLatest) {
			t.Errorf("err = %v, want ErrAlreadyLatest", err)
		}
	})

	t.Run("dev build always gets latest", func(t *testing.T) {
		client := &mockClient{
			latestRelease: &github.Release{TagName: "v1.0.0"},
		}
		up := NewUpdater("dev", client)

		tag, err := up.CheckLatest(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tag != "v1.0.0" {
			t.Errorf("tag = %q, want %q", tag, "v1.0.0")
		}
	})

	t.Run("API error", func(t *testing.T) {
		client := &mockClient{
			latestErr: errors.New("network error"),
		}
		up := NewUpdater("1.0.0", client)

		_, err := up.CheckLatest(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("version with v prefix", func(t *testing.T) {
		client := &mockClient{
			latestRelease: &github.Release{TagName: "v2.0.0"},
		}
		// goreleaser sets version without v prefix
		up := NewUpdater("1.0.0", client)

		tag, err := up.CheckLatest(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tag != "v2.0.0" {
			t.Errorf("tag = %q, want %q", tag, "v2.0.0")
		}
	})

	t.Run("patch version comparison", func(t *testing.T) {
		client := &mockClient{
			latestRelease: &github.Release{TagName: "v1.13.1"},
		}
		up := NewUpdater("1.13.0", client)

		tag, err := up.CheckLatest(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tag != "v1.13.1" {
			t.Errorf("tag = %q, want %q", tag, "v1.13.1")
		}
	})
}

func TestUpdaterValidateTargetVersion(t *testing.T) {
	releases := map[string]*github.Release{
		"v1.13.0": {TagName: "v1.13.0"},
		"v1.12.0": {TagName: "v1.12.0"},
	}

	t.Run("valid version with v prefix", func(t *testing.T) {
		client := &mockClient{tagReleases: releases}
		up := NewUpdater("1.0.0", client)

		tag, err := up.ValidateTargetVersion(context.Background(), "v1.13.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tag != "v1.13.0" {
			t.Errorf("tag = %q, want %q", tag, "v1.13.0")
		}
	})

	t.Run("valid version without v prefix", func(t *testing.T) {
		client := &mockClient{tagReleases: releases}
		up := NewUpdater("1.0.0", client)

		tag, err := up.ValidateTargetVersion(context.Background(), "1.13.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tag != "v1.13.0" {
			t.Errorf("tag = %q, want %q", tag, "v1.13.0")
		}
	})

	t.Run("invalid semver", func(t *testing.T) {
		client := &mockClient{tagReleases: releases}
		up := NewUpdater("1.0.0", client)

		_, err := up.ValidateTargetVersion(context.Background(), "not-a-version")
		if err == nil {
			t.Fatal("expected error for invalid semver")
		}
	})

	t.Run("release not found", func(t *testing.T) {
		client := &mockClient{tagReleases: releases}
		up := NewUpdater("1.0.0", client)

		_, err := up.ValidateTargetVersion(context.Background(), "v99.99.99")
		if err == nil {
			t.Fatal("expected error for nonexistent release")
		}
	})

	t.Run("API error", func(t *testing.T) {
		client := &mockClient{tagErr: errors.New("API failure")}
		up := NewUpdater("1.0.0", client)

		_, err := up.ValidateTargetVersion(context.Background(), "v1.0.0")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
