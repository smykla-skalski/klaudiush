// Package git provides an SDK-based implementation of git operations using go-git v6
package git

import (
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/go-git/go-git/v6"
)

// ConfigReader provides methods to read git configuration.
type ConfigReader interface {
	// GetUserName returns the user.name from git config.
	GetUserName() (string, error)

	// GetUserEmail returns the user.email from git config.
	GetUserEmail() (string, error)

	// GetSignoff returns the formatted signoff string (Name <email>).
	GetSignoff() (string, error)
}

// SDKConfigReader implements ConfigReader using go-git SDK.
type SDKConfigReader struct {
	repo *git.Repository
}

// NewConfigReader creates a new SDKConfigReader from a discovered repository.
func NewConfigReader() (ConfigReader, error) {
	repo, err := DiscoverRepository()
	if err != nil {
		return nil, errors.Wrap(err, "failed to discover repository")
	}

	return &SDKConfigReader{repo: repo.repo}, nil
}

// NewConfigReaderFromRepo creates a new SDKConfigReader from an existing repository.
func NewConfigReaderFromRepo(repo *SDKRepository) ConfigReader {
	return &SDKConfigReader{repo: repo.repo}
}

// GetUserName returns the user.name from git config.
func (r *SDKConfigReader) GetUserName() (string, error) {
	cfg, err := r.repo.Config()
	if err != nil {
		return "", errors.Wrap(err, "failed to get config")
	}

	name := cfg.User.Name
	if name == "" {
		return "", errors.New("user.name not set in git config")
	}

	return name, nil
}

// GetUserEmail returns the user.email from git config.
func (r *SDKConfigReader) GetUserEmail() (string, error) {
	cfg, err := r.repo.Config()
	if err != nil {
		return "", errors.Wrap(err, "failed to get config")
	}

	email := cfg.User.Email
	if email == "" {
		return "", errors.New("user.email not set in git config")
	}

	return email, nil
}

// GetSignoff returns the formatted signoff string (Name <email>).
func (r *SDKConfigReader) GetSignoff() (string, error) {
	name, err := r.GetUserName()
	if err != nil {
		return "", err
	}

	email, err := r.GetUserEmail()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s <%s>", name, email), nil
}
