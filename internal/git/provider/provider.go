package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	giturl "github.com/kubescape/go-git-url"
	giturlapis "github.com/kubescape/go-git-url/apis"
	azureparserv1 "github.com/kubescape/go-git-url/azureparser/v1"
)

// validateHostname rejects hostnames that contain path components,
// query strings, or other characters that could redirect API requests
// to unintended endpoints (SSRF).
func validateHostname(hostname string) error {
	if hostname == "" {
		return nil
	}
	if strings.ContainsAny(hostname, "/?#@\\") || strings.Contains(hostname, "..") {
		return fmt.Errorf("invalid hostname: %q", hostname)
	}
	return nil
}

type ProviderType string

const (
	ProviderGitHub          = ProviderType(giturlapis.ProviderGitHub)
	ProviderGitlab          = ProviderType(giturlapis.ProviderGitLab)
	ProviderBitbucket       = ProviderType(giturlapis.ProviderBitBucket)
	ProviderAzure           = ProviderType(giturlapis.ProviderAzure)
	ProviderBitbucketServer = ProviderType("bitbucketserver")
	ProviderGitea           = ProviderType("gitea")
)

type URLParserFn = func(repoURL string, options ...ProviderOption) (Provider, Repository, error)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . Provider

type Provider interface {
	ListPullRequests(ctx context.Context, repo Repository) ([]PullRequest, error)
	AddCommentToPullRequest(ctx context.Context, repo PullRequest, body []byte) (*Comment, error)
	GetLastComments(ctx context.Context, pr PullRequest, since time.Time) ([]*Comment, error)
	UpdateCommentOfPullRequest(ctx context.Context, pr PullRequest, commentID int, body []byte) error
	ListPullRequestChanges(ctx context.Context, pr PullRequest) ([]Change, error)

	SetLogger(logr.Logger) error
	SetToken(tokenType, token string) error
	SetHostname(hostname string) error

	Setup() error
}

func New(provider ProviderType, options ...ProviderOption) (Provider, error) {
	var p Provider
	switch provider {
	case ProviderGitHub:
		p = newGitHubProvider()
	case ProviderGitlab:
		p = newGitLabProvider()
	case ProviderBitbucket:
		p = newBitbucketCloudProvider()
	case ProviderBitbucketServer:
		p = newBitbucketServerProvider()
	case ProviderGitea:
		p = newGiteaProvider()
	case ProviderAzure:
		p = newAzureProvider()
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
	gitURL, err := giturl.NewGitURL(repoURL)
	if err != nil {
		return nil, Repository{}, fmt.Errorf("failed parsing repository url: %w", err)
	}

	targetProvider := ProviderType(gitURL.GetProvider())
	repo := Repository{
		Org:  gitURL.GetOwnerName(),
		Name: gitURL.GetRepoName(),
	}

	if targetProvider == ProviderAzure {
		repo.Project = gitURL.(*azureparserv1.AzureURL).GetProjectName()
	}

	// Pass the hostname from the URL so self-hosted instances
	// (e.g. gitlab.mycompany.com, github.enterprise.com) are
	// configured with the correct API endpoint.
	if hostname := gitURL.GetHostName(); hostname != "" {
		options = append([]ProviderOption{WithDomain(hostname)}, options...)
	}

	provider, err := New(targetProvider, options...)
	if err != nil {
		return nil, repo, err
	}

	return provider, repo, nil
}

// RepoFromURL parses a repository URL and returns the Repository without
// creating a provider. This is useful when the provider is already cached.
func RepoFromURL(repoURL string) (Repository, error) {
	gitURL, err := giturl.NewGitURL(repoURL)
	if err != nil {
		return Repository{}, fmt.Errorf("failed parsing repository url: %w", err)
	}

	repo := Repository{
		Org:  gitURL.GetOwnerName(),
		Name: gitURL.GetRepoName(),
	}

	if ProviderType(gitURL.GetProvider()) == ProviderAzure {
		repo.Project = gitURL.(*azureparserv1.AzureURL).GetProjectName()
	}

	return repo, nil
}
