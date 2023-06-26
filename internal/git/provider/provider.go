package provider

import (
	"fmt"

	"github.com/go-logr/logr"
	giturl "github.com/kubescape/go-git-url"
	azureparserv1 "github.com/kubescape/go-git-url/azureparser/v1"
	"golang.org/x/net/context"
)

const (
	GitHubProviderName = "github"
	AzureProviderName  = "azure"
)

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

	gitURL, err := giturl.NewGitURL(repoURL)
	if err != nil {
		return nil, repo, fmt.Errorf("failed parsing repository url: %w", err)
	}

	targetProvider = gitURL.GetProvider()
	repo.Org = gitURL.GetOwnerName()
	repo.Name = gitURL.GetRepoName()

	if targetProvider == AzureProviderName {
		repo.Project = gitURL.(*azureparserv1.AzureURL).GetProjectName()
	}

	provider, err := New(targetProvider, options...)
	if err != nil {
		return nil, repo, err
	}

	return provider, repo, nil
}
