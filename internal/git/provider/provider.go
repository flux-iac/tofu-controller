package provider

import (
	"fmt"

	"golang.org/x/net/context"
)

type Provider interface {
	ListPullRequests(ctx context.Context, repo Repository) []PullRequest
	AddCommentToPullREquest(ctx context.Context, repo PullRequest, comment []byte)
}

func New(provider string) (Provider, error) {
	switch provider {
	case "github":
		return newGitHubProvider(), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}
