package provider

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	"golang.org/x/net/context"
)

const GitHubProviderName = "github"

type Provider interface {
	ListPullRequests(ctx context.Context, repo Repository) ([]PullRequest, error)
	AddCommentToPullRequest(ctx context.Context, repo PullRequest, body []byte) (*Comment, error)

	SetLogger(logr.Logger) error
	SetToken(tokenType, token string) error
	SetHostname(hostname string) error

	Setup() error
}

func New(provider string, options ...ProviderOption) (Provider, error) {
	var p Provider
	switch provider {
	case GitHubProviderName:
		p = newGitHubProvider()
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}

	for _, opt := range options {
		if err := opt(p); err != nil {
			return nil, err
		}
	}

	if err := p.Setup(); err != nil {
		return p, err
	}

	return p, nil
}

func FromURL(repoURL string, options ...ProviderOption) (Provider, Repository, error) {
	targetProvider := ""
	repo := Repository{}

	parsedURL, err := url.Parse(repoURL)
	if err != nil {
		return nil, repo, fmt.Errorf("unable to parse url: %w", err)
	}

	// That's a pretty naiv check for now.
	// Use proper parsing later with a well tested library.
	if parsedURL.Hostname() == "github.com" {
		targetProvider = GitHubProviderName

		parts := strings.Split(parsedURL.Path, "/")
		if len(parts) != 3 {
			return nil, repo, fmt.Errorf("invalid github repository url: %s", repoURL)
		}

		repo.Org = parts[1]
		repo.Name = strings.TrimSuffix(parts[2], ".git")
	}

	if targetProvider == "" {
		return nil, repo, fmt.Errorf("could not parse provider from url: %s", repoURL)
	}

	provider, err := New(targetProvider, options...)
	if err != nil {
		return nil, repo, err
	}

	return provider, repo, nil
}
