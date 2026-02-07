// Package github provides GitHub API client with caching
package github

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/go-github/v80/github"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
)

const (
	// perPageLimit is the default per-page limit for GitHub API requests
	perPageLimit = 100
	// ghAuthTimeout is the timeout for gh auth token command
	ghAuthTimeout = 5 * time.Second
)

var (
	// ErrRateLimitExceeded is returned when GitHub API rate limit is exceeded
	ErrRateLimitExceeded = errors.New("github API rate limit exceeded")
	// ErrRepositoryNotFound is returned when repository is not found
	ErrRepositoryNotFound = errors.New("repository not found")
	// ErrNoReleases is returned when no releases are found
	ErrNoReleases = errors.New("no releases found")
	// ErrNoTags is returned when no tags are found
	ErrNoTags = errors.New("no tags found")
)

// Release represents a GitHub release
type Release struct {
	TagName string
	Name    string
	HTMLURL string
}

// Tag represents a GitHub tag
type Tag struct {
	Name string
	SHA  string
}

// Client defines the interface for GitHub API operations
type Client interface {
	// GetLatestRelease retrieves the latest release for a repository
	GetLatestRelease(ctx context.Context, owner, repo string) (*Release, error)
	// GetTags retrieves all tags for a repository
	GetTags(ctx context.Context, owner, repo string) ([]*Tag, error)
	// IsAuthenticated returns whether the client is authenticated
	IsAuthenticated() bool
}

// SDKClient implements Client using go-github SDK
type SDKClient struct {
	client        *github.Client
	authenticated bool
	cache         *Cache
}

var (
	clientInstance *SDKClient
	clientOnce     sync.Once
)

// getToken retrieves GitHub token from environment or gh CLI
func getToken() string {
	// Check GH_TOKEN first
	if token := os.Getenv("GH_TOKEN"); token != "" {
		return token
	}

	// Check GITHUB_TOKEN
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}

	// Fallback to gh auth token if gh CLI is available
	toolChecker := execpkg.NewToolChecker()
	if !toolChecker.IsAvailable("gh") {
		return ""
	}

	runner := execpkg.NewCommandRunner(ghAuthTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), ghAuthTimeout)
	defer cancel()

	result := runner.Run(ctx, "gh", "auth", "token")
	if result.Err != nil {
		return ""
	}

	return strings.TrimSpace(result.Stdout)
}

// NewClient creates or returns the singleton GitHub client
func NewClient() *SDKClient {
	clientOnce.Do(func() {
		token := getToken()
		authenticated := token != ""

		var httpClient *http.Client
		if authenticated {
			httpClient = &http.Client{
				Transport: &authTransport{
					token: token,
				},
			}
		}

		clientInstance = &SDKClient{
			client:        github.NewClient(httpClient),
			authenticated: authenticated,
			cache:         NewCache(),
		}
	})

	return clientInstance
}

// authTransport adds authentication header to requests
type authTransport struct {
	token string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.token)

	return http.DefaultTransport.RoundTrip(req)
}

// IsAuthenticated returns whether the client is authenticated
func (c *SDKClient) IsAuthenticated() bool {
	return c.authenticated
}

// GetLatestRelease retrieves the latest release for a repository
func (c *SDKClient) GetLatestRelease(
	ctx context.Context,
	owner, repo string,
) (*Release, error) {
	cacheKey := fmt.Sprintf("release:%s/%s", owner, repo)

	if cached, ok := c.cache.Get(cacheKey); ok {
		if rel, ok := cached.(*Release); ok {
			return rel, nil
		}
	}

	release, resp, err := c.client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, c.handleError(resp, err)
	}

	result := &Release{
		TagName: release.GetTagName(),
		Name:    release.GetName(),
		HTMLURL: release.GetHTMLURL(),
	}

	c.cache.Set(cacheKey, result)

	return result, nil
}

// GetTags retrieves all tags for a repository
func (c *SDKClient) GetTags(ctx context.Context, owner, repo string) ([]*Tag, error) {
	cacheKey := fmt.Sprintf("tags:%s/%s", owner, repo)

	if cached, ok := c.cache.Get(cacheKey); ok {
		if tags, ok := cached.([]*Tag); ok {
			return tags, nil
		}
	}

	opts := &github.ListOptions{PerPage: perPageLimit}

	ghTags, resp, err := c.client.Repositories.ListTags(ctx, owner, repo, opts)
	if err != nil {
		return nil, c.handleError(resp, err)
	}

	if len(ghTags) == 0 {
		return nil, ErrNoTags
	}

	tags := make([]*Tag, 0, len(ghTags))
	for _, t := range ghTags {
		tags = append(tags, &Tag{
			Name: t.GetName(),
			SHA:  t.GetCommit().GetSHA(),
		})
	}

	c.cache.Set(cacheKey, tags)

	return tags, nil
}

// handleError converts GitHub API errors to our error types
func (*SDKClient) handleError(resp *github.Response, err error) error {
	if resp == nil {
		return err
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return ErrRepositoryNotFound
	case http.StatusForbidden:
		// Check if it's rate limit
		if resp.Rate.Remaining == 0 {
			return ErrRateLimitExceeded
		}

		return err
	default:
		return err
	}
}
